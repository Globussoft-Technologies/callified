# Callified AI: Deployment & Environments

## Servers
Host Machine: `163.227.174.141`
User: `empcloud-development`

## Active Domains

### 1. `test.callified.ai`
This is your **primary development and testing sandbox**. All new features, experimental APIs, and code upgrades apply *directly* here.
- Directory: `/home/empcloud-development/callified-ai`
- Daemon: `systemctl status callified-ai.service`
- Proxy: `localhost:8001`
- Database: `callified_ai`

**Verified Live Telemetry (`callified-ai`):**
- `DEFAULT_PROVIDER`: `exotel`
- `LLM_PROVIDER`: `groq`
- `TTS_PROVIDER`: `elevenlabs`
- `PUBLIC_SERVER_URL`: `https://test.callified.ai`

### 2. `demo.callified.ai`
This is your **frozen Release 1 demo sandbox** solely provisioned for internal sales pitching. It represents the Sub-Second TTS/LLM upgrade. 
It must **NEVER** be updated during routine development to ensure it never crashes mid-pitch.
- Directory: `/home/empcloud-development/demo-callified-ai`
- Daemon: `systemctl status demo-callified-ai.service`
- Proxy: `localhost:8002`
- Database: `demo_callified_ai`

## Webhook Networking
If testing actively locally, you must utilize Ngrok:
`ngrok http 8000` -> map to `PUBLIC_SERVER_URL` inside your `.env`.

When deployed globally, the applications receive Exotel/Twilio callbacks precisely to `https://test.callified.ai/media-stream` and `https://demo.callified.ai/media-stream` autonomously. Let's Encrypt Certbot natively manages the NGINX SSL blocks.
