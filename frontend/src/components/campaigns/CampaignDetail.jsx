import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { formatDateTime } from '../../utils/dateFormat';
import { VOICE_RECOMMENDATIONS } from '../../constants/voices';
import AuthAudio from '../AuthAudio';
import { useToast, useConfirm } from '../../contexts/UIContext';
import { useHideAiFeatures } from '../../hooks/useHideAiFeatures';
import { useCall } from '../../contexts/CallContext';
// import TwilioBrowserCallModal from './TwilioBrowserCallModal';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', pink: '#ec4899', green: '#10b981',
  amber: '#f59e0b', red: '#ef4444', wa: '#25D366',
  text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.04)',
};

function withDate(label, tsMs) {
  label = String(label || '');
  const d = new Date(tsMs || Date.now());
  const dd = String(d.getDate()).padStart(2, '0');
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const yyyy = d.getFullYear();
  const dateStr = `${dd}/${mm}/${yyyy}`;
  if (/\[\d{2}:\d{2}:\d{2}\]/.test(label)) {
    return label.replace(/\[(\d{2}:\d{2}:\d{2})\]/, `[${dateStr} $1]`);
  }
  return `[${dateStr}] ${label}`;
}

function linkify(text) {
  if (!text) return text;
  const parts = text.split(/(https?:\/\/[^\s]+)/g);
  return parts.map((p, i) =>
    /^https?:\/\//.test(p)
      ? <a key={i} href={p} target="_blank" rel="noreferrer"
          style={{ color: '#6366f1', textDecoration: 'underline', wordBreak: 'break-all' }}
          onClick={e => e.stopPropagation()}>{p}</a>
      : p
  );
}

// ── WhatsApp Blast Panel ──────────────────────────────────────────────────────
function WhatsAppBlastPanel({ campaignId, apiFetch, API_URL }) {
  const [blasting, setBlasting] = useState(false);
  const [job, setJob] = useState(null);
  const [error, setError] = useState('');
  const pollRef = useRef(null);

  const stopPoll = () => { if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null; } };

  const pollStatus = (jobId) => {
    stopPoll();
    pollRef.current = setInterval(async () => {
      try {
        const res = await apiFetch(`${API_URL}/wa/campaign-blast/status/${jobId}`);
        const data = await res.json();
        setJob(data);
        if (data.status !== 'running') stopPoll();
      } catch { stopPoll();  }
    }, 2000);
  };

  useEffect(() => () => stopPoll(), []);

  const handleBlast = async () => {
    setError('');
    setBlasting(true);
    setJob(null);
    try {
      const res = await apiFetch(`${API_URL}/wa/campaign-blast/${campaignId}`, { method: 'POST' });
      const data = await res.json();
      if (!res.ok) { setError(data.error || 'Blast failed'); setBlasting(false); return; }
      if (data.sent !== undefined && data.total === 0) {
        setJob({ status: 'done', total: 0, sent: 0, failed: 0, errors: [] });
        setBlasting(false);
        return;
      }
      setJob({ status: 'running', total: data.total, sent: 0, failed: 0, errors: [] });
      pollStatus(data.job_id);
    } catch { setError('Network error');  }
    setBlasting(false);
  };

  const isRunning = job?.status === 'running';
  const isDone = job?.status === 'done';
  const progress = job ? Math.round(((job.sent + job.failed) / Math.max(job.total, 1)) * 100) : 0;

  return (
    <div style={{ marginBottom: '1rem' }}>
      {error && (
        <div style={{ background: '#fee2e2', border: `1px solid #fca5a5`, color: T.red, borderRadius: 8, padding: '10px 14px', marginBottom: 10, fontSize: '0.85rem' }}>
          ⚠️ {error}
        </div>
      )}
      {!isRunning && !isDone && (
        <button
          style={{ background: `linear-gradient(135deg, ${T.wa}, #128C7E)`, border: 'none', color: '#fff', fontSize: '0.85rem', padding: '8px 18px', borderRadius: 8, cursor: 'pointer', fontWeight: 600, fontFamily: T.font }}
          disabled={blasting}
          onClick={handleBlast}>
          {blasting ? 'Starting...' : '💬 Send to New Leads'}
        </button>
      )}
      {(isRunning || isDone) && (
        <div style={{ ...card, padding: '12px 16px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8, fontSize: '0.85rem', color: T.sub }}>
            <span>{isRunning ? '⏳ Sending...' : '✅ Blast complete'}</span>
            <span style={{ color: T.muted }}>{job.sent} sent · {job.failed} failed · {job.total} total</span>
          </div>
          <div style={{ background: T.border, borderRadius: 4, height: 6, overflow: 'hidden' }}>
            <div style={{ width: `${progress}%`, height: '100%', background: `linear-gradient(90deg, ${T.wa}, #128C7E)`, transition: 'width 0.4s' }} />
          </div>
          {isDone && job.failed > 0 && (
            <div style={{ marginTop: 8, fontSize: '0.75rem', color: T.amber }}>
              {job.errors?.slice(0, 3).map((e, i) => <div key={i}>{e}</div>)}
              {job.errors?.length > 3 && <div>…and {job.errors.length - 3} more</div>}
            </div>
          )}
          {isDone && (
            <button onClick={() => { setJob(null); setError(''); }}
              style={{ marginTop: 8, background: '#fff', border: `1px solid ${T.border}`, color: T.muted, borderRadius: 6, padding: '4px 10px', cursor: 'pointer', fontSize: '0.75rem', fontFamily: T.font }}>
              Send Again
            </button>
          )}
        </div>
      )}
    </div>
  );
}

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
  campVoice, setCampVoice, handleSaveCampVoice, handleResetCampVoice, campVoiceSaveStatus,
  INDIAN_VOICES, INDIAN_LANGUAGES,
  liveEvents, setLiveEvents,
  handleLeadStatusChange, handleEditLead, handleRemoveLead,
  handleViewTranscripts,
  onCampaignDial, onCampaignWebCall,
  dialingId, webCallActive,
  setSelectedLeadIds, setShowAddLeadsModal, setShowCsvImportModal, setCsvFile,
  apiFetch, API_URL, orgTimezone,
  handleEditCampaign,
  executives
}) {
  const stats = getCampaignStats(selectedCampaign);
  const toast = useToast();
  const confirm = useConfirm();
  const { triggerBrowserCall, browserCallLead, browserCallDialing, refreshScheduledCalls, clearDismissedScheduledCall } = useCall();
  const [callInsights, setCallInsights] = useState(null);
  const [callReviews, setCallReviews] = useState([]);
  const [insightsLoading, setInsightsLoading] = useState(false);
  const [insightsError, setInsightsError] = useState('');
  const [billingUsage, setBillingUsage] = useState(null);
  const [retries, setRetries] = useState([]);
  const [retriesLoading, setRetriesLoading] = useState(false);
  const [scheduleLead, setScheduleLead] = useState(null);
  const [scheduleAt, setScheduleAt] = useState('');
  const [scheduleNotes, setScheduleNotes] = useState('');
  const [scheduleSaving, setScheduleSaving] = useState(false);
  const [scheduleStatus, setScheduleStatus] = useState({ kind: '', text: '' });
  const [scheduleError, setScheduleError] = useState('');
  const [qaStatus, setQaStatus] = useState(null);
  const [leadSearch, setLeadSearch] = useState('');
  const [execFilter, setExecFilter] = useState([]);
  const [showExecFilter, setShowExecFilter] = useState(false);
  const [execSearch, setExecSearch] = useState('');
  const [scheduleFrom, setScheduleFrom] = useState('');
  const [scheduleTo, setScheduleTo] = useState('');

  // ── Auto-dialer state (Browser Call only) ───────────────────────────────────
  const [autoDialEnabled, setAutoDialEnabled] = useState(false);
  const [autoDialQueue, setAutoDialQueue] = useState([]);
  const [autoDialActiveId, setAutoDialActiveId] = useState(null);

  // Per-machine browser-call account: stored in localStorage so different systems
  // can dial from different Exotel voicebot accounts in parallel without changing
  // the campaign default used by AI/server calls.
  const [browserAccountId, setBrowserAccountId] = useState('');
  const browserAccountKey = useCallback((id) => `callified_browser_account_campaign_${id}`, []);

  useEffect(() => {
    setExecFilter([]);
    setExecSearch('');
    setShowExecFilter(false);
    setAutoDialEnabled(false);
    setAutoDialQueue([]);
    setAutoDialActiveId(null);
    setScheduleFrom('');
    setScheduleTo('');
  }, [selectedCampaign?.id]);

  const filteredLeads = useMemo(() => {
    let list = campaignLeads;
    if (execFilter.length > 0) {
      list = list.filter(l => execFilter.includes(String(l.executive_id || '')) || execFilter.includes(l.executive_id));
    }
    const q = leadSearch.trim().toLowerCase();
    if (q) {
      list = list.filter(l =>
        (l.first_name || '').toLowerCase().includes(q) ||
        (l.last_name || '').toLowerCase().includes(q) ||
        (l.phone || '').toLowerCase().includes(q) ||
        (l.source || '').toLowerCase().includes(q)
      );
    }
    if (scheduleFrom || scheduleTo) {
      const fromISO = scheduleFrom ? new Date(scheduleFrom).toISOString() : null;
      const toISO = scheduleTo ? new Date(scheduleTo).toISOString() : null;
      list = list.filter(l => {
        if (!l.next_scheduled_at) return false;
        const leadISO = l.next_scheduled_at.endsWith('Z')
          ? l.next_scheduled_at
          : l.next_scheduled_at.replace(' ', 'T') + 'Z';
        if (fromISO && leadISO < fromISO) return false;
        if (toISO && leadISO > toISO) return false;
        return true;
      });
    }
    return list;
  }, [campaignLeads, leadSearch, execFilter, scheduleFrom, scheduleTo]);

  // Keep the auto-dial queue in sync with the current filtered list.
  // The queue only moves forward; it never wraps around to leads before the
  // starting lead so auto-dial stops cleanly at the end of the list.
  useEffect(() => {
    if (!autoDialEnabled) return;
    const ids = filteredLeads.map(l => l.id);
    setAutoDialQueue(prev => {
      if (autoDialActiveId && ids.includes(autoDialActiveId)) {
        const idx = ids.indexOf(autoDialActiveId);
        return [autoDialActiveId, ...ids.slice(idx + 1)];
      }
      return ids;
    });
  }, [filteredLeads, autoDialEnabled, autoDialActiveId]);

  const [editingNote, setEditingNote] = useState(null);
  const [generatedNote, setGeneratedNote] = useState(null);
  const [noteSaving, setNoteSaving] = useState(false);
  const [noteGenerating, setNoteGenerating] = useState(false);

  // Quick-note modal state (moved here from CampaignsPage so we can refresh
  // campaign leads immediately after saving and show the note label at once).
  const [noteModalLead, setNoteModalLead] = useState(null);
  const [noteModalText, setNoteModalText] = useState('');
  const [noteModalSaving, setNoteModalSaving] = useState(false);

  const handleGenerateNote = async (lead) => {
    setNoteGenerating(true);
    setGeneratedNote(null);
    try {
      const res = await apiFetch(`${API_URL}/leads/${lead.id}/generate-followup-note`, { method: 'POST' });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) { toast(data.error || 'Could not generate note'); return; }
      setGeneratedNote({ leadId: lead.id, text: data.note || '', recordingUrl: data.recording_url || '', recordingFilename: data.recording_filename || '' });
      setEditingNote(null);
    } catch (e) {
      toast('Failed to generate note: ' + (e?.message || 'network error'));
    } finally {
      setNoteGenerating(false);
    }
  };

  const handleSaveInlineNote = async (lead) => {
    if (!editingNote) return;
    const trimmed = editingNote.text.trim();
    setNoteSaving(true);
    try {
      const res = await apiFetch(`${API_URL}/leads/${lead.id}/notes`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ note: trimmed }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast(data.error || `Failed to save (HTTP ${res.status})`);
        return;
      }
      lead.follow_up_note = trimmed;
      setEditingNote(null);
      setGeneratedNote(null);
      fetchCampaignLeads(selectedCampaign.id);
    } catch (e) {
      toast('Failed to save note: ' + (e?.message || 'network error'));
    } finally {
      setNoteSaving(false);
    }
  };

  const openNoteModal = (lead) => {
    setNoteModalLead(lead);
    setNoteModalText(lead.follow_up_note || '');
  };

  const handleSaveNoteModal = async () => {
    if (!noteModalLead) return;
    const trimmed = noteModalText.trim();
    if (!trimmed) { toast('Note cannot be empty'); return; }
    setNoteModalSaving(true);
    try {
      const res = await apiFetch(`${API_URL}/leads/${noteModalLead.id}/notes`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ note: trimmed }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast(data.error || data.detail || `Failed to save note (HTTP ${res.status})`);
        return;
      }
      setNoteModalLead(null);
      setNoteModalText('');
      fetchCampaignLeads(selectedCampaign.id);
    } catch (e) {
      toast('Failed to save note: ' + (e?.message || 'network error'));
    } finally {
      setNoteModalSaving(false);
    }
  };

  const [waSendingId, setWaSendingId] = useState(null);
  const [waSendStatus, setWaSendStatus] = useState({}); // lead.id → 'sent' | 'error'

  const handleSendWA = async (lead) => {
    setWaSendingId(lead.id);
    setWaSendStatus(s => ({ ...s, [lead.id]: null }));
    try {
      const res = await apiFetch(`${API_URL}/wa/campaign-blast/${selectedCampaign.id}/send-one`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ lead_id: lead.id }),
      });
      setWaSendStatus(s => ({ ...s, [lead.id]: res.ok ? 'sent' : 'error' }));
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast(data.error || `Send failed (HTTP ${res.status})`);
      }
    } catch {
      setWaSendStatus(s => ({ ...s, [lead.id]: 'error' }));
      toast('Network error — could not reach server');
    }
    setWaSendingId(null);
  };

  const [qaName, setQaName] = useState('');
  const [qaPhone, setQaPhone] = useState('');
  const [qaNameErr, setQaNameErr] = useState('');
  const [qaPhoneErr, setQaPhoneErr] = useState('');
  const [qaApiErr, setQaApiErr] = useState('');

  const [dndBlockedLeadIds, setDndBlockedLeadIds] = useState(() => new Set());
  const handleDialClick = async (lead) => {
    onCampaignDial(lead, selectedCampaign.id);
    try {
      const res = await apiFetch(`${API_URL}/dnd/check/${encodeURIComponent(lead.phone || '')}`);
      if (!res.ok) return;
      const data = await res.json();
      if (!data.is_dnd) return;
      setDndBlockedLeadIds(prev => {
        const next = new Set(prev);
        next.add(lead.id);
        return next;
      });
      setTimeout(() => {
        setDndBlockedLeadIds(prev => {
          if (!prev.has(lead.id)) return prev;
          const next = new Set(prev);
          next.delete(lead.id);
          return next;
        });
      }, 2000);
    } catch { /* network/permission — silently skip badge */  }
  };

  const handleHumanCallDial = async () => {
    if (!humanCallLead || !humanCallPhone.trim()) return;
    localStorage.setItem('humanCallAgentPhone', humanCallPhone.trim());
    setHumanCallStatus('dialing');
    setHumanCallError('');
    try {
      const res = await apiFetch(
        `${API_URL}/campaigns/${selectedCampaign.id}/human-call/${humanCallLead.id}`,
        { method: 'POST', body: JSON.stringify({ agent_phone: humanCallPhone.trim() }) }
      );
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        throw new Error(d.error || `HTTP ${res.status}`);
      }
      setHumanCallStatus('done');
      setTimeout(() => { setHumanCallLead(null); setHumanCallStatus('idle'); }, 2000);
    } catch (e) {
      setHumanCallError(e.message || 'Dial failed');
      setHumanCallStatus('error');
    }
  };

  // Refs keep auto-dial state fresh inside the ended-callback without
  // recreating the callback on every render.
  const autoDialEnabledRef = useRef(autoDialEnabled);
  const autoDialActiveIdRef = useRef(autoDialActiveId);
  const autoDialQueueRef = useRef(autoDialQueue);
  const campaignLeadsRef = useRef(campaignLeads);
  const filteredLeadsRef = useRef(filteredLeads);
  useEffect(() => { autoDialEnabledRef.current = autoDialEnabled; }, [autoDialEnabled]);
  useEffect(() => { autoDialActiveIdRef.current = autoDialActiveId; }, [autoDialActiveId]);
  useEffect(() => { autoDialQueueRef.current = autoDialQueue; }, [autoDialQueue]);
  useEffect(() => { campaignLeadsRef.current = campaignLeads; }, [campaignLeads]);
  useEffect(() => { filteredLeadsRef.current = filteredLeads; }, [filteredLeads]);

  const advanceAutoDial = useCallback((status, errorMsg) => {
    if (status === 'error') {
      toast('Auto dial stopped: browser call failed');
      setAutoDialEnabled(false);
      setAutoDialActiveId(null);
      setAutoDialQueue([]);
      return;
    }
    if (!autoDialEnabledRef.current || !autoDialActiveIdRef.current) return;
    const idx = autoDialQueueRef.current.indexOf(autoDialActiveIdRef.current);
    const nextIdx = idx >= 0 ? idx + 1 : autoDialQueueRef.current.length;
    const nextId = autoDialQueueRef.current[nextIdx];
    if (!nextId) {
      toast('Auto dial complete');
      setAutoDialEnabled(false);
      setAutoDialActiveId(null);
      setAutoDialQueue([]);
      return;
    }
    const nextLead = campaignLeadsRef.current.find(l => l.id === nextId) || filteredLeadsRef.current.find(l => l.id === nextId);
    if (!nextLead) {
      toast('Auto dial stopped: next lead not found');
      setAutoDialEnabled(false);
      setAutoDialActiveId(null);
      setAutoDialQueue([]);
      return;
    }
    setAutoDialActiveId(nextId);
    setTimeout(() => triggerBrowserCall(nextLead, selectedCampaign.id, advanceAutoDial, browserAccountId), 800);
  }, [toast, selectedCampaign.id, triggerBrowserCall, browserAccountId]);

  const startBrowserCallWithAutoDial = (lead) => {
    if (autoDialEnabled) {
      setAutoDialActiveId(lead.id);
      const ids = filteredLeads.map(l => l.id);
      const idx = ids.indexOf(lead.id);
      if (idx >= 0) {
        setAutoDialQueue([lead.id, ...ids.slice(idx + 1)]);
      } else {
        setAutoDialQueue([lead.id]);
      }
    }
    triggerBrowserCall(lead, selectedCampaign.id, autoDialEnabled ? advanceAutoDial : undefined, browserAccountId);
  };

  const [confirmRemoveLeadId, setConfirmRemoveLeadId] = useState(null);
  const [confirmDialAction, setConfirmDialAction] = useState(null); // { type: 'new'|'all'|'redial', label, count }

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
        body: JSON.stringify({ lead_id: scheduleLead.id, campaign_id: selectedCampaign.id, scheduled_at: utcTime, notes: scheduleNotes })
      });
      if (res.ok) {
        setScheduleLead(null);
        setScheduleAt('');
        setScheduleNotes('');
        fetchCampaignLeads(selectedCampaign.id);
      } else {
        const d = await res.json().catch(() => ({}));
        setScheduleError(d.detail || d.error || `Error ${res.status}`);
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

  // Auto-dismiss success toast after 4 s
  useEffect(() => {
    if (qaStatus?.type === 'success') {
      const t = setTimeout(() => setQaStatus(null), 4000);
      return () => clearTimeout(t);
    }
  }, [qaStatus]);

  const fetchInsights = async () => {
    setInsightsLoading(true);
    setInsightsError('');
    try {
      const [insightsRes, reviewsRes] = await Promise.all([
        apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/call-insights`),
        apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/call-reviews`),
      ]);
      if (!insightsRes.ok) {
        setCallInsights(null);
        setInsightsError(`Insights endpoint returned ${insightsRes.status}`);
      } else {
        setCallInsights(await insightsRes.json());
      }
      if (!reviewsRes.ok) {
        setCallReviews([]);
        if (!insightsError) setInsightsError(`Reviews endpoint returned ${reviewsRes.status}`);
      } else {
        setCallReviews(await reviewsRes.json());
      }
    } catch (e) {
      console.error('Failed to fetch insights', e);
      setInsightsError('Network error loading call insights');
    }
    setInsightsLoading(false);
  };

  const fetchRetries = async () => {
    setRetriesLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/retries`);
      const data = await res.json();
      setRetries(Array.isArray(data) ? data : (data?.retries || []));
    } catch (e) { console.error('Failed to fetch retries', e); }
    setRetriesLoading(false);
  };

  useEffect(() => {
     
    if (detailTab === 'insights') fetchInsights();
    if (detailTab === 'retries') fetchRetries();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailTab, selectedCampaign.id]);

  useEffect(() => {
    const fetchBilling = async () => {
      try {
        const res = await apiFetch(`${API_URL}/billing/usage`);
        const data = await res.json();
        if (data && data.has_subscription) setBillingUsage(data);
      } catch { /* no subscription — ignore */  }
    };
    fetchBilling();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── Exotel account selector state ─────────────────────────────────────────
  const [orgExotelAccounts, setOrgExotelAccounts] = useState([]);
  const [selectedExotelAccountId, setSelectedExotelAccountId] = useState('');
  const [exotelAccountSaveStatus, setExotelAccountSaveStatus] = useState('idle'); // idle | saving | saved | error

  const [humanCallLead, setHumanCallLead] = useState(null); // lead being human-called
  const [humanCallPhone, setHumanCallPhone] = useState(() => localStorage.getItem('humanCallAgentPhone') || '');
  const [humanCallStatus, setHumanCallStatus] = useState('idle'); // idle | dialing | done | error
  const [humanCallError, setHumanCallError] = useState('');

  // const [twilioBrowserLead, setTwilioBrowserLead] = useState(null); // lead for Twilio WebRTC call

  const hideAiFeatures = useHideAiFeatures();

  // Call-action visibility from Settings page (localStorage).
  const [visibleCallActions, setVisibleCallActions] = useState({
    dial: true,
    browserCall: true,
    simWebCall: true,
  });
  useEffect(() => {
    if (hideAiFeatures) {
      setVisibleCallActions({ dial: false, browserCall: true, simWebCall: false });
      return;
    }
    try {
      const saved = JSON.parse(localStorage.getItem('callified_call_actions') || '{}');
      setVisibleCallActions({
        dial: saved.dial !== false,
        browserCall: saved.browserCall !== false,
        simWebCall: saved.simWebCall !== false,
      });
    } catch { /* ignore */ }
  }, [hideAiFeatures]);

  useEffect(() => {
    if (selectedCampaign.channel === 'whatsapp') return;
    // Fetch all org accounts
    apiFetch(`${API_URL}/exotel-accounts`)
      .then(r => r.ok ? r.json() : [])
      .then(data => setOrgExotelAccounts(Array.isArray(data) ? data : []))
      .catch(() => {});
    // Fetch which account is linked to this campaign
    apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/exotel-account`)
      .then(r => r.ok ? r.json() : null)
      .then(data => { if (data?.exotel_account_id) setSelectedExotelAccountId(String(data.exotel_account_id)); })
      .catch(() => {});
    // Restore per-machine browser-call account from localStorage
    try {
      const saved = localStorage.getItem(browserAccountKey(selectedCampaign.id));
      if (saved != null) setBrowserAccountId(saved);
    } catch { /* ignore */ }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedCampaign.id, browserAccountKey]);

  const handleSaveExotelAccount = async () => {
    setExotelAccountSaveStatus('saving');
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/exotel-account`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ exotel_account_id: selectedExotelAccountId ? parseInt(selectedExotelAccountId) : 0 }),
      });
      setExotelAccountSaveStatus(res.ok ? 'saved' : 'error');
    } catch { setExotelAccountSaveStatus('error'); }
    setTimeout(() => setExotelAccountSaveStatus('idle'), 2000);
  };

  const scoreColor = (s) => {
    if (s >= 4) return T.green;
    if (s >= 3) return T.amber;
    if (s >= 2) return '#f97316';
    return T.red;
  };

  const sentimentColor = (s) => {
    if (s === 'positive') return T.green;
    if (s === 'neutral') return '#60a5fa';
    if (s === 'negative') return '#f97316';
    if (s === 'annoyed') return T.red;
    return T.muted;
  };

  const reviewByTranscript = {};
  callReviews.forEach(r => { reviewByTranscript[r.transcript_id] = r; });

  // ── shared mini styles ──────────────────────────────────────────
  const btnPrimary = {
    background: T.accent, border: 'none', color: '#fff',
    borderRadius: 8, padding: '8px 18px', cursor: 'pointer',
    fontSize: 13, fontWeight: 600, fontFamily: T.font,
  };
  const btnGhost = {
    background: '#fff', border: `1px solid ${T.border}`, color: T.sub,
    borderRadius: 8, padding: '6px 14px', cursor: 'pointer',
    fontSize: 12, fontWeight: 600, fontFamily: T.font,
  };
  const inputStyle = {
    padding: '7px 10px', border: `1px solid ${T.border}`, borderRadius: 8,
    fontSize: 13, fontFamily: T.font, color: T.text, background: '#fff', outline: 'none',
  };
  const thStyle = {
    padding: '10px 14px', fontSize: 11, fontWeight: 600, color: T.muted,
    textTransform: 'uppercase', letterSpacing: '0.06em', textAlign: 'left',
    borderBottom: `1px solid ${T.border}`, background: T.bg,
  };
  const tdStyle = { padding: '11px 14px', fontSize: 13, color: T.sub, borderBottom: `1px solid ${T.border}` };

  return (
    <div style={{ padding: '24px 28px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Back */}
      <button onClick={handleBack}
        style={{ background: 'none', border: 'none', color: T.accent, cursor: 'pointer', fontSize: '0.85rem', fontWeight: 600, marginBottom: '1.25rem', padding: 0, fontFamily: T.font }}>
        ← Back to Campaigns
      </button>

      {/* Campaign header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: '1.5rem', flexWrap: 'wrap' }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>{selectedCampaign.name}</h2>
        {selectedCampaign.product_id > 0 ? (
          <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: '#0891b2', background: 'rgba(8,145,178,0.1)' }}>
            {getProductName(selectedCampaign.product_id)}
          </span>
        ) : (
          <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: T.amber, background: 'rgba(245,158,11,0.1)' }}>
            ⚠ No product linked
          </span>
        )}
        {statusBadge(selectedCampaign.status)}
        <button onClick={() => handleEditCampaign(selectedCampaign)}
          style={{ background: 'rgba(245,158,11,0.08)', border: `1px solid rgba(245,158,11,0.3)`, color: '#92400e', borderRadius: 8, padding: '5px 14px', cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font }}>
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
          style={{ width: 'auto', height: 32, fontSize: '0.8rem', padding: '4px 10px', background: '#fff', border: `1px solid ${T.border}`, color: T.text, borderRadius: 8, fontFamily: T.font }}>
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

      {/* Metrics grid */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 12, marginBottom: 16 }}>
        {[
          { label: 'Total Leads', val: stats.total, color: T.accent },
          { label: 'Called', val: stats.called, color: T.sub },
          { label: 'Qualified', val: stats.qualified, color: T.pink },
          { label: 'Appointments', val: stats.booked, color: T.green },
        ].map(s => (
          <div key={s.label} style={{ ...card, padding: '18px 20px' }}>
            <div style={{ fontSize: 11, color: T.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 8 }}>{s.label}</div>
            <div style={{ fontSize: 28, fontWeight: 700, fontFamily: T.mono, color: s.color }}>{s.val}</div>
          </div>
        ))}
      </div>

      {/* Voice Settings — hidden for WhatsApp campaigns and AI-hidden users */}
      {selectedCampaign.channel !== 'whatsapp' && !hideAiFeatures && (
        <div style={{ ...card, marginBottom: 16, padding: '14px 18px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            <span style={{ fontSize: 12, color: T.muted, fontWeight: 700, whiteSpace: 'nowrap', textTransform: 'uppercase', letterSpacing: '0.05em' }}>🔊 Voice Settings</span>
            <select className="form-input" value={campVoice.tts_provider}
              onChange={e => { const p = e.target.value; setCampVoice(v => ({...v, tts_provider: p, tts_voice_id: (INDIAN_VOICES[p] || [])[0]?.id || ''})); }}
              style={{ ...inputStyle, height: 32, minWidth: 110 }}>
              <option value="">-- Provider --</option>
              <option value="elevenlabs">ElevenLabs</option>
              <option value="sarvam">Sarvam AI</option>
              <option value="smallest">Smallest AI</option>
            </select>
            <select className="form-input" value={campVoice.tts_voice_id}
              onChange={e => setCampVoice(v => ({...v, tts_voice_id: e.target.value}))}
              style={{ ...inputStyle, height: 32, minWidth: 160 }}>
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
              style={{ ...inputStyle, height: 32, minWidth: 100 }}>
              <option value="">-- Language --</option>
              {INDIAN_LANGUAGES.map(l => (
                <option key={l.code} value={l.code}>{l.name}</option>
              ))}
            </select>
            <button style={{
                background: campVoiceSaveStatus === 'saved' ? T.green
                  : campVoiceSaveStatus === 'error' ? T.red
                  : T.accent,
                border: 'none', color: '#fff', fontSize: 12, padding: '6px 14px', borderRadius: 8,
                cursor: campVoiceSaveStatus === 'saving' ? 'wait' : 'pointer', whiteSpace: 'nowrap',
                opacity: campVoiceSaveStatus === 'saving' ? 0.7 : 1, fontWeight: 600, fontFamily: T.font,
              }}
              disabled={campVoiceSaveStatus === 'saving'}
              onClick={handleSaveCampVoice}>
              {campVoiceSaveStatus === 'saving' ? 'Saving…'
                : campVoiceSaveStatus === 'saved' ? '✓ Saved'
                : campVoiceSaveStatus === 'error' ? '✗ Failed'
                : 'Save'}
            </button>
            <button style={{ ...btnGhost, fontSize: 12 }} onClick={handleResetCampVoice}>Reset to Org Default</button>
          </div>
          <div style={{ fontSize: '0.7rem', color: T.accent, marginTop: 6 }}>
            {campVoice.tts_provider
              ? (() => {
                  const providerLabel = campVoice.tts_provider === 'elevenlabs' ? 'ElevenLabs'
                    : campVoice.tts_provider === 'sarvam' ? 'Sarvam AI'
                    : 'Smallest AI';
                  const voiceLabel = (INDIAN_VOICES[campVoice.tts_provider] || [])
                    .find(v => v.id === campVoice.tts_voice_id)?.name
                    || campVoice.tts_voice_id || 'none';
                  const langLabel = INDIAN_LANGUAGES
                    .find(l => l.code === campVoice.tts_language)?.name
                    || campVoice.tts_language;
                  return `Current: ${providerLabel} - ${voiceLabel}` + (langLabel ? ` (${langLabel})` : '');
                })()
              : 'Using org default'}
          </div>
          {VOICE_RECOMMENDATIONS[campVoice.tts_language]?.[campVoice.tts_provider]?.note && (
            <div style={{ fontSize: '0.65rem', color: '#0891b2', marginTop: 4 }}>
              ℹ {VOICE_RECOMMENDATIONS[campVoice.tts_language][campVoice.tts_provider].note}
            </div>
          )}
        </div>
      )}

      {/* Browser Call Account (per-machine) — hidden for WhatsApp campaigns */}
      {selectedCampaign.channel !== 'whatsapp' && (
        <div style={{ ...card, marginBottom: 16, padding: '14px 18px' }}>
          <div style={{ fontSize: 12, color: T.muted, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 10 }}>
            🖥️ Browser Call Account (this machine)
          </div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
            <select
              className="form-input"
              value={browserAccountId}
              onChange={e => {
                const v = e.target.value;
                setBrowserAccountId(v);
                try {
                  localStorage.setItem(browserAccountKey(selectedCampaign.id), v);
                } catch { /* ignore */ }
              }}
              style={{ ...inputStyle, height: 34, minWidth: 280, maxWidth: 420 }}>
              <option value="">Use campaign default</option>
              {orgExotelAccounts.filter(a => a.app_type === 'voicebot').map(a => (
                <option key={a.id} value={String(a.id)}>
                  {'[Exotel]'} {a.name} · {a.account_sid} · {a.caller_id}
                </option>
              ))}
            </select>
          </div>
          <div style={{ fontSize: '0.7rem', color: T.muted, marginTop: 6 }}>
            {browserAccountId
              ? (() => {
                  const a = orgExotelAccounts.find(x => String(x.id) === browserAccountId);
                  return a ? `Dialing from: ${a.name} · ${a.account_sid} · ${a.caller_id}` : 'Account selected';
                })()
              : orgExotelAccounts.length === 0
                ? 'No saved voicebot accounts — go to More → Provider Accounts to add one'
                : 'Browser calls will use the campaign default. This choice is saved only in this browser.'}
          </div>
        </div>
      )}

      {/* Billing Minutes Widget */}
      {billingUsage && (
        <div style={{
          display: 'inline-flex', alignItems: 'center', gap: 10,
          background: 'rgba(99,102,241,0.06)', border: `1px solid rgba(99,102,241,0.2)`,
          borderRadius: 20, padding: '6px 16px', marginBottom: 14,
        }}>
          <span style={{ fontSize: '0.8rem', color: T.sub, fontWeight: 600, whiteSpace: 'nowrap' }}>
            ⏱ {billingUsage.minutes_remaining} / {billingUsage.minutes_included} min remaining
          </span>
          <div style={{ width: 80, height: 6, background: T.border, borderRadius: 3, overflow: 'hidden' }}>
            <div style={{
              width: `${Math.min(100, (billingUsage.minutes_used / billingUsage.minutes_included) * 100)}%`,
              height: '100%', borderRadius: 3,
              background: (billingUsage.minutes_used / billingUsage.minutes_included) > 0.9
                ? T.red : (billingUsage.minutes_used / billingUsage.minutes_included) > 0.7
                ? T.amber : T.accent,
              transition: 'width 0.5s ease',
            }} />
          </div>
        </div>
      )}

      {/* Live Dial Events Feed — AI dialer events; hide for AI-hidden users */}
      {!hideAiFeatures && <div style={{ ...card, marginBottom: 14, padding: 14, maxHeight: 200, overflowY: 'auto' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <span style={{ fontSize: 11, color: T.muted, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '1px' }}>📡 Live Campaign Activity</span>
          {liveEvents.length > 0 && (
            <button onClick={() => {
              setLiveEvents([]);
              try {
                localStorage.setItem(`liveEventsClearedAt:${selectedCampaign.id}`, String(Date.now()));
              } catch { /* ignore */ }
            }} style={{ background: 'none', border: 'none', color: T.muted, cursor: 'pointer', fontSize: '0.7rem', fontFamily: T.font }}>Clear</button>
          )}
        </div>
        {liveEvents.length === 0 ? (
          <div style={{ fontSize: '0.75rem', color: T.muted, fontStyle: 'italic', padding: '4px 0' }}>
            Listening for new events… start a dial to see activity here.
          </div>
        ) : (
          liveEvents.map((ev, i) => (
            <div key={i} style={{ fontSize: '0.8rem', color: T.sub, padding: '3px 0', borderBottom: `1px solid ${T.border}`, fontFamily: T.mono }}>
              {withDate(ev?.label, ev?.ts)}
            </div>
          ))
        )}
      </div>}

      {/* Quick Add Lead Form */}
      <div style={{ ...card, padding: '12px 16px', marginBottom: 14, display: 'flex', gap: 8, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <span style={{ fontSize: 12, color: T.muted, fontWeight: 700, height: 32, display: 'flex', alignItems: 'center', textTransform: 'uppercase', letterSpacing: '0.05em' }}>➕ Quick Add:</span>
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <input className="form-input" placeholder="Name" value={qaName}
            onChange={e => {
              const v = e.target.value;
              setQaName(v);
              const t = v.trim();
              if (!t) setQaNameErr('');
              else if (/\d/.test(t) || !/[A-Za-z]/.test(t)) setQaNameErr('Name must contain only letters');
              else setQaNameErr('');
            }}
            style={{ ...inputStyle, width: 130, height: 32, border: qaNameErr ? `1px solid ${T.red}` : `1px solid ${T.border}` }} />
          {qaNameErr && <span style={{ color: T.red, fontSize: '0.7rem', marginTop: 4 }}>{qaNameErr}</span>}
        </div>
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <input className="form-input" placeholder="Phone (10 digits)" value={qaPhone}
            inputMode="numeric" maxLength={10} pattern="\d{10}"
            onChange={e => {
              const raw = e.target.value;
              const v = raw.replace(/\D/g, '').slice(0, 10);
              setQaPhone(v);
              if (/\D/.test(raw)) {
                setQaPhoneErr('Only digits are accepted');
              } else if (qaPhoneErr) {
                setQaPhoneErr('');
              }
            }}
            style={{ ...inputStyle, width: 160, height: 32, border: qaPhoneErr ? `1px solid ${T.red}` : `1px solid ${T.border}` }} />
          {qaPhoneErr && <span style={{ color: T.red, fontSize: '0.7rem', marginTop: 4 }}>{qaPhoneErr}</span>}
        </div>
        <button style={{ ...btnPrimary, height: 32, padding: '4px 14px' }}
          onClick={async () => {
            const name = qaName.trim();
            const phone = qaPhone.trim();
            const nameErr = !name
              ? 'Name is required'
              : (!/[A-Za-z]/.test(name) || /\d/.test(name) ? 'Name must contain only letters' : '');
            const phoneErr = !phone
              ? 'Phone is required'
              : (!/^\d{10}$/.test(phone) ? 'Indian numbers must be exactly 10 digits' : '');
            setQaNameErr(nameErr);
            setQaPhoneErr(phoneErr);
            setQaApiErr('');
            if (nameErr || phoneErr) return;
            try {
              const res = await apiFetch(`${API_URL}/leads`, {
                method: 'POST', headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({ first_name: name, phone, source: 'Manual' })
              });
              const data = await res.json();
              let leadId = data.id;
              const errMsg = data.error || data.message || '';
              const isDuplicate = res.status === 409 || errMsg.includes('already exists');
              if (data.fields && typeof data.fields === 'object') {
                if (data.fields.first_name) setQaNameErr(data.fields.first_name);
                if (data.fields.phone) setQaPhoneErr(data.fields.phone);
                if (!isDuplicate) return;
              }
              if (!leadId && isDuplicate) {
                const searchRes = await apiFetch(`${API_URL}/leads/search?q=${encodeURIComponent(phone)}`);
                const found = await searchRes.json();
                if (Array.isArray(found) && found.length > 0) leadId = found[0].id;
              }
              if (leadId) {
                await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/leads`, {
                  method: 'POST', headers: {'Content-Type': 'application/json'},
                  body: JSON.stringify({ lead_ids: [leadId] })
                });
                setQaName('');
                setQaPhone('');
                fetchCampaignLeads(selectedCampaign.id);
                fetchCampaigns();
              } else if (!data.fields) { setQaApiErr(errMsg || `Error (${res.status})`); }
            } catch(e) { setQaApiErr('Failed: ' + (e?.message || 'network error')); }
          }}>Add & Assign</button>
        {qaApiErr && <span style={{ color: T.red, fontSize: '0.75rem', width: '100%', marginTop: 4 }}>{qaApiErr}</span>}
      </div>

      {selectedCampaign.channel === 'whatsapp' && !hideAiFeatures && (
        <div style={{ marginBottom: 14 }}>
          <WhatsAppBlastPanel campaignId={selectedCampaign.id} apiFetch={apiFetch} API_URL={API_URL} />
        </div>
      )}

      {/* Action buttons */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
        <button style={{ ...btnPrimary }} onClick={() => { setSelectedLeadIds([]); setShowAddLeadsModal(true); }}>+ Add from CRM</button>
        <button style={{ ...btnPrimary, background: '#0891b2' }}
          onClick={() => { setCsvFile(null); setShowCsvImportModal(true); }}>📤 Import CSV</button>
        <button
          style={{ ...btnPrimary, background: T.green }}
          onClick={() => {
            apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/export-recordings`)
              .then(res => res.blob())
              .then(blob => {
                const url = URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = `recordings_${(selectedCampaign.name || selectedCampaign.id).toString().replace(/\s+/g,'_')}.csv`;
                a.click();
                URL.revokeObjectURL(url);
              });
          }}>
          ⬇ Export
        </button>
        {!hideAiFeatures && campaignLeads.some(l => (l.status || '').toLowerCase() === 'new') && (
          <button style={{ ...btnPrimary, background: T.green }}
            onClick={async () => {
              const newCount = campaignLeads.filter(l => (l.status || '').toLowerCase() === 'new').length;
              if (!await confirm({ message: `Dial ALL ${newCount} new leads? (30s gap between calls)` })) return;
              try {
                const res = await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/dial-all`, { method: 'POST' });
                const data = await res.json();
                toast(data.message || 'Dialing started');
                const ri = setInterval(() => { fetchCampaignLeads(selectedCampaign.id); fetchCallLog(selectedCampaign.id); }, 15000);
                setTimeout(() => clearInterval(ri), 30 * 60 * 1000);
              } catch { toast('Dial failed');  }
            }}>
            📞 Dial All New ({campaignLeads.filter(l => (l.status || '').toLowerCase() === 'new').length})
          </button>
        )}
        {!hideAiFeatures && <button style={{ ...btnPrimary, background: '#7c3aed' }}
          onClick={async () => {
            if (!await confirm({ message: `Dial ALL ${campaignLeads.length} leads? (30s gap)` })) return;
            try {
              const res = await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/dial-all?force=true`, { method: 'POST' });
              const data = await res.json();
              toast(data.message || 'Dialing started');
              const ri = setInterval(() => { fetchCampaignLeads(selectedCampaign.id); fetchCallLog(selectedCampaign.id); }, 15000);
              setTimeout(() => clearInterval(ri), 30 * 60 * 1000);
            } catch { toast('Failed');  }
          }}>
          📞 Dial All ({campaignLeads.length})
        </button>}
        {selectedCampaign.channel !== 'whatsapp' && visibleCallActions.browserCall && (
          <button
            style={{
              ...btnPrimary,
              background: autoDialEnabled ? '#f59e0b' : '#475569',
              display: 'flex', alignItems: 'center', gap: 6,
            }}
            onClick={() => {
              const next = !autoDialEnabled;
              setAutoDialEnabled(next);
              if (next) {
                setAutoDialQueue(filteredLeads.map(l => l.id));
                toast('Auto dial enabled. Start a browser call to begin.');
              } else {
                setAutoDialActiveId(null);
                setAutoDialQueue([]);
                toast('Auto dial stopped');
              }
            }}
            title={autoDialActiveId ? 'Stop auto-dialing' : 'After a browser call ends, automatically dial the next filtered lead'}>
            {autoDialActiveId ? '⏹ Stop Auto Dial' : autoDialEnabled ? '⏸ Auto Dial On' : '▶ Auto Dial'}
          </button>
        )}
      </div>

      {/* Search + Tab Switcher */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16, flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', background: T.bg, border: `1px solid ${T.border}`, borderRadius: 8, padding: 3, gap: 2, width: 'fit-content' }}>
          {[
            { id: 'leads',   label: `👥 Leads (${(leadSearch.trim() || execFilter.length > 0) ? `${filteredLeads.length}/${campaignLeads.length}` : campaignLeads.length})`,   activeColor: T.accent },
          { id: 'calllog', label: `📞 Call Log (${callLog.length})`,       activeColor: T.green  },
          { id: 'insights',label: '📊 Call Insights',                      activeColor: '#a855f7', hidden: hideAiFeatures },
          { id: 'retries', label: '🔄 Retries',                            activeColor: T.amber,  hidden: hideAiFeatures },
          ].filter(tab => !tab.hidden).map(tab => (
            <button key={tab.id}
              onClick={() => {
                if (tab.id === 'calllog') { setDetailTab('calllog'); fetchCallLog(selectedCampaign.id); fetchInsights(); }
                else setDetailTab(tab.id);
              }}
              style={{
                padding: '6px 18px', borderRadius: 6, border: 'none', cursor: 'pointer',
                fontSize: 13, fontWeight: 600, fontFamily: T.font,
                background: detailTab === tab.id ? tab.activeColor : 'transparent',
                color: detailTab === tab.id ? '#fff' : T.muted,
                transition: 'all 0.15s',
              }}>
              {tab.label}
            </button>
          ))}
        </div>
        <input
          type="text"
          placeholder="Search leads by name, phone or source..."
          value={leadSearch}
          onChange={e => setLeadSearch(e.target.value)}
          style={{
            padding: '7px 12px', border: `1px solid ${T.border}`, borderRadius: 8,
            fontSize: 13, fontFamily: T.font, color: T.text, background: '#fff',
            outline: 'none', minWidth: 260,
          }}
        />
        {executives && executives.length > 0 && (
          <div style={{ position: 'relative' }}>
            <button
              onClick={() => setShowExecFilter(v => !v)}
              style={{
                padding: '7px 12px', border: `1px solid ${T.border}`, borderRadius: 8,
                fontSize: 13, fontFamily: T.font, color: T.text, background: '#fff',
                cursor: 'pointer', minWidth: 160, textAlign: 'left'
              }}>
              {execFilter.length === 0 ? 'Filter by Executive' : `${execFilter.length} executive${execFilter.length > 1 ? 's' : ''}`} ▾
            </button>
            {showExecFilter && (
              <div style={{
                position: 'absolute', top: 'calc(100% + 6px)', right: 0, minWidth: 220,
                background: '#fff', border: `1px solid ${T.border}`, borderRadius: 8,
                boxShadow: '0 8px 24px rgba(0,0,0,0.10)', padding: '8px 10px', zIndex: 50,
                maxHeight: 300, overflowY: 'auto'
              }}>
                <input
                  type="text"
                  placeholder="Search executives..."
                  value={execSearch}
                  onChange={e => setExecSearch(e.target.value)}
                  onClick={e => e.stopPropagation()}
                  style={{
                    width: '100%', boxSizing: 'border-box', padding: '6px 8px', marginBottom: 6,
                    border: `1px solid ${T.border}`, borderRadius: 6, fontSize: 13, fontFamily: T.font,
                    outline: 'none'
                  }}
                />
                {(() => {
                  const q = execSearch.trim().toLowerCase();
                  const filtered = q ? executives.filter(e => (e.name || '').toLowerCase().includes(q)) : executives;
                  if (filtered.length === 0) {
                    return <div style={{ color: T.muted, fontSize: 12, padding: '6px 0' }}>No executives found.</div>;
                  }
                  return filtered.map(e => {
                    const checked = execFilter.includes(String(e.id));
                    return (
                      <label key={e.id} style={{display: 'flex', alignItems: 'center', gap: 8, padding: '5px 0', color: T.text, fontSize: 13, cursor: 'pointer'}}>
                        <input type="checkbox" checked={checked}
                          onChange={() => {
                            const val = String(e.id);
                            setExecFilter(prev => checked ? prev.filter(id => id !== val) : [...prev, val]);
                          }} />
                        {e.name}
                      </label>
                    );
                  });
                })()}
              </div>
            )}
          </div>
        )}
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <input
            type="datetime-local"
            value={scheduleFrom}
            onChange={e => setScheduleFrom(e.target.value)}
            style={{
              padding: '7px 10px', border: `1px solid ${T.border}`, borderRadius: 8,
              fontSize: 12, fontFamily: T.font, color: T.text, background: '#fff', outline: 'none'
            }}
          />
          <span style={{ color: T.muted, fontSize: 12, fontWeight: 600 }}>to</span>
          <input
            type="datetime-local"
            value={scheduleTo}
            onChange={e => setScheduleTo(e.target.value)}
            style={{
              padding: '7px 10px', border: `1px solid ${T.border}`, borderRadius: 8,
              fontSize: 12, fontFamily: T.font, color: T.text, background: '#fff', outline: 'none'
            }}
          />
          <button
            onClick={() => { setScheduleFrom(''); setScheduleTo(''); }}
            disabled={!scheduleFrom && !scheduleTo}
            style={{
              padding: '7px 12px', border: `1px solid ${T.border}`, borderRadius: 8,
              fontSize: 12, fontFamily: T.font, color: T.text, background: '#fff',
              cursor: (!scheduleFrom && !scheduleTo) ? 'not-allowed' : 'pointer',
              opacity: (!scheduleFrom && !scheduleTo) ? 0.5 : 1,
            }}>
            Clear
          </button>
        </div>
      </div>

      {/* Call Log Tab — WhatsApp notice */}
      {detailTab === 'calllog' && selectedCampaign.channel === 'whatsapp' && (
        <div style={{ ...card, padding: '1.5rem', marginBottom: '1.5rem', textAlign: 'center', color: T.muted }}>
          💬 Conversation history is in the <strong style={{ color: T.wa }}>WhatsApp Comms</strong> tab.
        </div>
      )}

      {/* Call Log Table */}
      {detailTab === 'calllog' && selectedCampaign.channel !== 'whatsapp' && (
        <div style={{ ...card, overflowX: 'auto', marginBottom: '1.5rem' }}>
          <div style={{ display: 'flex', justifyContent: 'flex-end', padding: '10px 16px 0' }}>
            <a
              href={`${API_URL}/campaigns/${selectedCampaign.id}/export-recordings`}
              download
              onClick={e => {
                e.preventDefault();
                apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/export-recordings`)
                  .then(res => res.blob())
                  .then(blob => {
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement('a');
                    a.href = url;
                    a.download = `recordings_${selectedCampaign.name?.replace(/\s+/g,'_') || selectedCampaign.id}.csv`;
                    a.click();
                    URL.revokeObjectURL(url);
                  });
              }}
              style={{
                display: 'inline-flex', alignItems: 'center', gap: 6,
                background: T.green, color: '#fff', borderRadius: 7,
                padding: '6px 16px', fontSize: '0.8rem', fontWeight: 600,
                fontFamily: T.font, textDecoration: 'none', cursor: 'pointer',
              }}>
              ⬇ Export CSV
            </a>
          </div>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Lead','Phone','Source','Time','Outcome','Quality','Duration','Recording'].map(h => (
                  <th key={h} style={thStyle}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {callLog.length === 0 ? (
                <tr><td colSpan="8" style={{ ...tdStyle, textAlign: 'center', color: T.muted, padding: '2rem' }}>No calls made yet.</td></tr>
              ) : callLog.map(call => {
                const review = reviewByTranscript[call.id];
                const outcomeColors = {
                  'Completed': T.green, 'Connected': '#60a5fa', 'No Answer': T.amber,
                  'Busy': '#f97316', 'Failed': T.red, 'DND Blocked': '#dc2626'
                };
                const outcomeBg = {
                  'Completed': 'rgba(16,185,129,0.1)', 'Connected': 'rgba(96,165,250,0.1)', 'No Answer': 'rgba(245,158,11,0.1)',
                  'Busy': 'rgba(249,115,22,0.1)', 'Failed': 'rgba(239,68,68,0.1)', 'DND Blocked': 'rgba(220,38,38,0.1)'
                };
                return (
                  <tr key={call.id}>
                    <td style={{ ...tdStyle, fontWeight: 600, color: T.text }}>{call.first_name} {call.last_name || ''}</td>
                    <td style={{ ...tdStyle, fontFamily: T.mono, fontSize: '0.85rem' }}>{call.phone}</td>
                    <td style={tdStyle}><span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: T.accent, background: `${T.accent}15` }}>{call.source || '-'}</span></td>
                    <td style={{ ...tdStyle, fontSize: '0.8rem', color: T.muted }}>{formatDateTime(call.created_at, orgTimezone)}</td>
                    <td style={tdStyle}>
                      <span style={{
                        padding: '3px 10px', borderRadius: 20, fontSize: '0.75rem', fontWeight: 600,
                        color: outcomeColors[call.outcome] || T.muted,
                        background: outcomeBg[call.outcome] || 'rgba(156,163,175,0.1)',
                        border: `1px solid ${(outcomeColors[call.outcome] || T.muted)}30`
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
                    <td style={tdStyle}>
                      {review ? (() => {
                        const q = Math.max(0, Math.min(5, Math.round(Number(review.quality_score) || 0)));
                        return (
                          <span style={{
                            padding: '2px 8px', borderRadius: 10, fontSize: '0.75rem', fontWeight: 700,
                            color: scoreColor(q), background: `${scoreColor(q)}18`, border: `1px solid ${scoreColor(q)}40`
                          }}>
                            {'★'.repeat(q)}{'☆'.repeat(5 - q)}
                          </span>
                        );
                      })() : (
                        <span style={{ color: T.muted, fontSize: '0.75rem' }}>--</span>
                      )}
                    </td>
                    <td style={{ ...tdStyle, fontFamily: T.mono }}>
                      {call.call_duration_s > 0 ? `${Math.floor(call.call_duration_s / 60)}:${String(Math.floor(call.call_duration_s % 60)).padStart(2, '0')}` : '-'}
                    </td>
                    <td style={tdStyle}>
                      {call.recording_url ? (
                        <AuthAudio preload="none" src={call.recording_url} className="call-log-audio" style={{ height: 36, width: 260 }} />
                      ) : (
                        <span style={{ color: T.muted, fontSize: '0.8rem' }}>—</span>
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
        <div style={{ marginBottom: '1.5rem' }}>
          {insightsLoading ? (
            <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>Loading insights...</div>
          ) : insightsError ? (
            <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.red, border: `1px solid #fca5a5` }}>
              <div style={{ fontWeight: 600, marginBottom: 6 }}>Call Insights are temporarily unavailable</div>
              <div style={{ fontSize: '0.8rem', color: T.muted }}>{insightsError}</div>
            </div>
          ) : !callInsights || callInsights.total_reviews === 0 ? (
            <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>No call reviews yet. Reviews are generated automatically after each call.</div>
          ) : (
            <>
              {/* Summary Cards */}
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 12, marginBottom: 16 }}>
                <div style={{ ...card, padding: '18px 20px' }}>
                  <div style={{ fontSize: 11, color: T.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 8 }}>Avg Quality Score</div>
                  <div style={{ fontSize: 28, fontWeight: 700, fontFamily: T.mono, color: scoreColor(Math.round(callInsights.avg_quality_score)) }}>{callInsights.avg_quality_score}/5</div>
                </div>
                <div style={{ ...card, padding: '18px 20px' }}>
                  <div style={{ fontSize: 11, color: T.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 8 }}>Appointment Rate</div>
                  <div style={{ fontSize: 28, fontWeight: 700, fontFamily: T.mono, color: callInsights.appointment_rate > 30 ? T.green : T.amber }}>{callInsights.appointment_rate}%</div>
                </div>
                <div style={{ ...card, padding: '18px 20px' }}>
                  <div style={{ fontSize: 11, color: T.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 8 }}>Calls Analyzed</div>
                  <div style={{ fontSize: 28, fontWeight: 700, fontFamily: T.mono, color: T.text }}>{callInsights.total_reviews}</div>
                </div>
                <div style={{ ...card, padding: '18px 20px' }}>
                  <div style={{ fontSize: 11, color: T.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 8 }}>Top Sentiment</div>
                  <div style={{ fontSize: 20, fontWeight: 700, color: sentimentColor(Object.entries(callInsights.sentiment_breakdown || {}).sort((a,b)=>b[1]-a[1])[0]?.[0]) }}>
                    {Object.entries(callInsights.sentiment_breakdown || {}).sort((a,b)=>b[1]-a[1])[0]?.[0] || '-'}
                  </div>
                </div>
              </div>

              {/* Improvement Suggestions */}
              {callInsights.top_improvements && callInsights.top_improvements.length > 0 && (
                <div style={{ ...card, padding: 18, marginBottom: 14 }}>
                  <div style={{ fontSize: 11, color: '#a855f7', fontWeight: 700, marginBottom: 10, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Prompt Improvement Suggestions</div>
                  {callInsights.top_improvements.map((imp, i) => (
                    <div key={i} style={{ padding: '8px 12px', marginBottom: 6, background: 'rgba(168,85,247,0.06)', borderRadius: 8, borderLeft: '3px solid #a855f7', fontSize: '0.85rem', color: T.sub }}>
                      {imp.suggestion}
                      <span style={{ color: T.muted, fontSize: '0.75rem', marginLeft: 8 }}>({imp.count}x)</span>
                    </div>
                  ))}
                </div>
              )}

              {/* Top Failure Reasons */}
              {callInsights.top_failure_reasons && callInsights.top_failure_reasons.length > 0 && (
                <div style={{ ...card, padding: 18, marginBottom: 14 }}>
                  <div style={{ fontSize: 11, color: '#f97316', fontWeight: 700, marginBottom: 10, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Top Failure Reasons</div>
                  {callInsights.top_failure_reasons.map((fr, i) => (
                    <div key={i} style={{ padding: '8px 12px', marginBottom: 6, background: 'rgba(249,115,22,0.06)', borderRadius: 8, borderLeft: '3px solid #f97316', fontSize: '0.85rem', color: T.sub }}>
                      {fr.reason}
                      <span style={{ color: T.muted, fontSize: '0.75rem', marginLeft: 8 }}>({fr.count}x)</span>
                    </div>
                  ))}
                </div>
              )}

              {/* Per-Call Reviews Table */}
              <div style={{ ...card, overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr>
                      {['Lead','Quality','Appt Booked','Date / Time','Sentiment','What Went Well','What Went Wrong','Failure Reason'].map(h => (
                        <th key={h} style={thStyle}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {callReviews.map(r => (
                      <tr key={r.id}>
                        <td style={{ ...tdStyle, fontWeight: 600, color: T.text }}>{r.first_name} {r.last_name || ''}</td>
                        <td style={tdStyle}>
                          {(() => {
                            const q = Math.max(0, Math.min(5, Math.round(Number(r.quality_score) || 0)));
                            return (
                              <span style={{ fontWeight: 700, color: scoreColor(q), fontSize: '0.9rem' }}>
                                {'★'.repeat(q)}{'☆'.repeat(5 - q)}
                              </span>
                            );
                          })()}
                        </td>
                        <td style={tdStyle}>
                          <span style={{
                            padding: '2px 10px', borderRadius: 20, fontSize: '0.75rem', fontWeight: 600,
                            color: r.appointment_booked ? T.green : '#f97316',
                            background: r.appointment_booked ? 'rgba(16,185,129,0.1)' : 'rgba(249,115,22,0.1)',
                          }}>
                            {r.appointment_booked ? 'Yes' : 'No'}
                          </span>
                        </td>
                        <td style={{ ...tdStyle, fontSize: '0.8rem', color: T.muted, whiteSpace: 'nowrap' }}>{formatDateTime(r.created_at, orgTimezone)}</td>
                        <td style={tdStyle}><span style={{ color: sentimentColor(r.customer_sentiment), fontWeight: 600, fontSize: '0.85rem' }}>{r.customer_sentiment}</span></td>
                        <td style={{ ...tdStyle, fontSize: '0.8rem', color: T.muted, maxWidth: 200 }}>{r.what_went_well || '-'}</td>
                        <td style={{ ...tdStyle, fontSize: '0.8rem', color: T.red, maxWidth: 200 }}>{r.what_went_wrong || '-'}</td>
                        <td style={{ ...tdStyle, fontSize: '0.8rem', color: T.muted, maxWidth: 200 }}>{r.failure_reason || '-'}</td>
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
        <div style={{ marginBottom: '1.5rem' }}>
          {retriesLoading ? (
            <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>Loading retry queue...</div>
          ) : retries.length === 0 ? (
            <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>No retries queued for this campaign.</div>
          ) : (
            <div style={{ ...card, overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    {['Lead','Phone','Attempt','Retry Time','Status'].map(h => (
                      <th key={h} style={thStyle}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {retries.map(r => {
                    const retryStatusColors = {
                      pending:   { color: T.amber, bg: 'rgba(245,158,11,0.1)',  border: 'rgba(245,158,11,0.3)' },
                      dialing:   { color: '#60a5fa', bg: 'rgba(96,165,250,0.1)', border: 'rgba(96,165,250,0.3)' },
                      completed: { color: T.green,  bg: 'rgba(16,185,129,0.1)', border: 'rgba(16,185,129,0.3)' },
                      exhausted: { color: T.red,    bg: 'rgba(239,68,68,0.1)',  border: 'rgba(239,68,68,0.3)' },
                    };
                    const sc = retryStatusColors[r.status] || retryStatusColors.pending;
                    return (
                      <tr key={r.id}>
                        <td style={{ ...tdStyle, fontWeight: 600, color: T.text }}>{r.first_name || r.lead_name || '-'} {r.last_name || ''}</td>
                        <td style={{ ...tdStyle, fontFamily: T.mono, fontSize: '0.85rem' }}>{r.phone}</td>
                        <td style={{ ...tdStyle, fontWeight: 600 }}>{r.attempt || r.attempt_number || 1}/{r.max_attempts || 3}</td>
                        <td style={{ ...tdStyle, fontSize: '0.8rem', color: T.muted }}>{r.retry_time ? formatDateTime(r.retry_time, orgTimezone) : '-'}</td>
                        <td style={tdStyle}>
                          <span style={{
                            padding: '3px 10px', borderRadius: 20, fontSize: '0.75rem', fontWeight: 600,
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
        <div style={{ ...card, overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Name','Phone','Source','Executive','Status','Action'].map(h => (
                  <th key={h} style={thStyle}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filteredLeads.length === 0 ? (
                <tr><td colSpan="6" style={{ ...tdStyle, textAlign: 'center', color: T.muted, padding: '2rem' }}>{(leadSearch.trim() || execFilter.length > 0) ? 'No leads match your filters.' : 'No leads in this campaign yet. Add some to start dialing!'}</td></tr>
              ) : filteredLeads.map(lead => (
                <React.Fragment key={lead.id}>
                  <tr>
                    <td style={{ ...tdStyle, fontWeight: 600, color: T.text }}>
                      <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 6 }}>
                        <span>{lead.first_name} {lead.last_name}</span>
                        {lead.follow_up_note && (
                          <span
                            title={lead.follow_up_note}
                            style={{
                              fontSize: 10, fontWeight: 600, fontFamily: T.font,
                              padding: '2px 8px', borderRadius: 12,
                              background: 'rgba(168,85,247,0.12)', color: '#6b21a8',
                              border: '1px solid rgba(168,85,247,0.25)',
                              cursor: 'help', maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                            }}
                          >
                            📝 {lead.follow_up_note.slice(0, 24)}{lead.follow_up_note.length > 24 ? '…' : ''}
                          </span>
                        )}
                      </div>
                    </td>
                    <td style={{ ...tdStyle, fontFamily: T.mono }}>{lead.phone}</td>
                    <td style={tdStyle}>
                      <select className="form-input" value={lead.source || ''}
                        onChange={async e => {
                          const src = e.target.value;
                          try {
                            await apiFetch(`${API_URL}/leads/${lead.id}/source`, {
                              method: 'PUT',
                              headers: { 'Content-Type': 'application/json' },
                              body: JSON.stringify({ source: src })
                            });
                            fetchCampaignLeads(selectedCampaign.id);
                          } catch (err) { toast('Failed to update source'); }
                        }}
                        style={{ ...inputStyle, height: 30, fontSize: '0.8rem', padding: '2px 8px', minWidth: 120, background: '#fff' }}>
                        <option value="">No Source</option>
                        {['facebook','google','instagram','linkedin','website','referral','cold'].map(s => (
                          <option key={s} value={s}>{s[0].toUpperCase() + s.slice(1)}</option>
                        ))}
                      </select>
                    </td>
                    <td style={tdStyle}>
                      <select className="form-input" value={lead.executive_id || ''}
                        onChange={async e => {
                          const execId = e.target.value ? parseInt(e.target.value, 10) : 0;
                          try {
                            await apiFetch(`${API_URL}/leads/${lead.id}/executive`, {
                              method: 'PUT',
                              headers: { 'Content-Type': 'application/json' },
                              body: JSON.stringify({ executive_id: execId })
                            });
                            fetchCampaignLeads(selectedCampaign.id);
                          } catch (err) { toast('Failed to assign executive'); }
                        }}
                        style={{ ...inputStyle, height: 30, fontSize: '0.8rem', padding: '2px 8px', minWidth: 120 }}>
                        <option value="">— Unassigned —</option>
                        {executives.map(e => <option key={e.id} value={e.id}>{e.name}</option>)}
                      </select>
                    </td>
                    <td style={tdStyle}>
                      <select className="form-input" value={lead.status || 'New'}
                        onChange={e => handleLeadStatusChange(lead.id, e.target.value)}
                        style={{ ...inputStyle, height: 30, fontSize: '0.8rem', padding: '2px 8px' }}>
                        {['New','Contacted','Connected','Interested','Not Interested','Qualified','Appointment Set','Converted','Lost','Junk'].map(s => <option key={s} value={s}>{s}</option>)}
                      </select>
                    </td>
                    <td style={tdStyle}>
                      <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap' }}>
                        <button
                          onClick={() => handleEditLead(lead)}
                          style={{ fontSize: 11, padding: '4px 10px', cursor: 'pointer', background: 'rgba(245,158,11,0.08)', color: '#92400e', border: '1px solid rgba(245,158,11,0.25)', borderRadius: 6, fontWeight: 600, fontFamily: T.font }}>
                          ✏️ Edit
                        </button>
                        {visibleCallActions.dial && (
                          <button
                            onClick={() => handleDialClick(lead)}
                            disabled={dialingId === lead.id || webCallActive === lead.id}
                            style={{
                              fontSize: 11, padding: '4px 10px', fontWeight: 600, fontFamily: T.font,
                              cursor: (dialingId === lead.id || webCallActive === lead.id) ? 'not-allowed' : 'pointer',
                              opacity: (dialingId === lead.id || webCallActive === lead.id) ? 0.5 : 1,
                              background: 'rgba(16,185,129,0.08)', color: '#065f46',
                              border: '1px solid rgba(16,185,129,0.25)', borderRadius: 6,
                            }}>
                            {dialingId === lead.id ? '📞 Wait...' : '📞 Dial'}
                          </button>
                        )}
                        {/* Manual Call disabled — use Browser Call instead
                        {selectedCampaign.channel !== 'whatsapp' && (
                          <button
                            onClick={() => { setHumanCallLead(lead); setHumanCallStatus('idle'); setHumanCallError(''); }}
                            style={{
                              fontSize: 11, padding: '4px 10px', fontWeight: 600, fontFamily: T.font,
                              cursor: 'pointer',
                              background: 'rgba(234,179,8,0.08)', color: '#854d0e',
                              border: '1px solid rgba(234,179,8,0.3)', borderRadius: 6,
                            }}>
                            📲 Manual Call
                          </button>
                        )} */}
                        {selectedCampaign.channel !== 'whatsapp' && visibleCallActions.browserCall && (
                          <button
                            onClick={() => startBrowserCallWithAutoDial(lead)}
                            disabled={browserCallDialing || browserCallLead != null}
                            title={autoDialEnabled ? 'Auto-dial is enabled' : 'Call from browser mic — 1x cost'}
                            style={{
                              fontSize: 11, padding: '4px 10px', fontWeight: 600, fontFamily: T.font,
                              cursor: (browserCallDialing || browserCallLead != null) ? 'not-allowed' : 'pointer',
                              opacity: (browserCallDialing || browserCallLead != null) ? 0.6 : 1,
                              background: autoDialEnabled ? 'rgba(245,158,11,0.12)' : 'rgba(99,102,241,0.08)',
                              color: autoDialEnabled ? '#b45309' : '#3730a3',
                              border: `1px solid ${autoDialEnabled ? 'rgba(245,158,11,0.35)' : 'rgba(99,102,241,0.3)'}`, borderRadius: 6,
                            }}>
                            {autoDialEnabled ? '⏩ Browser Call' : '🎙 Browser Call'}
                          </button>
                        )}
                        {selectedCampaign.channel === 'whatsapp' && (
                          <button
                            onClick={() => handleSendWA(lead)}
                            disabled={waSendingId === lead.id}
                            style={{
                              fontSize: 11, padding: '4px 10px', fontWeight: 600, fontFamily: T.font,
                              cursor: waSendingId === lead.id ? 'not-allowed' : 'pointer',
                              opacity: waSendingId === lead.id ? 0.6 : 1,
                              background: waSendStatus[lead.id] === 'sent' ? 'rgba(37,211,102,0.15)' : 'rgba(37,211,102,0.08)',
                              color: waSendStatus[lead.id] === 'error' ? '#dc2626' : '#065f46',
                              border: `1px solid ${waSendStatus[lead.id] === 'error' ? 'rgba(239,68,68,0.3)' : 'rgba(37,211,102,0.35)'}`,
                              borderRadius: 6,
                            }}>
                            {waSendingId === lead.id ? '⏳ Sending...' : waSendStatus[lead.id] === 'sent' ? '✅ Sent' : '💬 Send WA'}
                          </button>
                        )}
                        {visibleCallActions.simWebCall && (
                          <button
                            onClick={() => onCampaignWebCall(lead, selectedCampaign.id)}
                            disabled={webCallActive != null && webCallActive !== lead.id}
                            style={{
                              fontSize: 11, padding: '4px 10px', fontWeight: 600, fontFamily: T.font,
                              cursor: (webCallActive != null && webCallActive !== lead.id) ? 'not-allowed' : 'pointer',
                              opacity: (webCallActive != null && webCallActive !== lead.id) ? 0.5 : 1,
                              borderRadius: 6,
                              border: webCallActive === lead.id ? `1px solid rgba(239,68,68,0.3)` : `1px solid rgba(99,102,241,0.25)`,
                              color: webCallActive === lead.id ? T.red : T.accent,
                              background: webCallActive === lead.id ? 'rgba(239,68,68,0.08)' : 'rgba(99,102,241,0.08)',
                            }}>
                            {webCallActive === lead.id ? '🔴 End Call' : '🌐 Sim Web Call'}
                          </button>
                        )}
                        {dndBlockedLeadIds.has(lead.id) && (
                          <span title="This number is on the DND list — outbound dials are blocked"
                            style={{ fontSize: 11, padding: '4px 10px', borderRadius: 6,
                              background: '#fee2e2', color: T.red,
                              border: '1px solid #fca5a5', fontWeight: 600,
                              display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                            🚫 DND — number blocked
                          </span>
                        )}
                        <button
                          onClick={() => handleViewTranscripts(lead)}
                          style={{ fontSize: 11, padding: '4px 10px', cursor: 'pointer', fontFamily: T.font, borderRadius: 6, fontWeight: lead.transcript_count > 0 ? 600 : 400,
                            background: lead.transcript_count > 0 ? 'rgba(16,185,129,0.08)' : T.bg,
                            color: lead.transcript_count > 0 ? '#065f46' : T.muted,
                            border: lead.transcript_count > 0 ? '1px solid rgba(16,185,129,0.25)' : `1px solid ${T.border}`,
                          }}>
                          {lead.transcript_count > 0 ? `📋 ${lead.transcript_count} Transcript${lead.transcript_count > 1 ? 's' : ''}` : '📋 No Calls'}
                          {lead.recording_count > 0 && ' 🔊'}
                          {lead.dial_attempts > 0 && ` (${lead.dial_attempts} dial${lead.dial_attempts > 1 ? 's' : ''})`}
                        </button>
                        <button
                          onClick={() => openNoteModal(lead)}
                          style={{ fontSize: 11, padding: '4px 10px', cursor: 'pointer', background: 'rgba(168,85,247,0.08)', color: '#6b21a8', border: '1px solid rgba(168,85,247,0.25)', borderRadius: 6, fontWeight: 600, fontFamily: T.font }}>
                          📝 Note
                        </button>
                        <button
                          onClick={() => {
                            setScheduleLead(lead);
                            const d = new Date(Date.now() + 60 * 60 * 1000);
                            const pad = n => String(n).padStart(2, '0');
                            setScheduleAt(`${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`);
                            setScheduleNotes('');
                          }}
                          style={{ fontSize: 11, padding: '4px 10px', cursor: 'pointer', background: 'rgba(59,130,246,0.08)', color: '#1e40af', border: '1px solid rgba(59,130,246,0.25)', borderRadius: 6, fontWeight: 600, fontFamily: T.font }}>
                          📅 Schedule
                        </button>
                        <button onClick={() => handleRemoveLead(lead.id)}
                          style={{ fontSize: 11, padding: '4px 10px', cursor: 'pointer',
                            background: '#fee2e2', border: '1px solid #fca5a5',
                            color: T.red, borderRadius: 6, fontWeight: 600, fontFamily: T.font }}>
                          Remove
                        </button>
                        {lead.has_pending_scheduled_call && lead.next_scheduled_at && (
                          <span style={{
                            fontSize: 11, padding: '4px 10px', borderRadius: 6,
                            background: 'rgba(59,130,246,0.12)', color: '#1e40af',
                            border: '1px solid rgba(59,130,246,0.3)', fontWeight: 600,
                            fontFamily: T.font, whiteSpace: 'nowrap', display: 'inline-flex', alignItems: 'center', gap: 6
                          }}>
                            📅 {formatDateTime(lead.next_scheduled_at, orgTimezone)}
                            {lead.scheduled_call_id > 0 && (
                              <button
                                onClick={async (e) => {
                                  e.stopPropagation();
                                  try {
                                    const res = await apiFetch(`${API_URL}/scheduled-calls/${lead.scheduled_call_id}`, { method: 'DELETE' });
                                    if (!res.ok) throw new Error('Failed to dismiss scheduled call');
                                    toast('Scheduled call dismissed');
                                    fetchCampaignLeads(selectedCampaign.id);
                                    refreshScheduledCalls?.();
                                  } catch (err) {
                                    toast(err?.message || 'Dismiss failed');
                                  }
                                }}
                                title="Dismiss scheduled call"
                                style={{
                                  background: 'rgba(59,130,246,0.2)', border: 'none', borderRadius: 4,
                                  color: '#1e40af', cursor: 'pointer', fontSize: 10, lineHeight: 1,
                                  padding: '2px 4px', fontWeight: 700
                                }}>
                                ✕
                              </button>
                            )}
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                  {!hideAiFeatures && (lead.follow_up_note || editingNote?.leadId === lead.id || generatedNote?.leadId === lead.id) && (
                    <tr>
                      <td colSpan="6" style={{ padding: '12px 24px', background: 'rgba(99,102,241,0.04)', borderLeft: `3px solid ${T.accent}`, borderBottom: `1px solid ${T.border}` }}>
                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
                          <div style={{ fontSize: '0.8rem', color: T.muted, textTransform: 'uppercase', letterSpacing: '1px', fontWeight: 600 }}>✨ AI Follow-Up Note</div>
                          {editingNote?.leadId !== lead.id && (
                            <button
                              onClick={() => handleGenerateNote(lead)}
                              disabled={noteGenerating}
                              style={{
                                background: 'rgba(99,102,241,0.08)', border: `1px solid rgba(99,102,241,0.25)`,
                                color: T.accent, borderRadius: 6, padding: '3px 12px',
                                fontSize: '0.75rem', fontWeight: 600, cursor: noteGenerating ? 'wait' : 'pointer', fontFamily: T.font,
                              }}>
                              {noteGenerating ? '⏳ Generating…' : '↺ Regenerate'}
                            </button>
                          )}
                        </div>
                        {editingNote?.leadId === lead.id ? (
                          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                            <textarea
                              autoFocus
                              value={editingNote.text}
                              onChange={e => setEditingNote({ ...editingNote, text: e.target.value })}
                              rows={4}
                              style={{
                                width: '100%', padding: '8px 10px', borderRadius: 6,
                                border: `1px solid ${T.accent}`, fontSize: '0.85rem',
                                fontFamily: T.font, lineHeight: 1.5, resize: 'vertical',
                                outline: 'none', color: T.text, boxSizing: 'border-box',
                              }}
                            />
                            <div style={{ display: 'flex', gap: 8 }}>
                              <button onClick={() => handleSaveInlineNote(lead)} disabled={noteSaving} style={{
                                background: T.accent, color: '#fff', border: 'none', borderRadius: 6,
                                padding: '5px 16px', fontSize: '0.8rem', fontWeight: 600,
                                cursor: noteSaving ? 'wait' : 'pointer', fontFamily: T.font,
                              }}>{noteSaving ? 'Saving…' : 'Save'}</button>
                              <button onClick={() => setEditingNote(null)} style={{
                                background: 'transparent', color: T.muted, border: `1px solid ${T.border}`,
                                borderRadius: 6, padding: '5px 16px', fontSize: '0.8rem',
                                fontWeight: 600, cursor: 'pointer', fontFamily: T.font,
                              }}>Cancel</button>
                            </div>
                          </div>
                        ) : (() => {
                          const noteData = generatedNote?.leadId === lead.id ? generatedNote : null;
                          const noteText = noteData?.text || lead.follow_up_note;
                          return (
                            <div>
                              <div
                                onClick={() => setEditingNote({ leadId: lead.id, text: noteText, recordingUrl: noteData?.recordingUrl || '', recordingFilename: noteData?.recordingFilename || '' })}
                                title="Click to edit"
                                style={{
                                  whiteSpace: 'pre-wrap', color: T.sub, fontSize: '0.85rem', lineHeight: 1.5,
                                  cursor: 'text', padding: '4px 6px', borderRadius: 6, margin: '-4px -6px',
                                  border: '1px solid transparent',
                                }}
                                onMouseEnter={e => { e.currentTarget.style.border = `1px solid ${T.border}`; e.currentTarget.style.background = '#fff'; }}
                                onMouseLeave={e => { e.currentTarget.style.border = '1px solid transparent'; e.currentTarget.style.background = 'transparent'; }}
                              >{linkify(noteText)}</div>
                              {noteData?.recordingUrl && (
                                <div style={{ marginTop: 8, fontSize: '0.75rem' }}>
                                  <a href={noteData.recordingUrl} target="_blank" rel="noreferrer"
                                    style={{ color: T.accent, textDecoration: 'underline', wordBreak: 'break-all' }}
                                    onClick={e => e.stopPropagation()}>
                                    {noteData.recordingUrl}
                                  </a>
                                </div>
                              )}
                            </div>
                          );
                        })()}
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
        <div
          className="modal-overlay"
          onClick={e => { if (e.target === e.currentTarget) setScheduleLead(null); }}
        >
          <div style={{ background: '#fff', border: `1px solid ${T.border}`, borderRadius: 16, boxShadow: '0 8px 40px rgba(0,0,0,0.12)', maxWidth: 440, width: '100%', padding: '1.5rem', fontFamily: T.font }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
              <h3 style={{ margin: 0, color: T.text, fontSize: 18, fontWeight: 700 }}>📅 Schedule Call</h3>
              <button onClick={() => { setScheduleLead(null); setScheduleStatus({ kind: '', text: '' }); }}
                style={{ background: 'transparent', border: 'none', color: T.muted, fontSize: '1.2rem', cursor: 'pointer' }}>✕</button>
            </div>
            <p style={{ color: T.muted, fontSize: '0.85rem', marginBottom: '1.25rem' }}>
              {scheduleLead.first_name} {scheduleLead.last_name} — {scheduleLead.phone}
            </p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <label style={{ fontSize: '0.8rem', color: T.sub, fontWeight: 600 }}>
                Date &amp; Time
                <input
                  type="datetime-local"
                  className="form-input"
                  value={scheduleAt}
                  onChange={e => { setScheduleAt(e.target.value); if (scheduleStatus.kind) setScheduleStatus({ kind: '', text: '' }); }}
                  min={(() => {
                    const d = new Date();
                    const pad = n => String(n).padStart(2, '0');
                    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
                  })()}
                  style={{ ...inputStyle, width: '100%', marginTop: 6 }}
                />
              </label>
              <label style={{ fontSize: '0.8rem', color: T.sub, fontWeight: 600 }}>
                Notes (optional)
                <textarea
                  className="form-input"
                  value={scheduleNotes}
                  onChange={e => setScheduleNotes(e.target.value)}
                  rows={3}
                  placeholder="e.g. follow-up on pricing discussion"
                  style={{ ...inputStyle, width: '100%', marginTop: 6, resize: 'vertical' }}
                />
              </label>
            </div>
            {scheduleStatus.kind === 'error' && (
              <div style={{
                marginTop: '1rem', padding: '8px 12px', borderRadius: 8, fontSize: '0.8rem',
                background: '#fee2e2', border: '1px solid #fca5a5', color: T.red
              }}>
                ⚠️ {scheduleStatus.text}
              </div>
            )}
            <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: '1.25rem' }}>
              <button onClick={() => { setScheduleLead(null); setScheduleStatus({ kind: '', text: '' }); }}
                style={{ ...btnGhost }}>
                Cancel
              </button>
              <button
                style={{ ...btnPrimary, opacity: (scheduleSaving || !scheduleAt) ? 0.6 : 1 }}
                disabled={scheduleSaving || !scheduleAt}
                onClick={async () => {
                  if (!scheduleAt) return;
                  if (new Date(scheduleAt).getTime() <= Date.now()) {
                    setScheduleStatus({ kind: 'error', text: 'Please pick a future date and time.' });
                    return;
                  }
                  setScheduleStatus({ kind: '', text: '' });
                  setScheduleSaving(true);
                  try {
                    const serverTime = new Date(scheduleAt).toISOString();
                    const res = await apiFetch(`${API_URL}/scheduled-calls`, {
                      method: 'POST',
                      headers: {'Content-Type': 'application/json'},
                      body: JSON.stringify({
                        lead_id: scheduleLead.id,
                        campaign_id: selectedCampaign.id,
                        scheduled_at: serverTime,
                        notes: scheduleNotes,
                        mode: 'manual',
                        executive_id: scheduleLead.executive_id || null,
                      }),
                    });
                    if (!res.ok) {
                      const data = await res.json().catch(() => ({}));
                      setScheduleStatus({ kind: 'error',
                        text: 'Failed to schedule: ' + (data.error || data.detail || res.status) });
                    } else {
                      const data = await res.json().catch(() => ({}));
                      setScheduleLead(null);
                      setScheduleStatus({ kind: '', text: '' });
                      toast('Call scheduled');
                      fetchCampaignLeads(selectedCampaign.id);
                      refreshScheduledCalls?.();
                      if (data.id) clearDismissedScheduledCall?.(data.id);
                    }
                  } catch { setScheduleStatus({ kind: 'error', text: 'Network error while scheduling.'  });
                  }
                  setScheduleSaving(false);
                }}>
                {scheduleSaving ? 'Scheduling…' : 'Schedule Call'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Human Call Modal */}
      {/* Browser Call Modal — Twilio WebRTC (zero delay) [disabled] */}
      {/* {twilioBrowserLead && (
        <TwilioBrowserCallModal
          lead={twilioBrowserLead}
          campaignId={selectedCampaign.id}
          callerPhone={orgExotelAccounts.find(a => String(a.id) === selectedExotelAccountId)?.caller_id || ''}
          onClose={() => setTwilioBrowserLead(null)}
        />
      )} */}

      {humanCallLead && (
        <div
          className="modal-overlay"
          onClick={e => { if (e.target === e.currentTarget) { setHumanCallLead(null); setHumanCallStatus('idle'); } }}
        >
          <div style={{ background: '#fff', border: `1px solid ${T.border}`, borderRadius: 16, boxShadow: '0 8px 40px rgba(0,0,0,0.12)', maxWidth: 420, width: '100%', padding: '1.5rem', fontFamily: T.font }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
              <h3 style={{ margin: 0, color: T.text, fontSize: 18, fontWeight: 700 }}>📲 Manual Call</h3>
              <button onClick={() => { setHumanCallLead(null); setHumanCallStatus('idle'); }}
                style={{ background: 'transparent', border: 'none', color: T.muted, fontSize: '1.2rem', cursor: 'pointer' }}>✕</button>
            </div>
            <p style={{ color: T.muted, fontSize: '0.85rem', marginBottom: '1.25rem' }}>
              Calling <strong>{humanCallLead.first_name} {humanCallLead.last_name}</strong> — {humanCallLead.phone}
            </p>
            <p style={{ color: T.sub, fontSize: '0.8rem', marginBottom: '1rem', lineHeight: 1.5 }}>
              Exotel will call <strong>your phone</strong> first. Pick up and you&apos;ll hear the customer&apos;s name announced, then be connected to them.
            </p>
            <label style={{ fontSize: '0.8rem', color: T.sub, fontWeight: 600 }}>
              Your phone number
              <input
                type="tel"
                className="form-input"
                value={humanCallPhone}
                onChange={e => setHumanCallPhone(e.target.value)}
                placeholder="+91XXXXXXXXXX"
                style={{ ...inputStyle, width: '100%', marginTop: 6 }}
                onKeyDown={e => e.key === 'Enter' && handleHumanCallDial()}
              />
            </label>
            {humanCallStatus === 'error' && (
              <div style={{ marginTop: '0.75rem', padding: '8px 12px', borderRadius: 8, fontSize: '0.8rem', background: '#fee2e2', border: '1px solid #fca5a5', color: T.red }}>
                ⚠️ {humanCallError}
              </div>
            )}
            {humanCallStatus === 'done' && (
              <div style={{ marginTop: '0.75rem', padding: '8px 12px', borderRadius: 8, fontSize: '0.8rem', background: 'rgba(16,185,129,0.08)', border: '1px solid rgba(16,185,129,0.25)', color: '#065f46' }}>
                ✅ Dialing your phone…
              </div>
            )}
            <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: '1.25rem' }}>
              <button onClick={() => { setHumanCallLead(null); setHumanCallStatus('idle'); }}
                style={{ ...btnGhost }}>
                Cancel
              </button>
              <button
                disabled={humanCallStatus === 'dialing' || humanCallStatus === 'done' || !humanCallPhone.trim()}
                onClick={handleHumanCallDial}
                style={{ ...btnPrimary, opacity: (humanCallStatus === 'dialing' || humanCallStatus === 'done' || !humanCallPhone.trim()) ? 0.6 : 1 }}>
                {humanCallStatus === 'dialing' ? '📞 Dialing…' : '📞 Call Me'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Quick Note Modal */}
      {noteModalLead && (
        <div className="modal-overlay" onClick={() => setNoteModalLead(null)}>
          <div className="glass-panel modal-content" onClick={e => e.stopPropagation()} style={{maxWidth: '520px'}}>
            <h2 style={{marginTop: 0, marginBottom: '0.5rem'}}>📝 Quick Note</h2>
            <p style={{color: '#94a3b8', fontSize: '0.85rem', marginBottom: '1.5rem'}}>
              {noteModalLead.first_name} {noteModalLead.last_name} — {noteModalLead.phone}
            </p>
            <textarea className="form-input" rows={5} value={noteModalText}
              onChange={e => setNoteModalText(e.target.value)}
              placeholder="Type your follow-up note here..."
              style={{width: '100%', minHeight: '120px', resize: 'vertical', fontSize: '0.9rem', lineHeight: 1.5}} />
            <div style={{display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '1.5rem'}}>
              <button onClick={() => setNoteModalLead(null)}
                style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', color: '#cbd5e1', padding: '8px 18px', borderRadius: '8px', cursor: 'pointer'}}>
                Cancel
              </button>
              <button className="btn-primary" onClick={handleSaveNoteModal}
                disabled={noteModalSaving || !noteModalText.trim()}
                style={{opacity: (noteModalSaving || !noteModalText.trim()) ? 0.5 : 1, cursor: (noteModalSaving || !noteModalText.trim()) ? 'not-allowed' : 'pointer'}}>
                {noteModalSaving ? 'Saving…' : 'Save Note'}
              </button>
            </div>
          </div>
        </div>
      )}

    </div>
  );
}
