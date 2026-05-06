import React, { createContext, useContext, useState, useRef, useCallback } from 'react';
import { API_URL } from '../constants/api';
import { useAuth } from './AuthContext';
import { useOrg } from './OrgContext';
import { useVoice } from './VoiceContext';

const CallContext = createContext(null);

export function CallProvider({ children }) {
  const { apiFetch } = useAuth();
  const { orgProducts } = useOrg();
  const { activeVoiceProvider, activeVoiceId, activeLanguage } = useVoice();

  const [dialingId, setDialingId] = useState(null);
  const [webCallActive, setWebCallActive] = useState(null);
  // rechargePrompt holds the backend's "insufficient credits" message when
  // a 402 comes back from the dial endpoints. Rendered as a themed modal
  // (matches the app's dark glass-panel UI) instead of the native browser
  // confirm() dialog, which used the OS theme and looked out of place.
  const [rechargePrompt, setRechargePrompt] = useState(null);
  const webCallWsRef = useRef(null);
  const webCallAudioCtxRef = useRef(null);

  const handleDial = useCallback(async (lead) => {
    setDialingId(lead.id);
    try {
      const res = await apiFetch(`${API_URL}/dial/${lead.id}`, { method: "POST" });
      const data = await res.json();
      if (!res.ok) {
        const msg = data.error || `Dial failed (HTTP ${res.status})`;
        if (res.status === 402) {
          setRechargePrompt(msg);
        } else {
          alert(msg);
        }
      } else {
        alert(`Status: ${data.message || 'Connecting call...'}`);
      }
    } catch(e) {
      alert("Failed to hit the dialer API. Check console.");
    }
    setTimeout(() => setDialingId(null), 10000);
  }, [apiFetch]);

  const handleWebCall = useCallback(async (lead) => {
    if (webCallActive === lead.id) {
      // Disconnect active simulation
      if (webCallWsRef.current) webCallWsRef.current.close();
      if (webCallAudioCtxRef.current) webCallAudioCtxRef.current.close();
      setWebCallActive(null);
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
          audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
        });
      const audioContext = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: 8000 });
      webCallAudioCtxRef.current = audioContext;

      // Create a destination node to capture mixed audio for recording
      const recDest = audioContext.createMediaStreamDestination();
      const mediaRecorder = new MediaRecorder(recDest.stream, { mimeType: 'audio/webm;codecs=opus' });
      const recordedChunks = [];
      mediaRecorder.ondataavailable = (e) => { if (e.data.size > 0) recordedChunks.push(e.data); };
      mediaRecorder.start(1000); // collect chunks every 1s

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const host = window.location.hostname;

      const qp = new URLSearchParams({
        name: lead.first_name || 'Customer',
        phone: lead.phone || '',
        interest: lead.interest || (orgProducts.length > 0 ? orgProducts[0].name : 'our platform'),
        lead_id: String(lead.id || ''),
        tts_provider: activeVoiceProvider,
        voice: activeVoiceId,
        tts_language: activeLanguage,
      }).toString();

      let wsUrl;
      if (host === 'localhost' || host === '127.0.0.1') {
        wsUrl = `ws://${host}:8001/media-stream?${qp}`;
      } else {
        wsUrl = `${protocol}//${window.location.host}/media-stream?${qp}`;
      }

      const ws = new WebSocket(wsUrl);
      webCallWsRef.current = ws;

      ws.onopen = () => {
        setWebCallActive(lead.id);
        ws.send(JSON.stringify({ event: 'connected' }));
        const sid = `web_sim_${lead.id}_${Date.now()}`;
        ws.send(JSON.stringify({ event: 'start', start: { stream_sid: sid }, stream_sid: sid }));

        const source = audioContext.createMediaStreamSource(stream);
        const processor = audioContext.createScriptProcessor(2048, 1, 1);

        source.connect(processor);
        processor.connect(audioContext.destination);
        // Also route mic to recording destination
        source.connect(recDest);

        // Echo suppression: mute mic while AI speaks through speakers
        let micMuted = true; // Start muted until greeting finishes
        let unmuteTimer = null;

        processor.onaudioprocess = (e) => {
          if (ws.readyState !== WebSocket.OPEN) return;
          if (micMuted) return; // Don't send mic audio while AI is speaking
          const float32Array = e.inputBuffer.getChannelData(0);

          const int16Buffer = new Int16Array(float32Array.length);
          for (let i = 0; i < float32Array.length; i++) {
            let s = Math.max(-1, Math.min(1, float32Array[i]));
            int16Buffer[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
          }

          let binary = '';
          const bytes = new Uint8Array(int16Buffer.buffer);
          for (let i = 0; i < bytes.byteLength; i++) {
            binary += String.fromCharCode(bytes[i]);
          }
          const base64 = window.btoa(binary);

          ws.send(JSON.stringify({
            event: 'media',
            media: { payload: base64 }
          }));
        };

        let nextPlayTime = audioContext.currentTime;
        let activeSources = [];
        ws.onmessage = (event) => {
          const data = JSON.parse(event.data);
          if (data.type === 'clear') {
            // Backend barge-in — stop all queued audio immediately
            console.log('[barge-in] clear received, stopping', activeSources.length, 'sources');
            activeSources.forEach(s => { try { s.stop(); } catch (_) {} });
            activeSources = [];
            nextPlayTime = audioContext.currentTime;
            if (unmuteTimer) clearTimeout(unmuteTimer);
          } else if (data.event === 'media') {
            if (unmuteTimer) clearTimeout(unmuteTimer);

            const audioStr = window.atob(data.media.payload);
            const audioBytes = new Uint8Array(audioStr.length);
            for (let i = 0; i < audioStr.length; i++) {
              audioBytes[i] = audioStr.charCodeAt(i);
            }
            const int16Array = new Int16Array(audioBytes.buffer);
            const float32Array = new Float32Array(int16Array.length);
            for (let i = 0; i < int16Array.length; i++) {
              float32Array[i] = int16Array[i] / 0x8000;
            }

            const buffer = audioContext.createBuffer(1, float32Array.length, 8000);
            buffer.getChannelData(0).set(float32Array);

            const destSource = audioContext.createBufferSource();
            destSource.buffer = buffer;
            destSource.connect(audioContext.destination);
            // Also route TTS to recording destination
            destSource.connect(recDest);

            if (audioContext.currentTime > nextPlayTime) nextPlayTime = audioContext.currentTime;
            destSource.start(nextPlayTime);
            nextPlayTime += buffer.duration;
            activeSources.push(destSource);
            destSource.onended = () => { activeSources = activeSources.filter(s => s !== destSource); };

            // Unmute mic once after first chunk so it stays live for barge-in
            if (micMuted) {
              unmuteTimer = setTimeout(() => { micMuted = false; }, 400);
            }
          }
        };

        ws.onclose = () => {
          stream.getTracks().forEach(track => track.stop());

          // Upload whatever recording chunks we have
          const uploadRecording = async () => {
            if (recordedChunks.length > 0) {
              const blob = new Blob(recordedChunks, { type: 'audio/webm' });
              const formData = new FormData();
              formData.append('file', blob, `call_${lead.id}_${Date.now()}.webm`);
              formData.append('lead_id', String(lead.id));
              try {
                await apiFetch(`${API_URL}/upload-recording`, { method: 'POST', body: formData });
              } catch(e) { console.error('Recording upload failed:', e); }
            }
          };

          if (mediaRecorder.state !== 'inactive') {
            mediaRecorder.stop();
            mediaRecorder.onstop = () => uploadRecording();
          } else {
            // MediaRecorder already stopped — upload whatever chunks we collected
            uploadRecording();
          }

          if (webCallAudioCtxRef.current) webCallAudioCtxRef.current.close();
          setWebCallActive(null);
        };
      };
    } catch (e) {
      alert("Microphone access denied or connection to WebSockets failed.");
      console.error(e);
      setWebCallActive(null);
    }
  }, [apiFetch, webCallActive, orgProducts, activeVoiceProvider, activeVoiceId, activeLanguage]);

  const handleCampaignDial = useCallback(async (lead, campaignId) => {
    setDialingId(lead.id);
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${campaignId}/dial/${lead.id}`, { method: "POST" });
      if (!res.ok) {
        // Surface the backend error so silent failures (especially the
        // 402 "insufficient credits" gate) don't look like nothing happened.
        const body = await res.json().catch(() => ({}));
        const msg = body.error || `Dial failed (HTTP ${res.status})`;
        if (res.status === 402) {
          // Insufficient credits — show the themed recharge modal instead
          // of native confirm() (which renders in the OS theme and clashes).
          setRechargePrompt(msg);
        } else {
          alert(msg);
        }
      }
    } catch(e) {
      alert('Network error: ' + (e?.message || 'unknown'));
    }
    setTimeout(() => setDialingId(null), 10000);
  }, [apiFetch]);

  const handleCampaignWebCall = useCallback(async (lead, campaignId) => {
    if (webCallActive === lead.id) {
      if (webCallWsRef.current) webCallWsRef.current.close();
      if (webCallAudioCtxRef.current) webCallAudioCtxRef.current.close();
      setWebCallActive(null);
      return;
    }
    // Fetch campaign voice settings before starting call
    let campVoice = {};
    try {
      const vRes = await apiFetch(`${API_URL}/campaigns/${campaignId}/voice-settings`);
      campVoice = await vRes.json();
    } catch(e) {}

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
          audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
        });
      const audioContext = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: 8000 });
      webCallAudioCtxRef.current = audioContext;

      const recDest = audioContext.createMediaStreamDestination();
      const mediaRecorder = new MediaRecorder(recDest.stream, { mimeType: 'audio/webm;codecs=opus' });
      const recordedChunks = [];
      mediaRecorder.ondataavailable = (e) => { if (e.data.size > 0) recordedChunks.push(e.data); };
      mediaRecorder.start(1000);

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const host = window.location.hostname;

      const qp = new URLSearchParams({
        name: lead.first_name || 'Customer',
        phone: lead.phone || '',
        interest: lead.interest || (orgProducts.length > 0 ? orgProducts[0].name : 'our platform'),
        lead_id: String(lead.id || ''),
        tts_provider: campVoice.tts_provider || activeVoiceProvider,
        voice: campVoice.tts_voice_id || activeVoiceId,
        tts_language: campVoice.tts_language || activeLanguage,
        campaign_id: String(campaignId),
      }).toString();

      let wsUrl;
      if (host === 'localhost' || host === '127.0.0.1') {
        wsUrl = `ws://${host}:8001/media-stream?${qp}`;
      } else {
        wsUrl = `${protocol}//${window.location.host}/media-stream?${qp}`;
      }

      const ws = new WebSocket(wsUrl);
      webCallWsRef.current = ws;

      ws.onopen = () => {
        setWebCallActive(lead.id);
        ws.send(JSON.stringify({ event: 'connected' }));
        const sid = `web_sim_${lead.id}_${Date.now()}`;
        ws.send(JSON.stringify({ event: 'start', start: { stream_sid: sid }, stream_sid: sid }));

        const source = audioContext.createMediaStreamSource(stream);
        const processor = audioContext.createScriptProcessor(2048, 1, 1);

        source.connect(processor);
        processor.connect(audioContext.destination);
        source.connect(recDest);

        let micMuted = true;
        let unmuteTimer = null;

        processor.onaudioprocess = (e) => {
          if (ws.readyState !== WebSocket.OPEN) return;
          if (micMuted) return;
          const float32Array = e.inputBuffer.getChannelData(0);

          const int16Buffer = new Int16Array(float32Array.length);
          for (let i = 0; i < float32Array.length; i++) {
            let s = Math.max(-1, Math.min(1, float32Array[i]));
            int16Buffer[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
          }

          let binary = '';
          const bytes = new Uint8Array(int16Buffer.buffer);
          for (let i = 0; i < bytes.byteLength; i++) {
            binary += String.fromCharCode(bytes[i]);
          }
          const base64 = window.btoa(binary);

          ws.send(JSON.stringify({
            event: 'media',
            media: { payload: base64 }
          }));
        };

        let nextPlayTime = audioContext.currentTime;
        let activeSources = [];
        ws.onmessage = (event) => {
          const data = JSON.parse(event.data);
          if (data.type === 'clear') {
            // Backend barge-in — stop all queued audio immediately
            console.log('[barge-in] clear received, stopping', activeSources.length, 'sources');
            activeSources.forEach(s => { try { s.stop(); } catch (_) {} });
            activeSources = [];
            nextPlayTime = audioContext.currentTime;
            if (unmuteTimer) clearTimeout(unmuteTimer);
          } else if (data.event === 'media') {
            if (unmuteTimer) clearTimeout(unmuteTimer);

            const audioStr = window.atob(data.media.payload);
            const audioBytes = new Uint8Array(audioStr.length);
            for (let i = 0; i < audioStr.length; i++) {
              audioBytes[i] = audioStr.charCodeAt(i);
            }
            const int16Array = new Int16Array(audioBytes.buffer);
            const float32Array = new Float32Array(int16Array.length);
            for (let i = 0; i < int16Array.length; i++) {
              float32Array[i] = int16Array[i] / 0x8000;
            }

            const buffer = audioContext.createBuffer(1, float32Array.length, 8000);
            buffer.getChannelData(0).set(float32Array);

            const destSource = audioContext.createBufferSource();
            destSource.buffer = buffer;
            destSource.connect(audioContext.destination);
            destSource.connect(recDest);

            if (audioContext.currentTime > nextPlayTime) nextPlayTime = audioContext.currentTime;
            destSource.start(nextPlayTime);
            nextPlayTime += buffer.duration;
            activeSources.push(destSource);
            destSource.onended = () => { activeSources = activeSources.filter(s => s !== destSource); };

            // Unmute mic once after first chunk so it stays live for barge-in
            if (micMuted) {
              unmuteTimer = setTimeout(() => { micMuted = false; }, 400);
            }
          }
        };

        ws.onclose = () => {
          stream.getTracks().forEach(track => track.stop());

          const uploadRecording = async () => {
            if (recordedChunks.length > 0) {
              const blob = new Blob(recordedChunks, { type: 'audio/webm' });
              const formData = new FormData();
              formData.append('file', blob, `call_${lead.id}_${Date.now()}.webm`);
              formData.append('lead_id', String(lead.id));
              try {
                await apiFetch(`${API_URL}/upload-recording`, { method: 'POST', body: formData });
              } catch(e) { console.error('Recording upload failed:', e); }
            }
          };

          if (mediaRecorder.state !== 'inactive') {
            mediaRecorder.stop();
            mediaRecorder.onstop = () => uploadRecording();
          } else {
            uploadRecording();
          }

          if (webCallAudioCtxRef.current) webCallAudioCtxRef.current.close();
          setWebCallActive(null);
        };
      };
    } catch (e) {
      alert("Microphone access denied or connection to WebSockets failed.");
      console.error(e);
      setWebCallActive(null);
    }
  }, [apiFetch, webCallActive, orgProducts, activeVoiceProvider, activeVoiceId, activeLanguage]);

  return (
    <CallContext.Provider value={{
      dialingId, setDialingId,
      webCallActive, setWebCallActive,
      handleDial, handleWebCall,
      handleCampaignDial, handleCampaignWebCall
    }}>
      {children}
      {rechargePrompt && (
        <div onClick={() => setRechargePrompt(null)} style={{
          position: 'fixed', inset: 0, background: 'rgba(2,6,23,0.75)',
          backdropFilter: 'blur(4px)', display: 'flex', alignItems: 'center',
          justifyContent: 'center', zIndex: 10000, padding: '1rem'
        }}>
          <div onClick={e => e.stopPropagation()} style={{
            maxWidth: '440px', width: '100%', padding: '1.75rem',
            background: '#0f172a',
            border: '1px solid rgba(239,68,68,0.3)',
            borderRadius: '12px',
            boxShadow: '0 24px 48px rgba(0,0,0,0.5), 0 0 0 1px rgba(255,255,255,0.04) inset',
            color: '#e2e8f0',
          }}>
            <div style={{display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '14px'}}>
              <div style={{
                width: '40px', height: '40px', borderRadius: '50%',
                background: 'rgba(239,68,68,0.12)', border: '1px solid rgba(239,68,68,0.3)',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: '1.2rem',
              }}>⚠️</div>
              <div>
                <h3 style={{margin: 0, fontSize: '1.05rem', fontWeight: 700, color: '#fca5a5'}}>Recharge Required</h3>
                <div style={{fontSize: '0.75rem', color: '#94a3b8', marginTop: '2px'}}>Outbound calls are paused</div>
              </div>
            </div>
            <p style={{
              margin: '0 0 18px 0', fontSize: '0.9rem', lineHeight: 1.55,
              color: '#cbd5e1',
            }}>
              {rechargePrompt}
            </p>
            <p style={{
              margin: '0 0 20px 0', fontSize: '0.85rem', color: '#94a3b8',
            }}>
              Open <strong style={{color: '#a5b4fc'}}>Billing</strong> to add call credits and continue dialing.
            </p>
            <div style={{display: 'flex', gap: '10px', justifyContent: 'flex-end'}}>
              <button onClick={() => setRechargePrompt(null)} style={{
                padding: '8px 16px', borderRadius: '8px', cursor: 'pointer',
                background: 'rgba(255,255,255,0.04)',
                border: '1px solid rgba(148,163,184,0.2)',
                color: '#cbd5e1', fontSize: '0.85rem', fontWeight: 600,
              }}>Cancel</button>
              <button onClick={() => { setRechargePrompt(null); window.location.assign('/billing'); }} style={{
                padding: '8px 18px', borderRadius: '8px', cursor: 'pointer',
                background: 'linear-gradient(135deg, #6366f1, #22d3ee)',
                border: 'none', color: '#fff', fontSize: '0.85rem', fontWeight: 700,
                boxShadow: '0 6px 16px rgba(99,102,241,0.35)',
              }}>Open Billing →</button>
            </div>
          </div>
        </div>
      )}
    </CallContext.Provider>
  );
}

export function useCall() {
  const ctx = useContext(CallContext);
  if (!ctx) throw new Error('useCall must be used within CallProvider');
  return ctx;
}
