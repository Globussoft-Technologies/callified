"""One-shot smoke test for the /api/manual-call feature.

Logs in, starts a web-sim call, opens the monitor WS, and prints every
event it receives. The web-sim flow also needs someone to open /media-stream
with the returned stream_sid (a browser with mic). For a pure no-mic test,
just watch this script — the session starts the moment /media-stream opens,
and you'll see transcript + audio events stream through the monitor socket.

  pip install requests websockets
  EMAIL=you@example.com PASSWORD=... python test_manual_call.py
"""

import asyncio
import json
import os
import sys

import requests
import websockets

BASE = os.environ.get("BASE", "http://127.0.0.1:8001")
EMAIL = os.environ["EMAIL"]
PASSWORD = os.environ["PASSWORD"]
MODE = os.environ.get("MODE", "web-sim")  # "dial" or "web-sim"
NAME = os.environ.get("NAME", "TestUser")
PHONE = os.environ.get("PHONE", "+911234567890")


def login() -> str:
    r = requests.post(
        f"{BASE}/api/auth/login",
        json={"email": EMAIL, "password": PASSWORD},
        timeout=10,
    )
    r.raise_for_status()
    token = r.json().get("token") or r.json().get("access_token")
    if not token:
        raise RuntimeError(f"no token in login response: {r.text}")
    return token


def start_call(token: str) -> dict:
    r = requests.post(
        f"{BASE}/api/manual-call",
        headers={"Authorization": f"Bearer {token}"},
        json={"name": NAME, "phone": PHONE, "mode": MODE},
        timeout=20,
    )
    print(f"POST /api/manual-call -> {r.status_code}")
    r.raise_for_status()
    return r.json()


async def monitor(monitor_url: str) -> None:
    ws_url = BASE.replace("http", "ws") + monitor_url
    print(f"connecting monitor: {ws_url}")
    async with websockets.connect(ws_url) as ws:
        print("monitor connected — waiting up to 60s for events")
        try:
            while True:
                raw = await asyncio.wait_for(ws.recv(), timeout=60)
                evt = json.loads(raw)
                t = evt.get("type")
                if t == "transcript":
                    print(f"[{evt['role']}] {evt['text']}")
                elif t == "audio":
                    print(f"audio[{evt['role']}/{evt['format']}] {len(evt['payload'])}B b64")
                else:
                    print("event:", evt)
        except asyncio.TimeoutError:
            print("no events for 60s — did anything connect to /media-stream?")


def main() -> None:
    token = login()
    print("logged in")
    info = start_call(token)
    print(json.dumps(info, indent=2))
    asyncio.run(monitor(info["monitor_url"]))


if __name__ == "__main__":
    main()
