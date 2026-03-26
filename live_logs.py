"""
live_logs.py — Server-Sent Events (SSE) live log streaming for the dashboard.
"""
import collections
import logging
import asyncio
import jwt
from fastapi import APIRouter
from starlette.responses import StreamingResponse
from fastapi.responses import JSONResponse

# ─── Log Buffer ──────────────────────────────────────────────────────────────

_live_log_buffer = collections.deque(maxlen=200)

class _LiveLogHandler(logging.Handler):
    def emit(self, record):
        msg = self.format(record)
        _live_log_buffer.append(msg)

_llh = _LiveLogHandler()
_llh.setFormatter(logging.Formatter('%(asctime)s %(levelname)s %(message)s', datefmt='%H:%M:%S'))
logging.getLogger('uvicorn.error').addHandler(_llh)

# ─── Router ──────────────────────────────────────────────────────────────────

live_logs_router = APIRouter()

@live_logs_router.get("/api/live-logs")
async def api_live_logs(token: str = ""):
    """SSE endpoint streaming server logs to the dashboard. Accepts token as query param (SSE can't set headers)."""
    from auth import SECRET_KEY
    if not token:
        return JSONResponse(status_code=401, content={"detail": "Token required"})
    try:
        jwt.decode(token, SECRET_KEY, algorithms=["HS256"])
    except Exception:
        return JSONResponse(status_code=401, content={"detail": "Invalid token"})

    async def _gen():
        sent = len(_live_log_buffer)
        for line in list(_live_log_buffer)[-50:]:
            yield f"data: {line}\n\n"
        while True:
            await asyncio.sleep(0.5)
            while sent < len(_live_log_buffer):
                idx = sent - len(_live_log_buffer)
                if idx >= 0:
                    break
                try:
                    yield f"data: {_live_log_buffer[idx]}\n\n"
                except IndexError:
                    break
                sent += 1

    return StreamingResponse(_gen(), media_type="text/event-stream", headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"})
