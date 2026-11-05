"""Private Hush WebSocket service. Audio is never gated on speech detection."""
import asyncio
import os
from contextlib import asynccontextmanager

from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from processor import CallProcessor, HushModel


@asynccontextmanager
async def lifespan(app):
    app.state.model = HushModel()
    app.state.attenuation = float(os.getenv('HUSH_ATTENUATION_DB', '6'))
    probe = CallProcessor(app.state.model, app.state.attenuation)
    probe.close()
    app.state.calls = 0
    yield
    app.state.model.close()


app = FastAPI(lifespan=lifespan)


@app.get('/health')
def health():
    return {'status': 'ok', 'model': 'weya-ai/hush', 'attenuation_db': app.state.attenuation}


@app.websocket('/process')
async def audio(ws: WebSocket):
    if ws.app.state.calls >= int(os.getenv('HUSH_MAX_CALLS', '16')):
        await ws.close(code=1013)
        return
    ws.app.state.calls += 1
    call = None
    try:
        await ws.accept()
        call = CallProcessor(ws.app.state.model, ws.app.state.attenuation)
        await ws.send_json({'ready': True, 'sample_rate': 8000, 'frame_bytes': call.frame_samples * 2})
        while True:
            # No fixed call-duration or inactivity cutoff. ASGI handles disconnect.
            pcm = await ws.receive_bytes()
            if not pcm or len(pcm) > 32768:
                await ws.close(code=1009)
                break
            work = asyncio.create_task(asyncio.to_thread(call.process, pcm))
            try:
                result = await asyncio.shield(work)
            except asyncio.CancelledError:
                await work  # never free native state while its thread is running
                raise
            await ws.send_bytes(result)  # empty means an incomplete model frame
    except WebSocketDisconnect:
        pass
    finally:
        if call:
            call.close()
        ws.app.state.calls -= 1
