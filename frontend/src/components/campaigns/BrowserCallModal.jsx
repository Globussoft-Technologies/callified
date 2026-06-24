import React, { useState, useEffect, useRef, useCallback } from 'react';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', red: '#ef4444',
  text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif",
};

const btnPrimary = {
  background: T.accent, color: '#fff', border: 'none',
  borderRadius: 8, padding: '8px 18px', fontWeight: 600,
  cursor: 'pointer', fontFamily: T.font, fontSize: '0.9rem',
};
const btnDanger = {
  background: T.red, color: '#fff', border: 'none',
  borderRadius: 8, padding: '8px 18px', fontWeight: 600,
  cursor: 'pointer', fontFamily: T.font, fontSize: '0.9rem',
};
const btnGhost = {
  background: 'transparent', color: T.sub,
  border: `1px solid ${T.border}`, borderRadius: 8,
  padding: '8px 18px', fontWeight: 600, cursor: 'pointer',
  fontFamily: T.font, fontSize: '0.9rem',
};

// Resample Float32 array from srcRate → 8000 Hz (linear interpolation).
function resampleTo8k(input, srcRate) {
  if (srcRate === 8000) return input;
  const ratio = srcRate / 8000;
  const outLen = Math.floor(input.length / ratio);
  const out = new Float32Array(outLen);
  for (let i = 0; i < outLen; i++) {
    const src = i * ratio;
    const lo = Math.floor(src);
    const hi = Math.min(lo + 1, input.length - 1);
    const frac = src - lo;
    out[i] = input[lo] * (1 - frac) + input[hi] * frac;
  }
  return out;
}

// Convert Float32 PCM to Int16 PCM bytes.
function float32ToInt16(input) {
  const out = new Int16Array(input.length);
  for (let i = 0; i < input.length; i++) {
    out[i] = Math.max(-32768, Math.min(32767, input[i] * 32767));
  }
  return out;
}

// base64 encode an ArrayBuffer / TypedArray.
function toBase64(typedArray) {
  const bytes = new Uint8Array(typedArray.buffer || typedArray);
  let binary = '';
  for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
  return btoa(binary);
}

// Decode base64 PCM-16 bytes to Float32.
function base64ToPcmFloat32(b64) {
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  const int16 = new Int16Array(bytes.buffer);
  const f32 = new Float32Array(int16.length);
  for (let i = 0; i < int16.length; i++) f32[i] = int16[i] / 32768;
  return f32;
}

// ── BrowserCallModal ──────────────────────────────────────────────────────────
export default function BrowserCallModal({ lead, callSid, wsBaseUrl, onClose, onEnded }) {
  const [status, setStatus] = useState('connecting'); // connecting | waiting | connected | ended | error
  const [errorMsg, setErrorMsg] = useState('');
  const [muted, setMuted] = useState(false);
  const [duration, setDuration] = useState(0);

  const wsRef = useRef(null);
  const audioCtxRef = useRef(null);
  const processorRef = useRef(null);
  const nextPlayTimeRef = useRef(0);
  const timerRef = useRef(null);
  const streamRef = useRef(null);
  const mutedRef = useRef(false);
  const connectedRef = useRef(false); // true only after server sends status:connected
  const endedNotifiedRef = useRef(false);

  // Sync muted state to ref for use inside audio callbacks.
  useEffect(() => { mutedRef.current = muted; }, [muted]);

  const stopAll = useCallback(() => {
    if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
    if (audioDrainRef.current) { clearInterval(audioDrainRef.current); audioDrainRef.current = null; }
    if (processorRef.current) { try { processorRef.current.disconnect(); } catch { /* ignore */ } processorRef.current = null; }
    if (streamRef.current) { streamRef.current.getTracks().forEach(t => t.stop()); streamRef.current = null; }
    if (audioCtxRef.current && audioCtxRef.current.state !== 'closed') {
      audioCtxRef.current.close().catch(() => {});
      audioCtxRef.current = null;
    }
    if (wsRef.current) { try { wsRef.current.close(); } catch { /* ignore */ } wsRef.current = null; }
    audioQueueRef.current = [];
    audioStartedRef.current = false;
  }, []);

  // Incoming audio jitter buffer: collect a few Exotel frames before starting
  // playback so a late or missing 20 ms chunk doesn't swallow a short word.
  const audioQueueRef = useRef([]);
  const audioStartedRef = useRef(false);
  const audioDrainRef = useRef(null);
  const JITTER_TARGET_S = 0.08; // 80 ms ≈ 4 frames
  const MAX_BUFFER_S = 0.3;     // drop oldest if we exceed 300 ms

  const scheduleChunk = useCallback((f32, startAt) => {
    const ctx = audioCtxRef.current;
    if (!ctx || ctx.state === 'closed') return;
    try {
      const buffer = ctx.createBuffer(1, f32.length, 8000);
      buffer.copyToChannel(f32, 0);
      const source = ctx.createBufferSource();
      source.buffer = buffer;
      source.connect(ctx.destination);
      source.start(startAt);
    } catch { /* ignore */ }
  }, []);

  const drainAudioQueue = useCallback(() => {
    const ctx = audioCtxRef.current;
    if (!ctx || ctx.state === 'closed') return;
    const q = audioQueueRef.current;
    if (q.length === 0) {
      audioStartedRef.current = false;
      return;
    }

    // Keep buffer from growing without bound during a network burst.
    let bufferedS = q.reduce((sum, c) => sum + c.length, 0) / 8000;
    while (bufferedS > MAX_BUFFER_S && q.length > 0) {
      bufferedS -= q.shift().length / 8000;
    }

    // Wait until we have enough audio before we start playing.
    if (!audioStartedRef.current) {
      if (bufferedS < JITTER_TARGET_S) return;
      audioStartedRef.current = true;
      nextPlayTimeRef.current = ctx.currentTime + 0.02;
    }

    const now = ctx.currentTime;
    // If the schedule has drifted too far ahead (e.g. tab was backgrounded),
    // reset it so we don't sit on stale audio.
    if (nextPlayTimeRef.current > now + 0.3) {
      nextPlayTimeRef.current = now + 0.05;
    }

    while (q.length > 0 && nextPlayTimeRef.current < now + 0.3) {
      const f32 = q.shift();
      const startAt = Math.max(nextPlayTimeRef.current, now + 0.01);
      scheduleChunk(f32, startAt);
      nextPlayTimeRef.current = startAt + f32.length / 8000;
    }
  }, [scheduleChunk]);

  const playAudioChunk = useCallback((b64) => {
    if (!audioCtxRef.current) return;
    const f32 = base64ToPcmFloat32(b64);
    audioQueueRef.current.push(f32);
    if (!audioDrainRef.current) {
      audioDrainRef.current = setInterval(drainAudioQueue, 10);
    }
  }, [drainAudioQueue]);

  useEffect(() => {
    let cancelled = false;

    async function init() {
      // 1. Request mic permission.
      let stream;
      try {
        stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
      } catch (e) {
        if (!cancelled) { setStatus('error'); setErrorMsg('Microphone access denied: ' + e.message); }
        return;
      }
      if (cancelled) { stream.getTracks().forEach(t => t.stop()); return; }
      streamRef.current = stream;

      // 2. Create AudioContext for playback + mic processing.
      const ctx = new (window.AudioContext || window.webkitAudioContext)();
      audioCtxRef.current = ctx;
      nextPlayTimeRef.current = ctx.currentTime;

      // 3. Set up mic processor.
      const source = ctx.createMediaStreamSource(stream);
      const processor = ctx.createScriptProcessor(512, 1, 1);
      processorRef.current = processor;

      processor.onaudioprocess = (e) => {
        // Only relay mic audio after the call is actually connected.
        // During the "waiting/ringing" phase the relay goroutine on the server
        // hasn't started yet — frames queue up in TCP buffers, then flood Exotel
        // the moment the goroutine starts, causing 10-15 s of stale silence
        // to play before the agent's real speech reaches the customer.
        if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN || mutedRef.current || !connectedRef.current) return;
        const raw = e.inputBuffer.getChannelData(0);
        const pcm8k = resampleTo8k(raw, ctx.sampleRate);
        const int16 = float32ToInt16(pcm8k);
        const b64 = toBase64(int16);
        wsRef.current.send(JSON.stringify({ type: 'audio', payload: b64 }));
      };

      source.connect(processor);
      processor.connect(ctx.destination);

      // 4. Connect WebSocket.
      const ws = new WebSocket(`${wsBaseUrl}/ws/agent?call_sid=${encodeURIComponent(callSid)}`);
      wsRef.current = ws;

      ws.onmessage = (ev) => {
        let msg;
        try { msg = JSON.parse(ev.data); } catch { return; }
        if (msg.type === 'status') {
          setStatus(msg.status); // 'waiting' | 'connected'
          if (msg.status === 'connected') {
            connectedRef.current = true; // ungate mic sends
            timerRef.current = setInterval(() => setDuration(d => d + 1), 1000);
          }
        } else if (msg.type === 'audio') {
          playAudioChunk(msg.payload);
        } else if (msg.type === 'hangup') {
          setStatus('ended');
          stopAll();
        } else if (msg.type === 'error') {
          setStatus('error');
          setErrorMsg(msg.msg || 'Connection error');
          stopAll();
        }
      };

      ws.onerror = () => {
        if (!cancelled) { setStatus('error'); setErrorMsg('WebSocket connection failed'); stopAll(); }
      };
      ws.onclose = () => {
        if (!cancelled && status !== 'ended') { setStatus('ended'); stopAll(); }
      };
    }

    init();
    return () => { cancelled = true; stopAll(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Notify the parent once the call reaches a terminal state.
  // This lets a campaign auto-dialer advance to the next lead automatically.
  useEffect(() => {
    if ((status === 'ended' || status === 'error') && onEnded && !endedNotifiedRef.current) {
      endedNotifiedRef.current = true;
      onEnded(status, errorMsg);
    }
  }, [status, onEnded, errorMsg]);

  const handleHangup = () => {
    // Tell the backend to hang up the carrier leg so the customer's phone
    // line is actually released. Closing the WebSocket alone only tears down
    // the agent's browser connection.
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      try {
        wsRef.current.send(JSON.stringify({ type: 'hangup' }));
      } catch { /* ignore */ }
    }
    stopAll();
    setStatus('ended');
  };

  const formatDuration = (s) => {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${String(sec).padStart(2, '0')}`;
  };

  const statusLabel = {
    connecting: '🔄 Connecting…',
    waiting: '📞 Ringing…',
    connected: `🟢 Connected${duration > 0 ? ' · ' + formatDuration(duration) : ''}`,
    ended: '✅ Call ended',
    error: '⚠️ Error',
  }[status] || status;

  return (
    <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget && (status === 'ended' || status === 'error')) onClose(); }}>
      <div style={{
        background: '#fff', border: `1px solid ${T.border}`, borderRadius: 16,
        boxShadow: '0 8px 40px rgba(0,0,0,0.12)', maxWidth: 420, width: '100%',
        padding: '1.5rem', fontFamily: T.font,
      }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h3 style={{ margin: 0, color: T.text, fontSize: 18, fontWeight: 700 }}>🎙 Browser Call</h3>
          {(status === 'ended' || status === 'error') && (
            <button onClick={onClose} style={{ background: 'transparent', border: 'none', color: T.muted, fontSize: '1.2rem', cursor: 'pointer' }}>✕</button>
          )}
        </div>

        {/* Lead info */}
        <p style={{ color: T.muted, fontSize: '0.85rem', marginBottom: '1.25rem' }}>
          Calling <strong>{lead.first_name} {lead.last_name}</strong> — {lead.phone}
        </p>

        {/* How it works note */}
        {(status === 'connecting' || status === 'waiting') && (
          <p style={{ color: T.sub, fontSize: '0.8rem', marginBottom: '1rem', lineHeight: 1.5, background: '#f8fafc', padding: '8px 12px', borderRadius: 8 }}>
            Exotel is dialing the customer's phone. <strong>Please wait for “Connected” before speaking</strong> — your mic is muted until then to prevent audio from buffering during ringing.
          </p>
        )}

        {/* Status */}
        <div style={{
          padding: '10px 14px', borderRadius: 8, marginBottom: '1rem', fontSize: '0.875rem', fontWeight: 600,
          background: status === 'connected' ? 'rgba(16,185,129,0.08)' : status === 'error' ? '#fee2e2' : 'rgba(99,102,241,0.06)',
          color: status === 'connected' ? '#065f46' : status === 'error' ? T.red : T.accent,
          border: `1px solid ${status === 'connected' ? 'rgba(16,185,129,0.25)' : status === 'error' ? '#fca5a5' : 'rgba(99,102,241,0.15)'}`,
        }}>
          {statusLabel}
        </div>

        {status === 'error' && errorMsg && (
          <p style={{ color: T.red, fontSize: '0.8rem', marginBottom: '1rem' }}>{errorMsg}</p>
        )}

        {/* Controls */}
        <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: '1.25rem' }}>
          {status === 'connected' && (
            <button
              onClick={() => setMuted(m => !m)}
              style={{ ...btnGhost, color: muted ? T.red : T.sub }}>
              {muted ? '🔇 Unmute' : '🎙 Mute'}
            </button>
          )}
          {(status === 'ended' || status === 'error') ? (
            <button onClick={onClose} style={btnPrimary}>Close</button>
          ) : (
            <button onClick={handleHangup} style={btnDanger}>
              📵 Hang Up
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
