# Manual-call integration examples

Two-step flow for starting a call from an external project and consuming the
live transcript + audio stream.

1. `POST /api/manual-call` with `{name, phone, mode, campaign_id?}` — returns a
   `monitor_url` (WebSocket path).
2. Open the WebSocket. Events arriving on it:
   - `{"type":"transcript","role":"user|agent","text":"..."}`
   - `{"type":"audio","role":"user|agent","format":"ulaw_8k|pcm16_8k","payload":"<base64>"}`

Messages you can send on the same socket:
- `{"action":"whisper","text":"hint for AI"}` — inject a silent hint
- `{"action":"takeover"}` — a human operator takes over; stops the AI
- `{"action":"audio_chunk","payload":"<base64>"}` — after takeover, push audio
  directly to the phone

## Modes

| Mode       | What happens                                      | Use case                        |
|------------|---------------------------------------------------|---------------------------------|
| `dial`     | Real outbound call via Exotel/Twilio              | Call a customer from any app    |
| `web-sim`  | No carrier — browser opens `/media-stream` itself | Test or fully in-app soft-phone |

## Auth

Send your API key as `Authorization: Bearer <key>`. Obtain one via
`POST /api/keys` on the dashboard.

## Files

- [`manual_call.js`](./manual_call.js) — Node / browser client
- [`manual_call.py`](./manual_call.py) — Python client
