# Callified WebSockets

Three WebSocket endpoints. Pick the one that matches your use case.

| Endpoint              | Who opens it              | What it's for                                               |
|-----------------------|---------------------------|-------------------------------------------------------------|
| `/media-stream`       | Carrier (Exotel / Twilio) or a browser web-sim | The audio pipe for one call — STT in, TTS out  |
| `/ws/monitor/{key}`   | Your app / dashboard      | Listen in — live transcripts + both-sides audio + inject whispers |
| `/ws/sandbox`         | Dev tooling               | Browser dev harness                                         |

External projects almost always want `/ws/monitor/{key}` after calling `POST /api/manual-call`.

---

## Quick start — external integration (3 steps)

```
┌──────────────┐      POST /api/manual-call       ┌──────────────┐
│  Your app    │ ────────────────────────────────▶│  Callified   │
│              │ ◀──────── {monitor_url} ─────────│              │
│              │                                  │              │
│              │   open ws://.../ws/monitor/X     │              │
│              │ ◀──── transcript + audio ──────  │              │
└──────────────┘                                  └──────────────┘
```

**1. Get a JWT** (or use a user session you already have):

```bash
curl -s -X POST http://HOST:8001/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"..."}' | jq -r .token
```

**2. Start a call:**

```bash
curl -s -X POST http://HOST:8001/api/manual-call \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"Akhil","phone":"+91xxxxxxxxxx","mode":"dial"}'
# → {"mode":"dial","call_sid":"...","monitor_url":"/ws/monitor/..."}
```

**3. Open the monitor WebSocket.** Events arrive as JSON text frames.

---

## `POST /api/manual-call`

Single entry point that covers both flows.

**Headers**: `Authorization: Bearer <JWT>`

**Body**:

| Field         | Type     | Required | Notes                                                 |
|---------------|----------|----------|-------------------------------------------------------|
| `name`        | string   | yes      | Lead name — used in the greeting                      |
| `phone`       | string   | yes      | E.164 phone number                                    |
| `mode`        | string   | no       | `"dial"` (default) or `"web-sim"`                     |
| `campaign_id` | integer  | no       | If set, voice settings are taken from this campaign   |
| `interest`    | string   | no       | Short product/interest hint for the AI                |

**Response — `dial` mode:**

```json
{
  "mode": "dial",
  "call_sid": "ae3f...",
  "monitor_url": "/ws/monitor/ae3f..."
}
```

**Response — `web-sim` mode:**

```json
{
  "mode": "web-sim",
  "stream_sid": "web_sim_1_1777039000000",
  "media_stream_url": "/media-stream?stream_sid=web_sim_1_1777039000000",
  "monitor_url": "/ws/monitor/web_sim_1_1777039000000"
}
```

In `web-sim` mode the browser has to open `media_stream_url` with mic input — that's what feeds STT and plays TTS locally. The monitor is independent.

---

## `/ws/monitor/{key}`

`{key}` is either a `stream_sid` (web-sim) or `call_sid` (dial). If you pass
a `call_sid` before the carrier has connected the media stream, the server
holds the WS open and waits up to **30 seconds** for the session to register.

### Events the server sends

```json
{"type":"transcript","role":"user","text":"hello, who is this?"}
{"type":"transcript","role":"agent","text":"Hi Akhil, I'm calling about..."}
{"type":"audio","role":"user","format":"ulaw_8k","payload":"<base64>"}
{"type":"audio","role":"agent","format":"ulaw_8k","payload":"<base64>"}
{"error":"session not found"}
```

- `format` is `ulaw_8k` for dial (Exotel/Twilio) and `pcm16_8k` for web-sim.
- `role:"user"` is the caller's audio, `role:"agent"` is the AI's TTS output.
- Audio chunks are ~20ms each — about 160 bytes after base64 decode for ulaw, 320 bytes for pcm16.

### Actions you can send

```json
{"action":"whisper","text":"mention our Q4 discount"}
{"action":"takeover"}
{"action":"audio_chunk","payload":"<base64 ulaw>"}
```

- `whisper` — silently injects a hint into the AI's next turn.
- `takeover` — stops the AI; further `audio_chunk` frames are relayed straight to the phone so a human can speak.
- `audio_chunk` — only accepted after `takeover`. Payload must be ulaw for dial, pcm16 for web-sim (same format the AI uses on that call).

---

## `/media-stream`

The audio pipe. You normally don't open this from an external app — the
carrier does it for `dial`, and your browser does it for `web-sim`.

- **Exotel / Twilio** protocol: JSON text frames with `start` / `media` / `mark` / `stop` events (payload base64 ulaw).
- **Web-sim** protocol: either JSON frames with `{"event":"media","media":{"payload":"<pcm16_8k>"}}` *or* raw binary PCM16 frames. Session identity is derived from `?stream_sid=web_sim_...` on connect.

---

## Smoke-testing the WebSocket

Quickest verification without a real call:

```bash
# 1. Is the upgrade accepted?
curl -sS -o /dev/null -w "HTTP %{http_code}\n" \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  http://127.0.0.1:8001/ws/monitor/probe
# expect HTTP 101

# 2. Does it respond with real frames?
pip install websockets
python3 -c "
import asyncio, websockets
async def t():
    async with websockets.connect('ws://127.0.0.1:8001/ws/monitor/probe') as ws:
        print(await asyncio.wait_for(ws.recv(), 5))
asyncio.run(t())
"
# expect: {"error":"session not found"}

# 3. End-to-end: run examples/manual-call/test_manual_call.py
```

---

## Example clients

- [`examples/manual-call/manual_call.js`](../examples/manual-call/manual_call.js) — Node / browser
- [`examples/manual-call/manual_call.py`](../examples/manual-call/manual_call.py) — Python
- [`examples/manual-call/test_manual_call.py`](../examples/manual-call/test_manual_call.py) — one-shot smoke test that logs in, starts a call, and prints every event

---

## Common pitfalls

| Symptom                                                  | Cause                                                                                                                                      |
|----------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `{"error":"session not found"}` and WS closes            | The `key` you passed never became an active call within 30s. For `dial` mode, callee didn't answer; for `web-sim`, nobody opened `/media-stream`. |
| Monitor opens but no events                              | A call is active but nobody is speaking and the AI hasn't started (TTS keys missing?). Check `docker logs callified-go-audio`.             |
| 401 on `POST /api/manual-call`                           | JWT expired or missing. Log in again.                                                                                                      |
| 502 on dial mode                                         | Carrier rejected the number, DND list hit, or outside TRAI calling hours (9 AM – 9 PM IST). Error text in body.                            |
| Audio events arrive but nothing plays in browser         | Payload is `ulaw_8k` — decode to PCM before feeding Web Audio. Web-sim is already `pcm16_8k`, which most APIs accept directly.              |
