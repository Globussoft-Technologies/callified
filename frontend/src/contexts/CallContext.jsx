import React, { createContext, useContext, useState, useRef, useCallback, useEffect } from 'react';
import { API_URL } from '../constants/api';
import BrowserCallModal from '../components/campaigns/BrowserCallModal';
import { useToast } from './UIContext';
import { useAuth } from './AuthContext';
import { useOrg } from './OrgContext';
import { useVoice } from './VoiceContext';

const CallContext = createContext(null);

export function CallProvider({ children }) {
  const { apiFetch, currentUser, authToken } = useAuth();
  const { orgProducts } = useOrg();
  const { activeVoiceProvider, activeVoiceId, activeLanguage } = useVoice();
  const toast = useToast();

  const [dialingId, setDialingId] = useState(null);
  const [webCallActive, setWebCallActive] = useState(null);
  // rechargePrompt holds the backend's "insufficient credits" message when
  // a 402 comes back from the dial endpoints. Rendered as a themed modal
  // (matches the app's dark glass-panel UI) instead of the native browser
  // confirm() dialog, which used the OS theme and looked out of place.
  const [rechargePrompt, setRechargePrompt] = useState(null);
  const webCallWsRef = useRef(null);
  const webCallAudioCtxRef = useRef(null);

  // Global Browser Call state (moved out of CampaignDetail so scheduled-call
  // reminders and auto-dialer can trigger calls from anywhere).
  const [browserCallLead, setBrowserCallLead] = useState(null);
  const [browserCallCampaignId, setBrowserCallCampaignId] = useState(null);
  const [browserCallSid, setBrowserCallSid] = useState(null);
  const [browserCallDialing, setBrowserCallDialing] = useState(false);

  // Manual scheduled-call reminder popup
  const [dueManualCalls, setDueManualCalls] = useState([]);
  const [showReminder, setShowReminder] = useState(false);
  const [reminderSearch, setReminderSearch] = useState('');
  const [dismissedIds, setDismissedIds] = useState(() => new Set());
  const browserCallEndedCbRef = useRef(null);

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
    } catch { alert("Failed to hit the dialer API. Check console.");
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
        ws.send(JSON.stringify({ event: 'start', start: { stream_sid: sid, user_email: currentUser?.email || '' }, stream_sid: sid, user_email: currentUser?.email || '' }));

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
            activeSources.forEach(s => { try { s.stop(); } catch { /* ignore */ } });
            activeSources = [];
            nextPlayTime = audioContext.currentTime;
            if (unmuteTimer) { clearTimeout(unmuteTimer); unmuteTimer = null; }
            micMuted = false;
          } else if (data.event === 'media') {
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

            // Unmute mic once, 400ms after the first chunk — don't reset on every chunk
            if (micMuted && !unmuteTimer) {
              unmuteTimer = setTimeout(() => { micMuted = false; unmuteTimer = null; }, 400);
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

  const triggerBrowserCall = useCallback(async (lead, campaignId, onEnded) => {
    if (!lead || !campaignId) return;
    browserCallEndedCbRef.current = onEnded || null;
    setBrowserCallLead(lead);
    setBrowserCallCampaignId(campaignId);
    setBrowserCallSid(null);
    setBrowserCallDialing(true);
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${campaignId}/leads/${lead.id}/browser-call`, { method: 'POST' });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || `Browser call failed (HTTP ${res.status})`);
      setBrowserCallSid(data.call_sid || data.sid);
    } catch (e) {
      toast({ message: e?.message || 'Browser call failed', kind: 'error' });
      setBrowserCallLead(null);
      setBrowserCallCampaignId(null);
      setBrowserCallSid(null);
      browserCallEndedCbRef.current = null;
    } finally {
      setBrowserCallDialing(false);
    }
  }, [apiFetch, toast]);

  const closeBrowserCall = useCallback(() => {
    browserCallEndedCbRef.current = null;
    setBrowserCallLead(null);
    setBrowserCallCampaignId(null);
    setBrowserCallSid(null);
  }, []);

  const handleBrowserCallEnded = useCallback((status, errorMsg) => {
    const cb = browserCallEndedCbRef.current;
    browserCallEndedCbRef.current = null;
    if (cb) cb(status, errorMsg);
  }, []);

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
    } catch { /* ignore */ }

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
            activeSources.forEach(s => { try { s.stop(); } catch { /* ignore */ } });
            activeSources = [];
            nextPlayTime = audioContext.currentTime;
            if (unmuteTimer) { clearTimeout(unmuteTimer); unmuteTimer = null; }
            micMuted = false;
          } else if (data.event === 'media') {
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

            // Unmute mic once, 400ms after the first chunk — don't reset on every chunk
            if (micMuted && !unmuteTimer) {
              unmuteTimer = setTimeout(() => { micMuted = false; unmuteTimer = null; }, 400);
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
              formData.append('campaign_id', String(campaignId));
              try {
                const res = await apiFetch(`${API_URL}/upload-recording`, { method: 'POST', body: formData });
                if (!res.ok) console.error(`[RECORDING] Upload failed: HTTP ${res.status}`);
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

  // Poll for due manual scheduled calls and show a global reminder popup.
  useEffect(() => {
    if (!authToken) return;
    const fetchDue = async () => {
      try {
        const res = await apiFetch(`${API_URL}/scheduled-calls?mode=manual&status=pending&due=true`);
        if (!res.ok) return;
        const calls = await res.json();
        setDueManualCalls(calls || []);
        const visible = (calls || []).filter(c => !dismissedIds.has(c.id));
        if (visible.length > 0) setShowReminder(true);
      } catch (e) {
        console.error('[scheduled-calls] poll failed', e);
      }
    };
    fetchDue();
    const id = setInterval(fetchDue, 15000);
    return () => clearInterval(id);
  }, [apiFetch, authToken]);

  const reminderFilteredCalls = dueManualCalls.filter(c => {
    if (dismissedIds.has(c.id)) return false;
    if (!reminderSearch.trim()) return true;
    const term = reminderSearch.trim().toLowerCase();
    const exec = String(c.executive_name || '').toLowerCase();
    const name = String(c.first_name || '').toLowerCase();
    const phone = String(c.phone || '').toLowerCase();
    return exec.includes(term) || name.includes(term) || phone.includes(term);
  });

  useEffect(() => {
    if (showReminder && reminderFilteredCalls.length === 0) {
      setShowReminder(false);
    }
  }, [showReminder, reminderFilteredCalls.length]);

  return (
    <CallContext.Provider value={{
      dialingId, setDialingId,
      webCallActive, setWebCallActive,
      handleDial, handleWebCall,
      handleCampaignDial, handleCampaignWebCall,
      browserCallLead, browserCallDialing,
      triggerBrowserCall, closeBrowserCall
    }}>
      {children}
      {browserCallLead && (
        <BrowserCallModal
          lead={browserCallLead}
          callSid={browserCallSid}
          wsBaseUrl={(window.location.protocol === 'https:' ? 'wss:' : 'ws:') + '//' + window.location.host}
          onClose={closeBrowserCall}
          onEnded={handleBrowserCallEnded}
        />
      )}
      {showReminder && dueManualCalls.length > 0 && (
        <div onClick={() => setShowReminder(false)} style={{
          position: 'fixed', inset: 0, background: 'rgba(2,6,23,0.75)',
          backdropFilter: 'blur(4px)', display: 'flex', alignItems: 'center',
          justifyContent: 'center', zIndex: 10001, padding: '1rem'
        }}>
          <div onClick={e => e.stopPropagation()} style={{
            maxWidth: '520px', width: '100%', maxHeight: '80vh', display: 'flex', flexDirection: 'column',
            background: '#0f172a',
            border: '1px solid rgba(148,163,184,0.2)',
            borderRadius: '12px',
            boxShadow: '0 24px 48px rgba(0,0,0,0.5), 0 0 0 1px rgba(255,255,255,0.04) inset',
            color: '#e2e8f0',
          }}>
            <div style={{ padding: '1.25rem 1.25rem 0.75rem', borderBottom: '1px solid rgba(148,163,184,0.12)' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <h3 style={{ margin: 0, fontSize: '1.05rem', fontWeight: 700 }}>📅 Scheduled calls due</h3>
                <button onClick={() => setShowReminder(false)} style={{
                  background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer', fontSize: '1.1rem'
                }}>✕</button>
              </div>
              <p style={{ margin: '6px 0 0', fontSize: '0.8rem', color: '#94a3b8' }}>
                These manual calls are ready to dial now.
              </p>
            </div>
            <div style={{ padding: '0.75rem 1.25rem' }}>
              <input
                type="text"
                placeholder="Search by executive, name or phone..."
                value={reminderSearch}
                onChange={e => setReminderSearch(e.target.value)}
                style={{
                  width: '100%', padding: '10px 12px', borderRadius: '8px', border: '1px solid rgba(148,163,184,0.2)',
                  background: '#0b1220', color: '#e2e8f0', fontSize: '0.85rem', outline: 'none',
                  boxSizing: 'border-box'
                }}
              />
            </div>
            <div style={{ overflowY: 'auto', padding: '0 1.25rem 1rem', display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {reminderFilteredCalls.length === 0 && (
                <div style={{ textAlign: 'center', color: '#94a3b8', fontSize: '0.85rem', padding: '1rem 0' }}>
                  No calls match your search.
                </div>
              )}
              {reminderFilteredCalls.map(call => (
                <div key={call.id} style={{
                  display: 'flex', alignItems: 'center', gap: '10px',
                  padding: '10px 12px', background: 'rgba(255,255,255,0.03)', borderRadius: '8px'
                }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: 600, fontSize: '0.9rem' }}>{call.first_name || 'Unnamed'}</div>
                    <div style={{ fontSize: '0.75rem', color: '#94a3b8', marginTop: '2px' }}>
                      {call.phone || 'No phone'} • {call.executive_name || 'Unassigned'} • {call.scheduled_time ? new Date(call.scheduled_time).toLocaleString() : ''}
                    </div>
                  </div>
                  <button
                    onClick={() => {
                      triggerBrowserCall({ id: call.lead_id, first_name: call.first_name || '', last_name: '', phone: call.phone || '' }, call.campaign_id);
                      setDismissedIds(prev => new Set(prev).add(call.id));
                      setShowReminder(false);
                    }}
                    disabled={browserCallDialing}
                    style={{
                      padding: '6px 12px', borderRadius: '6px', cursor: 'pointer', border: 'none',
                      background: 'linear-gradient(135deg, #16a34a, #22c55e)', color: '#fff', fontWeight: 600,
                      fontSize: '0.8rem', opacity: browserCallDialing ? 0.6 : 1
                    }}
                  >Call Now</button>
                  <button
                    onClick={() => {
                      setDismissedIds(prev => new Set(prev).add(call.id));
                    }}
                    style={{
                      padding: '6px 10px', borderRadius: '6px', cursor: 'pointer',
                      background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(148,163,184,0.2)', color: '#94a3b8',
                      fontSize: '0.8rem'
                    }}
                  >Dismiss</button>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
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

// eslint-disable-next-line react-refresh/only-export-components
export function useCall() {
  const ctx = useContext(CallContext);
  if (!ctx) throw new Error('useCall must be used within CallProvider');
  return ctx;
}
