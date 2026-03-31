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
    host = PUBLIC_URL.replace("https://", "").replace("http://", "")
    name = urllib.parse.quote(request.query_params.get("name", ""))
    interest = urllib.parse.quote(request.query_params.get("interest", ""))
    phone = urllib.parse.quote(request.query_params.get("phone", ""))
    ws_url = f"wss://{host}/media-stream?name={name}&interest={interest}&phone={phone}"
    return HTMLResponse(content=f'<Response><Connect><Stream url="{ws_url}" /></Connect></Response>', media_type="application/xml")

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

    log.info(f"[EXOTEL STATUS] {form}")
    status = form.get("Status", form.get("CallStatus", ""))
    detailed_status = form.get("DetailedStatus", "")
    phone = form.get("To", "")
    call_sid = form.get("CallSid", form.get("call_sid", ""))
    recording_url = form.get("RecordingUrl", form.get("recording_url", ""))

    terminal_error = None
    if detailed_status.lower() in ['busy', 'no-answer', 'failed', 'canceled', 'dnd']:
        terminal_error = detailed_status
    elif status.lower() in ['failed', 'busy', 'no-answer', 'canceled']:
        terminal_error = status
    if terminal_error:
        from database import log_call_status
        log_call_status(phone, terminal_error, "Exotel Call Error")
        # Emit user-friendly event — look up campaign_id and lead name from Redis
        from live_logs import emit_campaign_event
        phone_clean = "".join(filter(str.isdigit, str(phone)))[-10:]
        _pending = redis_store.get_pending_call(f"phone:{phone_clean}")
        _evt_campaign = _pending.get("campaign_id", 0) if _pending else 0
        _evt_name = _pending.get("name", phone_clean) if _pending else phone_clean
        emit_campaign_event(_evt_campaign, _evt_name, phone, terminal_error.lower(), f"Exotel: {terminal_error}")
    elif status.lower() == "completed":
        from live_logs import emit_campaign_event
        phone_clean = "".join(filter(str.isdigit, str(phone)))[-10:]
        _pending = redis_store.get_pending_call(f"phone:{phone_clean}")
        _evt_campaign = _pending.get("campaign_id", 0) if _pending else 0
        _evt_name = _pending.get("name", phone_clean) if _pending else phone_clean
        emit_campaign_event(_evt_campaign, _evt_name, phone, "completed", "Call completed")

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
