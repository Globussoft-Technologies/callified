# Callified WebSockets — React Integration Guide

> **Audience:** Frontend developers building React UIs that listen to live calls,
> show transcripts, monitor campaigns, or drive the AI Sandbox.
>
> **Companion doc:** [`WEBSOCKETS.md`](./WEBSOCKETS.md) is the backend API
> reference (URLs, payloads, smoke-tests). This document focuses on **how to
> use those APIs from React** — hooks, TypeScript types, reconnection,
> error handling, and copy-paste components.
>
> **Downloadable:** This file is plain Markdown. Save it as `.md`, print to PDF
> from any Markdown viewer, or open in Cursor / VS Code with a Markdown preview
> for an offline reference.

---

## Table of contents

1. [Quick start (60 seconds)](#1-quick-start-60-seconds)
2. [Endpoints at a glance](#2-endpoints-at-a-glance)
3. [Connection URLs & environment](#3-connection-urls--environment)
4. [Event reference (server → client)](#4-event-reference-server--client)
5. [Action reference (client → server)](#5-action-reference-client--server)
6. [TypeScript types](#6-typescript-types)
7. [The `useCallMonitor` hook (recommended)](#7-the-usecallmonitor-hook-recommended)
8. [Plain `useEffect` pattern (no hook)](#8-plain-useeffect-pattern-no-hook)
9. [Audio playback in React](#9-audio-playback-in-react)
10. [Reconnection & lifecycle](#10-reconnection--lifecycle)
11. [Error handling](#11-error-handling)
12. [Auth notes](#12-auth-notes)
13. [Testing in Postman / DevTools](#13-testing-in-postman--devtools)
14. [Troubleshooting](#14-troubleshooting)
15. [Production checklist](#15-production-checklist)

---

## 1. Quick start (60 seconds)

```tsx
import { useEffect, useRef, useState } from 'react';

export function MonitorCall({ streamSid, wsBase }) {
  const [transcripts, setTranscripts] = useState([]);
  const wsRef = useRef(null);

  useEffect(() => {
    const ws = new WebSocket(`${wsBase}/ws/monitor/${streamSid}`);
    wsRef.current = ws;

    ws.onmessage = (ev) => {
      const msg = JSON.parse(ev.data);
      if (msg.type === 'transcript') {
        setTranscripts((prev) => [...prev, msg]);
      }
    };

    return () => ws.close();
  }, [streamSid, wsBase]);

  return (
    <ul>
      {transcripts.map((t, i) => (
        <li key={i}><b>{t.role}:</b> {t.text}</li>
      ))}
    </ul>
  );
}
```

That's it. The rest of this guide explains every piece, plus production-grade
patterns (typed events, auto-reconnect, audio playback, takeover/whisper).

---

## 2. Endpoints at a glance

| Endpoint              | Purpose                                | When to use from React                                    |
|-----------------------|----------------------------------------|-----------------------------------------------------------|
| `/ws/monitor/{key}`   | Listen-in to one live call             | Supervisor dashboards, live transcript panels, takeover UI |
| `/media-stream`       | Bidirectional audio for one call       | The Sandbox / web-sim flow (mic in, TTS out)              |
| `/ws/sandbox`         | Alias for `/media-stream`              | Dev tooling                                               |

**`{key}`** = either `stream_sid` *or* `call_sid` (both work). The backend
holds the WS open for up to 30s waiting for the call to register, so it's
safe to open the monitor *immediately* after `POST /api/manual-call`
returns — no race between dial and connect.

For 95% of React use cases, you want **`/ws/monitor/{key}`**.

---

## 3. Connection URLs & environment

Build the URL from your existing API base by swapping `http`/`https` for `ws`/`wss`:

```ts
// frontend/src/lib/wsUrl.ts
export function wsBase(apiBase: string): string {
  return apiBase.replace(/^http/, 'ws');
}
```

Match how the rest of the app reads `API_URL`:

```ts
// frontend/src/lib/socket/config.ts
import { API_URL } from '../../constants/api';

export const WS_BASE = API_URL.replace(/^http/, 'ws');
// Examples:
//   https://testgo.callified.ai     →  wss://testgo.callified.ai
//   http://localhost:8000           →  ws://localhost:8000
```

> **Don't proxy WebSockets through Vite's `/api` proxy.** Vite's HTTP proxy
> doesn't upgrade cleanly in all setups. Open `ws://host:PORT/...` directly.

---

## 4. Event reference (server → client)

All frames are **JSON text frames**. Parse with `JSON.parse(ev.data)` and
switch on `msg.type` (or `msg.event` for media frames — see below).

### `transcript`
Live STT result for the user, or live agent line for the AI.

```json
{ "type": "transcript", "role": "user",  "text": "hello, who is this?" }
{ "type": "transcript", "role": "agent", "text": "Hi Akhil, I'm calling about..." }
```

| Field   | Type                | Notes                            |
|---------|---------------------|----------------------------------|
| `role`  | `"user" \| "agent"` | `user` = caller, `agent` = AI    |
| `text`  | `string`            | Final transcript line, no PII    |

### `audio`
A ~20 ms audio chunk for either side of the call. Base64-encoded.

```json
{ "type": "audio", "role": "user",  "format": "ulaw_8k", "payload": "<base64>" }
{ "type": "audio", "role": "agent", "format": "pcm16_8k", "payload": "<base64>" }
```

| Field     | Type                       | Notes                                      |
|-----------|----------------------------|--------------------------------------------|
| `role`    | `"user" \| "agent"`        | Direction                                  |
| `format`  | `"ulaw_8k" \| "pcm16_8k"`  | `ulaw_8k` for real dial, `pcm16_8k` for web-sim |
| `payload` | `string` (base64)          | Decode → AudioBuffer, see [§9](#9-audio-playback-in-react) |

### Error frame
Sent immediately after upgrade if the SID isn't valid or doesn't go live within 30s:

```json
{ "error": "session not found" }
```

After this frame the server closes the connection.

### Frames you can usually ignore in React
The monitor channel may also see `{"event":"clear"}` (barge-in marker) and
`{"event":"media", ...}` frames if you're attaching to the call WS itself
— those are protocol frames meant for the carrier, not for monitor UIs.

---

## 5. Action reference (client → server)

All actions are JSON text frames sent via `ws.send(JSON.stringify(...))`.

### `whisper`
Inject a hint into the AI's next turn — silently, not heard by the caller.

```json
{ "action": "whisper", "text": "mention our Q4 discount" }
```

### `takeover`
Disable the AI for this call. After this, `audio_chunk` frames are forwarded
straight to the phone — a human can speak through them.

```json
{ "action": "takeover" }
```

### `audio_chunk`
Live audio from a human operator (only honored after `takeover`).

```json
{ "action": "audio_chunk", "payload": "<base64>" }
```

> **Format must match the call:** `ulaw_8k` for dial, `pcm16_8k` for web-sim.
> The format you receive on `audio` events tells you which to send.

---

## 6. TypeScript types

Drop this in `frontend/src/lib/socket/types.ts`:

```ts
// ─── Server → Client ────────────────────────────────────────────────────────

export type Role = 'user' | 'agent';
export type AudioFormat = 'ulaw_8k' | 'pcm16_8k';

export interface TranscriptEvent {
  type: 'transcript';
  role: Role;
  text: string;
}

export interface AudioEvent {
  type: 'audio';
  role: Role;
  format: AudioFormat;
  payload: string; // base64
}

export interface ErrorFrame {
  error: string;
}

export type IncomingMessage = TranscriptEvent | AudioEvent | ErrorFrame;

// ─── Client → Server ────────────────────────────────────────────────────────

export interface WhisperAction {
  action: 'whisper';
  text: string;
}

export interface TakeoverAction {
  action: 'takeover';
}

export interface AudioChunkAction {
  action: 'audio_chunk';
  payload: string;
}

export type OutgoingAction = WhisperAction | TakeoverAction | AudioChunkAction;

// ─── Type guards (handy in switch statements) ───────────────────────────────

export const isTranscript = (m: IncomingMessage): m is TranscriptEvent =>
  'type' in m && m.type === 'transcript';

export const isAudio = (m: IncomingMessage): m is AudioEvent =>
  'type' in m && m.type === 'audio';

export const isError = (m: IncomingMessage): m is ErrorFrame =>
  'error' in m;
```

---

## 7. The `useCallMonitor` hook (recommended)

A self-contained hook with auto-reconnect, typed events, and lifecycle cleanup.
Drop in `frontend/src/lib/socket/useCallMonitor.ts`:

```ts
import { useEffect, useRef, useState, useCallback } from 'react';
import { WS_BASE } from './config';
import type {
  IncomingMessage,
  OutgoingAction,
  TranscriptEvent,
} from './types';
import { isTranscript, isAudio, isError } from './types';

export type MonitorStatus =
  | 'idle'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'disconnected'
  | 'error';

export interface UseCallMonitorOptions {
  /** Auto-reconnect with exponential backoff on disconnect (default: true) */
  autoReconnect?: boolean;
  /** Max reconnect attempts (default: 5) */
  maxReconnects?: number;
  /** Initial backoff in ms (default: 1000) */
  initialBackoffMs?: number;
}

export interface UseCallMonitorReturn {
  status: MonitorStatus;
  error: string | null;
  transcripts: TranscriptEvent[];
  /** Last audio frame — replace with a queue if you need history */
  lastAudio: IncomingMessage | null;
  /** Send a typed action to the server */
  send: (action: OutgoingAction) => void;
  /** Manually disconnect (cancels auto-reconnect) */
  disconnect: () => void;
}

export function useCallMonitor(
  streamSidOrCallSid: string | null,
  opts: UseCallMonitorOptions = {},
): UseCallMonitorReturn {
  const {
    autoReconnect = true,
    maxReconnects = 5,
    initialBackoffMs = 1000,
  } = opts;

  const [status, setStatus] = useState<MonitorStatus>('idle');
  const [error, setError] = useState<string | null>(null);
  const [transcripts, setTranscripts] = useState<TranscriptEvent[]>([]);
  const [lastAudio, setLastAudio] = useState<IncomingMessage | null>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectsRef = useRef(0);
  const backoffTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancelledRef = useRef(false);

  const cleanupTimer = () => {
    if (backoffTimerRef.current) {
      clearTimeout(backoffTimerRef.current);
      backoffTimerRef.current = null;
    }
  };

  const connect = useCallback(() => {
    if (!streamSidOrCallSid || cancelledRef.current) return;
    setError(null);
    setStatus(reconnectsRef.current === 0 ? 'connecting' : 'reconnecting');

    const ws = new WebSocket(
      `${WS_BASE}/ws/monitor/${encodeURIComponent(streamSidOrCallSid)}`,
    );
    wsRef.current = ws;
    let opened = false;

    ws.onopen = () => {
      opened = true;
      reconnectsRef.current = 0;
      setStatus('connected');
    };

    ws.onmessage = (ev) => {
      let msg: IncomingMessage;
      try {
        msg = JSON.parse(ev.data);
      } catch {
        return; // ignore non-JSON frames
      }

      if (isError(msg)) {
        setError(msg.error);
        setStatus('error');
        ws.close();
        return;
      }
      if (isTranscript(msg)) {
        setTranscripts((prev) => [...prev, msg]);
        return;
      }
      if (isAudio(msg)) {
        setLastAudio(msg);
        return;
      }
    };

    ws.onerror = () => {
      // onerror fires once before onclose; let onclose drive the state.
      if (!opened) setError('Connection failed');
    };

    ws.onclose = () => {
      wsRef.current = null;
      if (cancelledRef.current) {
        setStatus('disconnected');
        return;
      }
      if (
        autoReconnect &&
        reconnectsRef.current < maxReconnects &&
        opened // only reconnect if we ever connected — avoid loops on bad SIDs
      ) {
        const delay =
          initialBackoffMs * Math.pow(2, reconnectsRef.current);
        reconnectsRef.current += 1;
        setStatus('reconnecting');
        backoffTimerRef.current = setTimeout(connect, delay);
      } else {
        setStatus('disconnected');
      }
    };
  }, [streamSidOrCallSid, autoReconnect, maxReconnects, initialBackoffMs]);

  useEffect(() => {
    cancelledRef.current = false;
    reconnectsRef.current = 0;
    if (streamSidOrCallSid) connect();

    return () => {
      cancelledRef.current = true;
      cleanupTimer();
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [streamSidOrCallSid, connect]);

  const send = useCallback((action: OutgoingAction) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify(action));
  }, []);

  const disconnect = useCallback(() => {
    cancelledRef.current = true;
    cleanupTimer();
    if (wsRef.current) wsRef.current.close();
  }, []);

  return { status, error, transcripts, lastAudio, send, disconnect };
}
```

### Usage

```tsx
function LiveCallPanel({ callSid }: { callSid: string }) {
  const { status, transcripts, error, send } = useCallMonitor(callSid);

  return (
    <div>
      <header>Status: {status} {error && `— ${error}`}</header>
      <section>
        {transcripts.map((t, i) => (
          <div key={i} className={`turn turn-${t.role}`}>{t.text}</div>
        ))}
      </section>
      <button onClick={() => send({ action: 'whisper', text: 'Mention our discount' })}>
        Whisper
      </button>
      <button onClick={() => send({ action: 'takeover' })}>
        Take over
      </button>
    </div>
  );
}
```

---

## 8. Plain `useEffect` pattern (no hook)

If you don't want a hook abstraction, this is the minimum-correct pattern.
Mirrors how [`CallMonitor.jsx`](../../frontend/src/CallMonitor.jsx) is wired today.

```tsx
import { useEffect, useRef, useState } from 'react';

export function MonitorPanel({ streamSid, wsBase }) {
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState('');
  const [transcripts, setTranscripts] = useState([]);
  const wsRef = useRef(null);

  useEffect(() => {
    if (!streamSid) return;
    const ws = new WebSocket(`${wsBase}/ws/monitor/${streamSid}`);
    wsRef.current = ws;
    let opened = false;

    ws.onopen = () => { opened = true; setConnected(true); };
    ws.onmessage = (ev) => {
      const data = JSON.parse(ev.data);
      if (data.error) { setError(data.error); ws.close(); return; }
      if (data.type === 'transcript') {
        setTranscripts((prev) => [...prev, data]);
      }
    };
    ws.onclose = () => {
      setConnected(false);
      if (!opened) setError(`No active stream for "${streamSid}"`);
    };

    return () => ws.close();
  }, [streamSid, wsBase]);

  // ...render
}
```

**Watch out for:**
- The `opened` flag distinguishes "server rejected the SID" from a normal hang-up.
- `return () => ws.close()` is required to prevent zombie sockets across HMR / unmount.
- Don't put `wsRef` in the dep array — `useRef` is stable.

---

## 9. Audio playback in React

The monitor channel sends `audio` frames in two formats:

- **`pcm16_8k`** — signed 16-bit linear PCM at 8 kHz. Decode and feed Web Audio directly.
- **`ulaw_8k`** — μ-law compressed at 8 kHz. Decode μ-law → PCM16 first.

### `pcm16_8k` decoder

```ts
export function decodePcm16Base64(b64: string): Float32Array {
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  const view = new DataView(bytes.buffer);
  const samples = bytes.length / 2;
  const out = new Float32Array(samples);
  for (let i = 0; i < samples; i++) {
    out[i] = view.getInt16(i * 2, true) / 0x8000;
  }
  return out;
}
```

### `ulaw_8k` decoder (G.711)

```ts
export function decodeUlawBase64(b64: string): Float32Array {
  const bin = atob(b64);
  const out = new Float32Array(bin.length);
  for (let i = 0; i < bin.length; i++) {
    const u = ~bin.charCodeAt(i) & 0xff;
    const sign = u & 0x80;
    const exp = (u >> 4) & 0x07;
    const mant = u & 0x0f;
    let sample = ((mant << 3) + 0x84) << exp;
    sample -= 0x84;
    out[i] = (sign ? -sample : sample) / 0x8000;
  }
  return out;
}
```

### Streaming playback hook

```tsx
import { useEffect, useRef } from 'react';

export function useAudioPlayer() {
  const ctxRef = useRef<AudioContext | null>(null);
  const nextTimeRef = useRef(0);

  useEffect(() => {
    const ctx = new AudioContext({ sampleRate: 48000 });
    ctxRef.current = ctx;
    nextTimeRef.current = ctx.currentTime;
    return () => { ctx.close(); };
  }, []);

  function play(samples8k: Float32Array) {
    const ctx = ctxRef.current;
    if (!ctx) return;

    // Cheap 8 kHz → 48 kHz upsample (good enough for monitoring).
    const up = new Float32Array(samples8k.length * 6);
    for (let i = 0; i < up.length; i++) up[i] = samples8k[Math.floor(i / 6)];

    const buf = ctx.createBuffer(1, up.length, 48000);
    buf.getChannelData(0).set(up);

    const src = ctx.createBufferSource();
    src.buffer = buf;
    src.connect(ctx.destination);

    if (ctx.currentTime > nextTimeRef.current) {
      nextTimeRef.current = ctx.currentTime;
    }
    src.start(nextTimeRef.current);
    nextTimeRef.current += buf.duration;
  }

  return play;
}
```

### Wiring it up

```tsx
function ListenInPanel({ callSid }) {
  const { lastAudio } = useCallMonitor(callSid);
  const play = useAudioPlayer();

  useEffect(() => {
    if (!lastAudio || lastAudio.type !== 'audio') return;
    const samples = lastAudio.format === 'pcm16_8k'
      ? decodePcm16Base64(lastAudio.payload)
      : decodeUlawBase64(lastAudio.payload);
    play(samples);
  }, [lastAudio, play]);

  return <div>🎧 Listening live…</div>;
}
```

> **For production playback** consider an AudioWorklet over the `useAudioPlayer`
> pattern above — `BufferSource` works but loses precise scheduling during
> tab throttling. For supervisor dashboards (focused tab), the cheap version
> is fine.

---

## 10. Reconnection & lifecycle

### When to reconnect

| Scenario                          | Reconnect? |
|-----------------------------------|------------|
| Network blip (`onclose` after `onopen`) | ✅ Yes — exponential backoff |
| Server returned `{"error":...}` then closed | ❌ No — bad SID, won't fix |
| Component unmounted               | ❌ No — clean up                |
| User clicked "Disconnect"         | ❌ No — respect intent          |

The `useCallMonitor` hook above implements this: it only auto-reconnects if
the socket *actually opened* before closing. That avoids tight loops when
the SID is wrong.

### Backoff schedule

Default: 1s, 2s, 4s, 8s, 16s — capped at 5 attempts. Tune via `initialBackoffMs`
and `maxReconnects` options.

### Tab visibility

Browsers throttle WebSocket message timers in background tabs but **don't
close the socket**. If you're processing audio on a hidden tab, expect
queueing — UIs that just render transcripts are unaffected.

### Cleanup on unmount

Always close the socket in the `useEffect` cleanup. Otherwise React hot-reload
and route changes leak sockets and the backend's monitor list grows
unbounded:

```ts
useEffect(() => {
  const ws = new WebSocket(url);
  // ...
  return () => ws.close();   // ← critical
}, [url]);
```

---

## 11. Error handling

### Errors you'll see

| Symptom (frontend)                          | Cause                                                | Action |
|---------------------------------------------|------------------------------------------------------|--------|
| `onclose` fires before `onopen`             | Backend rejected the upgrade (bad query, length cap, missing route) | Show "couldn't connect" — don't reconnect |
| `{"error":"session not found"}` then close  | SID isn't a live call, or call hasn't started within 30s | Verify SID; for `dial` mode the callee may not have answered |
| `onerror` then `onclose` after 25–60s of silence | LB / proxy idle timeout                          | Reduce idle-timeout on LB or send keepalive client-side |
| `WebSocket is closed before connection is established` | URL wrong, mixed content (https page → ws://) | Use `wss://` from HTTPS pages; check `WS_BASE` |

### Mixed-content gotcha

A page loaded over `https://` **cannot** open `ws://`. Always derive WS scheme
from the page scheme or from your API base, never hardcode `ws://`:

```ts
const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
```

The `WS_BASE = API_URL.replace(/^http/, 'ws')` trick handles this automatically
because `https` → `wss`.

### Surfacing errors to users

```tsx
const { status, error } = useCallMonitor(sid);

if (status === 'error') return <ErrorBanner>{error}</ErrorBanner>;
if (status === 'reconnecting') return <Spinner>Reconnecting…</Spinner>;
if (status === 'connecting') return <Spinner>Connecting…</Spinner>;
```

---

## 12. Auth notes

**Today:** the monitor WS does **not** require auth. Anyone with a valid
`stream_sid` / `call_sid` can connect. SIDs are non-guessable but they're
also not secrets — treat them like CSRF tokens, not bearer tokens.

**Recommended hardening (server-side, not yet implemented):**
- Add ticket-based auth using the existing `/api/sse/ticket` endpoint.
- Frontend would fetch a 60-second ticket and append `?ticket=...` to the WS URL.
- Same pattern as SSE today (see [`backend/internal/api/auth.go`](../internal/api/auth.go)).

If you need this hardening, file a backend ticket — it's a small change but
not on the roadmap as of this writing.

### What auth *is* required for

`POST /api/manual-call` (which gives you the SID) **does** require a JWT bearer
token. So the practical protection is: only authenticated users can mint SIDs
in the first place.

---

## 13. Testing in Postman / DevTools

### Postman

1. **New → WebSocket Request**
2. URL: `wss://testgo.callified.ai/ws/monitor/<sid>` (or `ws://localhost:8000/...`)
3. **Connect**
4. Watch the *Messages* tab. Send actions from the message tab as JSON:
   ```json
   { "action": "whisper", "text": "test" }
   ```

If the SID is invalid you'll see `{"error":"session not found"}` then the
connection closes. That's the fast smoke-test for "is the WS endpoint up?".

### Chrome DevTools

1. Open the page that connects, then **Network → WS** filter
2. Click the WS row → **Messages** sub-tab
3. Each frame is shown with direction (↑ ↓), size, and a JSON tree expander.

### Quick `curl` upgrade probe

Verifies the route accepts upgrade requests without spinning up a real client:

```bash
curl -sS -o /dev/null -w "HTTP %{http_code}\n" \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  http://localhost:8000/ws/monitor/probe
# Expect: HTTP 101
```

---

## 14. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `WebSocket connection failed` immediately | URL wrong, server down, or HTTPS page hitting `ws://` | Use `WS_BASE = API_URL.replace(/^http/, 'ws')` |
| Connects then closes after ~1s with `{"error":"..."}` | Bad / expired SID | Verify SID is from a *current* call |
| Connects but no events arrive | Call is silent (nobody speaking yet) or AI hasn't started TTS | Wait, or check backend logs for `monitor connected` |
| Audio events arrive but speakers stay silent | Decoded wrong format | Check `format` field — `ulaw_8k` needs μ-law decode first |
| `transcripts` array grows forever | No bounding logic in component | Cap with `prev.slice(-200)` or virtualize |
| HMR leaves zombie sockets | Missing cleanup in `useEffect` | Always `return () => ws.close()` |
| Multiple monitors all show different transcripts | You're connected to the wrong session | Different `stream_sid` → different call |
| Whisper button doesn't work | Sent before `onopen` | Guard with `ws.readyState === WebSocket.OPEN` |

---

## 15. Production checklist

Before shipping a monitor UI to production:

- [ ] Use the typed `useCallMonitor` hook (or equivalent) — no inline `new WebSocket`
- [ ] Cleanup on unmount returns `() => ws.close()`
- [ ] Auto-reconnect only fires after a successful `onopen` (avoids tight loops on bad SIDs)
- [ ] Reconnection has exponential backoff and a max-attempts cap
- [ ] Connection status is shown to the user (`connecting` / `connected` / `reconnecting` / `error`)
- [ ] Error frames (`{"error":"..."}`) are surfaced, not silently swallowed
- [ ] WS URL is derived from API base (handles `http`→`ws`, `https`→`wss` automatically)
- [ ] Transcript array is bounded (cap at last N or virtualize) — long calls otherwise OOM the tab
- [ ] Audio playback uses `AudioContext` lifecycle correctly (closed on unmount)
- [ ] No hardcoded `localhost:8000` URLs — use `import.meta.env.VITE_*`
- [ ] CSP allows `connect-src` for the WS origin if you have a strict CSP
- [ ] Action sends are guarded with `ws.readyState === OPEN`
- [ ] Behind a reverse proxy: nginx/ALB has `proxy_read_timeout` ≥ 60s and WS upgrade headers configured
- [ ] Behind multiple backend pods: sticky sessions enabled (until Redis pub/sub bridge lands server-side)

---

## Appendix A — full integration example

End-to-end React component using `manual-call` API + `useCallMonitor`:

```tsx
import { useState } from 'react';
import { useCallMonitor } from './lib/socket/useCallMonitor';
import { API_URL } from './constants/api';

export function CallSupervisor() {
  const [callSid, setCallSid] = useState<string | null>(null);
  const [whisper, setWhisper] = useState('');
  const { status, error, transcripts, send } = useCallMonitor(callSid);

  async function startCall() {
    const r = await fetch(`${API_URL}/manual-call`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('token')}`,
      },
      body: JSON.stringify({
        name: 'Demo Lead',
        phone: '+919999900099',
        mode: 'dial',
        campaign_id: 940,
      }),
    });
    const { call_sid } = await r.json();
    setCallSid(call_sid);
  }

  return (
    <div>
      <button onClick={startCall}>Start call</button>
      <div>Status: {status} {error && <span style={{color:'red'}}>{error}</span>}</div>

      <div className="transcripts">
        {transcripts.map((t, i) => (
          <div key={i} className={`turn ${t.role}`}>
            <strong>{t.role}:</strong> {t.text}
          </div>
        ))}
      </div>

      <form onSubmit={(e) => {
        e.preventDefault();
        if (whisper) send({ action: 'whisper', text: whisper });
        setWhisper('');
      }}>
        <input
          value={whisper}
          onChange={(e) => setWhisper(e.target.value)}
          placeholder="Whisper a hint to the AI..."
        />
        <button type="submit">Send whisper</button>
      </form>

      <button onClick={() => send({ action: 'takeover' })}>
        Take over
      </button>
    </div>
  );
}
```

---

## Appendix B — file layout suggestion

Drop these files into your frontend repo:

```
frontend/src/lib/socket/
├── config.ts            # WS_BASE constant
├── types.ts             # Event + action types + guards
├── useCallMonitor.ts    # The recommended hook
├── audio.ts             # decodePcm16Base64, decodeUlawBase64
└── useAudioPlayer.ts    # Streaming playback hook
```

Then everywhere you need monitor functionality:

```ts
import { useCallMonitor } from '@/lib/socket/useCallMonitor';
```

---

## Appendix C — version & contact

| Item                | Value                                  |
|---------------------|----------------------------------------|
| Doc version         | 1.0 — initial React integration guide  |
| Backend WS routes   | Defined in [`backend/cmd/audiod/main.go`](../cmd/audiod/main.go) |
| Server handlers     | [`backend/internal/wshandler/`](../internal/wshandler/) |
| Existing reference  | [`WEBSOCKETS.md`](./WEBSOCKETS.md) — backend API ref |
| Sample JS client    | [`backend/examples/websocket-demo/index.html`](../examples/websocket-demo/index.html) |

For backend-side questions (event payloads, auth, scaling), see
[`WEBSOCKETS.md`](./WEBSOCKETS.md). For React-side questions, this doc.
