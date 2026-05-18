import React, { useState, useRef } from 'react';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

export default function CallMonitor({ apiUrl }) {
  const [streamSid, setStreamSid] = useState('');
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState('');
  const [transcripts, setTranscripts] = useState([]);
  const [whisperText, setWhisperText] = useState('');
  const [takeoverActive, setTakeoverActive] = useState(false);
  const wsRef = useRef(null);

  const connectToCall = () => {
    setError('');
    const sid = streamSid.trim();
    if (!sid) { setError('Stream SID is required'); return; }
    setConnecting(true);
    const wsUrl = apiUrl.replace('http', 'ws') + `/ws/monitor/${sid}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;
    let opened = false;

    ws.onopen = () => { opened = true; setConnecting(false); setConnected(true); };
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      if (data.error) { setError(data.error); ws.close(); return; }
      if (data.type === 'transcript') setTranscripts(prev => [...prev, data]);
    };
    ws.onclose = () => {
      if (!opened) setError(`No active stream found for SID "${sid}"`);
      setConnecting(false); setConnected(false);
    };
    ws.onerror = () => {
      if (!opened) setError('Could not connect to monitor stream. Check the SID and try again.');
    };
  };

  const sendWhisper = () => {
    if (wsRef.current && whisperText) {
      wsRef.current.send(JSON.stringify({ action: 'whisper', text: whisperText }));
      setTranscripts(prev => [...prev, { role: 'system', text: `Whisper sent: ${whisperText}` }]);
      setWhisperText('');
    }
  };

  const toggleTakeover = async () => {
    if (!takeoverActive && wsRef.current) {
      wsRef.current.send(JSON.stringify({ action: 'takeover' }));
      setTakeoverActive(true);
      setTranscripts(prev => [...prev, { role: 'system', text: 'Call Takeover Active. You are now speaking.' }]);
      try {
        await navigator.mediaDevices.getUserMedia({ audio: true });
      } catch (e) { console.error('Mic access denied'); }
    }
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title */}
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>🎙️ Live Call Monitor</h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Inject dynamic instructions into the AI's mind instantly, or take over the line if the client demands human interaction.
        </p>
      </div>

      <div style={{ ...card, padding: '24px 28px' }}>
        {!connected ? (
          <>
            {error && (
              <div style={{
                background: 'rgba(239,68,68,0.08)', border: `1px solid rgba(239,68,68,0.25)`,
                borderRadius: 8, padding: '10px 14px', marginBottom: 16,
                color: T.red, fontSize: 13,
              }}>
                {error}
              </div>
            )}
            <form onSubmit={(e) => { e.preventDefault(); connectToCall(); }}
              style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
              <input
                placeholder="Enter active Stream SID routing ID..."
                value={streamSid}
                onChange={(e) => { setStreamSid(e.target.value); if (error) setError(''); }}
                required disabled={connecting}
                style={{
                  flex: 1, padding: '10px 14px', borderRadius: 8, fontSize: 13,
                  border: `1px solid ${T.border}`, background: T.card,
                  color: T.text, fontFamily: T.font, outline: 'none',
                }}
              />
              <button type="submit" disabled={connecting || !streamSid.trim()} style={{
                padding: '10px 20px', borderRadius: 8, border: 'none',
                fontWeight: 600, fontSize: 13, fontFamily: T.font,
                background: connecting || !streamSid.trim() ? T.muted : T.accent,
                color: '#fff', cursor: connecting || !streamSid.trim() ? 'not-allowed' : 'pointer',
                whiteSpace: 'nowrap',
              }}>
                {connecting ? 'Connecting…' : 'Connect Monitor'}
              </button>
            </form>
          </>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

            {/* Connected bar */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{
                fontSize: 12, fontWeight: 600, padding: '4px 12px', borderRadius: 20,
                background: 'rgba(16,185,129,0.1)', color: T.green,
              }}>
                Connected to {streamSid}
              </span>
              <button onClick={() => wsRef.current?.close()} style={{
                padding: '7px 16px', borderRadius: 8, fontSize: 13, fontWeight: 600,
                cursor: 'pointer', fontFamily: T.font,
                border: `1px solid rgba(239,68,68,0.3)`,
                background: 'rgba(239,68,68,0.08)', color: T.red,
              }}>Disconnect</button>
            </div>

            {/* Transcript area */}
            <div style={{
              background: T.bg, borderRadius: 8, padding: '16px 20px',
              height: 350, overflowY: 'auto',
              display: 'flex', flexDirection: 'column', gap: 10,
            }}>
              {transcripts.length === 0 && (
                <p style={{ color: T.muted, textAlign: 'center', marginTop: '5rem', fontSize: 14 }}>
                  Waiting for speech...
                </p>
              )}
              {transcripts.map((t, idx) => (
                <div key={idx} style={{
                  alignSelf: t.role === 'user' ? 'flex-start' : t.role === 'system' ? 'center' : 'flex-end',
                  background: t.role === 'system'
                    ? 'rgba(245,158,11,0.08)'
                    : t.role === 'user'
                    ? 'rgba(99,102,241,0.08)'
                    : T.card,
                  border: `1px solid ${t.role === 'system' ? 'rgba(245,158,11,0.2)' : t.role === 'user' ? 'rgba(99,102,241,0.2)' : T.border}`,
                  padding: '10px 14px', borderRadius: 10, maxWidth: '80%', fontFamily: T.font,
                }}>
                  <strong style={{
                    display: 'block', fontSize: 10, textTransform: 'uppercase',
                    letterSpacing: '0.07em', marginBottom: 4, color: T.muted,
                  }}>
                    {t.role === 'user' ? 'Lead' : t.role === 'system' ? 'System' : 'AI Agent'}
                  </strong>
                  <span style={{ fontSize: 13, color: T.text }}>{t.text}</span>
                </div>
              ))}
            </div>

            {/* Whisper + Takeover */}
            <div style={{ display: 'flex', gap: 8 }}>
              <input
                placeholder="Type a whisper instruction to the AI (e.g. 'Offer 5% discount now')..."
                value={whisperText}
                onChange={e => setWhisperText(e.target.value)}
                disabled={takeoverActive}
                style={{
                  flex: 1, padding: '10px 14px', borderRadius: 8, fontSize: 13,
                  border: `1px solid ${T.border}`, background: T.card,
                  color: T.text, fontFamily: T.font, outline: 'none',
                }}
              />
              <button onClick={sendWhisper} disabled={takeoverActive} style={{
                padding: '10px 16px', borderRadius: 8, fontSize: 13, fontWeight: 600,
                cursor: takeoverActive ? 'not-allowed' : 'pointer', fontFamily: T.font,
                border: `1px solid rgba(99,102,241,0.3)`,
                background: 'rgba(99,102,241,0.08)', color: T.accent,
                whiteSpace: 'nowrap',
              }}>💬 Whisper</button>
              <button onClick={toggleTakeover} style={{
                padding: '10px 16px', borderRadius: 8, fontSize: 13, fontWeight: 600,
                cursor: 'pointer', fontFamily: T.font, whiteSpace: 'nowrap',
                border: `1px solid ${takeoverActive ? 'transparent' : 'rgba(239,68,68,0.3)'}`,
                background: takeoverActive ? T.red : 'rgba(239,68,68,0.08)',
                color: takeoverActive ? '#fff' : T.red,
              }}>
                {takeoverActive ? '🎤 Taking Over' : '🚨 Takeover Call'}
              </button>
            </div>

          </div>
        )}
      </div>
    </div>
  );
}
