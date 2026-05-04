import React, { useState, useEffect } from 'react';
import CampaignsTab from '../components/tabs/CampaignsTab';
import TranscriptModal from '../components/modals/TranscriptModal';
import { useToast } from '../contexts/ToastContext';

export default function CampaignsPage({
  apiFetch, API_URL, selectedOrg, orgTimezone, orgProducts,
  dialingId, webCallActive,
  handleCampaignDial, handleCampaignWebCall,
  activeVoiceProvider, activeVoiceId, activeLanguage,
  INDIAN_VOICES, INDIAN_LANGUAGES,
  campaigns, fetchCampaigns
}) {
  // Leads for adding to campaigns (the global leads pool)
  const [leads, setLeads] = useState([]);

  // Note state (for campaign lead notes)
  const [noteLead, setNoteLead] = useState(null);
  const [noteText, setNoteText] = useState('');
  const [noteSaving, setNoteSaving] = useState(false);
  const [noteError, setNoteError] = useState('');

  // Transcript state (for campaign lead transcripts)
  const [transcriptLead, setTranscriptLead] = useState(null);
  const [transcripts, setTranscripts] = useState([]);
  const { showToast } = useToast();

  useEffect(() => {
    fetchLeads();
  }, []);

  const fetchLeads = async () => {
    try {
      const res = await apiFetch(`${API_URL}/leads`);
      setLeads(await res.json());
    } catch(e) {}
  };

  const handleViewTranscripts = async (lead) => {
    setTranscriptLead(lead);
    try {
      const res = await apiFetch(`${API_URL}/leads/${lead.id}/transcripts`);
      setTranscripts(await res.json());
    } catch(e) { setTranscripts([]); }
  };

  const handleNote = (lead) => {
    setNoteLead(lead);
    setNoteText(lead.follow_up_note || '');
    setNoteError('');
  };

  const handleSaveNote = async () => {
    if (!noteLead) return;
    if (!noteText.trim()) { setNoteError('Note cannot be empty.'); return; }
    setNoteSaving(true);
    setNoteError('');
    try {
      const res = await apiFetch(`${API_URL}/leads/${noteLead.id}/notes`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ note: noteText.trim() })
      });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        showToast(d.detail || `Save failed (${res.status})`, 'error');
      } else {
        setNoteLead(null);
        showToast('Note saved');
      }
    } catch(e) {
      showToast('Network error — note not saved', 'error');
    }
    setNoteSaving(false);
  };

  return (
    <>
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

      <TranscriptModal
        transcriptLead={transcriptLead} setTranscriptLead={setTranscriptLead}
        transcripts={transcripts} orgTimezone={orgTimezone}
      />

      {/* Note Modal for campaign leads */}
      {noteLead && (
        <div className="modal-overlay" onClick={() => setNoteLead(null)}>
          <div className="glass-panel modal-content" onClick={e => e.stopPropagation()} style={{maxWidth: '520px'}}>
            <h2 style={{marginTop: 0, marginBottom: '0.5rem'}}>📝 Quick Note</h2>
            <p style={{color: '#94a3b8', fontSize: '0.85rem', marginBottom: '1.5rem'}}>
              {noteLead.first_name} {noteLead.last_name} — {noteLead.phone}
            </p>
            <textarea className="form-input" rows={5} value={noteText}
              onChange={e => { setNoteText(e.target.value); if (noteError) setNoteError(''); }}
              placeholder="Type your follow-up note here..."
              style={{width: '100%', minHeight: '120px', resize: 'vertical', fontSize: '0.9rem', lineHeight: 1.5,
                borderColor: noteError ? 'rgba(239,68,68,0.5)' : undefined}} />
            {noteError && (
              <p style={{color: '#f87171', fontSize: '0.8rem', margin: '6px 0 0'}}>⚠ {noteError}</p>
            )}
            <div style={{display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '1.5rem'}}>
              <button onClick={() => setNoteLead(null)}
                style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', color: '#cbd5e1', padding: '8px 18px', borderRadius: '8px', cursor: 'pointer'}}>
                Cancel
              </button>
              <button className="btn-primary" onClick={handleSaveNote}
                disabled={noteSaving || !noteText.trim()}
                style={{opacity: (noteSaving || !noteText.trim()) ? 0.5 : 1, cursor: (noteSaving || !noteText.trim()) ? 'not-allowed' : 'pointer'}}>
                {noteSaving ? 'Saving…' : 'Save Note'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
