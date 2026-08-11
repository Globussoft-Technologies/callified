import React, { useEffect, useRef, useState } from 'react';
import {
  SIM_WEB_AUDIO_CONSTRAINTS,
  SIM_WEB_MIC_SAMPLE_RATE,
  SIM_WEB_SEND_MEDIA_LOG_EVERY,
  SIM_WEB_UNMUTE_AFTER_MS,
  downsampleFloat32To8kHz,
  safeCloseAudioContext,
  simWebTrackMicSilence,
} from '../apiConfig';
import { useAuth } from '../contexts/AuthContext';
import { useOrg } from '../contexts/OrgContext';
import { useVoice } from '../contexts/VoiceContext';
import { API_URL } from '../constants/api';
import { INDIAN_LANGUAGES, INDIAN_VOICES, VOICE_RECOMMENDATIONS } from '../constants/voices';
import { formatDateTime } from '../utils/dateFormat';
import AuthAudio from '../components/AuthAudio';

const T = {
  bg: '#f6f7fb',
  panel: '#ffffff',
  border: '#e5e7eb',
  text: '#111827',
  sub: '#6b7280',
  muted: '#9ca3af',
  accent: '#2563eb',
  green: '#16a34a',
  red: '#dc2626',
  amber: '#d97706',
  font: "'DM Sans', sans-serif",
  mono: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
};

function pcm16Base64(float32Array) {
  const int16 = new Int16Array(float32Array.length);
  for (let i = 0; i < float32Array.length; i++) {
    const s = Math.max(-1, Math.min(1, float32Array[i]));
    int16[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
  }
  let binary = '';
  const bytes = new Uint8Array(int16.buffer);
  for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
  return window.btoa(binary);
}

function base64ToFloat32(payload) {
  const audioStr = window.atob(payload);
  const audioBytes = new Uint8Array(audioStr.length);
  for (let i = 0; i < audioStr.length; i++) audioBytes[i] = audioStr.charCodeAt(i);
  const int16 = new Int16Array(audioBytes.buffer);
  const out = new Float32Array(int16.length);
  for (let i = 0; i < int16.length; i++) out[i] = int16[i] / 0x8000;
  return out;
}

function scheduleMicUnmute(audioContext, timerRef, mutedRef, nextPlayTime, extraDelayMs) {
  if (timerRef.current) clearTimeout(timerRef.current);
  mutedRef.current = true;
  const queuedMs = Math.max(0, (nextPlayTime - audioContext.currentTime) * 1000);
  timerRef.current = setTimeout(() => {
    mutedRef.current = false;
    timerRef.current = null;
  }, queuedMs + extraDelayMs);
}

export default function ReceptionistPage() {
  const { currentUser, apiFetch } = useAuth();
  const { selectedOrg, orgTimezone } = useOrg();
  const {
    activeVoiceProvider, setActiveVoiceProvider,
    activeVoiceId, setActiveVoiceId,
    activeLanguage, setActiveLanguage,
    setSavedVoiceName,
  } = useVoice();

  const [active, setActive] = useState(false);
  const [status, setStatus] = useState('Ready');
  const [transcripts, setTranscripts] = useState([]);
  const [savedCalls, setSavedCalls] = useState([]);
  const [callsLoading, setCallsLoading] = useState(false);
  const [selectedCall, setSelectedCall] = useState(null);
  const [editingRows, setEditingRows] = useState({});
  const [savingRows, setSavingRows] = useState({});
  const [voiceSaveStatus, setVoiceSaveStatus] = useState('');

  const wsRef = useRef(null);
  const audioCtxRef = useRef(null);
  const streamRef = useRef(null);
  const sourceRef = useRef(null);
  const processorRef = useRef(null);
  const silentSinkRef = useRef(null);
  const activeSourcesRef = useRef([]);
  const nextPlayTimeRef = useRef(0);
  const sendCountRef = useRef(0);
  const silenceStateRef = useRef({ streak: 0, warned: false });
  const micMutedRef = useRef(true);
  const unmuteTimerRef = useRef(null);

  const cleanup = () => {
    if (unmuteTimerRef.current) {
      clearTimeout(unmuteTimerRef.current);
      unmuteTimerRef.current = null;
    }
    activeSourcesRef.current.forEach(source => {
      try { source.stop(); } catch { /* ignore */ }
    });
    activeSourcesRef.current = [];
    if (processorRef.current) {
      try { processorRef.current.disconnect(); } catch { /* ignore */ }
      processorRef.current = null;
    }
    if (silentSinkRef.current) {
      try { silentSinkRef.current.disconnect(); } catch { /* ignore */ }
      silentSinkRef.current = null;
    }
    if (sourceRef.current) {
      try { sourceRef.current.disconnect(); } catch { /* ignore */ }
      sourceRef.current = null;
    }
    if (streamRef.current) {
      streamRef.current.getTracks().forEach(track => track.stop());
      streamRef.current = null;
    }
    if (wsRef.current) {
      try { wsRef.current.close(); } catch { /* ignore */ }
      wsRef.current = null;
    }
    safeCloseAudioContext(audioCtxRef.current);
    audioCtxRef.current = null;
    micMutedRef.current = true;
    sendCountRef.current = 0;
    setActive(false);
  };

  const selectedProviderVoices = INDIAN_VOICES[activeVoiceProvider] || [];
  const selectedVoiceName = selectedProviderVoices.find(v => v.id === activeVoiceId)?.name || activeVoiceId || 'No voice selected';
  const selectedLanguageName = INDIAN_LANGUAGES.find(l => l.code === activeLanguage)?.name || activeLanguage || 'Default';
  const voiceRecommendation = VOICE_RECOMMENDATIONS[activeLanguage]?.[activeVoiceProvider]?.note || '';

  const handleProviderChange = (provider) => {
    const firstVoice = (INDIAN_VOICES[provider] || [])[0] || null;
    setActiveVoiceProvider(provider);
    setActiveVoiceId(firstVoice?.id || '');
    setSavedVoiceName(firstVoice?.name || '');
  };

  const handleVoiceChange = (voiceId) => {
    setActiveVoiceId(voiceId);
    const voice = selectedProviderVoices.find(v => v.id === voiceId);
    setSavedVoiceName(voice?.name || '');
  };

  const handleSaveVoice = async () => {
    if (!selectedOrg) return;
    setVoiceSaveStatus('saving');
    try {
      const res = await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/voice-settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tts_provider: activeVoiceProvider,
          tts_voice_id: activeVoiceId,
          tts_language: activeLanguage,
        }),
      });
      if (!res.ok) throw new Error('save failed');
      setVoiceSaveStatus('saved');
      setTimeout(() => setVoiceSaveStatus(''), 2000);
    } catch {
      setVoiceSaveStatus('error');
      setTimeout(() => setVoiceSaveStatus(''), 3000);
    }
  };

  useEffect(() => cleanup, []);

  const fetchSavedCalls = async () => {
    setCallsLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/receptionist/calls`);
      const data = await res.json().catch(() => []);
      if (res.ok) setSavedCalls(Array.isArray(data) ? data : []);
    } catch (e) {
      console.error('[AI Receptionist] saved calls fetch failed', e);
    } finally {
      setCallsLoading(false);
    }
  };

  const startEditRow = (call) => {
    setEditingRows(prev => ({
      ...prev,
      [call.transcript_id]: {
        first_name: call.first_name || '',
        last_name: call.last_name || '',
        phone: call.phone || '',
        interest: call.interest || '',
        status: call.status || 'new',
      },
    }));
  };

  const updateEditRow = (id, field, value) => {
    setEditingRows(prev => ({
      ...prev,
      [id]: { ...(prev[id] || {}), [field]: value },
    }));
  };

  const cancelEditRow = (id) => {
    setEditingRows(prev => {
      const next = { ...prev };
      delete next[id];
      return next;
    });
  };

  const saveEditRow = async (call) => {
    const id = call.transcript_id;
    const draft = editingRows[id];
    if (!draft) return;
    setSavingRows(prev => ({ ...prev, [id]: true }));
    try {
      const res = await apiFetch(`${API_URL}/receptionist/calls/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(draft),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || `Save failed (${res.status})`);
      cancelEditRow(id);
      await fetchSavedCalls();
    } catch (e) {
      alert(e?.message || 'Could not save row');
    } finally {
      setSavingRows(prev => ({ ...prev, [id]: false }));
    }
  };

  useEffect(() => {
    fetchSavedCalls();
    const id = setInterval(fetchSavedCalls, 10000);
    return () => clearInterval(id);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const startCall = async () => {
    if (active) {
      cleanup();
      setStatus('Call ended. If the caller gave a phone number, a CRM lead will be created shortly.');
      return;
    }

    try {
      setStatus('Requesting microphone...');
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: SIM_WEB_AUDIO_CONSTRAINTS,
        video: false,
      });
      streamRef.current = stream;

      const audioContext = new (window.AudioContext || window.webkitAudioContext)();
      audioCtxRef.current = audioContext;
      await audioContext.resume();

      const sid = `web_sim_inbound_${selectedOrg?.id || 0}_${Date.now()}`;
      setTranscripts([]);

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const qp = new URLSearchParams({
        mode: 'inbound-sim',
        stream_sid: sid,
        org_id: String(selectedOrg?.id || currentUser?.org_id || 0),
        phone: '',
        interest: 'inbound enquiry',
        tts_provider: activeVoiceProvider || '',
        voice: activeVoiceId || '',
        tts_language: activeLanguage || 'en',
      });
      const wsUrl = `${protocol}//${window.location.host}/media-stream?${qp.toString()}`;
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        setActive(true);
        setStatus('Listening');
        ws.send(JSON.stringify({ event: 'connected' }));
        ws.send(JSON.stringify({
          event: 'start',
          start: {
            stream_sid: sid,
            user_email: currentUser?.email || '',
          },
          stream_sid: sid,
          user_email: currentUser?.email || '',
        }));

        const source = audioContext.createMediaStreamSource(stream);
        const processor = audioContext.createScriptProcessor(2048, 1, 1);
        const silentSink = audioContext.createGain();
        silentSink.gain.value = 0;
        sourceRef.current = source;
        processorRef.current = processor;
        silentSinkRef.current = silentSink;
        source.connect(processor);
        processor.connect(silentSink);
        silentSink.connect(audioContext.destination);

        processor.onaudioprocess = (event) => {
          if (ws.readyState !== WebSocket.OPEN || micMutedRef.current) return;
          const input = event.inputBuffer.getChannelData(0);
          const downsampled = downsampleFloat32To8kHz(input, audioContext.sampleRate);
          simWebTrackMicSilence(downsampled, silenceStateRef.current);
          sendCountRef.current += 1;
          if (sendCountRef.current % SIM_WEB_SEND_MEDIA_LOG_EVERY === 0) {
            console.log('[InboundReceptionist] mic frame sent', sendCountRef.current);
          }
          ws.send(JSON.stringify({
            event: 'media',
            media: { payload: pcm16Base64(downsampled) },
          }));
        };

        nextPlayTimeRef.current = audioContext.currentTime;
        unmuteTimerRef.current = setTimeout(() => {
          micMutedRef.current = false;
          unmuteTimerRef.current = null;
        }, 2500);
      };

      ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        if (data.type === 'clear') {
          activeSourcesRef.current.forEach(source => {
            try { source.stop(); } catch { /* ignore */ }
          });
          activeSourcesRef.current = [];
          nextPlayTimeRef.current = audioCtxRef.current?.currentTime || 0;
          micMutedRef.current = false;
          return;
        }
        if (data.event === 'media') {
          const ctx = audioCtxRef.current;
          if (!ctx) return;
          const float32 = base64ToFloat32(data.media.payload);
          const buffer = ctx.createBuffer(1, float32.length, SIM_WEB_MIC_SAMPLE_RATE);
          buffer.copyToChannel(float32, 0);
          const source = ctx.createBufferSource();
          source.buffer = buffer;
          source.connect(ctx.destination);
          const startAt = Math.max(ctx.currentTime, nextPlayTimeRef.current);
          source.start(startAt);
          nextPlayTimeRef.current = startAt + buffer.duration;
          activeSourcesRef.current.push(source);
          source.onended = () => {
            activeSourcesRef.current = activeSourcesRef.current.filter(item => item !== source);
          };
          scheduleMicUnmute(
            ctx,
            unmuteTimerRef,
            micMutedRef,
            nextPlayTimeRef.current,
            SIM_WEB_UNMUTE_AFTER_MS
          );
          return;
        }
        if (data.type === 'transcript' && data.text) {
          setTranscripts(prev => [...prev, {
            role: data.role === 'agent' ? 'AI' : 'Customer',
            text: data.text,
          }]);
        }
      };

      ws.onerror = () => setStatus('WebSocket error');
      ws.onclose = () => {
        cleanup();
        setStatus('Call ended. Lead extraction runs after the transcript is saved.');
        setTimeout(fetchSavedCalls, 2500);
        setTimeout(fetchSavedCalls, 7000);
      };
    } catch (e) {
      console.error(e);
      cleanup();
      setStatus(e?.message || 'Could not start receptionist call');
    }
  };

  return (
    <div style={{ minHeight: '100%', background: T.bg, padding: '22px 28px', fontFamily: T.font }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16, marginBottom: 18 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 24, lineHeight: 1.2, color: T.text }}>AI Receptionist</h1>
          <div style={{ marginTop: 4, fontSize: 13, color: T.sub }}>
            Inbound web-sim calls start without a lead. Customer details are extracted after the call ends.
          </div>
        </div>
        <button
          onClick={startCall}
          style={{
            height: 38, padding: '0 16px', borderRadius: 7, border: 'none',
            background: active ? T.red : T.green, color: '#fff', cursor: 'pointer',
            fontWeight: 700, fontFamily: T.font,
          }}
        >
          {active ? 'End Call' : 'Start Inbound Call'}
        </button>
      </div>

      <section style={{ marginBottom: 16, background: T.panel, border: `1px solid ${T.border}`, borderRadius: 8, padding: 14 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
          <div style={{ fontSize: 12, fontWeight: 800, color: T.sub, textTransform: 'uppercase' }}>Voice Settings</div>
          <select
            value={activeVoiceProvider}
            onChange={e => handleProviderChange(e.target.value)}
            disabled={active}
            style={{ height: 34, minWidth: 130, border: `1px solid ${T.border}`, borderRadius: 6, padding: '0 10px', fontFamily: T.font, background: '#fff' }}
          >
            <option value="elevenlabs">ElevenLabs</option>
            <option value="sarvam">Sarvam AI</option>
            <option value="smallest">Smallest AI</option>
          </select>
          <select
            value={activeVoiceId}
            onChange={e => handleVoiceChange(e.target.value)}
            disabled={active}
            style={{ height: 34, minWidth: 220, border: `1px solid ${T.border}`, borderRadius: 6, padding: '0 10px', fontFamily: T.font, background: '#fff' }}
          >
            <option value="">Select voice</option>
            {(() => {
              const recs = VOICE_RECOMMENDATIONS[activeLanguage]?.[activeVoiceProvider]?.top || [];
              const recommended = selectedProviderVoices.filter(v => recs.includes(v.id));
              const others = selectedProviderVoices.filter(v => !recs.includes(v.id));
              return (
                <>
                  {recommended.length > 0 && (
                    <optgroup label="Recommended">
                      {recommended.map(v => <option key={v.id} value={v.id}>{v.name}</option>)}
                    </optgroup>
                  )}
                  {recommended.length > 0 && (
                    <optgroup label="All Voices">
                      {others.map(v => <option key={v.id} value={v.id}>{v.name}</option>)}
                    </optgroup>
                  )}
                  {recommended.length === 0 && selectedProviderVoices.map(v => <option key={v.id} value={v.id}>{v.name}</option>)}
                </>
              );
            })()}
          </select>
          <select
            value={activeLanguage}
            onChange={e => setActiveLanguage(e.target.value)}
            disabled={active}
            style={{ height: 34, minWidth: 130, border: `1px solid ${T.border}`, borderRadius: 6, padding: '0 10px', fontFamily: T.font, background: '#fff' }}
          >
            {INDIAN_LANGUAGES.map(l => <option key={l.code} value={l.code}>{l.name}</option>)}
          </select>
          <button
            onClick={handleSaveVoice}
            disabled={active || voiceSaveStatus === 'saving'}
            style={{
              height: 34, padding: '0 14px', borderRadius: 6, border: 'none',
              background: voiceSaveStatus === 'saved' ? T.green : voiceSaveStatus === 'error' ? T.red : T.accent,
              color: '#fff', cursor: active || voiceSaveStatus === 'saving' ? 'not-allowed' : 'pointer',
              fontWeight: 700, fontFamily: T.font,
            }}
          >
            {voiceSaveStatus === 'saving' ? 'Saving...' : voiceSaveStatus === 'saved' ? 'Saved' : voiceSaveStatus === 'error' ? 'Failed' : 'Save'}
          </button>
        </div>
        <div style={{ marginTop: 8, fontSize: 12, color: T.sub }}>
          Current: {activeVoiceProvider || 'default'} - {selectedVoiceName} ({selectedLanguageName})
        </div>
        {voiceRecommendation && (
          <div style={{ marginTop: 4, fontSize: 12, color: '#0891b2' }}>{voiceRecommendation}</div>
        )}
      </section>

      <div>
        <section style={{ background: T.panel, border: `1px solid ${T.border}`, borderRadius: 8, minHeight: 'calc(100vh - 190px)', display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: '14px 16px', borderBottom: `1px solid ${T.border}`, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <div style={{ fontSize: 13, fontWeight: 800, color: T.text }}>Live Transcript</div>
            <div style={{ fontSize: 12, color: active ? T.green : T.sub, fontWeight: 700 }}>{status}</div>
          </div>
          <div style={{ flex: 1, padding: 16, overflowY: 'auto' }}>
            {transcripts.length === 0 ? (
              <div style={{ color: T.muted, fontSize: 13 }}>Transcript will appear here once the receptionist starts speaking.</div>
            ) : transcripts.map((turn, idx) => (
              <div key={`${turn.role}-${idx}`} style={{ marginBottom: 12, maxWidth: '760px' }}>
                <div style={{ fontSize: 11, color: turn.role === 'AI' ? T.accent : T.amber, fontWeight: 800, marginBottom: 4 }}>
                  {turn.role}
                </div>
                <div style={{ fontSize: 14, color: T.text, lineHeight: 1.55, background: turn.role === 'AI' ? '#eff6ff' : '#fffbeb', border: `1px solid ${turn.role === 'AI' ? '#bfdbfe' : '#fde68a'}`, borderRadius: 8, padding: '10px 12px' }}>
                  {turn.text}
                </div>
              </div>
            ))}
          </div>
        </section>
      </div>

      <section style={{ marginTop: 16, background: T.panel, border: `1px solid ${T.border}`, borderRadius: 8, overflow: 'hidden' }}>
        <div style={{ padding: '14px 16px', borderBottom: `1px solid ${T.border}`, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div>
            <div style={{ fontSize: 13, fontWeight: 800, color: T.text }}>Captured Inbound Customers</div>
            <div style={{ fontSize: 12, color: T.sub, marginTop: 2 }}>Each ended inbound call is stored as its own row. Blank fields can be edited.</div>
          </div>
          <button
            onClick={fetchSavedCalls}
            disabled={callsLoading}
            style={{ height: 32, padding: '0 12px', borderRadius: 6, border: `1px solid ${T.border}`, background: '#fff', color: T.sub, cursor: callsLoading ? 'wait' : 'pointer', fontWeight: 700, fontFamily: T.font }}
          >
            {callsLoading ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ background: '#f9fafb', color: T.sub, textAlign: 'left' }}>
                <th style={{ padding: '10px 16px', borderBottom: `1px solid ${T.border}` }}>Name</th>
                <th style={{ padding: '10px 16px', borderBottom: `1px solid ${T.border}` }}>Phone</th>
                <th style={{ padding: '10px 16px', borderBottom: `1px solid ${T.border}` }}>Interest</th>
                <th style={{ padding: '10px 16px', borderBottom: `1px solid ${T.border}` }}>When</th>
                <th style={{ padding: '10px 16px', borderBottom: `1px solid ${T.border}` }}>Transcript</th>
                <th style={{ padding: '10px 16px', borderBottom: `1px solid ${T.border}` }}>Edit</th>
              </tr>
            </thead>
            <tbody>
              {savedCalls.length === 0 ? (
                <tr>
                  <td colSpan={6} style={{ padding: 18, color: T.muted, textAlign: 'center' }}>
                    No inbound receptionist rows yet.
                  </td>
                </tr>
              ) : savedCalls.map(call => {
                const fullName = `${call.first_name || ''} ${call.last_name || ''}`.trim();
                const draft = editingRows[call.transcript_id];
                const input = {
                  width: '100%', height: 32, boxSizing: 'border-box',
                  border: `1px solid ${T.border}`, borderRadius: 6,
                  padding: '0 8px', fontFamily: T.font, fontSize: 13,
                };
                return (
                  <tr key={call.transcript_id} style={{ borderBottom: `1px solid ${T.border}` }}>
                    <td style={{ padding: '12px 16px', color: T.text, fontWeight: 700, minWidth: 180 }}>
                      {draft ? (
                        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 6 }}>
                          <input value={draft.first_name} onChange={e => updateEditRow(call.transcript_id, 'first_name', e.target.value)} placeholder="First" style={input} />
                          <input value={draft.last_name} onChange={e => updateEditRow(call.transcript_id, 'last_name', e.target.value)} placeholder="Last" style={input} />
                        </div>
                      ) : (fullName || '')}
                    </td>
                    <td style={{ padding: '12px 16px', color: T.text, fontFamily: T.mono, minWidth: 150 }}>
                      {draft ? (
                        <input value={draft.phone} onChange={e => updateEditRow(call.transcript_id, 'phone', e.target.value)} placeholder="Phone" style={{ ...input, fontFamily: T.mono }} />
                      ) : (call.phone || '')}
                    </td>
                    <td style={{ padding: '12px 16px', color: T.sub, minWidth: 220 }}>
                      {draft ? (
                        <input value={draft.interest} onChange={e => updateEditRow(call.transcript_id, 'interest', e.target.value)} placeholder="Interest" style={input} />
                      ) : (call.interest || '')}
                    </td>
                    <td style={{ padding: '12px 16px', color: T.sub }}>{formatDateTime(call.created_at, orgTimezone)}</td>
                    <td style={{ padding: '12px 16px' }}>
                      <button
                        onClick={() => setSelectedCall(call)}
                        style={{ border: 'none', background: '#eff6ff', color: T.accent, borderRadius: 6, padding: '6px 10px', fontWeight: 800, cursor: 'pointer', fontFamily: T.font }}
                      >
                        Open
                      </button>
                    </td>
                    <td style={{ padding: '12px 16px', minWidth: 140 }}>
                      {draft ? (
                        <div style={{ display: 'flex', gap: 6 }}>
                          <button
                            onClick={() => saveEditRow(call)}
                            disabled={savingRows[call.transcript_id]}
                            style={{ border: 'none', background: T.green, color: '#fff', borderRadius: 6, padding: '6px 10px', fontWeight: 800, cursor: savingRows[call.transcript_id] ? 'wait' : 'pointer', fontFamily: T.font }}
                          >
                            {savingRows[call.transcript_id] ? 'Saving' : 'Save'}
                          </button>
                          <button
                            onClick={() => cancelEditRow(call.transcript_id)}
                            style={{ border: `1px solid ${T.border}`, background: '#fff', color: T.sub, borderRadius: 6, padding: '6px 10px', fontWeight: 800, cursor: 'pointer', fontFamily: T.font }}
                          >
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <button
                          onClick={() => startEditRow(call)}
                          style={{ border: `1px solid ${T.border}`, background: '#fff', color: T.sub, borderRadius: 6, padding: '6px 10px', fontWeight: 800, cursor: 'pointer', fontFamily: T.font }}
                        >
                          Edit
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {selectedCall && (
        <div
          onClick={(e) => { if (e.target === e.currentTarget) setSelectedCall(null); }}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => { if (e.key === 'Escape') setSelectedCall(null); }}
          style={{ position: 'fixed', inset: 0, background: 'rgba(15,23,42,0.55)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 18, zIndex: 10000 }}
        >
          <div style={{ width: 'min(760px, 96vw)', maxHeight: '88vh', overflow: 'hidden', background: '#fff', borderRadius: 8, border: `1px solid ${T.border}`, boxShadow: '0 24px 60px rgba(0,0,0,0.25)', display: 'flex', flexDirection: 'column' }}>
            <div style={{ padding: '16px 18px', borderBottom: `1px solid ${T.border}`, display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <div style={{ fontSize: 16, fontWeight: 900, color: T.text }}>
                  {`${selectedCall.first_name || ''} ${selectedCall.last_name || ''}`.trim() || 'Inbound Customer'}
                </div>
                <div style={{ marginTop: 3, fontSize: 12, color: T.sub }}>
                  {selectedCall.phone || 'No phone'} · {formatDateTime(selectedCall.created_at, orgTimezone)}
                </div>
              </div>
              <button onClick={() => setSelectedCall(null)} style={{ border: 'none', background: 'transparent', fontSize: 20, color: T.muted, cursor: 'pointer' }}>x</button>
            </div>
            <div style={{ padding: 18, overflowY: 'auto' }}>
              {selectedCall.recording_url ? (
                <div style={{ marginBottom: 14, padding: 12, border: `1px solid ${T.border}`, borderRadius: 8, background: '#f9fafb' }}>
                  <div style={{ fontSize: 12, color: T.sub, fontWeight: 800, marginBottom: 8 }}>Recording</div>
                  <AuthAudio src={selectedCall.recording_url} style={{ width: '100%', height: 36 }} />
                </div>
              ) : (
                <div style={{ marginBottom: 14, padding: 12, border: `1px solid ${T.border}`, borderRadius: 8, background: '#fffbeb', color: T.amber, fontSize: 13, fontWeight: 700 }}>
                  Recording is still processing.
                </div>
              )}

              <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                {(Array.isArray(selectedCall.transcript) ? selectedCall.transcript : []).filter(turn => turn.role !== 'system').map((turn, idx) => {
                  const isAI = turn.role === 'AI';
                  return (
                    <div key={idx} style={{ alignSelf: isAI ? 'flex-start' : 'flex-end', maxWidth: '78%' }}>
                      <div style={{ fontSize: 11, fontWeight: 900, color: isAI ? T.accent : T.amber, marginBottom: 4 }}>
                        {isAI ? 'AI' : 'Customer'}
                      </div>
                      <div style={{ padding: '10px 12px', borderRadius: 8, border: `1px solid ${isAI ? '#bfdbfe' : '#fde68a'}`, background: isAI ? '#eff6ff' : '#fffbeb', color: T.text, lineHeight: 1.5 }}>
                        {turn.text}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
