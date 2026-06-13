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

const VOICE_OPTIONS = {
  elevenlabs: {
    label: 'ElevenLabs',
    voices: [
      { id: 'oH8YmZXJYEZq5ScgoGn9', name: 'Aakash – Friendly Customer Support' },
      { id: 'X4ExprIXDKrWcHdtGysh', name: 'Anjura – Confident & Energetic' },
      { id: 'SXuKWBhKoIoAHKlf6Gt3', name: 'Gaurav – Professional Indian English' },
      { id: 'N09NFwYJJG9VSSgdLQbT', name: 'Ishan – Bold & Upbeat' },
      { id: 'U9wNM2BNANqtBCawWLgA', name: 'Himanshu – Calm & Serene' },
      { id: 'h061KGyOtpLYDxcoi8E3', name: 'Ravi – Gentle & Informative' },
      { id: 'Ock0AL5DBkvTUDePt4Hm', name: 'Viraj – Bold & Commanding' },
      { id: 'nwj0s2LU9bDWRKND5yzA', name: 'Bunty – Energetic & Fun' },
      { id: 'amiAXapsDOAiHJqbsAZj', name: 'Priya – Confident Female' },
      { id: '6JsmTroalVewG1gA6Jmw', name: 'Sia – Friendly Conversational' },
      { id: '9vP6R7VVxNwGIGLnpl17', name: 'Suhana – Young & Joyful' },
      { id: 'hO2yZ8lxM3axUxL8OeKX', name: 'Mini – Lively & Cute' },
      { id: 's0oIsoSJ9raiUm7DJNzW', name: '⭐ Current Default Voice' },
    ]
  },
  smallest: {
    label: 'Smallest AI',
    voices: [
      { id: 'mithali', name: 'Mithali (Hindi Female)' },
      { id: 'priya',   name: 'Priya (Hindi Female)' },
      { id: 'aravind', name: 'Aravind (Hindi Male)' },
      { id: 'raj',     name: 'Raj (Hindi Male)' },
      { id: 'arman',   name: 'Arman (Male)' },
      { id: 'jasmine', name: 'Jasmine (Female)' },
      { id: 'emily',   name: 'Emily (Female)' },
      { id: 'james',   name: 'James (Male)' },
    ]
  }
};

export default function Sandbox() {
  const [recording, setRecording] = useState(false);
  const [transcripts, setTranscripts] = useState([]);
  const [provider, setProvider] = useState('elevenlabs');
  const [voiceId, setVoiceId] = useState('oH8YmZXJYEZq5ScgoGn9');
  const [language, setLanguage] = useState('hi');
  const wsRef = useRef(null);
  const audioContextRef = useRef(null);
  const sourceRef = useRef(null);
  const processorRef = useRef(null);
  const activeSourcesRef = useRef([]);
  const nextPlayTimeRef = useRef(0);

  const handleProviderChange = (p) => {
    setProvider(p);
    setVoiceId(VOICE_OPTIONS[p].voices[0].id);
  };

  const startSandbox = async () => {
    setTranscripts([]);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: false, noiseSuppression: false, autoGainControl: false },
      });
      const audioContext = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: 16000 });
      audioContextRef.current = audioContext;

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const host = window.location.hostname;
      const qp = new URLSearchParams({
        name: 'Sandbox Tester', interest: 'product demo', lead_id: '0',
        tts_provider: provider, voice: voiceId, tts_language: 'multi',
      }).toString();

      let wsUrl;
      if (host === 'localhost' || host === '127.0.0.1') {
        wsUrl = `ws://${host}:8001/media-stream?${qp}`;
      } else {
        wsUrl = `${protocol}//${window.location.host}/media-stream?${qp}`;
      }

      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        setRecording(true);
        ws.send(JSON.stringify({ event: 'connected' }));
        const sid = `web_sim_sandbox_${Date.now()}`;
        ws.send(JSON.stringify({ event: 'start', start: { stream_sid: sid }, stream_sid: sid }));

        const source = audioContext.createMediaStreamSource(stream);
        sourceRef.current = source;
        const processor = audioContext.createScriptProcessor(2048, 1, 1);
        processorRef.current = processor;
        source.connect(processor);
        processor.connect(audioContext.destination);

        let micMuted = true;
        let unmuteTimer = null;
        // Failsafe: if greeting TTS doesn't fire media events (API failure), unmute after 5s
        const failsafeUnmute = setTimeout(() => { micMuted = false; }, 5000);

        processor.onaudioprocess = (e) => {
          if (ws.readyState !== WebSocket.OPEN || micMuted) return;
          const float32Array = e.inputBuffer.getChannelData(0);
          const outLen = Math.floor(float32Array.length / 2);
          const int16Buffer = new Int16Array(outLen);
          for (let i = 0; i < outLen; i++) {
            let s = Math.max(-1, Math.min(1, float32Array[i * 2]));
            int16Buffer[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
          }
          const bytes = new Uint8Array(int16Buffer.buffer);
          for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
          ws.send(JSON.stringify({ event: 'media', media: { payload: window.btoa(binary) } }));
        };

        nextPlayTimeRef.current = audioContext.currentTime;
        ws.onmessage = (event) => {
          const data = JSON.parse(event.data);
          if (data.type === 'clear') {
            activeSourcesRef.current.forEach(s => { try { s.stop(); } catch { /* ignore */ } });
            activeSourcesRef.current = [];
            nextPlayTimeRef.current = audioContext.currentTime;
            if (unmuteTimer) { clearTimeout(unmuteTimer); unmuteTimer = null; }
            micMuted = false;
          } else if (data.event === 'media') {
            const audioStr = window.atob(data.media.payload);
            const audioBytes = new Uint8Array(audioStr.length);
            for (let i = 0; i < audioStr.length; i++) audioBytes[i] = audioStr.charCodeAt(i);
            const int16Array = new Int16Array(audioBytes.buffer);
            const float32Array = new Float32Array(int16Array.length);
            for (let i = 0; i < int16Array.length; i++) float32Array[i] = int16Array[i] / 0x8000;
            const audioBuffer = audioContext.createBuffer(1, float32Array.length, 8000);
            audioBuffer.copyToChannel(float32Array, 0);
            const bufferSource = audioContext.createBufferSource();
            bufferSource.buffer = audioBuffer;
            bufferSource.connect(audioContext.destination);
            const now = audioContext.currentTime;
            const startAt = Math.max(now, nextPlayTimeRef.current);
            bufferSource.start(startAt);
            nextPlayTimeRef.current = startAt + audioBuffer.duration;
            activeSourcesRef.current.push(bufferSource);
            bufferSource.onended = () => {
              activeSourcesRef.current = activeSourcesRef.current.filter(s => s !== bufferSource);
            };
            if (micMuted && !unmuteTimer) {
              unmuteTimer = setTimeout(() => { micMuted = false; unmuteTimer = null; }, 400);
            }
          } else if (data.type === 'transcript') {
            const role = data.role === 'agent' ? 'assistant' : data.role;
            if (role && data.text) setTranscripts(prev => [...prev, { role, text: data.text }]);
          }
        };
      };

      ws.onerror = (e) => console.error('Sandbox WS error', e);
      ws.onclose = () => setRecording(false);
    } catch (e) {
      console.error('Sandbox failed', e);
      alert('Microphone access required. Please allow and retry.');
    }
  };

  const stopSandbox = () => {
    setRecording(false);
    if (processorRef.current && sourceRef.current) {
      sourceRef.current.disconnect();
      processorRef.current.disconnect();
    }
    if (wsRef.current) wsRef.current.close();
  };

  const currentVoices = VOICE_OPTIONS[provider]?.voices || [];
  const selectedVoiceName = currentVoices.find(v => v.id === voiceId)?.name || '';

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title */}
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>🎯 AI Training Sandbox</h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Roleplay and stress test the Voice Agent engine. Choose different TTS providers and voices to find the best fit.
        </p>
      </div>

    <div style={{ display: 'flex', gap: 16 }}>

      {/* Left: Simulation Controls */}
      <div style={{ ...card, padding: '24px 28px', flex: 1, display: 'flex', flexDirection: 'column', gap: 20 }}>
        <h3 style={{ margin: 0, fontSize: 15, fontWeight: 700, color: T.text, fontFamily: T.font }}>
          Simulation Controls
        </h3>

        {/* Provider toggle */}
        <div>
          <div style={{ fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 8, fontFamily: T.font }}>
            TTS Provider
          </div>
          <div style={{ display: 'flex', background: T.bg, borderRadius: 8, padding: 3, gap: 3 }}>
            {Object.entries(VOICE_OPTIONS).map(([key, val]) => (
              <button key={key} onClick={() => handleProviderChange(key)} disabled={recording}
                style={{
                  flex: 1, padding: '8px 14px', borderRadius: 6, border: 'none',
                  fontWeight: 600, fontSize: 13, fontFamily: T.font,
                  cursor: recording ? 'not-allowed' : 'pointer',
                  background: provider === key ? T.accent : 'transparent',
                  color: provider === key ? '#fff' : T.muted,
                  opacity: recording ? 0.6 : 1,
                  transition: 'all 0.15s',
                }}>
                {val.label}
              </button>
            ))}
          </div>
        </div>

        {/* Voice selector */}
        <div>
          <div style={{ fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 8, fontFamily: T.font }}>
            Voice
          </div>
          <select value={voiceId} onChange={e => setVoiceId(e.target.value)} disabled={recording}
            style={{
              width: '100%', padding: '9px 13px', borderRadius: 8, fontSize: 13,
              border: `1px solid ${T.border}`, background: T.card, color: T.text,
              fontFamily: T.font, outline: 'none', cursor: 'pointer',
              opacity: recording ? 0.6 : 1,
            }}>
            {currentVoices.map(v => (
              <option key={v.id} value={v.id}>{v.name}</option>
            ))}
          </select>
        </div>

        {/* Action buttons */}
        <div style={{ display: 'flex', gap: 8 }}>
          {!recording ? (
            <button onClick={startSandbox} style={{
              flex: 1, padding: '10px 18px', borderRadius: 8, border: 'none',
              fontWeight: 600, fontSize: 13, fontFamily: T.font,
              background: 'linear-gradient(135deg, #22c55e, #16a34a)',
              color: '#fff', cursor: 'pointer',
            }}>
              🎙️ Start Simulation
            </button>
          ) : (
            <button onClick={stopSandbox} style={{
              flex: 1, padding: '10px 18px', borderRadius: 8,
              fontWeight: 600, fontSize: 13, fontFamily: T.font,
              border: `1px solid ${T.red}`, background: 'rgba(239,68,68,0.08)',
              color: T.red, cursor: 'pointer',
            }}>
              ⏹️ Stop
            </button>
          )}
          <button onClick={() => setTranscripts([])} style={{
            padding: '10px 16px', borderRadius: 8, fontSize: 13, fontWeight: 600,
            fontFamily: T.font, cursor: 'pointer',
            border: `1px solid ${T.border}`, background: T.card, color: T.sub,
          }}>🗑️ Clear</button>
        </div>

        {/* Status panel */}
        <div style={{ background: T.bg, borderRadius: 8, padding: '14px 16px', display: 'flex', flexDirection: 'column', gap: 6 }}>
          {[
            { label: 'Mic', value: recording ? <span style={{ color: T.green }}>Active 🟢</span> : <span style={{ color: T.red }}>Off 🔴</span> },
            { label: 'Provider', value: <span style={{ color: T.accent }}>{VOICE_OPTIONS[provider].label}</span> },
            { label: 'Voice', value: <span style={{ color: T.accent }}>{selectedVoiceName}</span> },
          ].map(({ label, value }) => (
            <div key={label} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, fontFamily: T.font }}>
              <span style={{ color: T.muted }}>{label}:</span>
              <span style={{ fontWeight: 500 }}>{value}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Right: Live Transcripts */}
      <div style={{ ...card, padding: '24px 28px', flex: 2, display: 'flex', flexDirection: 'column' }}>
        <h3 style={{ margin: '0 0 16px', fontSize: 15, fontWeight: 700, color: T.text, fontFamily: T.font }}>
          Live Transcripts
        </h3>
        <div style={{
          flex: 1, background: T.bg, borderRadius: 8, padding: '20px',
          minHeight: 350, maxHeight: 450, display: 'flex', flexDirection: 'column',
          gap: 10, overflowY: 'auto',
        }}>
          {transcripts.length === 0 && (
            <p style={{ color: T.muted, textAlign: 'center', marginTop: '5rem', fontSize: 14, fontFamily: T.font }}>
              Click "Start Simulation" and speak...
            </p>
          )}
          {transcripts.map((t, idx) => (
            <div key={idx} style={{
              alignSelf: t.role === 'user' ? 'flex-start' : 'flex-end',
              background: t.role === 'user' ? 'rgba(99,102,241,0.08)' : T.card,
              border: `1px solid ${t.role === 'user' ? 'rgba(99,102,241,0.2)' : T.border}`,
              padding: '10px 14px', borderRadius: 12, maxWidth: '80%',
              fontFamily: T.font,
            }}>
              <strong style={{ display: 'block', fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 4, color: T.muted }}>
                {t.role === 'user' ? '👤 You' : '🤖 AI Agent'}
              </strong>
              <span style={{ fontSize: 13, color: T.text }}>{t.text}</span>
            </div>
          ))}
        </div>
      </div>

    </div>
    </div>
  );
}
