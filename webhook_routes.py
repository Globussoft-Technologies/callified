"""
webhook_routes.py — Webhook handlers extracted from main.py.
Handles CRM webhooks, Twilio/Exotel status callbacks, and recording processing.
"""
import os
import json
import urllib.parse
import httpx
import base64
from fastapi import APIRouter, Request, BackgroundTasks
from fastapi.responses import HTMLResponse

import redis_store
from database import save_call_transcript, update_lead_status

# ─── Config ──────────────────────────────────────────────────────────────────

EXOTEL_API_KEY = (os.getenv("EXOTEL_API_KEY") or "").strip()
EXOTEL_API_TOKEN = (os.getenv("EXOTEL_API_TOKEN") or "").strip()

DEFAULT_PROVIDER = os.getenv("DEFAULT_PROVIDER", "twilio").lower()
PUBLIC_URL = os.getenv("PUBLIC_SERVER_URL", "http://localhost:8000")

webhook_router = APIRouter()

# ─── Webhook Endpoints ───────────────────────────────────────────────────────

@webhook_router.post("/crm-webhook")
async def handle_crm_webhook(request: Request, background_tasks: BackgroundTasks):
    from dial_routes import initiate_call
    try:
        payload = await request.json()
    except Exception:
        return {"status": "error"}
    if "challenge" in payload:
        return {"challenge": payload["challenge"]}
    lead_data = payload.get("event", {}).get("lead", {})
    phone = lead_data.get("phone")
    if not phone:
        return {"status": "ignored"}
    background_tasks.add_task(initiate_call, {
        "name": lead_data.get("first_name", "Customer"), "phone_number": phone,
        "interest": lead_data.get("source", "our website"),
        "provider": lead_data.get("provider", DEFAULT_PROVIDER).lower()
    })
    return {"status": "success"}

@webhook_router.post("/webhook/{provider}")
@webhook_router.get("/webhook/{provider}")
async def dynamic_webhook(provider: str, request: Request):
    import logging as _log
    log = _log.getLogger("uvicorn.error")
    host = PUBLIC_URL.replace("https://", "").replace("http://", "")
    qp = request.query_params
    name = qp.get("name", "")
    interest = qp.get("interest", "")
    phone = qp.get("phone", "")
    tts_language = qp.get("tts_language", "")
    tts_provider = qp.get("tts_provider", "")
    voice = qp.get("voice", "")

    # Exotel Passthru sends a POST with form data; "From" is the lead's phone.
    # Read it and hydrate context from Redis so the WS URL carries full params.
    if provider == "exotel" and not phone:
        try:
            form = await request.form()
            raw_phone = str(form.get("From", "") or form.get("CallFrom", "") or "").strip().lstrip("+")
            log.info(f"[EXOTEL-WEBHOOK] Passthru hit: From={raw_phone} form_keys={list(form.keys())}")
            if raw_phone:
                phone = raw_phone
                pending = redis_store.get_pending_call(f"phone:{phone}")
                if not pending and len(phone) > 10:
                    pending = redis_store.get_pending_call(f"phone:{phone[-10:]}")
                if not pending:
                    pending = redis_store.get_pending_call("latest")
                if pending:
                    name = name or pending.get("name", "")
                    interest = interest or pending.get("interest", "")
                    tts_language = tts_language or pending.get("tts_language", "")
                    tts_provider = tts_provider or pending.get("tts_provider", "")
                    voice = voice or pending.get("tts_voice_id", "")
                    log.info(f"[EXOTEL-WEBHOOK] Redis hydrated: name={name} lang={tts_language} provider={tts_provider}")
        except Exception as _e:
            log.warning(f"[EXOTEL-WEBHOOK] Failed to read form body: {_e}")

    ws_url = (
        f"wss://{host}/media-stream"
        f"?name={urllib.parse.quote(name)}&interest={urllib.parse.quote(interest)}&phone={urllib.parse.quote(phone)}"
        f"&tts_language={urllib.parse.quote(tts_language)}&tts_provider={urllib.parse.quote(tts_provider)}&voice={urllib.parse.quote(voice)}"
    )
    log.info(f"[{provider.upper()}-WEBHOOK] Serving ExoML/TwiML ws_url={ws_url}")
    return HTMLResponse(content=f'<?xml version="1.0" encoding="UTF-8"?><Response><Connect><Stream url="{ws_url}" /></Connect></Response>', media_type="application/xml")

@webhook_router.post("/webhook/twilio/status")
async def twilio_status_webhook(request: Request):
    form = await request.form()
    status = form.get("CallStatus", "")
    phone = form.get("To", "")
    if status.lower() in ['failed', 'busy', 'no-answer', 'canceled']:
        from database import log_call_status
        log_call_status(phone, status, "Twilio Call Error")
    return {"status": "ok"}

@webhook_router.post("/webhook/exotel/status")
async def exotel_status_webhook(request: Request, background_tasks: BackgroundTasks):
    import logging
    log = logging.getLogger("uvicorn.error")
    try:
        form = dict(await request.form())
    except Exception:
        try:
            form = await request.json()
        except Exception:
            form = {}

    # Log full payload so we can see exact Exotel field names in production logs
    log.info(f"[EXOTEL STATUS] keys={list(form.keys())} payload={dict(form)}")
    status = form.get("Status", form.get("CallStatus", ""))
    detailed_status = form.get("DetailedStatus", "")
    # Exotel connect.json outbound: "From"=lead phone, "To"=Exotel caller ID.
    # Try every known variation to be safe across Exotel API versions.
    phone = (form.get("From") or form.get("CallFrom") or
             form.get("To") or form.get("CallTo") or "")
    call_sid = form.get("CallSid", form.get("call_sid", ""))
    recording_url = form.get("RecordingUrl", form.get("recording_url", ""))

    def _resolve_pending(phone_raw: str, sid: str) -> dict:
        """Look up pending call — try every phone variant then call_sid key."""
        digits = "".join(filter(str.isdigit, str(phone_raw)))
        # Try last-10, full digits, and the call_sid as a phone: key
        for key in filter(None, (digits[-10:] if len(digits) >= 10 else None,
                                  digits[-10:], digits, sid)):
            pending = redis_store.get_pending_call(f"phone:{key}") or {}
            if pending:
                log.info(f"[EXOTEL STATUS] resolved pending via phone:{key}")
                return pending
        # Last resort: call_sid stored directly (set by dial_exotel after SID arrives)
        if sid:
            pending = redis_store.get_pending_call(sid) or {}
            if pending:
                log.info(f"[EXOTEL STATUS] resolved pending via sid:{sid}")
                return pending
        # Ultimate fallback: "latest" key always has the most recent call
        pending = redis_store.get_pending_call("latest") or {}
        if pending:
            log.info(f"[EXOTEL STATUS] resolved pending via latest key")
        return pending

    terminal_error = None
    if detailed_status.lower() in ['busy', 'no-answer', 'failed', 'canceled', 'dnd']:
        terminal_error = detailed_status
    elif status.lower() in ['failed', 'busy', 'no-answer', 'canceled']:
        terminal_error = status
    _pending = _resolve_pending(phone, call_sid)
    _evt_campaign = _pending.get("campaign_id", 0)
    _evt_name = _pending.get("name") or phone or call_sid
    _evt_phone = _pending.get("phone") or phone
    _sched_id = _pending.get("scheduled_call_id")

    # DB fallback: if Redis expired, find the most-recent dialing scheduled call for this phone
    if not _sched_id and phone:
        try:
            from database import get_conn
            conn = get_conn()
            cur = conn.cursor()
            digits = "".join(filter(str.isdigit, str(phone)))
            phone10 = digits[-10:] if len(digits) >= 10 else digits
            cur.execute("""
                SELECT sc.id FROM scheduled_calls sc
                JOIN leads l ON sc.lead_id = l.id
                WHERE sc.status = 'dialing' AND RIGHT(REPLACE(REPLACE(l.phone,'+',''),' ',''), 10) = %s
                ORDER BY sc.scheduled_time DESC LIMIT 1
            """, (phone10,))
            row = cur.fetchone()
            conn.close()
            if row:
                _sched_id = row["id"]
                log.info(f"[EXOTEL STATUS] Resolved scheduled_call_id={_sched_id} from DB for phone {phone10}")
        except Exception as _dbe:
            log.warning(f"[EXOTEL STATUS] DB fallback failed: {_dbe}")

    if terminal_error:
        from database import log_call_status
        log_call_status(phone, terminal_error, "Exotel Call Error")
        from live_logs import emit_campaign_event
        emit_campaign_event(_evt_campaign, _evt_name, _evt_phone, terminal_error.lower(), f"via exotel | {terminal_error.lower()}")
        if _sched_id:
            from database import update_scheduled_call_status
            update_scheduled_call_status(_sched_id, "failed")
            log.info(f"[EXOTEL STATUS] Scheduled call {_sched_id} → failed ({terminal_error})")
    elif status.lower() == "completed":
        from live_logs import emit_campaign_event
        emit_campaign_event(_evt_campaign, _evt_name, _evt_phone, "completed", "via exotel | completed")
        if _sched_id:
            from database import update_scheduled_call_status
            update_scheduled_call_status(_sched_id, "completed")
            log.info(f"[EXOTEL STATUS] Scheduled call {_sched_id} → completed")

    if recording_url and call_sid:
        log.error(f"[EXOTEL-WEBHOOK] Status payload contained a RecordingUrl for {call_sid}!")
        background_tasks.add_task(process_recording, recording_url, call_sid, phone)

    return {"status": "ok"}

@webhook_router.api_route("/exotel/recording-ready", methods=["GET", "POST"])
async def handle_exotel_recording(request: Request, background_tasks: BackgroundTasks):
    if request.method == "POST":
        try:
            form_data = dict(await request.form())
        except Exception:
            try:
                form_data = await request.json()
            except Exception:
                form_data = {}
    else:
        form_data = dict(request.query_params)

    recording_url = form_data.get("RecordingUrl", form_data.get("recording_url", ""))
    call_sid = form_data.get("CallSid", form_data.get("call_sid", ""))
    to_phone = form_data.get("To", form_data.get("to_phone", ""))

    print(f"[EXOTEL-WEBHOOK] /recording-ready Hit! RecordingUrl={recording_url}, CallSid={call_sid}")

    if recording_url and call_sid:
        background_tasks.add_task(process_recording, recording_url, call_sid, to_phone)
    return {"status": "success"}

async def process_recording(recording_url: str, call_sid: str, phone: str):
    import os
    import time
    import logging
    from database import get_conn
    log = logging.getLogger("uvicorn.error")

    log.info(f"[RECORDING] Downloading for {call_sid} from {recording_url}")
    creds = f"{EXOTEL_API_KEY}:{EXOTEL_API_TOKEN}"
    auth_b64 = base64.b64encode(creds.encode()).decode()
    async with httpx.AsyncClient(timeout=60.0, follow_redirects=True) as client:
        try:
            resp = await client.get(recording_url, headers={"Authorization": f"Basic {auth_b64}"})
            audio_bytes = resp.content
            log.info(f"[RECORDING] Download: status={resp.status_code}, size={len(audio_bytes)} bytes")

            if resp.status_code != 200 or len(audio_bytes) < 1000:
                log.warning(f"[RECORDING] Skipping corrupt/empty download: {resp.status_code}, {len(audio_bytes)} bytes")
                return

            # Save file physically
            os.makedirs("/home/empcloud-development/callified-ai/recordings", exist_ok=True)
            mp3_filename = f"call_{call_sid}_{int(time.time() * 1000)}.mp3"
            mp3_path = os.path.join("/home/empcloud-development/callified-ai/recordings", mp3_filename)
            with open(mp3_path, "wb") as f:
                f.write(audio_bytes)

            public_audio_url = f"/api/recordings/{mp3_filename}"
            log.error(f"[WEBHOOK SAVED] Successfully wrote {len(audio_bytes)} bytes to {mp3_path}")

            # Update Database!
            conn = get_conn()
            cursor = conn.cursor()
            # Match to the most recent transcript WITHOUT a recording for this phone
            phone_match = f"%{phone[-10:]}%" if len(phone) >= 10 else f"%{phone}%"
            cursor.execute('''
                UPDATE call_transcripts
                SET recording_url = %s
                WHERE id = (
                    SELECT t.id FROM (
                        SELECT ct.id FROM call_transcripts ct
                        JOIN leads l ON ct.lead_id = l.id
                        WHERE l.phone LIKE %s AND (ct.recording_url IS NULL OR ct.recording_url = '')
                        ORDER BY ct.created_at DESC LIMIT 1
                    ) as t
                )
            ''', (public_audio_url, phone_match))
            if cursor.rowcount == 0:
                # Fallback: attach to most recent transcript for this phone even if it has a recording
                cursor.execute('''
                    UPDATE call_transcripts
                    SET recording_url = %s
                    WHERE id = (
                        SELECT t.id FROM (
                            SELECT ct.id FROM call_transcripts ct
                            JOIN leads l ON ct.lead_id = l.id
                            WHERE l.phone LIKE %s
                            ORDER BY ct.created_at DESC LIMIT 1
                        ) as t
                    )
                ''', (public_audio_url, phone_match))
            conn.commit()
            log.error(f"[WEBHOOK DB SYNC] Attached {public_audio_url} to phone {phone}")

        except Exception as e:
            log.error(f"Failed to download recording: {e}")
            import traceback
            log.error(traceback.format_exc())
            return

    try:
        llm = genai.Client(api_key=os.getenv("GEMINI_API_KEY", "dummy"))
        reply = await llm.aio.models.generate_content(
            model="gemini-2.5-flash", contents=transcript,
            config=types.GenerateContentConfig(system_instruction="You are a professional AI assistant. Analyze the sales call transcript and produce a structured Follow-Up Note.")
        )
        summary = reply.text
    except Exception as e:
        print("Summarization failed:", e)
        return
    from database import update_call_note, get_conn
    update_call_note(call_sid, summary, phone)
