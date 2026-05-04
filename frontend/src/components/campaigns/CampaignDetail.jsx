import React, { useState, useEffect, useMemo } from 'react';
import { formatDateTime } from '../../utils/dateFormat';
import { VOICE_RECOMMENDATIONS } from '../../constants/voices';

function AuthAudioPlayer({ src, style }) {
  const [blobUrl, setBlobUrl] = React.useState(null);
  const [err, setErr] = React.useState(false);
  const audioRef = React.useRef(null);
  const seekedRef = React.useRef(false);

  React.useEffect(() => {
    if (!src) return;
    let objectUrl;
    const token = localStorage.getItem('authToken');
    fetch(src, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
      .then(r => { if (!r.ok) throw new Error(r.status); return r.blob(); })
      .then(blob => { objectUrl = URL.createObjectURL(blob); setBlobUrl(objectUrl); })
      .catch(() => setErr(true));
    return () => { if (objectUrl) URL.revokeObjectURL(objectUrl); };
  }, [src]);

  // WebM files from MediaRecorder omit duration metadata; seeking to a huge
  // timestamp forces the browser to scan the file and report the real duration.
  const handleLoadedMetadata = () => {
    const a = audioRef.current;
    if (!a || seekedRef.current) return;
    if (!isFinite(a.duration) || a.duration === 0) {
      seekedRef.current = true;
      a.currentTime = 1e101;
    }
  };
  const handleSeeked = () => {
    const a = audioRef.current;
    if (!a || !seekedRef.current) return;
    seekedRef.current = false;
    a.currentTime = 0;
  };

  if (err) return <span style={{color:'#f87171',fontSize:'0.75rem'}}>Unavailable</span>;
  if (!blobUrl) return <span style={{color:'#64748b',fontSize:'0.75rem'}}>Loading…</span>;
  return (
    <audio ref={audioRef} controls style={style} src={blobUrl}
      onLoadedMetadata={handleLoadedMetadata} onSeeked={handleSeeked} />
  );
}

export default function CampaignDetail({
  selectedCampaign, setSelectedCampaign,
  campaignLeads, callLog, detailTab, setDetailTab,
  handleBack, fetchCampaignLeads, fetchCallLog, fetchCampaigns,
  statusBadge, getProductName, getCampaignStats,
  campVoice, setCampVoice, handleSaveCampVoice, handleResetCampVoice, voiceSaveStatus = 'idle',
  INDIAN_VOICES, INDIAN_LANGUAGES,
  liveEvents, setLiveEvents,
  handleLeadStatusChange, handleEditLead, handleRemoveLead,
  handleViewTranscripts, handleNote,
  onCampaignDial, onCampaignWebCall,
  dialingId, webCallActive,
  setSelectedLeadIds, setShowAddLeadsModal, setShowCsvImportModal, setCsvFile,
  apiFetch, API_URL, orgTimezone,
  handleEditCampaign
}) {
  const stats = getCampaignStats(selectedCampaign);
  const [callInsights, setCallInsights] = useState(null);
  const [callReviews, setCallReviews] = useState([]);
  const [insightsLoading, setInsightsLoading] = useState(false);
  const [billingUsage, setBillingUsage] = useState(null);
  const [retries, setRetries] = useState([]);
  const [retriesLoading, setRetriesLoading] = useState(false);

  const [confirmRemoveLeadId, setConfirmRemoveLeadId] = useState(null);
  const [confirmDialAction, setConfirmDialAction] = useState(null); // { type: 'new'|'all'|'redial', label, count }

  // Schedule Call state
  const [scheduleLead, setScheduleLead] = useState(null);
  const [scheduleAt, setScheduleAt] = useState('');
  const [scheduleNotes, setScheduleNotes] = useState('');
  const [scheduleSaving, setScheduleSaving] = useState(false);
  const [scheduleError, setScheduleError] = useState('');

  // Refresh call log after sim web call ends — poll multiple times to catch
  // transcripts that are written asynchronously (Deepgram, WAV mux, DB write).
  const prevWebCallActiveRef = React.useRef(webCallActive);
  useEffect(() => {
    if (prevWebCallActiveRef.current !== null && webCallActive === null) {
      const id = selectedCampaign.id;
      // t=4s catches fast calls; t=9s and t=16s catch slow Deepgram/WAV paths
      [4000, 9000, 16000].forEach(delay =>
        setTimeout(() => fetchCallLog(id), delay)
      );
    }
    prevWebCallActiveRef.current = webCallActive;
  }, [webCallActive]);

  // Pre-fill date/time to current time every time the modal opens for a lead.
  useEffect(() => {
    if (!scheduleLead) return;
    const d = new Date();
    const p = n => String(n).padStart(2, '0');
    setScheduleAt(`${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`);
    setScheduleNotes('');
    setScheduleError('');
  }, [scheduleLead]);

  const handleScheduleCall = async () => {
    if (!scheduleLead || !scheduleAt) return;
    // Reject times in the past or less than 1 minute from now
    if (new Date(scheduleAt) <= new Date(Date.now() - 60 * 1000)) {
      setScheduleError('Please select a future date and time.');
      return;
    }
    setScheduleSaving(true);
    setScheduleError('');
    try {
      // Convert browser-local datetime → UTC ISO so the backend scheduler
      // (which compares against UTC NOW()) fires at exactly the right moment.
      const utcTime = new Date(scheduleAt).toISOString();
      const res = await apiFetch(`${API_URL}/scheduled-calls`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ lead_id: scheduleLead.id, campaign_id: selectedCampaign.id, scheduled_time: utcTime })
      });
      if (res.ok) {
        setScheduleLead(null);
        setScheduleAt('');
        setScheduleNotes('');
      } else {
        const d = await res.json().catch(() => ({}));
        setScheduleError(d.detail || `Error ${res.status}`);
      }
    } catch(e) { setScheduleError('Network error — please try again.'); }
    setScheduleSaving(false);
  };

  // DND inline block messages: { [lead.id]: true } — auto-cleared after 4s
  const [dndBlocked, setDndBlocked] = useState({});
  const showDndBlock = (leadId) => {
    setDndBlocked(p => ({ ...p, [leadId]: true }));
    setTimeout(() => setDndBlocked(p => { const n = { ...p }; delete n[leadId]; return n; }), 4000);
  };

  // Dial wrappers that pre-check DND before proceeding
  const handleDialWithDndCheck = async (lead, campaignId) => {
    try {
      const res = await apiFetch(`${API_URL}/dnd/check/${encodeURIComponent(lead.phone)}`);
      if (res.ok) {
        const data = await res.json();
        if (data.is_dnd) { showDndBlock(lead.id); return; }
      }
    } catch (_) {}
    onCampaignDial(lead, campaignId);
  };

  const handleWebCallWithDndCheck = async (lead, campaignId) => {
    // If call is already active for this lead, let End Call through without DND check
    if (webCallActive === lead.id) { onCampaignWebCall(lead, campaignId); return; }
    try {
      const res = await apiFetch(`${API_URL}/dnd/check/${encodeURIComponent(lead.phone)}`);
      if (res.ok) {
        const data = await res.json();
        if (data.is_dnd) { showDndBlock(lead.id); return; }
      }
    } catch (_) {}
    onCampaignWebCall(lead, campaignId);
  };

  // Quick Add controlled state
  const [qaName, setQaName] = useState('');
  const [qaPhone, setQaPhone] = useState('');
  const [qaErrors, setQaErrors] = useState({});
  const [qaStatus, setQaStatus] = useState(null); // { type: 'success'|'error', message }
  const [qaLoading, setQaLoading] = useState(false);

  // Auto-dismiss success toast after 4 s
  useEffect(() => {
    if (qaStatus?.type === 'success') {
      const t = setTimeout(() => setQaStatus(null), 4000);
      return () => clearTimeout(t);
    }
  }, [qaStatus]);

  // Derive which lead phones are currently ringing from SSE event strings.
  // Requires a DIALING event to appear before RINGING so stale replayed events
  // from a previous call never flip the button to "Ringing..." prematurely.
  const ringingPhones = useMemo(() => {
    const phones = new Set();
    const dialed = new Set(); // phones that got a DIALING in this event window
    for (const ev of liveEvents) {
      const m = ev.match(/\((\d+)\)/);
      if (!m) continue;
      const phone = m[1];
      if (/— (DIALING|RETRY_DIALING)/.test(ev)) {
        dialed.add(phone);
        phones.delete(phone); // reset ringing on any new dial attempt
      } else if (ev.includes('— RINGING') && dialed.has(phone)) {
        phones.add(phone); // only accept RINGING if we saw the DIALING first
      } else if (/— (COMPLETED|NO-ANSWER|BUSY|FAILED|HANGUP|CONNECTED)/.test(ev)) {
        phones.delete(phone);
        dialed.delete(phone);
      }
    }
    return phones;
  }, [liveEvents]);

  const fetchInsights = async () => {
    setInsightsLoading(true);
    try {
      const [insightsRes, reviewsRes] = await Promise.all([
        apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/call-insights`),
        apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/call-reviews`),
      ]);
      if (insightsRes.ok) setCallInsights(await insightsRes.json());
      if (reviewsRes.ok) setCallReviews(await reviewsRes.json());
    } catch (e) { console.error('Failed to fetch insights', e); }
    setInsightsLoading(false);
  };

  useEffect(() => {
    if (detailTab === 'insights') fetchInsights();
    if (detailTab === 'retries') fetchRetries();
  }, [detailTab, selectedCampaign.id]);

  // Fetch billing usage for the widget
  useEffect(() => {
    const fetchBilling = async () => {
      try {
        const res = await apiFetch(`${API_URL}/billing/usage`);
        const data = await res.json();
        if (data && data.has_subscription) setBillingUsage(data);
      } catch (e) { /* no subscription — ignore */ }
    };
    fetchBilling();
  }, []);

  const fetchRetries = async () => {
    setRetriesLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/retries`);
      setRetries(await res.json());
    } catch (e) { console.error('Failed to fetch retries', e); }
    setRetriesLoading(false);
  };

  const scoreColor = (s) => {
    if (s >= 4) return '#22c55e';
    if (s >= 3) return '#f59e0b';
    if (s >= 2) return '#f97316';
    return '#ef4444';
  };

  const sentimentColor = (s) => {
    if (s === 'positive') return '#22c55e';
    if (s === 'neutral') return '#60a5fa';
    if (s === 'negative') return '#f97316';
    if (s === 'annoyed') return '#ef4444';
    return '#94a3b8';
  };

  // Build a map of transcript_id -> review for the call log badges
  const reviewByTranscript = {};
  callReviews.forEach(r => { reviewByTranscript[r.transcript_id] = r; });

  return (
    <div style={{padding: '1rem'}}>
      <button onClick={handleBack}
        style={{background: 'none', border: 'none', color: '#60a5fa', cursor: 'pointer', fontSize: '0.9rem', fontWeight: 600, marginBottom: '1rem', padding: 0}}>
        &larr; Back to Campaigns
      </button>

      <div style={{display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '1.5rem', flexWrap: 'wrap'}}>
        <h2 style={{margin: 0, color: '#e2e8f0'}}>{selectedCampaign.name}</h2>
        {selectedCampaign.product_id && (
          <span className="badge" style={{background: 'rgba(6,182,212,0.2)', color: '#22d3ee', fontSize: '0.75rem', padding: '2px 10px', borderRadius: '12px'}}>
            {getProductName(selectedCampaign.product_id)}
          </span>
        )}
        {statusBadge(selectedCampaign.status)}
        <button onClick={() => handleEditCampaign(selectedCampaign)}
          style={{background: 'rgba(250,204,21,0.1)', border: '1px solid rgba(250,204,21,0.3)',
            color: '#facc15', borderRadius: '6px', padding: '4px 12px', cursor: 'pointer', fontSize: '0.8rem', fontWeight: 600}}>
          Edit Campaign
        </button>
        <select className="form-input" value={selectedCampaign.lead_source || ''}
          onChange={async (e) => {
            const src = e.target.value;
            await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}`, {
              method: 'PUT', headers: {'Content-Type': 'application/json'},
              body: JSON.stringify({ lead_source: src })
            });
            setSelectedCampaign({...selectedCampaign, lead_source: src});
          }}
          style={{width: 'auto', height: '28px', fontSize: '0.75rem', padding: '2px 8px', background: 'rgba(0,0,0,0.3)', border: '1px solid rgba(255,255,255,0.1)', color: '#e2e8f0', borderRadius: '6px'}}>
          <option value="">No Source</option>
          <option value="facebook">Facebook / Meta</option>
          <option value="google">Google Ads</option>
          <option value="instagram">Instagram</option>
          <option value="linkedin">LinkedIn</option>
          <option value="website">Website</option>
          <option value="referral">Referral</option>
          <option value="cold">Cold Outreach</option>
        </select>
      </div>

      <div className="metrics-grid" style={{marginBottom: '1.5rem'}}>
        <div className="glass-panel metric-card"><div className="metric-label">Total Leads</div><div className="metric-value">{stats.total}</div></div>
        <div className="glass-panel metric-card"><div className="metric-label">Called</div><div className="metric-value">{stats.called}</div></div>
        <div className="glass-panel metric-card"><div className="metric-label">Qualified</div><div className="metric-value">{stats.qualified}</div></div>
        <div className="glass-panel metric-card"><div className="metric-label">Appointments</div><div className="metric-value">{stats.booked}</div></div>
      </div>

      {/* Voice Settings */}
      <div className="glass-panel" style={{marginBottom: '1.5rem', padding: '12px 16px'}}>
        <div style={{display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap'}}>
          <span style={{fontSize: '0.8rem', color: '#64748b', fontWeight: 600, whiteSpace: 'nowrap'}}>🔊 Campaign Voice Settings</span>
          <select className="form-input" value={campVoice.tts_provider}
            onChange={e => { const p = e.target.value; setCampVoice(v => ({...v, tts_provider: p, tts_voice_id: (INDIAN_VOICES[p] || [])[0]?.id || ''})); }}
            style={{width: 'auto', height: '32px', fontSize: '0.8rem', padding: '4px 8px', minWidth: '100px'}}>
            <option value="">-- Provider --</option>
            <option value="elevenlabs">ElevenLabs</option>
            <option value="sarvam">Sarvam AI</option>
            <option value="smallest">Smallest AI</option>
          </select>
          <select className="form-input" value={campVoice.tts_voice_id}
            onChange={e => setCampVoice(v => ({...v, tts_voice_id: e.target.value}))}
            style={{width: 'auto', height: '32px', fontSize: '0.8rem', padding: '4px 8px', minWidth: '160px'}}>
            <option value="">-- Voice --</option>
            {(() => {
              const recs = VOICE_RECOMMENDATIONS[campVoice.tts_language]?.[campVoice.tts_provider]?.top || [];
              const voices = INDIAN_VOICES[campVoice.tts_provider] || [];
              const recommended = voices.filter(v => recs.includes(v.id));
              const others = voices.filter(v => !recs.includes(v.id));
              return (<>
                {recommended.length > 0 && <optgroup label="★ Recommended">
                  {recommended.map(v => <option key={v.id} value={v.id}>★ {v.name}</option>)}
                </optgroup>}
                {recommended.length > 0 && <optgroup label="All Voices">
                  {others.map(v => <option key={v.id} value={v.id}>{v.name}</option>)}
                </optgroup>}
                {recommended.length === 0 && voices.map(v => <option key={v.id} value={v.id}>{v.name}</option>)}
              </>);
            })()}
          </select>
          <select className="form-input" value={campVoice.tts_language}
            onChange={e => setCampVoice(v => ({...v, tts_language: e.target.value}))}
            style={{width: 'auto', height: '32px', fontSize: '0.8rem', padding: '4px 8px', minWidth: '90px'}}>
            <option value="">-- Language --</option>
            {INDIAN_LANGUAGES.map(l => (
              <option key={l.code} value={l.code}>{l.name}</option>
            ))}
          </select>
          <button
            style={{
              background: voiceSaveStatus === 'saved' ? 'linear-gradient(135deg,#22c55e,#16a34a)' : voiceSaveStatus === 'error' ? 'linear-gradient(135deg,#ef4444,#b91c1c)' : 'linear-gradient(135deg,#8b5cf6,#6d28d9)',
              border: 'none', color: '#fff', fontSize: '0.75rem', padding: '6px 10px', borderRadius: '6px',
              cursor: voiceSaveStatus === 'saving' ? 'not-allowed' : 'pointer', whiteSpace: 'nowrap', minWidth: '60px',
              opacity: voiceSaveStatus === 'saving' ? 0.7 : 1, transition: 'all 0.2s'
            }}
            onClick={handleSaveCampVoice}
            disabled={voiceSaveStatus === 'saving'}>
            {voiceSaveStatus === 'saving' ? 'Saving…' : voiceSaveStatus === 'saved' ? 'Saved ✓' : voiceSaveStatus === 'error' ? 'Error ✗' : 'Save'}
          </button>
          <button style={{background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.1)', color: '#94a3b8', fontSize: '0.75rem', padding: '6px 10px', borderRadius: '6px', cursor: 'pointer', whiteSpace: 'nowrap'}}
            onClick={handleResetCampVoice}>Reset to Org Default</button>
        </div>
        <div style={{fontSize: '0.7rem', color: '#a78bfa', marginTop: '6px'}}>
          {campVoice.tts_provider
            ? `Current: ${campVoice.tts_provider === 'elevenlabs' ? 'ElevenLabs' : campVoice.tts_provider === 'sarvam' ? 'Sarvam AI' : 'Smallest AI'} - ${(INDIAN_VOICES[campVoice.tts_provider] || []).find(v => v.id === campVoice.tts_voice_id)?.name || campVoice.tts_voice_id || 'none'}`
            : 'Using org default'}
        </div>
        {VOICE_RECOMMENDATIONS[campVoice.tts_language]?.[campVoice.tts_provider]?.note && (
          <div style={{fontSize: '0.65rem', color: '#22d3ee', marginTop: '4px'}}>
            ℹ {VOICE_RECOMMENDATIONS[campVoice.tts_language][campVoice.tts_provider].note}
          </div>
        )}
      </div>

      {/* Billing Minutes Widget */}
      {billingUsage && (
        <div style={{
          display: 'inline-flex', alignItems: 'center', gap: '10px',
          background: 'rgba(99,102,241,0.08)', border: '1px solid rgba(99,102,241,0.2)',
          borderRadius: '20px', padding: '6px 16px', marginBottom: '1rem',
        }}>
          <span style={{fontSize: '0.8rem', color: '#e2e8f0', fontWeight: 600, whiteSpace: 'nowrap'}}>
            {'\u23F1'} {billingUsage.minutes_remaining} / {billingUsage.minutes_included} min remaining
          </span>
          <div style={{
            width: '80px', height: '6px', background: 'rgba(100,116,139,0.3)',
            borderRadius: '3px', overflow: 'hidden',
          }}>
            <div style={{
              width: `${Math.min(100, (billingUsage.minutes_used / billingUsage.minutes_included) * 100)}%`,
              height: '100%', borderRadius: '3px',
              background: (billingUsage.minutes_used / billingUsage.minutes_included) > 0.9
                ? '#ef4444' : (billingUsage.minutes_used / billingUsage.minutes_included) > 0.7
                ? '#f59e0b' : '#6366f1',
              transition: 'width 0.5s ease',
            }} />
          </div>
        </div>
      )}

      {/* Live Dial Events Feed — always visible */}
      <div className="glass-panel" style={{marginBottom: '1rem', padding: '12px', maxHeight: '200px', overflowY: 'auto'}}>
        <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px'}}>
          <span style={{fontSize: '0.8rem', color: '#94a3b8', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '1px'}}>📡 Live Campaign Activity</span>
          <button onClick={async () => {
            setLiveEvents([]);
            try { await apiFetch(`${API_URL}/campaign-events/clear?campaign_id=${selectedCampaign?.id || 0}`, { method: 'POST' }); } catch(e) {}
          }} style={{background: 'none', border: 'none', color: '#64748b', cursor: 'pointer', fontSize: '0.7rem'}}>Clear</button>
        </div>
        {liveEvents.length === 0 ? (
          <div style={{fontSize: '0.75rem', color: '#475569', fontStyle: 'italic', padding: '4px 0'}}>
            Waiting for campaign activity...
          </div>
        ) : (
          liveEvents.map((ev, i) => (
            <div key={i} style={{fontSize: '0.8rem', color: '#e2e8f0', padding: '3px 0', borderBottom: '1px solid rgba(255,255,255,0.03)', fontFamily: 'SFMono-Regular, Consolas, monospace'}}>
              {ev}
            </div>
          ))
        )}
      </div>

      {/* Quick Add Lead Form */}
      <div className="glass-panel" style={{padding: '12px', marginBottom: '1rem', display: 'flex', gap: '8px', alignItems: 'flex-start', flexWrap: 'wrap'}}>
        <span style={{fontSize: '0.8rem', color: '#94a3b8', fontWeight: 600, lineHeight: '32px'}}>➕ Quick Add:</span>

        <div style={{display: 'flex', flexDirection: 'column', gap: '2px'}}>
          <input className="form-input" placeholder="Name" value={qaName}
            onChange={e => {
              // Allow letters, digits and spaces only
              const val = e.target.value.replace(/[^a-zA-Z0-9 ]/g, '');
              setQaName(val);
              setQaErrors(p => ({...p, name: ''}));
            }}
            style={{width: '120px', height: '32px', fontSize: '0.8rem', padding: '4px 8px',
              borderColor: qaErrors.name ? '#ef4444' : undefined}} />
          {qaErrors.name && (
            <span style={{fontSize: '0.7rem', color: '#ef4444', lineHeight: 1.2}}>{qaErrors.name}</span>
          )}
        </div>

        {/* Phone: accepts 10-digit Indian OR E.164 (+country…) */}
        <div style={{display: 'flex', flexDirection: 'column', gap: '2px'}}>
          <input className="form-input" placeholder="Phone"
            value={qaPhone}
            maxLength={16}
            onChange={e => {
              let val = e.target.value;
              if (val.startsWith('+')) {
                // E.164 path: keep leading +, strip everything else that isn't a digit
                val = '+' + val.slice(1).replace(/\D/g, '');
                if (val.length > 16) val = val.slice(0, 16); // E.164 max = +15 digits
              } else {
                // Indian 10-digit path: digits only
                val = val.replace(/\D/g, '').slice(0, 10);
              }
              setQaPhone(val);
              setQaErrors(p => ({...p, phone: ''}));
            }}
            style={{width: '185px', height: '32px', fontSize: '0.8rem', padding: '4px 8px',
              borderColor: qaErrors.phone ? '#ef4444' : undefined}} />
          {qaErrors.phone
            ? <span style={{fontSize: '0.7rem', color: '#ef4444', lineHeight: 1.2}}>{qaErrors.phone}</span>
            : <span style={{fontSize: '0.65rem', color: '#64748b', lineHeight: 1.2}}>10 digits or +[code][number]</span>
          }
        </div>

        <button className="btn-primary" style={{height: '32px', fontSize: '0.8rem', padding: '4px 12px'}}
          disabled={qaLoading}
          onClick={async () => {
            const name = qaName.trim();
            const raw  = qaPhone.trim();

            // Normalise: bare 10 digits → +91, otherwise use as-is
            const fullPhone = /^\d{10}$/.test(raw) ? '+91' + raw : raw;

            // --- client-side validation (no API call on failure) ---
            const errors = {};
            if (!name) errors.name = 'Name is required';
            else if (!/[a-zA-Z]/.test(name)) errors.name = 'Name must contain at least one letter';
            if (!raw) {
              errors.phone = 'Phone is required';
            } else if (/^\d+$/.test(raw) && raw.length !== 10) {
              errors.phone = 'Indian numbers must be exactly 10 digits';
            } else if (raw.startsWith('+') && !/^\+[1-9]\d{7,14}$/.test(raw)) {
              errors.phone = 'E.164 format: +[country code][number] (8–15 digits total)';
            } else if (!/^\d{10}$/.test(raw) && !/^\+[1-9]\d{7,14}$/.test(raw)) {
              errors.phone = 'Enter 10 digits or E.164 format (e.g. +14155552671)';
            }
            // Reject invalid input before touching the backend
            if (Object.keys(errors).length) { setQaErrors(errors); return; }

            setQaLoading(true);
            setQaStatus(null);
            try {
              const res = await apiFetch(`${API_URL}/leads`, {
                method: 'POST', headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({ first_name: name, phone: fullPhone, source: 'Manual' })
              });
              const data = await res.json();
              let leadId = data.id;
              if (!leadId && data.message && data.message.includes('already exists')) {
                const searchRes = await apiFetch(`${API_URL}/leads/search?q=${encodeURIComponent(fullPhone)}`);
                const found = await searchRes.json();
                if (found.length > 0) leadId = found[0].id;
              }
              if (leadId) {
                await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/leads`, {
                  method: 'POST', headers: {'Content-Type': 'application/json'},
                  body: JSON.stringify({ lead_ids: [leadId] })
                });
                setQaName('');
                setQaPhone('');
                setQaErrors({});
                setQaStatus({ type: 'success', message: `${name} added!` });
                fetchCampaignLeads(selectedCampaign.id);
                fetchCampaigns();
              } else {
                setQaStatus({ type: 'error', message: data.message || 'Failed to add lead' });
              }
            } catch(e) {
              setQaStatus({ type: 'error', message: 'Network error — please try again' });
            } finally {
              setQaLoading(false);
            }
          }}>
          {qaLoading ? 'Adding...' : 'Add & Assign'}
        </button>

        {qaStatus && (
          <span style={{
            fontSize: '0.8rem', lineHeight: '32px', padding: '0 10px', borderRadius: '6px',
            color: qaStatus.type === 'success' ? '#22c55e' : '#ef4444',
            background: qaStatus.type === 'success' ? 'rgba(34,197,94,0.1)' : 'rgba(239,68,68,0.1)',
            border: `1px solid ${qaStatus.type === 'success' ? 'rgba(34,197,94,0.3)' : 'rgba(239,68,68,0.3)'}`,
          }}>
            {qaStatus.type === 'success' ? '✓' : '⚠'} {qaStatus.message}
          </span>
        )}
      </div>

      <div style={{display: 'flex', gap: '10px', marginBottom: confirmDialAction ? '0.5rem' : '1rem', flexWrap: 'wrap'}}>
        <button className="btn-primary" onClick={() => { setSelectedLeadIds([]); setShowAddLeadsModal(true); }}>+ Add from CRM</button>
        <button className="btn-primary" style={{background: 'linear-gradient(135deg, #22d3ee, #06b6d4)'}}
          onClick={() => { setCsvFile(null); setShowCsvImportModal(true); }}>📤 Import CSV</button>
        <a href={`${API_URL}/leads/sample-csv`} download style={{color: '#94a3b8', fontSize: '0.8rem', textDecoration: 'underline', alignSelf: 'center'}}>📋 Sample CSV</a>
        {campaignLeads.some(l => (l.status || '').startsWith('Call Failed')) && (
          <button className="btn-call" style={{background: 'rgba(245,158,11,0.15)', color: '#f59e0b', borderColor: 'rgba(245,158,11,0.3)', fontSize: '0.85rem', padding: '8px 16px'}}
            onClick={() => {
              const count = campaignLeads.filter(l => (l.status || '').startsWith('Call Failed')).length;
              setConfirmDialAction({ type: 'redial', label: `Redial ${count} failed lead${count !== 1 ? 's' : ''}`, count });
            }}>
            🔄 Redial Failed ({campaignLeads.filter(l => (l.status || '').startsWith('Call Failed')).length})
          </button>
        )}
        {campaignLeads.some(l => (l.status || '').toLowerCase() === 'new') && (
          <button className="btn-primary" style={{background: 'linear-gradient(135deg, #22c55e, #16a34a)', fontSize: '0.85rem', padding: '8px 16px'}}
            onClick={() => {
              const count = campaignLeads.filter(l => (l.status || '').toLowerCase() === 'new').length;
              setConfirmDialAction({ type: 'new', label: `Dial all ${count} new lead${count !== 1 ? 's' : ''}`, count });
            }}>
            📞 Dial All New ({campaignLeads.filter(l => (l.status || '').toLowerCase() === 'new').length})
          </button>
        )}
        <button className="btn-primary" style={{background: 'linear-gradient(135deg, #8b5cf6, #6d28d9)', fontSize: '0.85rem', padding: '8px 16px'}}
          onClick={() => setConfirmDialAction({ type: 'all', label: `Dial all ${campaignLeads.length} lead${campaignLeads.length !== 1 ? 's' : ''}`, count: campaignLeads.length })}>
          📞 Dial All ({campaignLeads.length})
        </button>
      </div>

      {confirmDialAction && (
        <div style={{display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '1rem', padding: '10px 14px',
          background: 'rgba(245,158,11,0.08)', border: '1px solid rgba(245,158,11,0.25)', borderRadius: '8px', flexWrap: 'wrap'}}>
          <span style={{fontSize: '0.85rem', color: '#fbbf24', flex: 1}}>
            ⚠ {confirmDialAction.label}? Calls are placed with a 30s gap to avoid spam.
          </span>
          <button
            style={{background: 'rgba(34,197,94,0.15)', border: '1px solid rgba(34,197,94,0.3)', color: '#4ade80',
              padding: '5px 14px', borderRadius: '6px', cursor: 'pointer', fontSize: '0.82rem', fontWeight: 600}}
            onClick={async () => {
              const action = confirmDialAction;
              setConfirmDialAction(null);
              try {
                const url = action.type === 'redial'
                  ? `${API_URL}/campaigns/${selectedCampaign.id}/redial-failed`
                  : action.type === 'new'
                    ? `${API_URL}/campaigns/${selectedCampaign.id}/dial-all`
                    : `${API_URL}/campaigns/${selectedCampaign.id}/dial-all?force=true`;
                await apiFetch(url, { method: 'POST' });
                const ri = setInterval(() => { fetchCampaignLeads(selectedCampaign.id); fetchCallLog(selectedCampaign.id); }, 15000);
                setTimeout(() => clearInterval(ri), 30 * 60 * 1000);
              } catch(e) { console.error('Dial action failed', e); }
            }}>
            Yes, proceed
          </button>
          <button
            style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', color: '#94a3b8',
              padding: '5px 14px', borderRadius: '6px', cursor: 'pointer', fontSize: '0.82rem'}}
            onClick={() => setConfirmDialAction(null)}>
            Cancel
          </button>
        </div>
      )}

      {/* Tab Switcher: Leads | Call Log */}
      <div style={{display: 'flex', gap: '0', marginBottom: '1rem', borderRadius: '8px', overflow: 'hidden', border: '1px solid rgba(255,255,255,0.1)', width: 'fit-content'}}>
        <button onClick={() => setDetailTab('leads')}
          style={{padding: '8px 20px', border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: '0.85rem',
            background: detailTab === 'leads' ? 'rgba(99,102,241,0.2)' : 'transparent',
            color: detailTab === 'leads' ? '#818cf8' : '#64748b'}}>
          👥 Leads ({campaignLeads.length})
        </button>
        <button onClick={() => { setDetailTab('calllog'); fetchCallLog(selectedCampaign.id); fetchInsights(); }}
          style={{padding: '8px 20px', border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: '0.85rem',
            background: detailTab === 'calllog' ? 'rgba(34,197,94,0.2)' : 'transparent',
            color: detailTab === 'calllog' ? '#22c55e' : '#64748b'}}>
          📞 Call Log ({callLog.length})
        </button>
        <button onClick={() => setDetailTab('insights')}
          style={{padding: '8px 20px', border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: '0.85rem',
            background: detailTab === 'insights' ? 'rgba(168,85,247,0.2)' : 'transparent',
            color: detailTab === 'insights' ? '#a855f7' : '#64748b'}}>
          📊 Call Insights
        </button>
        <button onClick={() => setDetailTab('retries')}
          style={{padding: '8px 20px', border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: '0.85rem',
            background: detailTab === 'retries' ? 'rgba(245,158,11,0.2)' : 'transparent',
            color: detailTab === 'retries' ? '#f59e0b' : '#64748b'}}>
          🔄 Retries
        </button>
      </div>

      {/* Call Log Table */}
      {detailTab === 'calllog' && (
        <div className="glass-panel" style={{overflowX: 'auto', marginBottom: '1.5rem'}}>
          <table className="leads-table" style={{width: '100%'}}>
            <thead>
              <tr>
                <th>Lead</th>
                <th>Phone</th>
                <th>Source</th>
                <th>Time</th>
                <th>Outcome</th>
                <th>Quality</th>
                <th>Duration</th>
                <th>Recording</th>
              </tr>
            </thead>
            <tbody>
              {callLog.length === 0 ? (
                <tr><td colSpan="8" style={{textAlign: 'center', color: '#64748b', padding: '2rem'}}>No calls made yet.</td></tr>
              ) : callLog.map(call => {
                const review = reviewByTranscript[call.id];
                const outcomeColors = {
                  'Completed': '#22c55e', 'Connected': '#60a5fa', 'No Answer': '#f59e0b',
                  'Busy': '#f97316', 'Failed': '#ef4444', 'DND Blocked': '#dc2626'
                };
                const outcomeBg = {
                  'Completed': 'rgba(34,197,94,0.1)', 'Connected': 'rgba(96,165,250,0.1)', 'No Answer': 'rgba(245,158,11,0.1)',
                  'Busy': 'rgba(249,115,22,0.1)', 'Failed': 'rgba(239,68,68,0.1)', 'DND Blocked': 'rgba(220,38,38,0.1)'
                };
                return (
                  <tr key={call.id}>
                    <td style={{fontWeight: 600}}>{call.first_name} {call.last_name || ''}</td>
                    <td style={{fontFamily: 'SFMono-Regular, Consolas, monospace', color: '#cbd5e1', fontSize: '0.85rem'}}>{call.phone}</td>
                    <td><span className="badge" style={{background: call.call_source === 'exotel' ? 'rgba(99,102,241,0.15)' : call.call_source === 'sim_web_call' ? 'rgba(34,197,94,0.15)' : undefined}}>{call.call_source === 'sim_web_call' ? '🌐 Web Call' : call.call_source === 'exotel' ? '📞 Exotel' : call.source || '-'}</span></td>
                    <td style={{fontSize: '0.8rem', color: '#94a3b8'}}>{formatDateTime(call.created_at, orgTimezone)}</td>
                    <td>
                      <span style={{
                        padding: '3px 10px', borderRadius: '12px', fontSize: '0.75rem', fontWeight: 600,
                        color: outcomeColors[call.outcome] || '#94a3b8',
                        background: outcomeBg[call.outcome] || 'rgba(148,163,184,0.1)',
                        border: `1px solid ${outcomeColors[call.outcome] || '#94a3b8'}30`
                      }}>
                        {call.outcome === 'Completed' && '✅ '}
                        {call.outcome === 'Connected' && '📞 '}
                        {call.outcome === 'No Answer' && '❌ '}
                        {call.outcome === 'Busy' && '📵 '}
                        {call.outcome === 'Failed' && '⚠️ '}
                        {call.outcome === 'DND Blocked' && '🚫 '}
                        {call.outcome}
                      </span>
                    </td>
                    <td>
                      {review ? (
                        <span style={{
                          padding: '2px 8px', borderRadius: '10px', fontSize: '0.75rem', fontWeight: 700,
                          color: scoreColor(review.quality_score),
                          background: `${scoreColor(review.quality_score)}18`,
                          border: `1px solid ${scoreColor(review.quality_score)}40`
                        }}>
                          {'★'.repeat(review.quality_score)}{'☆'.repeat(5 - review.quality_score)}
                        </span>
                      ) : (
                        <span style={{color: '#64748b', fontSize: '0.75rem'}}>--</span>
                      )}
                    </td>
                    <td style={{fontSize: '0.85rem', color: call.call_duration_s > 0 ? '#e2e8f0' : '#64748b'}}>
                      {call.call_duration_s > 0 ? `${Math.floor(call.call_duration_s / 60)}:${String(Math.floor(call.call_duration_s % 60)).padStart(2, '0')}` : '-'}
                    </td>
                    <td>
                      {call.recording_url ? (
                        <AuthAudioPlayer src={call.recording_url} style={{height: '28px', width: '150px'}} />
                      ) : (
                        <span style={{color: '#64748b', fontSize: '0.8rem'}}>—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Call Insights Tab */}
      {detailTab === 'insights' && (
        <div style={{marginBottom: '1.5rem'}}>
          {insightsLoading ? (
            <div className="glass-panel" style={{padding: '2rem', textAlign: 'center', color: '#94a3b8'}}>Loading insights...</div>
          ) : !callInsights || !callInsights.total_reviews ? (
            <div className="glass-panel" style={{padding: '2rem', textAlign: 'center', color: '#64748b'}}>No call reviews yet. Reviews are generated automatically after each call.</div>
          ) : (
            <>
              {/* Summary Cards */}
              <div className="metrics-grid" style={{marginBottom: '1.5rem'}}>
                <div className="glass-panel metric-card">
                  <div className="metric-label">Avg Quality Score</div>
                  <div className="metric-value" style={{color: scoreColor(Math.round(callInsights.avg_quality_score))}}>
                    {callInsights.avg_quality_score}/5
                  </div>
                </div>
                <div className="glass-panel metric-card">
                  <div className="metric-label">Appointment Rate</div>
                  <div className="metric-value" style={{color: callInsights.appointment_rate > 30 ? '#22c55e' : '#f59e0b'}}>
                    {callInsights.appointment_rate}%
                  </div>
                </div>
                <div className="glass-panel metric-card">
                  <div className="metric-label">Calls Analyzed</div>
                  <div className="metric-value">{callInsights.total_reviews}</div>
                </div>
                <div className="glass-panel metric-card">
                  <div className="metric-label">Top Sentiment</div>
                  <div className="metric-value" style={{fontSize: '1.1rem', color: sentimentColor(
                    Object.entries(callInsights.sentiment_breakdown || {}).sort((a, b) => b[1] - a[1])[0]?.[0]
                  )}}>
                    {Object.entries(callInsights.sentiment_breakdown || {}).sort((a, b) => b[1] - a[1])[0]?.[0] || '-'}
                  </div>
                </div>
              </div>

              {/* Improvement Suggestions */}
              {callInsights.top_improvements && callInsights.top_improvements.length > 0 && (
                <div className="glass-panel" style={{padding: '16px', marginBottom: '1.5rem'}}>
                  <div style={{fontSize: '0.85rem', color: '#a855f7', fontWeight: 700, marginBottom: '10px', textTransform: 'uppercase', letterSpacing: '0.5px'}}>
                    Prompt Improvement Suggestions
                  </div>
                  {callInsights.top_improvements.map((imp, i) => (
                    <div key={i} style={{padding: '8px 12px', marginBottom: '6px', background: 'rgba(168,85,247,0.06)', borderRadius: '6px', borderLeft: '3px solid #a855f7', fontSize: '0.85rem', color: '#e2e8f0'}}>
                      {imp.suggestion}
                      <span style={{color: '#64748b', fontSize: '0.75rem', marginLeft: '8px'}}>({imp.count}x)</span>
                    </div>
                  ))}
                </div>
              )}

              {/* Top Failure Reasons */}
              {callInsights.top_failure_reasons && callInsights.top_failure_reasons.length > 0 && (
                <div className="glass-panel" style={{padding: '16px', marginBottom: '1.5rem'}}>
                  <div style={{fontSize: '0.85rem', color: '#f97316', fontWeight: 700, marginBottom: '10px', textTransform: 'uppercase', letterSpacing: '0.5px'}}>
                    Top Failure Reasons
                  </div>
                  {callInsights.top_failure_reasons.map((fr, i) => (
                    <div key={i} style={{padding: '8px 12px', marginBottom: '6px', background: 'rgba(249,115,22,0.06)', borderRadius: '6px', borderLeft: '3px solid #f97316', fontSize: '0.85rem', color: '#e2e8f0'}}>
                      {fr.reason}
                      <span style={{color: '#64748b', fontSize: '0.75rem', marginLeft: '8px'}}>({fr.count}x)</span>
                    </div>
                  ))}
                </div>
              )}

              {/* Per-Call Reviews Table */}
              <div className="glass-panel" style={{overflowX: 'auto'}}>
                <table className="leads-table" style={{width: '100%'}}>
                  <thead>
                    <tr>
                      <th>Lead</th>
                      <th>Quality</th>
                      <th>Appt Booked</th>
                      <th>Date / Time</th>
                      <th>Sentiment</th>
                      <th>What Went Well</th>
                      <th>What Went Wrong</th>
                      <th>Failure Reason</th>
                    </tr>
                  </thead>
                  <tbody>
                    {callReviews.map(r => (
                      <tr key={r.id}>
                        <td style={{fontWeight: 600}}>{r.first_name} {r.last_name || ''}</td>
                        <td>
                          <span style={{fontWeight: 700, color: scoreColor(r.quality_score), fontSize: '0.9rem'}}>
                            {'★'.repeat(r.quality_score)}{'☆'.repeat(5 - r.quality_score)}
                          </span>
                        </td>
                        <td>
                          <span style={{
                            padding: '2px 10px', borderRadius: '12px', fontSize: '0.75rem', fontWeight: 600,
                            color: r.appointment_booked ? '#22c55e' : '#f97316',
                            background: r.appointment_booked ? 'rgba(34,197,94,0.1)' : 'rgba(249,115,22,0.1)',
                          }}>
                            {r.appointment_booked ? 'Yes' : 'No'}
                          </span>
                        </td>
                        <td style={{fontSize: '0.8rem', color: '#94a3b8', whiteSpace: 'nowrap'}}>{r.created_at ? formatDateTime(r.created_at, orgTimezone) : '-'}</td>
                        <td>
                          <span style={{color: sentimentColor(r.customer_sentiment), fontWeight: 600, fontSize: '0.85rem'}}>
                            {r.customer_sentiment}
                          </span>
                        </td>
                        <td style={{fontSize: '0.8rem', color: '#94a3b8', maxWidth: '200px'}}>{r.what_went_well || '-'}</td>
                        <td style={{fontSize: '0.8rem', color: '#f87171', maxWidth: '200px'}}>{r.what_went_wrong || '-'}</td>
                        <td style={{fontSize: '0.8rem', color: '#94a3b8', maxWidth: '200px'}}>{r.failure_reason || '-'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      )}

      {/* Retries Tab */}
      {detailTab === 'retries' && (
        <div style={{marginBottom: '1.5rem'}}>
          {retriesLoading ? (
            <div className="glass-panel" style={{padding: '2rem', textAlign: 'center', color: '#94a3b8'}}>Loading retry queue...</div>
          ) : retries.length === 0 ? (
            <div className="glass-panel" style={{padding: '2rem', textAlign: 'center', color: '#64748b'}}>No retries queued for this campaign.</div>
          ) : (
            <div className="glass-panel" style={{overflowX: 'auto'}}>
              <table className="leads-table" style={{width: '100%'}}>
                <thead>
                  <tr>
                    <th>Lead</th>
                    <th>Phone</th>
                    <th>Attempt</th>
                    <th>Retry Time</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {retries.map(r => {
                    const retryStatusColors = {
                      pending: { color: '#f59e0b', bg: 'rgba(245,158,11,0.1)', border: 'rgba(245,158,11,0.3)' },
                      dialing: { color: '#60a5fa', bg: 'rgba(96,165,250,0.1)', border: 'rgba(96,165,250,0.3)' },
                      completed: { color: '#22c55e', bg: 'rgba(34,197,94,0.1)', border: 'rgba(34,197,94,0.3)' },
                      exhausted: { color: '#ef4444', bg: 'rgba(239,68,68,0.1)', border: 'rgba(239,68,68,0.3)' },
                    };
                    const sc = retryStatusColors[r.status] || retryStatusColors.pending;
                    return (
                      <tr key={r.id}>
                        <td style={{fontWeight: 600}}>{r.first_name || r.lead_name || '-'} {r.last_name || ''}</td>
                        <td style={{fontFamily: 'SFMono-Regular, Consolas, monospace', color: '#cbd5e1', fontSize: '0.85rem'}}>{r.phone}</td>
                        <td style={{fontSize: '0.85rem', color: '#e2e8f0', fontWeight: 600}}>{r.attempt || r.attempt_number || 1}/{r.max_attempts || 3}</td>
                        <td style={{fontSize: '0.8rem', color: '#94a3b8'}}>{r.retry_time ? formatDateTime(r.retry_time, orgTimezone) : '-'}</td>
                        <td>
                          <span style={{
                            padding: '3px 10px', borderRadius: '12px', fontSize: '0.75rem', fontWeight: 600,
                            color: sc.color, background: sc.bg, border: `1px solid ${sc.border}`,
                          }}>
                            {r.status}
                          </span>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Leads Table */}
      {detailTab === 'leads' && (
      <div className="glass-panel" style={{overflowX: 'auto'}}>
        <table className="leads-table" style={{width: '100%'}}>
          <thead>
            <tr>
              <th>Name</th><th>Phone</th><th>Source</th><th>Status</th><th>Action</th>
            </tr>
          </thead>
          <tbody>
            {campaignLeads.length === 0 ? (
              <tr><td colSpan="5" style={{textAlign: 'center', color: '#64748b', padding: '2rem'}}>No leads in this campaign yet. Add some to start dialing!</td></tr>
            ) : campaignLeads.map(lead => (
              <React.Fragment key={lead.id}>
              <tr>
                <td style={{fontWeight: 600}}>{lead.first_name} {lead.last_name}</td>
                <td>{lead.phone}</td>
                <td>{lead.source || '-'}</td>
                <td>
                  <select className="form-input" value={lead.status || 'New'}
                    onChange={e => handleLeadStatusChange(lead.id, e.target.value)}
                    style={{width: 'auto', height: '30px', fontSize: '0.75rem', padding: '2px 6px'}}>
                    {['New','Contacted','Qualified','Appointment Set','Converted','Lost'].map(s => <option key={s} value={s}>{s}</option>)}
                  </select>
                </td>
                <td>
                  <div style={{display: 'flex', gap: '6px', flexWrap: 'wrap'}}>
                    <button className="btn-call"
                      onClick={() => handleEditLead(lead)}
                      style={{fontSize: '0.75rem', padding: '4px 10px', cursor: 'pointer', background: 'rgba(250,204,21,0.15)', color: '#facc15', borderColor: 'rgba(250,204,21,0.3)'}}>
                      ✏️ Edit
                    </button>
                    <button className="btn-call"
                      onClick={() => handleDialWithDndCheck(lead, selectedCampaign.id)}
                      disabled={dialingId === lead.id}
                      style={{fontSize: '0.75rem', padding: '4px 10px', cursor: dialingId === lead.id ? 'not-allowed' : 'pointer', opacity: dialingId === lead.id ? 0.5 : 1}}>
                      {dialingId === lead.id
                        ? (ringingPhones.has(lead.phone) ? '🔔 Ringing...' : '📞 Dialing...')
                        : '📞 Dial'}
                    </button>
                    <button className="btn-call"
                      onClick={() => handleWebCallWithDndCheck(lead, selectedCampaign.id)}
                      disabled={dialingId === lead.id && webCallActive !== lead.id}
                      style={{
                        fontSize: '0.75rem', padding: '4px 10px', cursor: (dialingId === lead.id && webCallActive !== lead.id) ? 'not-allowed' : 'pointer',
                        opacity: (dialingId === lead.id && webCallActive !== lead.id) ? 0.5 : 1,
                        borderColor: webCallActive === lead.id ? '#ef4444' : '#8b5cf6',
                        color: webCallActive === lead.id ? '#ef4444' : '#8b5cf6',
                        background: webCallActive === lead.id ? 'rgba(239,68,68,0.1)' : 'rgba(139,92,246,0.1)'
                      }}>
                      {webCallActive === lead.id ? '🔴 End Call' : '🌐 Sim Web Call'}
                    </button>
                    {dndBlocked[lead.id] && (
                      <span style={{
                        display: 'inline-flex', alignItems: 'center', gap: '4px',
                        fontSize: '0.72rem', fontWeight: 600, padding: '3px 10px',
                        borderRadius: '6px', background: 'rgba(220,38,38,0.15)',
                        border: '1px solid rgba(220,38,38,0.4)', color: '#fca5a5',
                        whiteSpace: 'nowrap'
                      }}>
                        🚫 DND — number blocked
                      </span>
                    )}
                    <button className="btn-call"
                      onClick={() => handleViewTranscripts(lead)}
                      style={{fontSize: '0.75rem', padding: '4px 10px', cursor: 'pointer',
                        background: lead.transcript_count > 0 ? 'rgba(34,197,94,0.15)' : 'rgba(99,102,241,0.08)',
                        color: lead.transcript_count > 0 ? '#22c55e' : '#64748b',
                        borderColor: lead.transcript_count > 0 ? 'rgba(34,197,94,0.3)' : 'rgba(99,102,241,0.15)',
                        fontWeight: lead.transcript_count > 0 ? 600 : 400}}>
                      {lead.transcript_count > 0 ? `📋 ${lead.transcript_count} Transcript${lead.transcript_count > 1 ? 's' : ''}` : '📋 No Calls'}
                      {lead.recording_count > 0 && ' 🔊'}
                      {lead.dial_attempts > 0 && ` (${lead.dial_attempts} dial${lead.dial_attempts > 1 ? 's' : ''})`}
                    </button>
                    <button className="btn-call"
                      onClick={() => handleNote(lead)}
                      style={{fontSize: '0.75rem', padding: '4px 10px', cursor: 'pointer', background: 'rgba(168,85,247,0.15)', color: '#a855f7', borderColor: 'rgba(168,85,247,0.3)'}}>
                      📝 Note
                    </button>
                    <button className="btn-call"
                      onClick={() => {
                        const d = new Date();
                        const pad = n => String(n).padStart(2, '0');
                        setScheduleAt(`${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`);
                        setScheduleNotes('');
                        setScheduleError('');
                        setScheduleLead(lead);
                      }}
                      style={{fontSize: '0.75rem', padding: '4px 10px', cursor: 'pointer', background: 'rgba(96,165,250,0.15)', color: '#60a5fa', borderColor: 'rgba(96,165,250,0.3)'}}>
                      📅 Schedule
                    </button>
                    {confirmRemoveLeadId === lead.id ? (
                      <span style={{display: 'flex', gap: '6px', alignItems: 'center'}}>
                        <span style={{fontSize: '0.75rem', color: '#fca5a5'}}>Remove?</span>
                        <button onClick={() => { handleRemoveLead(lead.id); setConfirmRemoveLeadId(null); }}
                          style={{background: '#ef4444', border: 'none', color: '#fff', borderRadius: '5px', padding: '3px 10px', cursor: 'pointer', fontSize: '0.72rem', fontWeight: 600}}>Yes</button>
                        <button onClick={() => setConfirmRemoveLeadId(null)}
                          style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.15)', color: '#94a3b8', borderRadius: '5px', padding: '3px 10px', cursor: 'pointer', fontSize: '0.72rem'}}>No</button>
                      </span>
                    ) : (
                      <button onClick={() => setConfirmRemoveLeadId(lead.id)}
                        style={{fontSize: '0.75rem', padding: '4px 10px', cursor: 'pointer',
                          background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
                          color: '#fca5a5', borderRadius: '6px'}}>
                        Remove
                      </button>
                    )}
                  </div>
                </td>
              </tr>
              {lead.follow_up_note && (
                <tr>
                  <td colSpan="5" style={{padding: '12px 24px', background: 'rgba(0,0,0,0.2)', borderLeft: '3px solid #6366f1'}}>
                    <div style={{fontSize: '0.8rem', color: '#94a3b8', marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '1px', fontWeight: 600}}>AI Follow-Up Note</div>
                    <div style={{whiteSpace: 'pre-wrap', color: '#e2e8f0', fontSize: '0.85rem', lineHeight: 1.5}}>{lead.follow_up_note}</div>
                  </td>
                </tr>
              )}
              </React.Fragment>
            ))}
          </tbody>
        </table>
      </div>
      )}

      {/* Schedule Call Modal */}
      {scheduleLead && (
        <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setScheduleLead(null); }}>
          <div className="glass-panel modal-content" style={{maxWidth: '440px', padding: '1.5rem'}}>
            <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.75rem'}}>
              <h3 style={{margin: 0, color: '#e2e8f0'}}>📅 Schedule Call</h3>
              <button onClick={() => setScheduleLead(null)}
                style={{background: 'transparent', border: 'none', color: '#94a3b8', fontSize: '1.2rem', cursor: 'pointer'}}>✕</button>
            </div>
            <p style={{color: '#94a3b8', fontSize: '0.85rem', marginBottom: '1.25rem'}}>
              {scheduleLead.first_name} {scheduleLead.last_name} — {scheduleLead.phone}
            </p>
            {scheduleError && (
              <div style={{background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', color: '#fca5a5',
                borderRadius: '8px', padding: '8px 12px', marginBottom: '1rem', fontSize: '0.82rem'}}>
                ⚠️ {scheduleError}
              </div>
            )}
            <div style={{display: 'flex', flexDirection: 'column', gap: '0.75rem'}}>
              {/* Date + Time split so each picker closes on selection and "Now" replaces the broken "Today" */}
              <div>
                <div style={{fontSize: '0.8rem', color: '#cbd5e1', fontWeight: 600, marginBottom: '6px'}}>
                  Date &amp; Time
                </div>
                <div style={{display: 'flex', gap: '8px', alignItems: 'center'}}>
                  <input
                    type="date"
                    className="form-input"
                    value={scheduleAt.split('T')[0] || ''}
                    min={(() => { const d = new Date(); const p = n => String(n).padStart(2,'0'); return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}`; })()}
                    onChange={e => {
                      const t = scheduleAt.split('T')[1] || '00:00';
                      setScheduleAt(`${e.target.value}T${t}`);
                      setScheduleError('');
                    }}
                    style={{flex: 1, colorScheme: 'dark'}}
                  />
                  <input
                    type="time"
                    className="form-input"
                    value={scheduleAt.split('T')[1] || ''}
                    onChange={e => {
                      const date = scheduleAt.split('T')[0];
                      if (!date) return; // date must be selected first
                      setScheduleAt(`${date}T${e.target.value}`);
                      setScheduleError('');
                    }}
                    style={{width: '150px', colorScheme: 'dark'}}
                  />
                </div>
              </div>
              <label style={{fontSize: '0.8rem', color: '#cbd5e1', fontWeight: 600}}>
                Notes (optional)
                <textarea
                  className="form-input"
                  value={scheduleNotes}
                  onChange={e => setScheduleNotes(e.target.value)}
                  rows={2}
                  placeholder="e.g. follow-up on pricing discussion"
                  style={{width: '100%', marginTop: '6px', resize: 'vertical'}}
                />
              </label>
            </div>
            <div style={{display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '1.25rem'}}>
              <button onClick={() => setScheduleLead(null)}
                style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', color: '#cbd5e1', padding: '8px 18px', borderRadius: '8px', cursor: 'pointer'}}>
                Cancel
              </button>
              <button className="btn-primary" onClick={handleScheduleCall}
                disabled={scheduleSaving || !scheduleAt}
                style={{opacity: (scheduleSaving || !scheduleAt) ? 0.5 : 1, cursor: (scheduleSaving || !scheduleAt) ? 'not-allowed' : 'pointer'}}>
                {scheduleSaving ? 'Scheduling…' : 'Schedule Call'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
