import React, { useCallback, useEffect, useRef, useState } from 'react';
import AppIcon from './components/common/AppIcon';

const T = { bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb', accent: '#6366f1', green: '#10b981', red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af', font: "'DM Sans', sans-serif" };

function bytesFromBase64(value) {
  const binary = atob(value);
  return Uint8Array.from(binary, char => char.charCodeAt(0));
}

function base64FromBytes(bytes) {
  let binary = '';
  bytes.forEach(byte => { binary += String.fromCharCode(byte); });
  return btoa(binary);
}

function decodeMuLaw(value) {
  const mu = (~value) & 0xff;
  const sign = mu & 0x80;
  const exponent = (mu >> 4) & 0x07;
  const mantissa = mu & 0x0f;
  const sample = (((mantissa << 3) + 0x84) << exponent) - 0x84;
  return (sign ? -sample : sample) / 32768;
}

function encodeMuLaw(sample) {
  const bias = 0x84;
  let pcm = Math.max(-1, Math.min(1, sample)) * 32767;
  const sign = pcm < 0 ? 0x80 : 0;
  if (pcm < 0) pcm = -pcm;
  pcm = Math.min(32635, Math.round(pcm)) + bias;
  let exponent = 7;
  for (let mask = 0x4000; exponent > 0 && (pcm & mask) === 0; mask >>= 1) exponent -= 1;
  const mantissa = (pcm >> (exponent + 3)) & 0x0f;
  return (~(sign | (exponent << 4) | mantissa)) & 0xff;
}

function decodeAudio(payload, format) {
  const bytes = bytesFromBase64(payload);
  if (format === 'ulaw_8k') return Float32Array.from(bytes, decodeMuLaw);
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const samples = new Float32Array(Math.floor(bytes.length / 2));
  for (let i = 0; i < samples.length; i += 1) samples[i] = view.getInt16(i * 2, true) / 32768;
  return samples;
}

function resampleTo8k(input, inputRate) {
  if (inputRate === 8000) return input;
  const ratio = inputRate / 8000;
  const output = new Float32Array(Math.max(1, Math.floor(input.length / ratio)));
  for (let i = 0; i < output.length; i += 1) {
    const start = Math.floor(i * ratio);
    const end = Math.min(input.length, Math.floor((i + 1) * ratio));
    let sum = 0;
    for (let j = start; j < end; j += 1) sum += input[j];
    output[i] = sum / Math.max(1, end - start);
  }
  return output;
}

function encodeAudio(samples, format) {
  if (format === 'ulaw_8k') return base64FromBytes(Uint8Array.from(samples, encodeMuLaw));
  const bytes = new Uint8Array(samples.length * 2);
  const view = new DataView(bytes.buffer);
  samples.forEach((sample, index) => view.setInt16(index * 2, Math.max(-32768, Math.min(32767, Math.round(sample * 32767))), true));
  return base64FromBytes(bytes);
}

export default function CallMonitor({ apiUrl, apiFetch }) {
  const campaignFilter = new URLSearchParams(window.location.search).get('campaign_id');
  const [calls, setCalls] = useState([]);
  const [loadingCalls, setLoadingCalls] = useState(true);
  const [selectedCall, setSelectedCall] = useState(null);
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState('');
  const [transcripts, setTranscripts] = useState([]);
  const [whisperText, setWhisperText] = useState('');
  const [takeoverActive, setTakeoverActive] = useState(false);
  const wsRef = useRef(null);
  const audioContextRef = useRef(null);
  const playbackTimesRef = useRef({ user: 0, agent: 0 });
  const micRef = useRef(null);

  const fetchActiveCalls = useCallback(async () => {
    try {
      const res = await apiFetch(`${apiUrl}/active-calls`);
      if (!res.ok) throw new Error('Unable to load active calls');
      const data = await res.json();
      const activeCalls = Array.isArray(data.active_calls) ? data.active_calls : [];
      setCalls(campaignFilter ? activeCalls.filter(call => String(call.campaign_id) === campaignFilter) : activeCalls);
      setError('');
    } catch (fetchError) {
      setError(fetchError.message);
    } finally {
      setLoadingCalls(false);
    }
  }, [apiFetch, apiUrl, campaignFilter]);

  useEffect(() => {
    fetchActiveCalls();
    const timer = window.setInterval(fetchActiveCalls, 3000);
    return () => window.clearInterval(timer);
  }, [fetchActiveCalls]);

  const stopMicrophone = useCallback(() => {
    const mic = micRef.current;
    if (!mic) return;
    mic.processor.disconnect();
    mic.source.disconnect();
    mic.silent.disconnect();
    mic.stream.getTracks().forEach(track => track.stop());
    micRef.current = null;
  }, []);

  const disconnect = useCallback(() => {
    stopMicrophone();
    wsRef.current?.close();
    wsRef.current = null;
    setConnected(false);
    setConnecting(false);
    setTakeoverActive(false);
    setSelectedCall(null);
  }, [stopMicrophone]);

  useEffect(() => () => {
    stopMicrophone();
    wsRef.current?.close();
    audioContextRef.current?.close();
  }, [stopMicrophone]);

  const playAudio = useCallback(message => {
    const context = audioContextRef.current;
    if (!context || context.state === 'closed') return;
    const samples = decodeAudio(message.payload, message.format);
    if (!samples.length) return;
    const buffer = context.createBuffer(1, samples.length, 8000);
    buffer.copyToChannel(samples, 0);
    const source = context.createBufferSource();
    source.buffer = buffer;
    source.connect(context.destination);
    const role = message.role === 'user' ? 'user' : 'agent';
    const startAt = Math.max(context.currentTime + 0.02, playbackTimesRef.current[role] || 0);
    source.start(startAt);
    playbackTimesRef.current[role] = startAt + buffer.duration;
  }, []);

  const connectToCall = async call => {
    setError('');
    setConnecting(true);
    setTranscripts([]);
    setSelectedCall(call);
    try {
      if (!audioContextRef.current || audioContextRef.current.state === 'closed') audioContextRef.current = new AudioContext();
      await audioContextRef.current.resume();
      playbackTimesRef.current = { user: 0, agent: 0 };
      const ticketRes = await apiFetch(`${apiUrl}/monitor/ticket`);
      if (!ticketRes.ok) throw new Error('Unable to authorize live monitoring');
      const { ticket } = await ticketRes.json();
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws/monitor/${encodeURIComponent(call.stream_sid)}?ticket=${encodeURIComponent(ticket)}`;
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;
      ws.onopen = () => { setConnecting(false); setConnected(true); };
      ws.onmessage = event => {
        const data = JSON.parse(event.data);
        if (data.error) { setError(data.error); ws.close(); return; }
        if (data.type === 'transcript') setTranscripts(previous => [...previous.slice(-199), data]);
        if (data.type === 'audio') playAudio(data);
      };
      ws.onclose = () => {
        stopMicrophone();
        setConnecting(false);
        setConnected(false);
        setTakeoverActive(false);
      };
      ws.onerror = () => setError('Could not connect to this live call');
    } catch (connectError) {
      setError(connectError.message);
      setConnecting(false);
      setSelectedCall(null);
    }
  };

  const sendWhisper = () => {
    const text = whisperText.trim();
    if (!text || wsRef.current?.readyState !== WebSocket.OPEN) return;
    wsRef.current.send(JSON.stringify({ action: 'whisper', text }));
    setTranscripts(previous => [...previous, { role: 'system', text: `Whisper sent: ${text}` }]);
    setWhisperText('');
  };

  const startTakeover = async () => {
    if (!selectedCall || wsRef.current?.readyState !== WebSocket.OPEN) return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } });
      const context = audioContextRef.current;
      const source = context.createMediaStreamSource(stream);
      const processor = context.createScriptProcessor(2048, 1, 1);
      const silent = context.createGain();
      silent.gain.value = 0;
      processor.onaudioprocess = event => {
        if (wsRef.current?.readyState !== WebSocket.OPEN) return;
        const samples = resampleTo8k(event.inputBuffer.getChannelData(0), context.sampleRate);
        wsRef.current.send(JSON.stringify({ action: 'audio_chunk', payload: encodeAudio(samples, selectedCall.audio_format) }));
      };
      source.connect(processor);
      processor.connect(silent);
      silent.connect(context.destination);
      micRef.current = { stream, source, processor, silent };
      wsRef.current.send(JSON.stringify({ action: 'takeover' }));
      setTakeoverActive(true);
      setTranscripts(previous => [...previous, { role: 'system', text: 'Human takeover started' }]);
    } catch {
      setError('Microphone permission is required for takeover');
      stopMicrophone();
    }
  };

  const endTakeover = () => {
    stopMicrophone();
    wsRef.current?.send(JSON.stringify({ action: 'release_takeover' }));
    setTakeoverActive(false);
    setTranscripts(previous => [...previous, { role: 'system', text: 'Human takeover ended; AI resumed' }]);
  };

  const card = { background: T.card, border: `1px solid ${T.border}`, borderRadius: 8 };
  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 20, gap: 16 }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 22, color: T.text }}><AppIcon name="audio" /> Live Call Monitor</h2>
          <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>Listen to active calls, follow transcripts, guide the AI, or take over when required.</p>
        </div>
        {!connected && <button onClick={fetchActiveCalls} style={{ padding: '8px 14px', borderRadius: 7, border: `1px solid ${T.border}`, background: T.card, color: T.sub, cursor: 'pointer' }}><AppIcon name="refresh" /> Refresh</button>}
      </div>

      {error && <div style={{ marginBottom: 14, padding: '10px 14px', borderRadius: 7, background: 'rgba(239,68,68,.08)', border: '1px solid rgba(239,68,68,.25)', color: T.red }}>{error}</div>}

      {!connected ? (
        <div style={{ ...card, overflow: 'hidden' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'minmax(220px,1.4fr) minmax(150px,.8fr) 120px 130px', padding: '11px 16px', borderBottom: `1px solid ${T.border}`, color: T.muted, fontSize: 11, fontWeight: 800, textTransform: 'uppercase' }}><span>Lead</span><span>Campaign</span><span>Duration</span><span>Action</span></div>
          {loadingCalls ? <div style={{ padding: 32, textAlign: 'center', color: T.muted }}>Loading active calls...</div> : calls.length === 0 ? <div style={{ padding: 40, textAlign: 'center', color: T.muted }}>No calls are currently active.</div> : calls.map(call => (
            <div key={call.stream_sid} style={{ display: 'grid', gridTemplateColumns: 'minmax(220px,1.4fr) minmax(150px,.8fr) 120px 130px', alignItems: 'center', padding: '13px 16px', borderBottom: `1px solid ${T.border}` }}>
              <div><strong style={{ color: T.text }}>{call.lead_name || 'Unknown lead'}</strong><div style={{ color: T.muted, fontSize: 12 }}>{call.lead_phone || 'No phone'}</div></div>
              <span style={{ color: T.sub }}>Campaign #{call.campaign_id || '-'}</span>
              <span style={{ color: T.sub }}>{Math.floor(call.duration_s / 60)}:{String(call.duration_s % 60).padStart(2, '0')}</span>
              <button onClick={() => connectToCall(call)} disabled={connecting} style={{ padding: '7px 13px', border: '1px solid rgba(99,102,241,.35)', borderRadius: 7, background: 'rgba(99,102,241,.1)', color: T.accent, fontWeight: 700, cursor: connecting ? 'wait' : 'pointer' }}><AppIcon name="audio" /> Listen</button>
            </div>
          ))}
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) 320px', gap: 14 }}>
          <div style={{ ...card, padding: 18 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
              <div><strong style={{ color: T.text }}>{selectedCall?.lead_name || 'Live call'}</strong><div style={{ color: T.muted, fontSize: 12 }}>{selectedCall?.lead_phone} · Audio connected</div></div>
              <button onClick={disconnect} style={{ padding: '7px 12px', borderRadius: 7, border: '1px solid rgba(239,68,68,.3)', background: 'rgba(239,68,68,.08)', color: T.red, cursor: 'pointer' }}>Stop Listening</button>
            </div>
            <div style={{ minHeight: 360, maxHeight: 520, overflow: 'auto', padding: 14, borderRadius: 7, background: T.bg }}>
              {transcripts.length === 0 ? <div style={{ marginTop: 130, textAlign: 'center', color: T.muted }}>Listening for speech...</div> : transcripts.map((item, index) => (
                <div key={`${index}-${item.text}`} style={{ marginBottom: 10, textAlign: item.role === 'agent' ? 'right' : item.role === 'system' ? 'center' : 'left' }}>
                  <span style={{ display: 'inline-block', maxWidth: '78%', padding: '8px 11px', borderRadius: 7, background: item.role === 'system' ? 'rgba(245,158,11,.1)' : item.role === 'agent' ? T.card : 'rgba(99,102,241,.1)', color: T.text, border: `1px solid ${T.border}` }}>{item.text}</span>
                </div>
              ))}
            </div>
          </div>
          <aside style={{ ...card, padding: 18, alignSelf: 'start' }}>
            <h3 style={{ margin: '0 0 12px', color: T.text, fontSize: 15 }}>Call Controls</h3>
            <textarea value={whisperText} onChange={event => setWhisperText(event.target.value)} disabled={takeoverActive} placeholder="Guide the AI on its next response" style={{ width: '100%', minHeight: 90, boxSizing: 'border-box', padding: 10, border: `1px solid ${T.border}`, borderRadius: 7, background: T.card, color: T.text, resize: 'vertical' }} />
            <button onClick={sendWhisper} disabled={takeoverActive || !whisperText.trim()} style={{ width: '100%', marginTop: 8, padding: 9, borderRadius: 7, border: '1px solid rgba(99,102,241,.3)', background: 'rgba(99,102,241,.1)', color: T.accent, fontWeight: 700, cursor: 'pointer' }}><AppIcon name="message" /> Send Whisper</button>
            <div style={{ height: 1, background: T.border, margin: '18px 0' }} />
            <p style={{ margin: '0 0 10px', color: T.muted, fontSize: 12 }}>Takeover pauses the AI and sends your microphone audio to the caller.</p>
            <button onClick={takeoverActive ? endTakeover : startTakeover} style={{ width: '100%', padding: 10, borderRadius: 7, border: `1px solid ${takeoverActive ? 'rgba(16,185,129,.4)' : 'rgba(239,68,68,.35)'}`, background: takeoverActive ? 'rgba(16,185,129,.12)' : 'rgba(239,68,68,.09)', color: takeoverActive ? T.green : T.red, fontWeight: 700, cursor: 'pointer' }}><AppIcon name={takeoverActive ? 'stop' : 'takeover'} /> {takeoverActive ? 'End Takeover' : 'Take Over Call'}</button>
          </aside>
        </div>
      )}
    </div>
  );
}
