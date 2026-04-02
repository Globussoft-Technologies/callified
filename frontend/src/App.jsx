import React, { useState, useEffect, useRef } from 'react';
import MonitorPage from './pages/MonitorPage';
import KnowledgePage from './pages/KnowledgePage';
import SandboxPage from './pages/SandboxPage';
import AuthPage from './components/AuthPage';
import TopHeader from './components/TopHeader';
import CrmTab from './components/tabs/CrmTab';
import OpsPage from './pages/OpsPage';
import AnalyticsPage from './pages/AnalyticsPage';
import WhatsAppPage from './pages/WhatsAppPage';
import IntegrationsPage from './pages/IntegrationsPage';
import SettingsTab from './components/tabs/SettingsTab';
import LogsPage from './pages/LogsPage';
import CheckInPage from './pages/CheckInPage';
import CampaignsTab from './components/tabs/CampaignsTab';
import LeadModals from './components/modals/LeadModals';
import DocumentVault from './components/modals/DocumentVault';
import TranscriptModal from './components/modals/TranscriptModal';
import EmailDraftModal from './components/modals/EmailDraftModal';
import './index.css';
import { API_URL } from './constants/api';
import { INDIAN_VOICES, INDIAN_LANGUAGES } from './constants/voices';

export default function App() {
  // Auth State
  const [authToken, setAuthToken] = useState(localStorage.getItem('authToken') || null);
  const apiFetch = async (url, options = {}) => {
    return fetch(url, {
      ...options,
      headers: {
        ...options.headers,
        'Authorization': `Bearer ${authToken}`
      }
    });
  };
  const [currentUser, setCurrentUser] = useState(null);
  const [authPage, setAuthPage] = useState('login'); // 'login' or 'signup'
  const [authError, setAuthError] = useState('');
  const [authLoading, setAuthLoading] = useState(false);
  const [authForm, setAuthForm] = useState({ org_name: '', full_name: '', email: 'sumit@globussoft.com', password: 'sumit1234' });

  // Check token on mount
  useEffect(() => {
    if (authToken) {
      fetch(`${API_URL}/auth/me`, { headers: { 'Authorization': `Bearer ${authToken}` } })
        .then(r => r.ok ? r.json() : Promise.reject())
        .then(u => setCurrentUser(u))
        .catch(() => { setAuthToken(null); localStorage.removeItem('authToken'); });
    }
  }, [authToken]);

  const handleLogin = async (e) => {
    e.preventDefault();
    setAuthError(''); setAuthLoading(true);
    try {
      const res = await fetch(`${API_URL}/auth/login`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: authForm.email, password: authForm.password })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.detail || 'Login failed');
      localStorage.setItem('authToken', data.access_token);
      setAuthToken(data.access_token);
      setCurrentUser(data.user);
    } catch (err) { setAuthError(err.message); }
    setAuthLoading(false);
  };

  const handleSignup = async (e) => {
    e.preventDefault();
    setAuthError(''); setAuthLoading(true);
    try {
      const res = await fetch(`${API_URL}/auth/signup`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(authForm)
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.detail || 'Signup failed');
      localStorage.setItem('authToken', data.access_token);
      setAuthToken(data.access_token);
      setCurrentUser(data.user);
    } catch (err) { setAuthError(err.message); }
    setAuthLoading(false);
  };

  const handleLogout = () => {
    localStorage.removeItem('authToken');
    setAuthToken(null);
    setCurrentUser(null);
    setAuthForm({ org_name: '', full_name: '', email: '', password: '' });
  };


  const [activeTab, setActiveTab] = useState('crm');
  const [leads, setLeads] = useState([]);
  // sites state — moved to CheckInPage
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [dialingId, setDialingId] = useState(null);
  const [webCallActive, setWebCallActive] = useState(null);
  const webCallWsRef = useRef(null);
  const webCallAudioCtxRef = useRef(null);
  
  const [formData, setFormData] = useState({ first_name: '', last_name: '', phone: '', source: 'Manual Entry' });

  // Edit Lead State
  const [editModalOpen, setEditModalOpen] = useState(false);
  const [editingLead, setEditingLead] = useState(null);
  const [editFormData, setEditFormData] = useState({ first_name: '', last_name: '', phone: '', source: '' });

  // fieldOpsData, punchStatus, punching — moved to CheckInPage

  // Workflow State — moved to OpsPage

  // WhatsApp State — moved to WhatsAppPage

  // Document Vault State
  const [activeLeadDocs, setActiveLeadDocs] = useState(null);
  const [docs, setDocs] = useState([]);
  const [docFormData, setDocFormData] = useState({ file_name: '', file_url: '' });

  // Analytics State — moved to AnalyticsPage

  // Search Engine State
  const [searchQuery, setSearchQuery] = useState('');
  
  // Integrations State — moved to IntegrationsPage

  // RBAC Global State
  const userRole = currentUser?.role || 'Agent';

  // GenAI Email Modal State
  const [emailDraft, setEmailDraft] = useState(null);

  // Pronunciation Guide State
  const [pronunciations, setPronunciations] = useState([]);
  const [pronFormData, setPronFormData] = useState({ word: '', phonetic: '' });

  // Call Transcript State
  const [transcriptLead, setTranscriptLead] = useState(null);
  const [transcripts, setTranscripts] = useState([]);

  // Product Knowledge State
  const [orgs, setOrgs] = useState([]);
  const [selectedOrg, setSelectedOrg] = useState(null);
  const [orgTimezone, setOrgTimezone] = useState(Intl.DateTimeFormat().resolvedOptions().timeZone);
  const [orgProducts, setOrgProducts] = useState([]);
  const [scraping, setScraping] = useState(null); // product_id being scraped
  const [newOrgName, setNewOrgName] = useState('');
  const [showOrgInput, setShowOrgInput] = useState(false);
  const [newProductName, setNewProductName] = useState('');
  const [showProductInput, setShowProductInput] = useState(false);
  const [systemPromptAuto, setSystemPromptAuto] = useState('');
  const [systemPromptCustom, setSystemPromptCustom] = useState('');
  const [promptSaving, setPromptSaving] = useState(false);
  const [promptDirty, setPromptDirty] = useState(false);
  const [activeVoiceProvider, setActiveVoiceProvider] = useState('elevenlabs');
  const [activeVoiceId, setActiveVoiceId] = useState('');
  const [savedVoiceName, setSavedVoiceName] = useState('');
  const [activeLanguage, setActiveLanguage] = useState('hi');

  const [campaigns, setCampaigns] = useState([]);


  // Auth block moved down to fix React hooks violation

  const fetchLeads = async () => {
    try {
      const res = await apiFetch(`${API_URL}/leads`);
      const data = await res.json();
      setLeads(data);
    } catch (e) {
      console.error("Make sure FastAPI is running with CORS enabled!", e);
    }
  };

  // fetchSites — moved to CheckInPage

  // fetchTasks, fetchReports — moved to OpsPage

  // fetchWhatsappLogs — moved to WhatsAppPage (WhatsAppTab manages its own data)

  // fetchAnalytics — moved to AnalyticsPage

  // fetchIntegrations — moved to IntegrationsPage

  const fetchCampaigns = async () => {
    try { const res = await apiFetch(`${API_URL}/campaigns`); setCampaigns(await res.json()); } catch(e){}
  };

  const fetchPronunciations = async () => {
    try { const res = await apiFetch(`${API_URL}/pronunciation`); setPronunciations(await res.json()); } catch(e){}
  };

  const fetchOrgs = async () => {
    try {
      const res = await apiFetch(`${API_URL}/organizations`);
      const data = await res.json();
      setOrgs(data);
      // Auto-select user's org if only one
      if (data.length === 1 && !selectedOrg) {
        setSelectedOrg(data[0]);
        // Set org timezone (or auto-detect and save)
        const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone;
        if (data[0].timezone) {
          setOrgTimezone(data[0].timezone);
        } else {
          // First time — save browser timezone to org
          setOrgTimezone(browserTz);
          apiFetch(`${API_URL}/organizations/${data[0].id}/timezone`, {
            method: 'PUT', headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ timezone: browserTz })
          }).catch(() => {});
        }
        fetchOrgProducts(data[0].id);
        fetchSystemPrompt(data[0].id);
        // Load voice settings
        try {
          const vRes = await apiFetch(`${API_URL}/organizations/${data[0].id}/voice-settings`);
          const vs = await vRes.json();
          if (vs.tts_provider) {
            setActiveVoiceProvider(vs.tts_provider);
            if (vs.tts_voice_id) {
              setActiveVoiceId(vs.tts_voice_id);
              const allV = [...(INDIAN_VOICES[vs.tts_provider] || []), ...(INDIAN_VOICES.elevenlabs || []), ...(INDIAN_VOICES.smallest || [])];
              const found = allV.find(v => v.id === vs.tts_voice_id);
              if (found) setSavedVoiceName(found.name);
            }
            if (vs.tts_language) setActiveLanguage(vs.tts_language);
          }
        } catch(e){}
      }
    } catch(e){}
  };

  const fetchOrgProducts = async (orgId) => {
    try { const res = await apiFetch(`${API_URL}/organizations/${orgId}/products`); setOrgProducts(await res.json()); } catch(e){}
  };

  useEffect(() => {
    if (!currentUser) return;
    fetchLeads();
    fetchPronunciations();
    fetchCampaigns();
    fetchOrgs();
  }, [currentUser]);

  const handleStatusChange = async (leadId, newStatus) => {
    try {
      await apiFetch(`${API_URL}/leads/${leadId}/status`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: newStatus })
      });
      fetchLeads();
    } catch (e) { console.error(e); }
  };

  // handleCompleteTask — moved to OpsPage

  const handleCreateLead = async (e) => {
    e.preventDefault();
    setLoading(true);
    try {
      await apiFetch(`${API_URL}/leads`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(formData)
      });
      setFormData({ first_name: '', last_name: '', phone: '', source: 'Manual Entry' });
      setIsModalOpen(false);
      fetchLeads();
    } catch(e) {
      console.error(e);
    }
    setLoading(false);
  };

  const handleDial = async (lead) => {
    setDialingId('global');
    try {
      const res = await apiFetch(`${API_URL}/dial/${lead.id}`, { method: "POST" });
      const data = await res.json();
      alert(`Status: ${data.message || 'Connecting call...'}`);
    } catch(e) {
      alert("Failed to hit the dialer API. Check console.");
    }
    setTimeout(() => setDialingId(null), 10000);
  };

  const handleWebCall = async (lead) => {
    if (webCallActive === lead.id) {
      // Disconnect active simulation
      if (webCallWsRef.current) webCallWsRef.current.close();
      if (webCallAudioCtxRef.current) webCallAudioCtxRef.current.close();
      setWebCallActive(null);
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
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
        ws.onmessage = (event) => {
          const data = JSON.parse(event.data);
          if (data.event === 'media') {
            // Mute mic while AI is talking to prevent echo feedback
            micMuted = true;
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

            // Unmute mic 500ms after last TTS chunk finishes playing
            const remainingPlayMs = Math.max(0, (nextPlayTime - audioContext.currentTime) * 1000) + 500;
            unmuteTimer = setTimeout(() => { micMuted = false; }, remainingPlayMs);
          } else if (data.event === 'clear') {
            nextPlayTime = audioContext.currentTime; // Discard TTS queue on barge-in
            micMuted = false; // Immediately unmute on barge-in clear
            if (unmuteTimer) clearTimeout(unmuteTimer);
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
  };

  const handleCampaignDial = async (lead, campaignId) => {
    setDialingId('global');
    try {
      await apiFetch(`${API_URL}/campaigns/${campaignId}/dial/${lead.id}`, { method: "POST" });
    } catch(e) {}
    setTimeout(() => setDialingId(null), 10000);
  };

  const handleCampaignWebCall = async (lead, campaignId) => {
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
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
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
        ws.onmessage = (event) => {
          const data = JSON.parse(event.data);
          if (data.event === 'media') {
            micMuted = true;
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

            const remainingPlayMs = Math.max(0, (nextPlayTime - audioContext.currentTime) * 1000) + 500;
            unmuteTimer = setTimeout(() => { micMuted = false; }, remainingPlayMs);
          } else if (data.event === 'clear') {
            nextPlayTime = audioContext.currentTime;
            micMuted = false;
            if (unmuteTimer) clearTimeout(unmuteTimer);
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
  };

  const handleOpenDocs = async (lead) => {
    setActiveLeadDocs(lead);
    try {
      const res = await apiFetch(`${API_URL}/leads/${lead.id}/documents`);
      setDocs(await res.json());
    } catch(e) {}
  };

  const handleUploadDoc = async (e) => {
    e.preventDefault();
    try {
      await apiFetch(`${API_URL}/leads/${activeLeadDocs.id}/documents`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify(docFormData)
      });
      setDocFormData({ file_name: '', file_url: '' });
      const res = await apiFetch(`${API_URL}/leads/${activeLeadDocs.id}/documents`);
      setDocs(await res.json());
    } catch(e) { console.error(e); }
  };

  // handlePunchIn — moved to CheckInPage

  const handleSearch = async (e) => {
    const query = e.target.value;
    setSearchQuery(query);
    if (query.trim().length >= 2) {
      try {
        const res = await apiFetch(`${API_URL}/leads/search?q=${encodeURIComponent(query)}`);
        setLeads(await res.json());
      } catch(e) {}
    } else if (query.trim().length === 0) {
      fetchLeads();
    }
  };

  const [noteLead, setNoteLead] = useState(null);
  const [noteText, setNoteText] = useState('');

  const handleNote = (lead) => {
    setNoteLead(lead);
    setNoteText(lead.follow_up_note || '');
  };

  const handleSaveNote = async () => {
    if (!noteLead) return;
    try {
      await apiFetch(`${API_URL}/leads/${noteLead.id}/notes`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ note: noteText })
      });
      fetchLeads();
      setNoteLead(null);
    } catch(e) {
      console.error("Error saving note", e);
    }
  };

  const handleDraftEmail = async (lead) => {
    setDialingId(lead.id); // Reuse the dialing spinner temporarily
    try {
      const res = await apiFetch(`${API_URL}/leads/${lead.id}/draft-email`);
      const data = await res.json();
      setEmailDraft(data);
    } catch(e) {
      console.error("Error generating email", e);
    }
    setDialingId(null);
  };

  const handleEditLead = (lead) => {
    setEditingLead(lead);
    setEditFormData({
      first_name: lead.first_name || '',
      last_name: lead.last_name || '',
      phone: lead.phone || '',
      source: lead.source || 'Manual Entry'
    });
    setEditModalOpen(true);
  };

  const handleSaveEdit = async (e) => {
    e.preventDefault();
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/leads/${editingLead.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(editFormData)
      });
      const data = await res.json();
      if (!res.ok || data.status === 'error') {
        throw new Error(data.message || 'Error updating lead details');
      }
      setEditModalOpen(false);
      setEditingLead(null);
      fetchLeads();
    } catch (e) {
      alert(e.message);
      console.error('Error updating lead', e);
    }
    setLoading(false);
  };

  const handleDeleteLead = async (lead) => {
    if (!window.confirm(`Are you sure you want to delete ${lead.first_name} ${lead.last_name}?`)) return;
    try {
      await apiFetch(`${API_URL}/leads/${lead.id}`, { method: 'DELETE' });
      fetchLeads();
    } catch (e) {
      console.error('Error deleting lead', e);
    }
  };

  // handleCreateIntegration — moved to IntegrationsPage

  const handleAddPronunciation = async (e) => {
    e.preventDefault();
    if (!pronFormData.word.trim() || !pronFormData.phonetic.trim()) return;
    try {
      await apiFetch(`${API_URL}/pronunciation`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(pronFormData)
      });
      setPronFormData({ word: '', phonetic: '' });
      fetchPronunciations();
    } catch(e) { console.error(e); }
  };

  const handleDeletePronunciation = async (id) => {
    try {
      await apiFetch(`${API_URL}/pronunciation/${id}`, { method: 'DELETE' });
      fetchPronunciations();
    } catch(e) { console.error(e); }
  };

  const handleViewTranscripts = async (lead) => {
    setTranscriptLead(lead);
    try {
      const res = await apiFetch(`${API_URL}/leads/${lead.id}/transcripts`);
      setTranscripts(await res.json());
    } catch(e) { setTranscripts([]); }
  };

  const handleCreateOrg = async () => {
    if (!newOrgName.trim()) return;
    await apiFetch(`${API_URL}/organizations`, { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({ name: newOrgName.trim() }) });
    setNewOrgName(''); setShowOrgInput(false);
    fetchOrgs();
  };

  const handleDeleteOrg = async (orgId) => {
    if (!confirm('Delete this organization and all its products?')) return;
    await apiFetch(`${API_URL}/organizations/${orgId}`, { method: 'DELETE' });
    if (selectedOrg?.id === orgId) { setSelectedOrg(null); setOrgProducts([]); }
    fetchOrgs();
  };

  const handleSelectOrg = (org) => {
    setSelectedOrg(org);
    setShowProductInput(false); setNewProductName('');
    fetchOrgProducts(org.id);
    fetchSystemPrompt(org.id);
  };

  const handleAddProduct = async () => {
    if (!selectedOrg || !newProductName.trim()) return;
    await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/products`, {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ name: newProductName.trim() })
    });
    setNewProductName(''); setShowProductInput(false);
    fetchOrgProducts(selectedOrg.id);
  };

  const handleScrapeProduct = async (productId) => {
    setScraping(productId);
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/scrape`, { method: 'POST' });
      const data = await res.json();
      if (data.scraped_info) fetchOrgProducts(selectedOrg.id);
    } catch(e) { console.error(e); }
    setScraping(null);
  };

  const handleSaveProduct = async (productId, updates) => {
    await apiFetch(`${API_URL}/products/${productId}`, {
      method: 'PUT', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(updates)
    });
    fetchOrgProducts(selectedOrg.id);
    // Refresh system prompt preview after product update
    if (selectedOrg) fetchSystemPrompt(selectedOrg.id);
  };

  const fetchSystemPrompt = async (orgId) => {
    try {
      const res = await apiFetch(`${API_URL}/organizations/${orgId}/system-prompt`);
      const data = await res.json();
      setSystemPromptAuto(data.auto_generated || '');
      setSystemPromptCustom(data.custom_prompt || '');
      setPromptDirty(false);
    } catch(e) {}
  };

  const handleSaveSystemPrompt = async () => {
    if (!selectedOrg) return;
    setPromptSaving(true);
    await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/system-prompt`, {
      method: 'PUT', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ custom_prompt: systemPromptCustom })
    });
    setPromptSaving(false);
    setPromptDirty(false);
  };

  const handleDeleteProduct = async (productId) => {
    if (!confirm('Delete this product?')) return;
    await apiFetch(`${API_URL}/products/${productId}`, { method: 'DELETE' });
    fetchOrgProducts(selectedOrg.id);
  };

  // ─── AUTH PAGES (after all hooks) ───
  if (!authToken || !currentUser) {
    return (
      <AuthPage 
        authPage={authPage} setAuthPage={setAuthPage}
        authError={authError} setAuthError={setAuthError}
        authLoading={authLoading}
        authForm={authForm} setAuthForm={setAuthForm}
        handleLogin={handleLogin} handleSignup={handleSignup}
      />
    );
  }

  return (
    <div className="dashboard-container">
      <TopHeader 
        activeTab={activeTab} setActiveTab={setActiveTab}
        userRole={userRole} currentUser={currentUser}
        handleLogout={handleLogout}
      />
      
      {activeTab === 'crm' ? (
        <CrmTab 
          searchQuery={searchQuery} handleSearch={handleSearch} setIsModalOpen={setIsModalOpen}
          userRole={userRole} leads={leads} API_URL={API_URL} authToken={authToken} fetchLeads={fetchLeads}
          activeVoiceProvider={activeVoiceProvider} setActiveVoiceProvider={setActiveVoiceProvider}
          activeVoiceId={activeVoiceId} setActiveVoiceId={setActiveVoiceId}
          activeLanguage={activeLanguage} setActiveLanguage={setActiveLanguage}
          INDIAN_VOICES={INDIAN_VOICES} INDIAN_LANGUAGES={INDIAN_LANGUAGES}
          selectedOrg={selectedOrg} apiFetch={apiFetch}
          savedVoiceName={savedVoiceName} setSavedVoiceName={setSavedVoiceName}
          handleStatusChange={handleStatusChange} handleEditLead={handleEditLead}
          handleDeleteLead={handleDeleteLead} handleOpenDocs={handleOpenDocs}
          handleViewTranscripts={handleViewTranscripts} handleNote={handleNote}
          handleDraftEmail={handleDraftEmail} dialingId={dialingId}
          webCallActive={webCallActive} handleWebCall={handleWebCall} handleDial={handleDial}
          campaigns={campaigns}
          onCampaignClick={(campaign) => { setActiveTab('campaigns'); }}
        />
      ) : activeTab === 'campaigns' ? (
        <CampaignsTab
          campaigns={campaigns} fetchCampaigns={fetchCampaigns}
          orgProducts={orgProducts} leads={leads}
          apiFetch={apiFetch} API_URL={API_URL} selectedOrg={selectedOrg}
          onCampaignDial={handleCampaignDial} onCampaignWebCall={handleCampaignWebCall}
          activeVoiceProvider={activeVoiceProvider} activeVoiceId={activeVoiceId}
          activeLanguage={activeLanguage}
          INDIAN_VOICES={INDIAN_VOICES} INDIAN_LANGUAGES={INDIAN_LANGUAGES}
          dialingId={dialingId} webCallActive={webCallActive}
          handleViewTranscripts={handleViewTranscripts} handleNote={handleNote}
          orgTimezone={orgTimezone}
        />
      ) : activeTab === 'ops' ? (
        <OpsPage apiFetch={apiFetch} API_URL={API_URL} />
      ) : activeTab === 'analytics' ? (
        <AnalyticsPage apiFetch={apiFetch} API_URL={API_URL} />
      ) : activeTab === 'whatsapp' ? (
        <WhatsAppPage apiFetch={apiFetch} API_URL={API_URL} orgProducts={orgProducts} selectedOrg={selectedOrg} orgTimezone={orgTimezone} />
      ) : activeTab === 'integrations' ? (
        <IntegrationsPage apiFetch={apiFetch} API_URL={API_URL} orgTimezone={orgTimezone} />
      ) : activeTab === 'monitor' ? (
        <MonitorPage API_URL={API_URL} />
      ) : activeTab === 'knowledge' ? (
        <KnowledgePage API_URL={API_URL} />
      ) : activeTab === 'sandbox' ? (
        <SandboxPage API_URL={API_URL} />
      ) : activeTab === 'settings' ? (
        <SettingsTab orgTimezone={orgTimezone}
          handleAddPronunciation={handleAddPronunciation} pronFormData={pronFormData}
          setPronFormData={setPronFormData} pronunciations={pronunciations}
          handleDeletePronunciation={handleDeletePronunciation} selectedOrg={selectedOrg}
          orgs={orgs} showProductInput={showProductInput} setShowProductInput={setShowProductInput}
          newProductName={newProductName} setNewProductName={setNewProductName}
          handleAddProduct={handleAddProduct} orgProducts={orgProducts}
          handleDeleteProduct={handleDeleteProduct} handleSaveProduct={handleSaveProduct}
          scraping={scraping} handleScrapeProduct={handleScrapeProduct}
          promptDirty={promptDirty} handleSaveSystemPrompt={handleSaveSystemPrompt}
          promptSaving={promptSaving} systemPromptAuto={systemPromptAuto}
          systemPromptCustom={systemPromptCustom} setSystemPromptCustom={setSystemPromptCustom}
          setPromptDirty={setPromptDirty}
          apiFetch={apiFetch} API_URL={API_URL}
        />
      ) : activeTab === 'logs' ? (
        <LogsPage API_URL={API_URL} authToken={authToken} />
      ) : (
        <CheckInPage apiFetch={apiFetch} API_URL={API_URL} />
      )}

      <LeadModals 
        isModalOpen={isModalOpen} setIsModalOpen={setIsModalOpen}
        handleCreateLead={handleCreateLead} formData={formData}
        setFormData={setFormData} loading={loading}
        editModalOpen={editModalOpen} setEditModalOpen={setEditModalOpen}
        editingLead={editingLead} handleSaveEdit={handleSaveEdit}
        editFormData={editFormData} setEditFormData={setEditFormData}
      />
      <DocumentVault
        activeLeadDocs={activeLeadDocs} setActiveLeadDocs={setActiveLeadDocs}
        handleUploadDoc={handleUploadDoc} docFormData={docFormData}
        setDocFormData={setDocFormData} docs={docs} orgTimezone={orgTimezone}
      />
      <TranscriptModal
        transcriptLead={transcriptLead} setTranscriptLead={setTranscriptLead}
        transcripts={transcripts} orgTimezone={orgTimezone}
      />
      <EmailDraftModal
        emailDraft={emailDraft} setEmailDraft={setEmailDraft}
      />

      {/* Note Modal */}
      {noteLead && (
        <div className="modal-overlay" onClick={() => setNoteLead(null)}>
          <div className="glass-panel modal-content" onClick={e => e.stopPropagation()} style={{maxWidth: '520px'}}>
            <h2 style={{marginTop: 0, marginBottom: '0.5rem'}}>📝 Quick Note</h2>
            <p style={{color: '#94a3b8', fontSize: '0.85rem', marginBottom: '1.5rem'}}>
              {noteLead.first_name} {noteLead.last_name} — {noteLead.phone}
            </p>
            <textarea className="form-input" rows={5} value={noteText}
              onChange={e => setNoteText(e.target.value)}
              placeholder="Type your follow-up note here..."
              style={{width: '100%', minHeight: '120px', resize: 'vertical', fontSize: '0.9rem', lineHeight: 1.5}} />
            <div style={{display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '1.5rem'}}>
              <button onClick={() => setNoteLead(null)}
                style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', color: '#cbd5e1', padding: '8px 18px', borderRadius: '8px', cursor: 'pointer'}}>
                Cancel
              </button>
              <button className="btn-primary" onClick={handleSaveNote}>
                Save Note
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

