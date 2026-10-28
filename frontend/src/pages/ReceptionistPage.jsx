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
import ReceptionistInstructions from '../components/ReceptionistInstructions';
import IncomingCallRouting from '../components/IncomingCallRouting';
import AppSelect from '../components/common/AppSelect';

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
  const { selectedOrg } = useOrg();
  const {
    setActiveVoiceProvider,
    activeVoiceId, setActiveVoiceId,
    activeLanguage, setActiveLanguage,
    setSavedVoiceName,
  } = useVoice();

  const [active, setActive] = useState(false);
  const [status, setStatus] = useState('Ready');
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

  const receptionistProvider = 'gemini_live';
  const selectedProviderVoices = INDIAN_VOICES[receptionistProvider] || [];
  const selectedVoiceId = selectedProviderVoices.some(v => v.id === activeVoiceId) ? activeVoiceId : 'Kore';
  const selectedVoiceName = selectedProviderVoices.find(v => v.id === selectedVoiceId)?.name || selectedVoiceId;
  const selectedLanguageName = INDIAN_LANGUAGES.find(l => l.code === activeLanguage)?.name || activeLanguage || 'Default';
  const voiceRecommendation = VOICE_RECOMMENDATIONS[activeLanguage]?.[receptionistProvider]?.note || '';

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
          tts_provider: receptionistProvider,
          tts_voice_id: selectedVoiceId,
          tts_language: activeLanguage,
        }),
      });
      if (!res.ok) throw new Error('save failed');
      setActiveVoiceProvider(receptionistProvider);
      setActiveVoiceId(selectedVoiceId);
      setSavedVoiceName(selectedVoiceName);
      setVoiceSaveStatus('saved');
      setTimeout(() => setVoiceSaveStatus(''), 2000);
    } catch {
      setVoiceSaveStatus('error');
      setTimeout(() => setVoiceSaveStatus(''), 3000);
    }
  };

  useEffect(() => cleanup, []);

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

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const qp = new URLSearchParams({
        mode: 'inbound-sim',
        stream_sid: sid,
        org_id: String(selectedOrg?.id || currentUser?.org_id || 0),
        phone: '',
        interest: 'inbound enquiry',
        tts_provider: receptionistProvider,
        voice: selectedVoiceId,
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

      };

      ws.onerror = () => setStatus('WebSocket error');
      ws.onclose = () => {
        cleanup();
        setStatus('Call ended. Lead extraction runs after the transcript is saved.');
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
          <AppSelect
            value={receptionistProvider}
            disabled
            size="small"
            width={150}
            options={[
              { value: 'gemini_live', label: 'Gemini Live' },
            ]}
          />
          <AppSelect
            value={selectedVoiceId}
            onChange={handleVoiceChange}
            disabled={active}
            searchable
            size="small"
            width={260}
            placeholder="Select voice"
            options={(() => {
              const recs = VOICE_RECOMMENDATIONS[activeLanguage]?.[receptionistProvider]?.top || [];
              const recommended = selectedProviderVoices.filter(v => recs.includes(v.id));
              const others = selectedProviderVoices.filter(v => !recs.includes(v.id));
              const toOptions = voices => voices.map(v => ({ value: v.id, label: v.name }));
              return recommended.length > 0
                ? [
                    { label: 'Recommended', options: toOptions(recommended) },
                    { label: 'All Voices', options: toOptions(others) },
                  ]
                : toOptions(selectedProviderVoices);
            })()}
          />
          <AppSelect
            value={activeLanguage}
            onChange={setActiveLanguage}
            disabled={active}
            searchable
            size="small"
            width={160}
            options={INDIAN_LANGUAGES.map(language => ({ value: language.code, label: language.name }))}
          />
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
          Current: Gemini Live - {selectedVoiceName} ({selectedLanguageName})
        </div>
        {voiceRecommendation && (
          <div style={{ marginTop: 4, fontSize: 12, color: '#0891b2' }}>{voiceRecommendation}</div>
        )}
      </section>

      <IncomingCallRouting key={`routing-${selectedOrg?.id}`} orgId={selectedOrg?.id} />
      <ReceptionistInstructions key={selectedOrg?.id} orgId={selectedOrg?.id} active={active} />
      <div role="status" style={{ marginTop: 8, fontSize: 12, color: T.sub }}>{status}</div>

    </div>
  );
}
