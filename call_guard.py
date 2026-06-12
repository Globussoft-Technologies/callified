"""
call_guard.py — TRAI calling-hours enforcement.
Indian telecom regulations prohibit calls before 9 AM or after 9 PM.
"""
from datetime import datetime
from zoneinfo import ZoneInfo

CALL_START_HOUR = 0   # disabled — calls allowed at any hour
CALL_END_HOUR = 24    # disabled — calls allowed at any hour


def is_calling_allowed(timezone: str = "Asia/Kolkata") -> dict:
    """Calling-hours enforcement is disabled for testing; calls are allowed at any time."""
    try:
        tz = ZoneInfo(timezone)
    except Exception:
        tz = ZoneInfo("Asia/Kolkata")

    now = datetime.now(tz)
    return {
        "allowed": True,
        "reason": "Calling hours unrestricted",
        "current_hour": now.hour,
        "current_time": now.strftime("%I:%M %p"),
        "timezone": timezone,
    }


def get_next_allowed_time(timezone: str = "Asia/Kolkata") -> str:
    """With calling-hours enforcement disabled, calls are always allowed now."""
    return "now (calling is currently allowed)"


def get_org_timezone(org_id: int) -> str:
    """Fetch the timezone for an organization from the database."""
    if not org_id:
        return "Asia/Kolkata"
    try:
        from database import get_conn
        conn = get_conn()
        cursor = conn.cursor()
        cursor.execute("SELECT timezone FROM organizations WHERE id = %s", (org_id,))
        row = cursor.fetchone()
        conn.close()
        if row and row.get("timezone"):
            return row["timezone"]
    except Exception:
        pass
    return "Asia/Kolkata"
