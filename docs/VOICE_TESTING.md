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

## Plivo incoming AI calls

1. Save a Plivo account with the Auth ID, Auth Token and the voice-enabled Plivo number. An existing Outbound Plivo account can receive calls too; keep it Outbound to continue using it for campaigns. An Inbound-only account is also supported.
2. In Provider Accounts, expand **Incoming call setup** on that account. Copy the Answer URL and Hangup URL generated for the saved account and organization. `PUBLIC_SERVER_URL` must be public HTTPS.
3. In the Plivo console, create an XML application with those URLs and set both methods to **POST**. Assign that application to the matching number. See [Plivo application configuration](https://www.plivo.com/docs/account/api/application).
4. Configure the organization's voice, language and maximum call duration, and ensure it has available calling credits. Incoming calls use these settings and the existing AI receptionist prompt, without requiring a campaign or an existing lead.
5. Call the Plivo number from another phone. Verify the incoming greeting, conversation in the configured language, hangup, recording, and inbound transcript/receptionist entry. Test an anonymous caller as well; their phone number should be requested rather than invented.

The backend accepts `POST /webhook/plivo/inbound` and `POST /webhook/plivo/inbound/status`. Both require Plivo V3 signatures and a called number matching the saved account. Answer retries reuse one log; completed calls cannot be reopened. Media uses single-use signed tickets, rejects duplicate connections and cannot fall back to another call's pending context. Incoming status callbacks do not trigger outbound retries.

This implementation answers incoming calls with the **AI receptionist**. It does not add a ringing/accept screen for a human browser agent. Showing setup URLs does not create a Plivo application or change live number routing.

### Editable incoming instructions

In AI Receptionist, enter **Opening greeting** and **AI instructions**, then click
**Save Instructions**. These organization-specific settings apply to subsequent
provider incoming calls and inbound browser tests. Include the desired questions,
required contact fields, offers and closing behavior in the instructions.
Voice and language continue to use Voice Settings. Existing organizations without
saved instructions use a minimal generic greeting/help prompt. Outgoing campaign
prompts are unchanged. Product knowledge is included as reference information.

The `org_receptionist_settings` table is ensured at backend startup; the equivalent
manual migration is `backend/scripts/migrations/20261028_receptionist_settings.sql`.
GET/PUT `/api/organizations/{id}/receptionist-settings` are authenticated and
organization-scoped; saving requires `settings.manage` permission.

Verify by saving distinctive instructions, starting a new incoming phone call and
a browser test, and confirming both follow the saved greeting and rules. Verify
another organization cannot read or edit these settings.

### Incoming call forwarding (Plivo)

Open **AI Receptionist → Call Forwarding**. Add a contact name and destination
phone number, then select **AI Receptionist** or that contact for the Plivo account
and click **Save Destination**. Choose the account whose Answer URL is assigned
to the number in Plivo; multiple saved accounts may represent the same number.
No webhook URL changes are required. Browser simulations always exercise AI.

Routing defaults to AI. Forwarding returns Plivo Dial/Number XML, rings for 30
seconds, limits the connected call to one hour, then hangs up. Busy/unanswered
calls end without automatically switching to AI. Plivo charges apply to the
forwarding leg; no AI session or AI transcript is created. Incoming call logs and
the existing signed hangup callback still track the original call. Routing is
snapshotted by organization and call SID, so retries keep the initial destination.

Numbers and routes are organization-scoped. Saving requires provider-account
management permission. A selected number cannot be deleted until its routes
are switched to AI. Own provider numbers and calls back to the original caller
are blocked to avoid direct loops.

Schema is ensured at startup; equivalent SQL:
`backend/scripts/migrations/20261028_incoming_routing.sql`.
Plivo XML reference: https://www.plivo.com/docs/voice/xml/routing

Manual checks: route a real incoming call to a saved contact; answer and confirm
both sides hear each other; test busy/unanswered behavior; switch to AI and
confirm the saved greeting; verify another organization cannot select/delete the
contact; verify deleting a selected contact is rejected. Real carrier forwarding
requires a live-call test after deployment.

### Incoming call handling history

Captured Inbound Customers includes a **Handled by** column: AI Receptionist or
Forwarded to the saved contact name. Ended forwarded calls are read from call
logs and have no AI transcript; they are not editable through the transcript
editor. New routing snapshots retain the contact name at call start. Older
snapshots fall back to a currently saved contact with the same destination phone,
or the destination phone when no name is available.

Startup adds `incoming_route_snapshots.destination_name`; equivalent manual SQL
is in `backend/scripts/migrations/20261028_incoming_route_names.sql`.
