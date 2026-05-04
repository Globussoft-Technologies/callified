"""
live_logs.py — Server-Sent Events (SSE) live log streaming for the dashboard.
Includes both verbose technical logs and user-friendly campaign dial events.
"""
import collections
import logging
import asyncio
import datetime
import jwt
from fastapi import APIRouter
from starlette.responses import StreamingResponse
from fastapi.responses import JSONResponse

# All timestamps shown to users are in IST (UTC+5:30) regardless of the
# Docker container's system timezone (which defaults to UTC).
_IST = datetime.timezone(datetime.timedelta(hours=5, minutes=30))

def _ist_now() -> datetime.datetime:
    return datetime.datetime.now(_IST)

def _ist_ts() -> str:
    """HH:MM:SS in IST for embedding in live event strings."""
    return _ist_now().strftime('%H:%M:%S')

# ─── Log Buffer ──────────────────────────────────────────────────────────────

_live_log_buffer = collections.deque(maxlen=200)
_live_log_count = 0  # total items ever appended (never decrements)

# ─── Campaign Dial Events (user-friendly) ────────────────────────────────────

_campaign_events = collections.deque(maxlen=500)
_campaign_events_count = 0  # total events ever appended
# campaign_id (or 0=all) → datetime when the user last cleared the feed
_campaign_cleared_at: dict = {}

# Small cache: campaign_id → org_id so emit_campaign_event can tag events with
# the owning org without a DB query on every single call event.
_campaign_org_cache: dict = {}

def _resolve_org_for_campaign(campaign_id: int) -> int:
    """Return the org_id that owns this campaign (cached, best-effort)."""
    if campaign_id in _campaign_org_cache:
        return _campaign_org_cache[campaign_id]
    try:
        from database import get_conn
        conn = get_conn()
        cur = conn.cursor()
        cur.execute("SELECT org_id FROM campaigns WHERE id = %s", (campaign_id,))
        row = cur.fetchone()
        conn.close()
        if row:
            _campaign_org_cache[campaign_id] = int(row['org_id'])
            return _campaign_org_cache[campaign_id]
    except Exception:
        pass
    return 0

def emit_campaign_event(campaign_id: int, lead_name: str, phone: str, event_type: str, detail: str = ""):
    """Emit a user-friendly campaign dial event."""
    global _campaign_events_count
    icons = {
        "dialing": "📞", "ringing": "🔔", "connected": "✅", "no-answer": "❌", "busy": "📵",
        "failed": "⚠️", "completed": "🎯", "dnd": "🚫", "hangup": "👋",
        "error": "💥", "started": "🚀", "finished": "🏁",
        "retry_dialing": "🔁", "retry_exhausted": "🛑", "dnd_skipped": "🚫",
    }
    icon = icons.get(event_type, "📋")
    ts = _ist_ts()
    name_part = f"{lead_name} ({phone})" if phone else lead_name
    msg = f"{icon} [{ts}] {name_part} — {event_type.upper()}"
    if detail:
        msg += f" | {detail}"
    org_id = _resolve_org_for_campaign(campaign_id) if campaign_id else 0
    _campaign_events.append({"campaign_id": campaign_id, "org_id": org_id, "message": msg, "type": event_type})
    _campaign_events_count += 1

class _ISTFormatter(logging.Formatter):
    """Logging formatter that stamps times in IST regardless of host timezone."""
    def formatTime(self, record, datefmt=None):
        dt = datetime.datetime.fromtimestamp(record.created, tz=_IST)
        return dt.strftime(datefmt or '%H:%M:%S')

class _LiveLogHandler(logging.Handler):
    def emit(self, record):
        global _live_log_count
        msg = self.format(record)
        _live_log_buffer.append(msg)
        _live_log_count += 1

_llh = _LiveLogHandler()
_llh.setFormatter(_ISTFormatter('%(asctime)s %(levelname)s %(message)s', datefmt='%H:%M:%S'))
logging.getLogger('uvicorn.error').addHandler(_llh)

# ─── Router ──────────────────────────────────────────────────────────────────

live_logs_router = APIRouter()

@live_logs_router.post("/api/campaign-events/clear")
async def api_clear_campaign_events(campaign_id: int = 0):
    """Clear the in-memory campaign events buffer and record a cleared-at timestamp
    so the DB seeding fallback is suppressed on the next SSE reconnect."""
    global _campaign_events, _campaign_cleared_at
    now = datetime.datetime.utcnow()
    if campaign_id:
        keep = [ev for ev in list(_campaign_events) if ev.get("campaign_id") != campaign_id]
        _campaign_events = collections.deque(keep, maxlen=500)
        _campaign_cleared_at[campaign_id] = now
    else:
        _campaign_events.clear()
        _campaign_cleared_at[0] = now  # 0 = "all campaigns cleared"
    return {"status": "ok"}

@live_logs_router.post("/api/live-logs/clear")
async def api_clear_live_logs():
    """Clear the verbose log buffer so the next SSE reconnect starts fresh."""
    _live_log_buffer.clear()
    return {"status": "ok"}

@live_logs_router.get("/api/live-logs")
async def api_live_logs(token: str = ""):
    """SSE endpoint streaming server logs to the dashboard."""
    from auth import SECRET_KEY
    if not token:
        return JSONResponse(status_code=401, content={"detail": "Token required"})
    try:
        jwt.decode(token, SECRET_KEY, algorithms=["HS256"])
    except Exception:
        return JSONResponse(status_code=401, content={"detail": "Invalid token"})

    async def _gen():
        import time as _time
        # Send last 50 buffered lines on connect
        for line in list(_live_log_buffer)[-50:]:
            yield f"data: {line}\n\n"
        cursor = _live_log_count
        last_heartbeat = _time.monotonic()
        while True:
            await asyncio.sleep(0.5)
            if _time.monotonic() - last_heartbeat >= 15:
                yield ": keep-alive\n\n"
                last_heartbeat = _time.monotonic()
            if _live_log_count > cursor:
                new_count = _live_log_count - cursor
                for item in list(_live_log_buffer)[-new_count:]:
                    yield f"data: {item}\n\n"
                cursor = _live_log_count
                last_heartbeat = _time.monotonic()

    return StreamingResponse(_gen(), media_type="text/event-stream",
                             headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"})

@live_logs_router.get("/api/campaign-events")
async def api_campaign_events(token: str = "", campaign_id: int = 0):
    """SSE endpoint for user-friendly campaign dial progress."""
    from auth import SECRET_KEY
    if not token:
        return JSONResponse(status_code=401, content={"detail": "Token required"})
    try:
        payload = jwt.decode(token, SECRET_KEY, algorithms=["HS256"])
    except Exception:
        return JSONResponse(status_code=401, content={"detail": "Invalid token"})

    # Scope all events to the authenticated user's org — prevents cross-tenant leaks.
    caller_org_id = int(payload.get("org_id") or 0)

    def _matches(ev: dict) -> bool:
        """Return True if this event should be delivered to this connection."""
        if campaign_id != 0 and ev.get("campaign_id") != campaign_id:
            return False
        ev_org = ev.get("org_id", 0)
        # Events emitted before the org-tagging fix have org_id=0 — pass them through
        # so existing deployments don't lose history on first upgrade.
        if caller_org_id and ev_org and ev_org != caller_org_id:
            return False
        return True

    async def _gen():
        import time as _time
        import asyncio as _asyncio
        # Replay last 20 in-memory events for this campaign/org
        mem_events = [ev for ev in list(_campaign_events) if _matches(ev)]
        for ev in mem_events[-20:]:
            yield f"data: {ev['message']}\n\n"

        # Seed from DB only if the user has never cleared this campaign's feed.
        cleared = _campaign_cleared_at.get(campaign_id) or _campaign_cleared_at.get(0)
        if not mem_events and cleared is None:
            try:
                from database import get_conn
                def _fetch():
                    conn = get_conn()
                    cur = conn.cursor()
                    if campaign_id == 0:
                        # Filter by org so users only see their own org's history.
                        cur.execute("""
                            SELECT ct.created_at, l.first_name, l.phone, ct.call_duration_s
                            FROM call_transcripts ct
                            LEFT JOIN leads l ON ct.lead_id = l.id
                            JOIN campaigns ca ON ct.campaign_id = ca.id
                            WHERE ca.org_id = %s
                            ORDER BY ct.created_at DESC LIMIT 20
                        """, (caller_org_id,))
                    else:
                        cur.execute("""
                            SELECT ct.created_at, l.first_name, l.phone, ct.call_duration_s
                            FROM call_transcripts ct
                            LEFT JOIN leads l ON ct.lead_id = l.id
                            WHERE ct.campaign_id = %s
                            ORDER BY ct.created_at DESC LIMIT 20
                        """, (campaign_id,))
                    rows = cur.fetchall()
                    conn.close()
                    return rows
                rows = await _asyncio.get_event_loop().run_in_executor(None, _fetch)
                for row in reversed(rows):
                    raw = row['created_at']
                    if raw:
                        # MySQL returns naive datetimes stored as UTC → convert to IST
                        dt_utc = raw.replace(tzinfo=datetime.timezone.utc)
                        ts = dt_utc.astimezone(_IST).strftime('%H:%M:%S')
                    else:
                        ts = '--:--:--'
                    name = row.get('first_name') or 'Unknown'
                    phone = row.get('phone') or ''
                    dur = int(row.get('call_duration_s') or 0)
                    icon = '🎯' if dur > 0 else '❌'
                    label = 'COMPLETED' if dur > 0 else 'NO-ANSWER'
                    detail = f"{dur}s" if dur > 0 else "no answer"
                    msg = f"{icon} [{ts}] {name} ({phone}) — {label} | {detail}"
                    yield f"data: {msg}\n\n"
            except Exception:
                pass

        cursor = _campaign_events_count
        last_heartbeat = _time.monotonic()
        while True:
            await asyncio.sleep(1)
            if _time.monotonic() - last_heartbeat >= 15:
                yield ": keep-alive\n\n"
                last_heartbeat = _time.monotonic()
            if _campaign_events_count > cursor:
                new_count = _campaign_events_count - cursor
                for ev in list(_campaign_events)[-new_count:]:
                    if _matches(ev):
                        yield f"data: {ev['message']}\n\n"
                cursor = _campaign_events_count
                last_heartbeat = _time.monotonic()

    return StreamingResponse(_gen(), media_type="text/event-stream",
                             headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"})
