"""Python example: start a manual outbound call and stream transcripts + audio.

Usage:
    BASE=http://localhost:8001 API_KEY=sk_live_... \
        python manual_call.py "Akhil" "+91xxxxxxxxxx"

Requires:  pip install requests websockets
"""

import asyncio
import base64
import json
import os
import sys

import requests
import websockets

BASE = os.environ.get("BASE", "http://localhost:8001")
API_KEY = os.environ["API_KEY"]


def start_call(name: str, phone: str, mode: str = "dial") -> dict:
    r = requests.post(
        f"{BASE}/api/manual-call",
        headers={"Authorization": f"Bearer {API_KEY}"},
        json={"name": name, "phone": phone, "mode": mode},
        timeout=20,
    )
    r.raise_for_status()
    return r.json()


async def monitor(monitor_url: str) -> None:
    ws_url = BASE.replace("http", "ws") + monitor_url
    async with websockets.connect(ws_url) as ws:
        print("monitor connected — waiting for events")
        async for raw in ws:
            evt = json.loads(raw)
            t = evt.get("type")
            if t == "transcript":
                print(f"[{evt['role']}] {evt['text']}")
            elif t == "audio":
                # evt["payload"] is base64; evt["format"] is "ulaw_8k" or "pcm16_8k".
                # Decode + play however your app wants; we just tally bytes here.
                raw_bytes = base64.b64decode(evt["payload"])
                print(f"audio[{evt['role']}/{evt['format']}] {len(raw_bytes)}B")
            elif evt.get("error"):
                print("monitor error:", evt["error"])


def main() -> None:
    if len(sys.argv) < 3:
        print("usage: python manual_call.py <name> <phone>", file=sys.stderr)
        sys.exit(1)
    name, phone = sys.argv[1], sys.argv[2]
    info = start_call(name, phone, mode="dial")
    print(f"call_sid={info['call_sid']}  monitor={info['monitor_url']}")
    asyncio.run(monitor(info["monitor_url"]))


if __name__ == "__main__":
    main()
