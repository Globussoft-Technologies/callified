# Voice Testing Results

Voice quality testing results for each TTS provider and language combination. Updated as new voices and languages are tested.

## Testing Methodology

- Each voice is tested via the Sarvam/SmallestAI/ElevenLabs REST or WebSocket API
- Test phrase sent in the target language
- Audio response size (chars of base64) used as a proxy for naturalness — longer = more natural pacing, not rushed
- Qualitative assessment from live calls (prosody, clarity, accent)

---

## Bengali (bn-IN)

**Tested:** 2026-04-10

### Sarvam AI (Bulbul v3)

All 23 Sarvam voices support Bengali. Results sorted by audio quality:

| Voice | Gender | Audio Size | Style | Recommended |
|-------|--------|-----------|-------|-------------|
| amit | M | 90,376 | Professional | **Yes** |
| neha | F | 85,360 | Professional | **Yes** |
| roopa | F | 85,360 | Gentle | **Yes** |
| advait | M | 80,344 | Smooth | **Yes** |
| aayan | M | 75,324 | Young | **Yes** |
| manan | M | 75,324 | Warm | **Yes** |
| ishita | F | 75,324 | Confident | **Yes** |
| pooja | F | 70,304 | Kind | **Yes** |
| shubh | M | 70,304 | Energetic | |
| kabir | M | 60,272 | Bold | |
| simran | F | 60,272 | Cheerful | |
| rahul | M | 55,256 | Conversational | |
| rohan | M | 55,256 | Friendly | |
| sumit | M | 55,256 | Natural | |
| ratan | M | 55,256 | Mature | |
| priya | F | 55,256 | Friendly | |
| shreya | F | 55,256 | Bright | |
| aditya | M | 50,236 | Default | |
| ashutosh | M | 50,236 | Deep | |
| kavya | F | 65,288 | Soft | |
| varun | M | 45,216 | Calm | |
| ritu | F | 45,216 | Warm | |
| dev | M | 40,200 | Young | |

**STT Note:** Deepgram requires **nova-3** model for Bengali. Nova-2 does not support `bn` and returns HTTP 400.

### SmallestAI

Not yet tested for Bengali. Voices like `arnav` and `saurabh` are available but untested.

### ElevenLabs

Not yet tested for Bengali. ElevenLabs voices use opaque IDs and may not support `bn-IN` natively.

---

## Hindi (hi-IN)

**Tested:** 2026-03 (production usage)

### Sarvam AI (Bulbul v3)

All voices work well with Hindi. Most production campaigns use:

| Voice | Gender | Notes |
|-------|--------|-------|
| kabir | M | Bold, good for sales. Most tested voice. |
| amit | M | Professional tone, good for B2B |
| priya | F | Friendly, good engagement |
| neha | F | Professional, clear |

**STT:** Deepgram **nova-2** works well for Hindi.

---

## Marathi (mr-IN)

**Tested:** 2026-03 (production usage)

### Sarvam AI (Bulbul v3)

| Voice | Gender | Notes |
|-------|--------|-------|
| kabir | M | Primary voice for Marathi campaigns |
| amit | M | Professional alternative |
| rohan | M | Friendly tone |
| priya | F | Recommended female voice |
| neha | F | Clear, professional |

**STT:** Deepgram requires **nova-3** for Marathi. Nova-2 does not support `mr`.

---

## Provider Comparison

| Feature | Sarvam AI | SmallestAI | ElevenLabs |
|---------|-----------|------------|------------|
| Indian languages | 10 (hi, bn, mr, ta, te, kn, ml, gu, pa, en) | ~10 | Limited |
| Bengali support | All 23 voices | Untested | Untested |
| Voice count | 23 (14M + 9F) | 23 (10M + 13F) | 13 (8M + 5F) |
| Streaming | WebSocket | WebSocket | WebSocket |
| Best for | Indian languages | Indian languages | English fallback |
| Latency | ~2-3s | ~2-3s | ~1-2s |

---

## How to Test a New Voice/Language

Run on the server:

```python
import requests
KEY = "your_sarvam_api_key"
url = "https://api.sarvam.ai/text-to-speech"
headers = {"Api-Subscription-Key": KEY, "Content-Type": "application/json"}
payload = {
    "inputs": ["Test phrase in target language"],
    "target_language_code": "bn-IN",  # or hi-IN, mr-IN, etc.
    "speaker": "amit",
    "model": "bulbul:v3",
}
r = requests.post(url, json=payload, headers=headers, timeout=15)
print(r.status_code, len(r.json().get("audios", [[]])[0]))
```

## Adding Recommendations to the UI

Voice recommendations are defined in `frontend/src/constants/voices.js` in the `VOICE_RECOMMENDATIONS` object. When a language + provider combo has recommendations, the campaign voice dropdown shows them in a "Recommended" group with a star.

## Plivo AI and manual browser calls

Use a saved outbound Plivo account with an Auth ID, Auth Token, and caller number in international format. Select it in the campaign's Calling Account selector. The backend needs a public HTTPS `PUBLIC_SERVER_URL` for Plivo answer/status webhooks and the authenticated media stream.

- **AI call:** check the greeting and a longer response for crackle, gaps, and speed changes. Interrupt once, then let the final goodbye finish. Plivo's Gemini audio uses a stateful low-pass converter from 24 kHz to 8 kHz and continuous 20 ms G.711 packets; an interrupted response must not resume later.
- **Browser call:** allow microphone access, answer the customer phone, and speak in both directions. The browser should show Connected as soon as Plivo sends customer audio, with no AI greeting. Check mute/unmute and hangup.
- **Cancel while ringing:** hang up before the customer answers. The authenticated HTTP hangup endpoint cancels the carrier request even though no media WebSocket exists yet.
- **Authorization:** `/ws/agent` requires the short-lived call-specific ticket returned in `agent_url`. The dashboard can refresh it using `GET /api/calls/{call_sid}/agent-ticket`; only the initiating user in the same organization can obtain one. Long-lived login tokens and monitor tickets cannot join the bridge.

Run local regression tests without placing real calls:

```bash
cd backend
go test -race ./internal/audio ./internal/dial ./internal/wshandler ./internal/api -timeout 90s
```

These tests simulate the carrier and browser sockets. A real call is still required to validate carrier/network audio quality after deployment.
