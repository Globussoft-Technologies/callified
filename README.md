# Callified AI Dialer

A scalable, AI-powered conversational voice agent platform.

## Architecture & Modules

The backend is built with FastAPI and is organized into focused modules for maintainability:

1. **`main.py`**: The application entry point. Handles FastAPI app initialization, middleware setup, CRM polling background tasks, Twilio/Exotel webhooks, and routing definitions.
2. **`auth.py`**: Handles all authentication logic, including JWT token generation, password hashing (bcrypt), user login, signup, and user retrieval dependencies.
3. **`routes.py`**: Contains all REST API endpoints (`/api/*`). Manages CRUD operations for leads, organizations, products, CRM integrations, documents, reporting, and recording retrieval.
4. **`ws_handler.py`**: The core voice agent pipeline. Manages active WebSocket streams, Deepgram Speech-to-Text (STT), Google Gemini LLM reasoning, call state, barge-in detection, whispering, and call recording persistence.
5. **`tts.py`**: Handles Text-to-Speech synthesis. Streams synthesized audio from providers like ElevenLabs and SmallestAI via WebSockets to the active caller, handling format conversions (e.g., PCM to u-law).
6. **`live_logs.py`**: Implements Server-Sent Events (SSE) to stream live application logs from the backend directly to the frontend dashboard.
7. **`llm_provider.py`**: Abstraction layer for LLM generation, featuring primary routing to Groq with automatic fallback to Google Gemini for resilience.
8. **`database.py`**: Manages all raw PostgreSQL/MySQL queries and schema initializations.

## Call Recording Logic

Recordings are securely downloaded from the Exotel/Twilio API at the end of every call. The application detects whether the API returns a JSON pointer or raw audio bytes (MP3/WAV/OGG) and automatically saves the correct audio file locally in the `recordings/` directory to be served to the frontend dashboard.

## Deployment

Deployments to the production server can be achieved by running the deployment script. Note that you must have SSH access and a `.env` file configured.

```bash
python deploy_webhooks.py
```
