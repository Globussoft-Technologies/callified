# Callified AI: Architecture & Modules

The backend operates strictly via FastAPI (`uvicorn main:app`). As of Release 1, it successfully processes Bidirectional WebSockets for Sub-Second Audio Pipeline Streaming.

## Voice Pipeline (`ws_handler.py`)
This file is the heartbeat of the AI Dialer.

1. **Inbound Connection**: Twilio/Exotel Webhook invokes the WebSocket route.
2. **Deepgram STT (Listener)**: Ingests raw audio bytes from caller. We use an aggressive `LiveOptions(endpointing=300)` for hyper-fast barge-in detection.
3. **LLM Loop (`llm_provider.py`)**: When the user stops speaking, Gemini/Groq begins **Streaming** the LLM tokens immediately chunk by chunk through an asynchronous generator.
4. **Sentence Aggregator (`ws_handler.py`)**: As chunks arrive, NLTK/RegEx looks for boundary tokens (`?`, `!`, `.`, `\n`). The moment a boundary is hit, the finished sentence is dropped into a background `asyncio.Queue`.
5. **TTS Worker (`tts.py`)**: A parallel background loop constantly consumes sentences from the `asyncio.Queue`. It fires the sentence to ElevenLabs, instantly dumping `PCM/u-law` audio back over the phone port before the LLM has even finished reasoning the next paragraph.

*Result*: Sub-800ms Time-To-First-Byte audio latency.

## Frontend Dashboard
The frontend is a containerized `Vite React` architecture located perfectly in `frontend/src`.
- Modularized tabs: `<CrmTab />`, `<OpsTab />`, `<SettingsTab />`.
- Contextual state flows using purely hooks to fetch from the FastAPI `auth` routes natively seamlessly.

## External Endpoints
- `/api/auth/` (Login, Registration, Verification)
- `/api/leads` (Lead array dumping, CSV export engines)
- `/api/products` (Dynamic KB ingestion)

All queries bypass ORMs and use bare `pymysql.cursors.DictCursor` mapping for millisecond velocity in `database.py`.
