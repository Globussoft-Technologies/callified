import React, { useState, useEffect, useRef, useCallback } from 'react';
import { Device } from '@twilio/voice-sdk';
import { API_URL } from '../../constants/api';
import { useAuth } from '../../contexts/AuthContext';

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

// ── TwilioBrowserCallModal ───────────────────────────────────────────────────
// Uses the Twilio Voice SDK for direct WebRTC browser-to-phone calls.
// Audio goes: browser mic → Twilio WebRTC → PSTN → customer phone.
// No server relay = zero buffering delay (vs Exotel streaming bridge).
export default function TwilioBrowserCallModal({ lead, campaignId, callerPhone, onClose }) {
  const { apiFetch } = useAuth();
  const [status, setStatus] = useState('init'); // init | ready | calling | connected | ended | error
  const [errorMsg, setErrorMsg] = useState('');
  const [muted, setMuted] = useState(false);
  const [duration, setDuration] = useState(0);

  const deviceRef = useRef(null);
  const callRef = useRef(null);
  const timerRef = useRef(null);

  const stopTimer = () => {
    if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
  };

  const cleanup = useCallback(() => {
    stopTimer();
    if (callRef.current) {
      try { callRef.current.disconnect(); } catch {}
      callRef.current = null;
    }
    if (deviceRef.current) {
      try { deviceRef.current.destroy(); } catch {}
      deviceRef.current = null;
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function init() {
      // 1. Fetch Access Token from backend.
      let token;
      try {
        const res = await apiFetch(`${API_URL}/campaigns/${campaignId}/twilio-token`);
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
        token = data.token;
      } catch (e) {
        if (!cancelled) { setStatus('error'); setErrorMsg('Failed to get call token: ' + e.message); }
        return;
      }
      if (cancelled) return;

      // 2. Create Twilio Device.
      let device;
      try {
        device = new Device(token, { logLevel: 'warn', codecPreferences: ['opus', 'pcmu'] });
        deviceRef.current = device;
      } catch (e) {
        if (!cancelled) { setStatus('error'); setErrorMsg('Failed to init audio device: ' + e.message); }
        return;
      }

      device.on('error', (err) => {
        if (!cancelled) { setStatus('error'); setErrorMsg(err.message || 'Device error'); cleanup(); }
      });

      // 3. Register and then place the call.
      try {
        await device.register();
      } catch (e) {
        if (!cancelled) { setStatus('error'); setErrorMsg('Device registration failed: ' + e.message); }
        return;
      }
      if (cancelled) return;

      setStatus('calling');

      // 4. Place the call — Twilio POSTs to /webhook/twilio/voice with To + CallerId.
      let call;
      try {
        call = await device.connect({
          params: {
            To: lead.phone,
            CallerId: callerPhone || '',
          },
        });
        callRef.current = call;
      } catch (e) {
        if (!cancelled) { setStatus('error'); setErrorMsg('Failed to place call: ' + e.message); }
        return;
      }

      call.on('ringing', () => { if (!cancelled) setStatus('calling'); });
      call.on('accept', () => {
        if (!cancelled) {
          setStatus('connected');
          timerRef.current = setInterval(() => setDuration(d => d + 1), 1000);
        }
      });
      call.on('disconnect', () => {
        if (!cancelled) { setStatus('ended'); stopTimer(); }
      });
      call.on('cancel', () => {
        if (!cancelled) { setStatus('ended'); stopTimer(); }
      });
      call.on('error', (err) => {
        if (!cancelled) { setStatus('error'); setErrorMsg(err.message || 'Call error'); stopTimer(); }
      });
    }

    init();
    return () => { cancelled = true; cleanup(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleHangup = () => {
    if (callRef.current) { try { callRef.current.disconnect(); } catch {} }
    setStatus('ended');
    stopTimer();
  };

  const handleMute = () => {
    if (!callRef.current) return;
    const next = !muted;
    callRef.current.mute(next);
    setMuted(next);
  };

  const formatDuration = (s) => {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${String(sec).padStart(2, '0')}`;
  };

  const statusLabel = {
    init:      '🔄 Initializing…',
    calling:   '📞 Ringing…',
    connected: `🟢 Connected${duration > 0 ? ' · ' + formatDuration(duration) : ''}`,
    ended:     '✅ Call ended',
    error:     '⚠️ Error',
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

        {/* Info note */}
        {(status === 'init' || status === 'calling') && (
          <p style={{ color: T.sub, fontSize: '0.8rem', marginBottom: '1rem', lineHeight: 1.5, background: '#f8fafc', padding: '8px 12px', borderRadius: 8 }}>
            Dialing via <strong>Twilio WebRTC</strong> — audio goes directly to the customer's phone with no server delay.
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
            <button onClick={handleMute} style={{ ...btnGhost, color: muted ? T.red : T.sub }}>
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
