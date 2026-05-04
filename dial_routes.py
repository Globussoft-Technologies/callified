"""
dial_routes.py — Dial endpoints extracted from main.py.
Handles initiating calls via Twilio and Exotel, single-lead and campaign dialing.
"""
import os
import json
import asyncio
import urllib.parse
import httpx
import base64
from datetime import datetime
from fastapi import APIRouter, BackgroundTasks

import call_logger
import redis_store
import logging as _logging
from database import get_lead_by_id, update_lead_status, save_call_transcript, is_dnd_number, get_conn
from billing import get_usage_summary
from call_guard import is_calling_allowed, get_next_allowed_time, get_org_timezone

# ─── Telephony Config ────────────────────────────────────────────────────────

EXOTEL_API_KEY = (os.getenv("EXOTEL_API_KEY") or "").strip()
EXOTEL_API_TOKEN = (os.getenv("EXOTEL_API_TOKEN") or "").strip()
EXOTEL_ACCOUNT_SID = (os.getenv("EXOTEL_ACCOUNT_SID") or "YOUR_EXOTEL_ACCOUNT_SID").strip()
EXOTEL_CALLER_ID = (os.getenv("EXOTEL_CALLER_ID") or "YOUR_EXOTEL_NUMBER").strip()

TWILIO_ACCOUNT_SID = os.getenv("TWILIO_ACCOUNT_SID")
TWILIO_AUTH_TOKEN = os.getenv("TWILIO_AUTH_TOKEN")
TWILIO_PHONE_NUMBER = os.getenv("TWILIO_PHONE_NUMBER")

DEFAULT_PROVIDER = os.getenv("DEFAULT_PROVIDER", "twilio").lower()
PUBLIC_URL = os.getenv("PUBLIC_SERVER_URL", "http://localhost:8000")

dial_router = APIRouter()
last_dial_result = {}

# ─── Core Dial Functions ─────────────────────────────────────────────────────

async def initiate_call(lead: dict):
    provider = lead.get("provider", "twilio")
    phone_clean = lead.get("phone_number", "").lstrip("+")
    pending = {
        "name": lead.get("name", "Customer"),
        "interest": lead.get("interest", "our platform"),
        "phone": phone_clean,
        "lead_id": lead.get("lead_id"),
    }
    if lead.get("campaign_id"):
        pending["campaign_id"] = lead["campaign_id"]
    if lead.get("product_id"):
        pending["product_id"] = lead["product_id"]
    if lead.get("tts_provider"):
        pending["tts_provider"] = lead["tts_provider"]
    if lead.get("tts_voice_id"):
        pending["tts_voice_id"] = lead["tts_voice_id"]
    if lead.get("tts_language"):
        pending["tts_language"] = lead["tts_language"]
    if lead.get("scheduled_call_id"):
        pending["scheduled_call_id"] = lead["scheduled_call_id"]
    redis_store.set_pending_call("latest", pending)
    # Also store by phone number for concurrent dial support
    redis_store.set_pending_call(f"phone:{phone_clean}", pending)
    # Store by last 10 digits too (for matching)
    if len(phone_clean) > 10:
        redis_store.set_pending_call(f"phone:{phone_clean[-10:]}", pending)
    if provider == "twilio":
        await dial_twilio(lead)
    elif provider == "exotel":
        await dial_exotel(lead)

async def dial_twilio(lead: dict):
    if not TWILIO_ACCOUNT_SID or not TWILIO_AUTH_TOKEN:
        return
    from twilio.rest import Client
    twiml_url = f"{PUBLIC_URL}/webhook/twilio?name={urllib.parse.quote(lead['name'])}&interest={urllib.parse.quote(lead['interest'])}&phone={urllib.parse.quote(lead['phone_number'])}"
    def _create_call():
        client = Client(TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN)
        return client.calls.create(
            url=twiml_url, to=lead["phone_number"], from_=TWILIO_PHONE_NUMBER,
            status_callback=f"{PUBLIC_URL}/webhook/twilio/status",
            status_callback_event=['completed', 'no-answer', 'busy', 'failed', 'canceled']
        )
    try:
        call = await asyncio.get_event_loop().run_in_executor(None, _create_call)
        print(f"Twilio Call Triggered. SID: {call.sid}")
    except Exception as e:
        print(f"Failed to trigger Twilio call: {e}")

async def _poll_exotel_call_status(call_sid: str, lead: dict, phone_clean: str, auth_b64: str):
    """Poll Exotel every 3s for a terminal call status. Stops as soon as one is found."""
    import asyncio as _asyncio, logging as _logging
    log = _logging.getLogger("uvicorn.error")
    campaign_id = lead.get("campaign_id", 0)
    lead_name = lead.get("name", phone_clean)
    lead_phone = lead.get("phone_number", phone_clean)
    terminal_map = {
        "no-answer": ("no-answer", "No Answer"),
        "busy":      ("busy",      "Line Busy"),
        "failed":    ("failed",    "Call Failed"),
        "canceled":  ("failed",    "Canceled"),
    }
    url = f"https://api.exotel.com/v1/Accounts/{EXOTEL_ACCOUNT_SID}/Calls/{call_sid}.json"
    await _asyncio.sleep(2)          # brief wait for Exotel to record the call
    ringing_emitted = False
    for attempt in range(40):        # poll up to 40×3s = 2 min max
        try:
            async with httpx.AsyncClient(timeout=12.0) as client:
                resp = await client.get(url, headers={"Authorization": f"Basic {auth_b64}"})
            if resp.status_code == 200:
                status = resp.json().get("Call", {}).get("Status", "").lower()
                log.info(f"[POLL] {call_sid} #{attempt+1}: {status}")
                if status in terminal_map:
                    if campaign_id:
                        from live_logs import emit_campaign_event
                        evt, detail = terminal_map[status]
                        emit_campaign_event(campaign_id, lead_name, lead_phone, evt, f"via exotel | {detail}")
                    return
                if status == "completed":
                    return   # ws_handler already emitted completed
                # First time we see in-progress → phone is ringing, show it
                if status == "in-progress" and not ringing_emitted and campaign_id:
                    from live_logs import emit_campaign_event
                    emit_campaign_event(campaign_id, lead_name, lead_phone, "ringing", "via exotel | phone ringing")
                    ringing_emitted = True
            else:
                log.warning(f"[POLL] {call_sid} #{attempt+1}: HTTP {resp.status_code}")
        except Exception as e:
            log.warning(f"[POLL] {call_sid} #{attempt+1} error: {e!r} — retrying")
        await _asyncio.sleep(3)

async def dial_exotel(lead: dict):
    import logging
    global last_dial_result
    logger = logging.getLogger("uvicorn.error")
    phone_clean = lead["phone_number"].strip().lstrip("+")
    if len(phone_clean) == 10 and not phone_clean.startswith("0"):
        phone_clean = "91" + phone_clean
    logger.info(f"Phone normalized: '{lead['phone_number']}' -> '{phone_clean}'")

    # Build a dynamic ExoML URL on OUR server so language/voice params are
    # forwarded into the WebSocket URL that Exotel ultimately connects to.
    _exo_params = urllib.parse.urlencode({
        "name": lead.get("name", "Customer"),
        "interest": lead.get("interest", "our platform"),
        "phone": phone_clean,
        "tts_language": lead.get("tts_language", "hi"),
        "tts_provider": lead.get("tts_provider", "elevenlabs"),
        "voice": lead.get("tts_voice_id", ""),
    })
    exoml_url = f"{PUBLIC_URL}/webhook/exotel?{_exo_params}"

    url = f"https://api.exotel.com/v1/Accounts/{EXOTEL_ACCOUNT_SID}/Calls/connect.json"
    data = {"From": phone_clean, "CallerId": EXOTEL_CALLER_ID, "Url": exoml_url, "CallType": "trans", "StatusCallback": f"{PUBLIC_URL}/webhook/exotel/status"}
    logger.info(f"[DIAL] Exotel attempt: From={phone_clean}, ExoML={exoml_url}")
    call_logger.call_event(phone_clean, "DIAL_INITIATED", f"From={phone_clean}, Url={exoml_url}")
    last_dial_result = {"timestamp": datetime.now().isoformat(), "phone": phone_clean, "url": url, "exoml": exoml_url, "status": "pending"}
    try:
        creds = f"{EXOTEL_API_KEY}:{EXOTEL_API_TOKEN}"
        auth_b64 = base64.b64encode(creds.encode()).decode()
        headers = {"Content-Type": "application/x-www-form-urlencoded", "Authorization": f"Basic {auth_b64}"}
        async with httpx.AsyncClient(timeout=15.0) as client:
            resp = await client.post(url, data=data, headers=headers)
        logger.info(f"[DIAL] Exotel response ({resp.status_code}): {resp.text[:300]}")
        call_logger.call_event(phone_clean, "DIAL_RESPONSE", f"status={resp.status_code}", response=resp.text[:200])
        last_dial_result.update({"status": resp.status_code, "response": resp.text[:500]})

        # Handle DND/NDNC blocked calls
        if resp.status_code == 403 and "NDNC" in resp.text:
            logger.warning(f"[DIAL] DND blocked: {phone_clean}")
            lead_id = lead.get("lead_id")
            if lead_id:
                save_call_transcript(
                    lead_id=lead_id,
                    transcript_json=json.dumps([{"role": "System", "text": "Call blocked — this number is registered on TRAI NDNC (Do Not Call) registry. Exotel cannot connect to DND numbers without compliance approval."}], ensure_ascii=False),
                    recording_url=None,
                    call_duration_s=0,
                    campaign_id=lead.get("campaign_id"),
                )
                update_lead_status(lead_id, "DND Blocked")
                from database import update_lead_note
                update_lead_note(lead_id, "⛔ DND Blocked — This number is on TRAI NDNC (Do Not Call) registry. Exotel cannot dial DND numbers without compliance approval. Submit letterhead + CRM screenshot to Exotel to enable DND calling.")
            return

        try:
            dial_json = resp.json()
            exotel_sid = dial_json.get("Call", {}).get("Sid", "")
            if exotel_sid:
                latest = redis_store.get_pending_call("latest")
                latest["exotel_call_sid"] = exotel_sid
                redis_store.set_pending_call("latest", latest)
                redis_store.set_pending_call(exotel_sid, latest)
                # Update phone-keyed entry too
                redis_store.set_pending_call(f"phone:{phone_clean}", latest)
                if len(phone_clean) > 10:
                    redis_store.set_pending_call(f"phone:{phone_clean[-10:]}", latest)
                logger.info(f"[DIAL] Stored Exotel Call SID mapped: {exotel_sid}")
                # Poll Exotel for final call status (catches no-answer/busy/failed
                # when the WebSocket never opens and StatusCallback doesn't fire)
                asyncio.ensure_future(_poll_exotel_call_status(
                    exotel_sid, lead, phone_clean, auth_b64
                ))
        except Exception:
            pass
    except Exception as e:
        logger.error(f"[DIAL] Failed to trigger Exotel call: {e}")
        call_logger.call_event(phone_clean, "DIAL_ERROR", str(e))
        last_dial_result.update({"status": "error", "error": str(e)})

# ─── Dial Endpoints ──────────────────────────────────────────────────────────

@dial_router.post("/api/dial/{lead_id}")
async def api_dial_lead(lead_id: int, background_tasks: BackgroundTasks):
    lead = get_lead_by_id(lead_id)
    if not lead:
        return {"status": "error", "message": "Lead not found"}
    # Enforce TRAI calling hours (9 AM - 9 PM)
    org_id = lead.get("org_id")
    tz = get_org_timezone(org_id)
    guard = is_calling_allowed(tz)
    if not guard["allowed"]:
        next_time = get_next_allowed_time(tz)
        return {"status": "error", "message": f"Calling not allowed at this hour. TRAI rules: calls only between 9 AM - 9 PM. Next allowed: {next_time}"}
    # DND check
    if org_id and is_dnd_number(org_id, lead["phone"]):
        return {"status": "error", "message": "This number is on the DND list and cannot be called."}
    # Enforce plan minute limits
    if org_id:
        try:
            usage = get_usage_summary(org_id)
            if usage.get("has_subscription") and usage.get("minutes_remaining", 1) <= 0:
                return {"status": "error", "message": "No minutes remaining. Please upgrade your plan."}
        except Exception:
            pass  # Don't block calls if billing check fails
    from database import get_org_voice_settings
    call_data = {
        "name": lead["first_name"], "phone_number": lead["phone"],
        "interest": lead.get("interest") or lead["source"],
        "provider": DEFAULT_PROVIDER, "lead_id": lead_id,
    }
    if org_id:
        voice = get_org_voice_settings(org_id)
        if voice.get("tts_provider"):
            call_data["tts_provider"] = voice["tts_provider"]
        if voice.get("tts_voice_id"):
            call_data["tts_voice_id"] = voice["tts_voice_id"]
        if voice.get("tts_language"):
            call_data["tts_language"] = voice["tts_language"]
    background_tasks.add_task(initiate_call, call_data)
    return {"status": "success", "message": f"Dialing {lead['first_name']}..."}

@dial_router.post("/api/campaigns/{campaign_id}/dial/{lead_id}")
async def api_campaign_dial_lead(campaign_id: int, lead_id: int, background_tasks: BackgroundTasks):
    from database import get_campaign_by_id, get_campaign_voice_settings
    lead = get_lead_by_id(lead_id)
    campaign = get_campaign_by_id(campaign_id)
    if not lead:
        return {"status": "error", "message": "Lead not found"}
    if not campaign:
        return {"status": "error", "message": "Campaign not found"}
    # Enforce TRAI calling hours (9 AM - 9 PM)
    org_id = campaign.get("org_id")
    tz = get_org_timezone(org_id)
    guard = is_calling_allowed(tz)
    if not guard["allowed"]:
        next_time = get_next_allowed_time(tz)
        return {"status": "error", "message": f"Calling not allowed at this hour. TRAI rules: calls only between 9 AM - 9 PM. Next allowed: {next_time}"}
    # DND check
    if org_id and is_dnd_number(org_id, lead["phone"]):
        return {"status": "error", "message": "This number is on the DND list and cannot be called."}
    # Enforce plan minute limits
    if org_id:
        try:
            usage = get_usage_summary(org_id)
            if usage.get("has_subscription") and usage.get("minutes_remaining", 1) <= 0:
                return {"status": "error", "message": "No minutes remaining. Please upgrade your plan."}
        except Exception:
            pass  # Don't block calls if billing check fails
    # Multi-campaign guard: log if same lead is assigned to multiple campaigns
    try:
        _mcg_conn = get_conn()
        _mcg_cur = _mcg_conn.cursor()
        _mcg_cur.execute(
            "SELECT campaign_id FROM campaign_leads WHERE lead_id = %s AND campaign_id != %s",
            (lead_id, campaign_id),
        )
        _other_campaigns = _mcg_cur.fetchall()
        _mcg_conn.close()
        if _other_campaigns:
            _logging.getLogger("uvicorn.error").warning(
                "[MULTI-CAMPAIGN] Lead %s is in multiple campaigns: %s + current %s — check for duplicate assignment",
                lead_id, [r["campaign_id"] for r in _other_campaigns], campaign_id,
            )
    except Exception:
        pass

    voice = get_campaign_voice_settings(campaign_id, org_id)
    call_data = {
        "name": lead["first_name"], "phone_number": lead["phone"],
        "interest": campaign.get("product_name", lead.get("interest", "our platform")),
        "provider": DEFAULT_PROVIDER, "lead_id": lead_id,
        "campaign_id": campaign_id, "product_id": campaign.get("product_id"),
    }
    if voice.get("tts_provider"):
        call_data["tts_provider"] = voice["tts_provider"]
    if voice.get("tts_voice_id"):
        call_data["tts_voice_id"] = voice["tts_voice_id"]
    if voice.get("tts_language"):
        call_data["tts_language"] = voice["tts_language"]
    # Prevent retry worker from batch-dialing other campaign leads within 5 min
    redis_store.set_campaign_dial_active(campaign_id, ttl_seconds=300)
    background_tasks.add_task(initiate_call, call_data)
    from live_logs import emit_campaign_event
    emit_campaign_event(campaign_id, lead["first_name"], lead["phone"], "dialing",
                        f"via {DEFAULT_PROVIDER}")
    return {"status": "success", "message": f"Dialing {lead['first_name']} for campaign '{campaign['name']}'..."}

@dial_router.post("/api/campaigns/{campaign_id}/redial-failed")
async def api_campaign_redial_failed(campaign_id: int, background_tasks: BackgroundTasks):
    """Queue all Call Failed leads in a campaign for sequential redialing with 30s delay."""
    from database import get_campaign_by_id, get_campaign_leads, get_campaign_voice_settings
    import logging
    log = logging.getLogger("uvicorn.error")

    campaign = get_campaign_by_id(campaign_id)
    if not campaign:
        return {"status": "error", "message": "Campaign not found"}
    # Enforce TRAI calling hours (9 AM - 9 PM)
    org_id = campaign.get("org_id")
    tz = get_org_timezone(org_id)
    guard = is_calling_allowed(tz)
    if not guard["allowed"]:
        next_time = get_next_allowed_time(tz)
        return {"status": "error", "message": f"Calling not allowed at this hour. TRAI rules: calls only between 9 AM - 9 PM. Next allowed: {next_time}"}
    # Enforce plan minute limits
    if org_id:
        try:
            usage = get_usage_summary(org_id)
            if usage.get("has_subscription") and usage.get("minutes_remaining", 1) <= 0:
                return {"status": "error", "message": "No minutes remaining. Please upgrade your plan."}
        except Exception:
            pass

    leads = get_campaign_leads(campaign_id)
    failed_leads = [l for l in leads if l.get("status", "").startswith("Call Failed")]
    if not failed_leads:
        return {"status": "error", "message": "No failed leads to redial"}

    voice = get_campaign_voice_settings(campaign_id, campaign.get("org_id"))

    async def _redial_queue():
        from live_logs import emit_campaign_event
        camp_name = campaign.get("name", "")
        emit_campaign_event(campaign_id, "Campaign", camp_name, "started", f"Redialing {len(failed_leads)} failed leads")
        for i, lead in enumerate(failed_leads):
            if i > 0:
                await asyncio.sleep(30)
            # Skip DND numbers
            if org_id and is_dnd_number(org_id, lead['phone']):
                log.info(f"[REDIAL] Skipping DND number: {lead['phone']}")
                emit_campaign_event(campaign_id, lead['first_name'], lead['phone'], "dnd_skipped", "DND list")
                continue
            log.info(f"[REDIAL] {i+1}/{len(failed_leads)}: Dialing {lead['first_name']} ({lead['phone']})")
            emit_campaign_event(campaign_id, lead['first_name'], lead['phone'], "dialing", f"via {DEFAULT_PROVIDER} | redial {i+1}/{len(failed_leads)}")
            call_data = {
                "name": lead["first_name"], "phone_number": lead["phone"],
                "interest": campaign.get("product_name", lead.get("interest", "our platform")),
                "provider": DEFAULT_PROVIDER, "lead_id": lead["id"],
                "campaign_id": campaign_id, "product_id": campaign.get("product_id"),
            }
            if voice.get("tts_provider"):
                call_data["tts_provider"] = voice["tts_provider"]
            if voice.get("tts_voice_id"):
                call_data["tts_voice_id"] = voice["tts_voice_id"]
            if voice.get("tts_language"):
                call_data["tts_language"] = voice["tts_language"]
            try:
                await initiate_call(call_data)
            except Exception as e:
                log.error(f"[REDIAL] Failed for {lead['phone']}: {e}")
                emit_campaign_event(campaign_id, lead['first_name'], lead['phone'], "error", str(e)[:50])
        emit_campaign_event(campaign_id, "Campaign", camp_name, "finished", f"Redial complete: {len(failed_leads)} leads")

    # Hold the dial lock for the full expected duration of the queue
    redis_store.set_campaign_dial_active(campaign_id, ttl_seconds=len(failed_leads) * 35 + 60)
    background_tasks.add_task(_redial_queue)
    return {"status": "success", "message": f"Redialing {len(failed_leads)} failed leads (30s gap between calls)"}

@dial_router.post("/api/campaigns/{campaign_id}/dial-all")
async def api_campaign_dial_all(campaign_id: int, background_tasks: BackgroundTasks, force: bool = False):
    """Queue leads in a campaign for sequential dialing. force=true dials ALL regardless of status."""
    from database import get_campaign_by_id, get_campaign_leads, get_campaign_voice_settings
    import logging
    log = logging.getLogger("uvicorn.error")

    campaign = get_campaign_by_id(campaign_id)
    if not campaign:
        return {"status": "error", "message": "Campaign not found"}
    # Enforce TRAI calling hours (9 AM - 9 PM)
    org_id = campaign.get("org_id")
    tz = get_org_timezone(org_id)
    guard = is_calling_allowed(tz)
    if not guard["allowed"]:
        next_time = get_next_allowed_time(tz)
        return {"status": "error", "message": f"Calling not allowed at this hour. TRAI rules: calls only between 9 AM - 9 PM. Next allowed: {next_time}"}
    # Enforce plan minute limits
    if org_id:
        try:
            usage = get_usage_summary(org_id)
            if usage.get("has_subscription") and usage.get("minutes_remaining", 1) <= 0:
                return {"status": "error", "message": "No minutes remaining. Please upgrade your plan."}
        except Exception:
            pass

    leads = get_campaign_leads(campaign_id)
    if force:
        dialable = leads
    else:
        dialable = [l for l in leads if l.get("status", "new") in ("new", "New")]
    if not dialable:
        return {"status": "error", "message": "No leads to dial"}

    voice = get_campaign_voice_settings(campaign_id, campaign.get("org_id"))

    async def _dial_all_queue():
        from live_logs import emit_campaign_event
        camp_name = campaign.get("name", "")
        emit_campaign_event(campaign_id, "Campaign", camp_name, "started", f"Dialing {len(dialable)} new leads")
        for i, lead in enumerate(dialable):
            if i > 0:
                await asyncio.sleep(30)
            # Skip DND numbers
            if org_id and is_dnd_number(org_id, lead['phone']):
                log.info(f"[DIAL-ALL] Skipping DND number: {lead['phone']}")
                emit_campaign_event(campaign_id, lead['first_name'], lead['phone'], "dnd_skipped", "DND list")
                continue
            log.info(f"[DIAL-ALL] {i+1}/{len(dialable)}: Dialing {lead['first_name']} ({lead['phone']})")
            emit_campaign_event(campaign_id, lead['first_name'], lead['phone'], "dialing", f"via {DEFAULT_PROVIDER} | {i+1}/{len(dialable)}")
            call_data = {
                "name": lead["first_name"], "phone_number": lead["phone"],
                "interest": campaign.get("product_name", lead.get("interest", "our platform")),
                "provider": DEFAULT_PROVIDER, "lead_id": lead["id"],
                "campaign_id": campaign_id, "product_id": campaign.get("product_id"),
            }
            if voice.get("tts_provider"):
                call_data["tts_provider"] = voice["tts_provider"]
            if voice.get("tts_voice_id"):
                call_data["tts_voice_id"] = voice["tts_voice_id"]
            if voice.get("tts_language"):
                call_data["tts_language"] = voice["tts_language"]
            try:
                await initiate_call(call_data)
            except Exception as e:
                log.error(f"[DIAL-ALL] Failed for {lead['phone']}: {e}")
                emit_campaign_event(campaign_id, lead['first_name'], lead['phone'], "error", str(e)[:50])
        emit_campaign_event(campaign_id, "Campaign", camp_name, "finished", f"Dial complete: {len(dialable)} leads")
        log.info(f"[DIAL-ALL] Campaign {campaign_id} dial-all complete: {len(dialable)} leads")

    redis_store.set_campaign_dial_active(campaign_id, ttl_seconds=len(dialable) * 35 + 60)
    background_tasks.add_task(_dial_all_queue)
    return {"status": "success", "message": f"Dialing {len(dialable)} new leads (30s gap between calls)"}
