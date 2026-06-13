import React, { useState, useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useToast } from '../../contexts/UIContext';
import CampaignDetail from '../campaigns/CampaignDetail';
import CampaignModals from '../campaigns/CampaignModals';
import { CAMPAIGN_TEMPLATES } from '../../constants/campaignTemplates';
import { useAuth } from '../../contexts/AuthContext';

export default function CampaignsTab({
  campaigns, fetchCampaigns, orgProducts, leads,
  apiFetch, API_URL,
  onCampaignDial, onCampaignWebCall,
  handleViewTranscripts, handleNote,
  INDIAN_VOICES, INDIAN_LANGUAGES,
  dialingId, webCallActive, orgTimezone
}) {
  const { fetchSseTicket } = useAuth();
  const toast = useToast();
  const location = useLocation();
  const navigate = useNavigate();
  const [view, setView] = useState('list'); // 'list' or 'detail'
  const [selectedCampaign, setSelectedCampaign] = useState(null);
  const [campaignLeads, setCampaignLeads] = useState([]);
  const [callLog, setCallLog] = useState([]);
  const [detailTab, setDetailTab] = useState('leads'); // 'leads' or 'calllog'
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showAddLeadsModal, setShowAddLeadsModal] = useState(false);
  const [editLead, setEditLead] = useState(null);
  const [editForm, setEditForm] = useState({ first_name: '', last_name: '', phone: '', source: '' });
  const [createForm, setCreateForm] = useState({ name: '', product_id: '', lead_source: '', channel: 'voice', exotel_account_id: '' });
  const [orgExotelAccounts, setOrgExotelAccounts] = useState([]);
  const [selectedLeadIds, setSelectedLeadIds] = useState([]);
  const [loading, setLoading] = useState(false);
  const [showCsvImportModal, setShowCsvImportModal] = useState(false);
  const [csvFile, setCsvFile] = useState(null);
  const [liveEvents, setLiveEvents] = useState([]);
  const [showEditCampaignModal, setShowEditCampaignModal] = useState(false);
  const [editCampaignForm, setEditCampaignForm] = useState({ name: '', product_id: '', lead_source: '' });
  // ID of the campaign whose row is currently showing the inline "Delete? Yes No"
  // prompt. Null when no row is in confirm mode.
  const [deleteConfirmId, setDeleteConfirmId] = useState(null);
  const [deleting, setDeleting] = useState(false);
  const [selectedTemplate, setSelectedTemplate] = useState(null);
  const [createError, setCreateError] = useState('');
  const [editCampaignError, setEditCampaignError] = useState('');
  const eventSourceRef = React.useRef(null);
  const [campVoice, setCampVoice] = useState({ tts_provider: '', tts_voice_id: '', tts_language: '' });
  const [campVoiceSaveStatus, setCampVoiceSaveStatus] = useState(''); // '', 'saving', 'saved', 'error'

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    fetchCampaigns();
    apiFetch(`${API_URL}/exotel-accounts`)
      .then(d => setOrgExotelAccounts(Array.isArray(d) ? d : []))
      .catch(() => {});
  }, []);

  // Open a specific campaign's detail directly when ?id=N is in the URL —
  // lets the CRM dashboard's "Active Campaigns" cards navigate straight into
  // the right campaign instead of dropping the user on the list (issue #40).
  // Runs whenever the campaigns array refreshes so we can resolve the id
  // once the list has loaded; clears the param afterwards so a Back-to-list
  // doesn't keep re-opening the same campaign.
  useEffect(() => {
    if (view === 'detail') return;
    if (!Array.isArray(campaigns) || campaigns.length === 0) return;
    const params = new URLSearchParams(window.location.search);
    const idStr = params.get('id');
    if (!idStr) return;
    const id = parseInt(idStr, 10);
    if (!Number.isFinite(id)) return;
    const target = campaigns.find(c => c.id === id);
    if (!target) return;
    handleViewCampaign(target);
    // Strip ?id= from the URL so refreshes / Back don't loop.
    window.history.replaceState({}, '', window.location.pathname);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [campaigns, view]);

  // Auto-open a specific campaign when navigated from the CRM dashboard.
  // After opening, clear the navigation state so that clicking "Back to Campaigns"
  // shows the list without re-triggering this effect.
  useEffect(() => {
    const openId = location.state?.openCampaignId;
    if (!openId || !campaigns?.length) return;
    const target = campaigns.find(c => c.id === openId);
    if (target) {
      handleViewCampaign(target);
      navigate('/campaigns', { replace: true, state: {} });
    }
  }, [location.state?.openCampaignId, campaigns]);

  const fetchCampaignLeads = async (campaignId) => {
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${campaignId}/leads`);
      if (!res.ok) { setCampaignLeads([]); return; }
      const data = await res.json();
      setCampaignLeads(Array.isArray(data) ? data : []);
    } catch { setCampaignLeads([]);  }
  };

  const fetchCallLog = async (campaignId) => {
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${campaignId}/call-log`);
      if (!res.ok) { setCallLog([]); return; }
      const data = await res.json();
      setCallLog(Array.isArray(data) ? data : []);
    } catch { setCallLog([]);  }
  };

  const fetchCampVoice = async (campaignId) => {
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${campaignId}/voice-settings`);
      if (res.ok) {
        const data = await res.json();
        setCampVoice({ tts_provider: data.tts_provider || '', tts_voice_id: data.tts_voice_id || '', tts_language: data.tts_language || '' });
      } else {
        setCampVoice({ tts_provider: '', tts_voice_id: '', tts_language: '' });
      }
    } catch { setCampVoice({ tts_provider: '', tts_voice_id: '', tts_language: ''  }); }
  };

  const handleSaveCampVoice = async () => {
    if (!selectedCampaign) return;
    setCampVoiceSaveStatus('saving');
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/voice-settings`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tts_provider: campVoice.tts_provider, tts_voice_id: campVoice.tts_voice_id, tts_language: campVoice.tts_language })
      });
      if (!res.ok) {
        setCampVoiceSaveStatus('error');
        setTimeout(() => setCampVoiceSaveStatus(''), 3000);
        return;
      }
      setCampVoiceSaveStatus('saved');
      setTimeout(() => setCampVoiceSaveStatus(''), 2000);
    } catch { setCampVoiceSaveStatus('error');
      setTimeout(() => setCampVoiceSaveStatus(''), 3000);
     }
  };

  const handleResetCampVoice = async () => {
    if (!selectedCampaign) return;
    await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/voice-settings`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tts_provider: '', tts_voice_id: '', tts_language: '' })
    });
    setCampVoice({ tts_provider: '', tts_voice_id: '', tts_language: '' });
  };

  const handleViewCampaign = (campaign) => {
    setSelectedCampaign(campaign);
    setView('detail');
    fetchCampaignLeads(campaign.id);
    fetchCallLog(campaign.id);
    fetchCampVoice(campaign.id);
    startEventStream(campaign.id).catch(() => {});
    setDetailTab('leads');
  };

  const handleBack = () => {
    stopEventStream();
    setView('list');
    setSelectedCampaign(null);
    setCampaignLeads([]);
    setLiveEvents([]);
    fetchCampaigns();
  };

  const startEventStream = async (campaignId) => {
    stopEventStream();
    let ticket;
    try { ticket = await fetchSseTicket(); } catch { return;  }
    const es = new EventSource(`${API_URL}/campaign-events?ticket=${encodeURIComponent(ticket)}&campaign_id=${campaignId}`);
    es.onmessage = (e) => {
      // Backend publishes a JSON envelope with a pre-formatted `label` field;
      // legacy events arrive as plain strings, so fall back to the raw line
      // when JSON parse fails or no label is present.
      let display = e.data;
      let ts = Date.now();
      try {
        const j = JSON.parse(e.data);
        if (j && typeof j.label === 'string') display = j.label;
        if (j && j.ts) {
          const parsed = new Date(j.ts).getTime();
          if (!Number.isNaN(parsed)) ts = parsed;
        }
      } catch { /* plain-text legacy event */  }
      // Drop replayed events older than the user's last Clear timestamp for
      // this campaign — the backend replays the last 20 events from Redis on
      // every SSE connect, so without this filter a page reload would
      // resurrect everything the user just cleared.
      const clearedAt = parseInt(localStorage.getItem(`liveEventsClearedAt:${campaignId}`) || '0', 10);
      if (clearedAt > 0 && ts <= clearedAt) return;
      setLiveEvents(prev => [...prev.slice(-49), { ts, label: display }]);
    };
    // Don't call es.close() here — that prevents EventSource's built-in
    // auto-reconnect. Cloudflare/nginx idle-timeout the SSE stream after
    // ~30s of silence; we want the browser to transparently re-open so new
    // call events still appear in the panel after the user clicks Clear or
    // simply waits idle for a while.
    es.onerror = (e) => {
      // Native EventSource will set readyState to CLOSED only when the
      // server explicitly returns a non-200; CONNECTING means a retry is
      // already in flight. Just log so we can see it in DevTools.
       
      console.warn('campaign-events SSE error; readyState=', es.readyState, e);
    };
    eventSourceRef.current = es;
  };

  const stopEventStream = () => {
    if (eventSourceRef.current) { eventSourceRef.current.close(); eventSourceRef.current = null; }
  };

  const handleCreateCampaign = async (e) => {
    e.preventDefault();
    if (!createForm.name.trim()) return;
    setLoading(true);
    setCreateError('');
    try {
      const res = await apiFetch(`${API_URL}/campaigns`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: createForm.name.trim(),
          product_id: createForm.product_id ? parseInt(createForm.product_id) : null,
          lead_source: createForm.lead_source || null,
          channel: createForm.channel || 'voice',
          exotel_account_id: createForm.exotel_account_id ? parseInt(createForm.exotel_account_id) : null,
        })
      });

      let newCampaign;
      try { newCampaign = await res.json(); } catch { newCampaign = {}; }

      if (!res.ok) {
        const msg = newCampaign?.detail || `Server error (${res.status})`;
        setCreateError(typeof msg === 'string' ? msg : JSON.stringify(msg));
        return;
      }

      // Apply template settings if one was selected
      if (selectedTemplate && newCampaign?.id) {
        await apiFetch(`${API_URL}/campaigns/${newCampaign.id}/voice-settings`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            tts_provider: selectedTemplate.tts_provider,
            tts_voice_id: selectedTemplate.tts_voice_id,
            tts_language: selectedTemplate.language
          })
        });

        const productId = createForm.product_id || newCampaign.product_id;
        if (productId) {
          await apiFetch(`${API_URL}/products/${productId}/prompt`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              agent_persona: selectedTemplate.agent_persona,
              call_flow_instructions: selectedTemplate.call_flow_instructions
            })
          });
        }
      }

      setCreateForm({ name: '', product_id: '', lead_source: '', channel: 'voice' });
      setSelectedTemplate(null);
      setCreateError('');
      setShowCreateModal(false);
      fetchCampaigns();
    } catch (err) {
      console.error(err);
      setCreateError('Network error — please try again.');
    } finally {
      setLoading(false);
    }
  };

  const confirmDeleteCampaign = async (campaignId) => {
    if (deleting) return;
    setDeleting(true);
    try {
      await apiFetch(`${API_URL}/campaigns/${campaignId}`, { method: 'DELETE' });
      setDeleteConfirmId(null);
      fetchCampaigns();
    } catch (e) {
      console.error(e);
    } finally {
      setDeleting(false);
    }
  };

  const handleEditCampaign = (campaign) => {
    setEditCampaignForm({
      id: campaign.id,
      name: campaign.name || '',
      product_id: campaign.product_id || '',
      lead_source: campaign.lead_source || '',
      channel: campaign.channel || 'voice'
    });
    setEditCampaignError('');
    setShowEditCampaignModal(true);
  };

  const handleSaveEditCampaign = async (e) => {
    e.preventDefault();
    if (!editCampaignForm.name.trim()) { setEditCampaignError('Campaign name is required.'); return; }
    setLoading(true);
    try {
      await apiFetch(`${API_URL}/campaigns/${editCampaignForm.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: editCampaignForm.name.trim(),
          product_id: editCampaignForm.product_id ? parseInt(editCampaignForm.product_id) : null,
          lead_source: editCampaignForm.lead_source || null,
          channel: editCampaignForm.channel || 'voice'
        })
      });
      setShowEditCampaignModal(false);
      fetchCampaigns();
      if (selectedCampaign?.id === editCampaignForm.id) {
        setSelectedCampaign(prev => ({
          ...prev,
          name: editCampaignForm.name.trim(),
          product_id: editCampaignForm.product_id ? parseInt(editCampaignForm.product_id) : prev.product_id,
          lead_source: editCampaignForm.lead_source || null,
          channel: editCampaignForm.channel || 'voice'
        }));
      }
      toast('Campaign updated');
    } catch (e) { console.error(e); toast('Failed to update campaign', 'error'); }
    setLoading(false);
  };

  const handleAddLeads = async () => {
    if (selectedLeadIds.length === 0) return;
    setLoading(true);
    try {
      await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/leads`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ lead_ids: selectedLeadIds })
      });
      setSelectedLeadIds([]);
      setShowAddLeadsModal(false);
      fetchCampaignLeads(selectedCampaign.id);
      fetchCampaigns();
    } catch (e) { console.error(e); }
    setLoading(false);
  };

  const handleRemoveLead = async (leadId) => {
    try {
      await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/leads/${leadId}`, { method: 'DELETE' });
      fetchCampaignLeads(selectedCampaign.id);
      fetchCampaigns();
    } catch (e) { console.error(e); }
  };

  const handleEditLead = (lead) => {
    setEditLead(lead);
    setEditForm({ first_name: lead.first_name || '', last_name: lead.last_name || '', phone: lead.phone || '', source: lead.source || '' });
  };

  const handleSaveEdit = async () => {
    if (!editLead) return;
    try {
      await apiFetch(`${API_URL}/leads/${editLead.id}`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(editForm)
      });
      setEditLead(null);
      fetchCampaignLeads(selectedCampaign.id);
    } catch { toast('Save failed');  }
  };

  const handleLeadStatusChange = async (leadId, newStatus) => {
    try {
      await apiFetch(`${API_URL}/leads/${leadId}/status`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: newStatus })
      });
      fetchCampaignLeads(selectedCampaign.id);
    } catch (e) { console.error(e); }
  };

  const toggleLeadSelection = (leadId) => {
    setSelectedLeadIds(prev =>
      prev.includes(leadId) ? prev.filter(id => id !== leadId) : [...prev, leadId]
    );
  };

  const statusBadge = (status) => {
    const colors = { active: '#22c55e', paused: '#eab308', completed: '#6b7280' };
    const bg = { active: 'rgba(34,197,94,0.15)', paused: 'rgba(234,179,8,0.15)', completed: 'rgba(107,114,128,0.15)' };
    return (
      <span style={{
        padding: '2px 10px', borderRadius: '12px', fontSize: '0.75rem', fontWeight: 600,
        color: colors[status] || '#94a3b8', background: bg[status] || 'rgba(148,163,184,0.15)'
      }}>
        {status}
      </span>
    );
  };

  const getProductName = (productId) => {
    const p = orgProducts.find(p => p.id === productId);
    return p ? p.name : '';
  };

  const getCampaignStats = (campaign) => {
    const s = campaign.stats || {};
    return { total: s.total || 0, called: s.called || 0, qualified: s.qualified || 0, booked: s.appointments || 0 };
  };

  const handleCsvImport = async () => {
    if (!csvFile || !selectedCampaign) return;
    setLoading(true);
    try {
      const formData = new FormData();
      formData.append('file', csvFile);
      const res = await apiFetch(`${API_URL}/campaigns/${selectedCampaign.id}/import-csv`, {
        method: 'POST', body: formData
      });
      const data = await res.json();
      toast(`Imported ${data.imported} leads, ${data.added_to_campaign} added to campaign.${data.errors?.length ? '\nErrors: ' + data.errors.join(', ') : ''}`);
      setCsvFile(null);
      setShowCsvImportModal(false);
      fetchCampaignLeads(selectedCampaign.id);
      fetchCampaigns();
    } catch (e) { console.error(e); }
    setLoading(false);
  };

  // Available leads = org leads not already in this campaign
  const availableLeads = leads.filter(l => !campaignLeads.some(cl => cl.id === l.id));

  // ─── DETAIL VIEW ───
  if (view === 'detail' && selectedCampaign) {
    return (
      <>
        <CampaignDetail
          selectedCampaign={selectedCampaign}
          setSelectedCampaign={setSelectedCampaign}
          campaignLeads={campaignLeads}
          callLog={callLog}
          detailTab={detailTab}
          setDetailTab={setDetailTab}
          handleBack={handleBack}
          fetchCampaignLeads={fetchCampaignLeads}
          fetchCallLog={fetchCallLog}
          fetchCampaigns={fetchCampaigns}
          statusBadge={statusBadge}
          getProductName={getProductName}
          getCampaignStats={getCampaignStats}
          campVoice={campVoice}
          setCampVoice={setCampVoice}
          handleSaveCampVoice={handleSaveCampVoice}
          handleResetCampVoice={handleResetCampVoice}
          campVoiceSaveStatus={campVoiceSaveStatus}
          INDIAN_VOICES={INDIAN_VOICES}
          INDIAN_LANGUAGES={INDIAN_LANGUAGES}
          liveEvents={liveEvents}
          setLiveEvents={setLiveEvents}
          handleLeadStatusChange={handleLeadStatusChange}
          handleEditLead={handleEditLead}
          handleRemoveLead={handleRemoveLead}
          handleViewTranscripts={handleViewTranscripts}
          handleNote={handleNote}
          onCampaignDial={onCampaignDial}
          onCampaignWebCall={onCampaignWebCall}
          dialingId={dialingId}
          webCallActive={webCallActive}
          setSelectedLeadIds={setSelectedLeadIds}
          setShowAddLeadsModal={setShowAddLeadsModal}
          setShowCsvImportModal={setShowCsvImportModal}
          setCsvFile={setCsvFile}
          apiFetch={apiFetch}
          API_URL={API_URL}
          orgTimezone={orgTimezone}
          handleEditCampaign={handleEditCampaign}
        />
        <CampaignModals
          showCreateModal={false}
          setShowCreateModal={setShowCreateModal}
          createForm={createForm}
          setCreateForm={setCreateForm}
          handleCreateCampaign={handleCreateCampaign}
          loading={loading}
          orgProducts={orgProducts}
          orgExotelAccounts={orgExotelAccounts}
          selectedTemplate={selectedTemplate}
          setSelectedTemplate={setSelectedTemplate}
          showAddLeadsModal={showAddLeadsModal}
          setShowAddLeadsModal={setShowAddLeadsModal}
          availableLeads={availableLeads}
          selectedLeadIds={selectedLeadIds}
          toggleLeadSelection={toggleLeadSelection}
          handleAddLeads={handleAddLeads}
          showCsvImportModal={showCsvImportModal}
          setShowCsvImportModal={setShowCsvImportModal}
          csvFile={csvFile}
          setCsvFile={setCsvFile}
          handleCsvImport={handleCsvImport}
          editLead={editLead}
          setEditLead={setEditLead}
          editForm={editForm}
          setEditForm={setEditForm}
          handleSaveEdit={handleSaveEdit}
          showEditCampaignModal={showEditCampaignModal}
          setShowEditCampaignModal={setShowEditCampaignModal}
          editCampaignForm={editCampaignForm}
          setEditCampaignForm={setEditCampaignForm}
          handleSaveEditCampaign={handleSaveEditCampaign}
          editCampaignError={editCampaignError}
          setEditCampaignError={setEditCampaignError}
        />
      </>
    );
  }

  // ─── LIST VIEW ───
  const cardStyle = {
    background: '#fff', border: '1px solid #e5e7eb',
    borderRadius: 14, boxShadow: '0 1px 4px rgba(0,0,0,0.05)',
    padding: '20px 22px', display: 'flex', flexDirection: 'column', gap: 12,
  };
  const smallBtn = (bg, color, border) => ({
    padding: '5px 14px', borderRadius: 8, border: `1px solid ${border}`,
    background: bg, color, fontSize: 12, fontWeight: 600,
    cursor: 'pointer', fontFamily: 'inherit',
  });

  return (
    <div style={{ padding: '28px 32px', background: '#f4f5f9', minHeight: '100%' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: '#111827', display: 'flex', alignItems: 'center', gap: 10 }}>
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#6366f1" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/><path d="M15.54 8.46a5 5 0 010 7.07"/></svg>
          Campaigns
        </h2>
        <button
          onClick={() => setShowCreateModal(true)}
          style={{ background: '#6366f1', border: 'none', color: '#fff', borderRadius: 8, padding: '8px 20px', fontWeight: 700, fontSize: 13, cursor: 'pointer', fontFamily: 'inherit' }}>
          + Create Campaign
        </button>
      </div>

      {campaigns.length === 0 ? (
        <div style={{ ...cardStyle, textAlign: 'center', padding: '3rem', color: '#9ca3af' }}>
          No campaigns yet. Create one to start dialing!
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 16 }}>
          {campaigns.map(campaign => {
            const stats = getCampaignStats(campaign);
            const calledPct = stats.total > 0 ? Math.round((stats.called / stats.total) * 100) : 0;
            const typeColor = campaign.channel === 'whatsapp' ? '#25D366' : '#6366f1';
            return (
              <div key={campaign.id} style={cardStyle}>
                {/* Card header: name + edit/delete */}
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <div style={{ fontSize: 15, fontWeight: 700, color: '#111827', wordBreak: 'break-word', flex: 1, marginRight: 10 }}>
                    {campaign.name}
                  </div>
                  <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
                    {deleteConfirmId === campaign.id ? (
                      <>
                        <span style={{ color: '#ef4444', fontSize: 12, alignSelf: 'center', fontWeight: 600 }}>Delete?</span>
                        <button onClick={(e) => { e.stopPropagation(); confirmDeleteCampaign(campaign.id); }}
                          disabled={deleting}
                          style={smallBtn('#fee2e2', '#ef4444', '#fca5a5')}>
                          {deleting ? '…' : 'Yes'}
                        </button>
                        <button onClick={(e) => { e.stopPropagation(); setDeleteConfirmId(null); }}
                          disabled={deleting}
                          style={smallBtn('#fff', '#6b7280', '#e5e7eb')}>
                          No
                        </button>
                      </>
                    ) : (
                      <>
                        <button onClick={(e) => { e.stopPropagation(); handleEditCampaign(campaign); }}
                          style={smallBtn('#fff', '#374151', '#e5e7eb')}>
                          Edit
                        </button>
                        <button onClick={(e) => { e.stopPropagation(); setDeleteConfirmId(campaign.id); }}
                          style={smallBtn('#fee2e2', '#ef4444', '#fca5a5')}>
                          Delete
                        </button>
                      </>
                    )}
                  </div>
                </div>

                {/* Badges */}
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: typeColor, background: `${typeColor}18` }}>
                    {campaign.channel === 'whatsapp' ? 'WhatsApp' : 'Voice'}
                  </span>
                  {campaign.product_id > 0 ? (
                    <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: '#0891b2', background: 'rgba(8,145,178,0.1)' }}>
                      {getProductName(campaign.product_id)}
                    </span>
                  ) : (
                    <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: '#f59e0b', background: 'rgba(245,158,11,0.1)' }}>
                      ⚠ No product
                    </span>
                  )}
                  {statusBadge(campaign.status || 'active')}
                </div>

                {/* Stats */}
                <div style={{ display: 'flex', gap: 24 }}>
                  {[
                    { label: 'Total',     val: stats.total,     color: '#111827' },
                    { label: 'Called',    val: stats.called,    color: '#111827' },
                    { label: 'Qualified', val: stats.qualified, color: '#10b981' },
                    { label: 'Booked',    val: stats.booked,    color: '#6366f1' },
                  ].map(({ label, val, color }) => (
                    <div key={label}>
                      <span style={{ fontSize: 12, color: '#9ca3af' }}>{label}: </span>
                      <span style={{ fontSize: 13, fontWeight: 700, fontFamily: "'DM Mono', monospace", color: val === 0 ? '#9ca3af' : color }}>{val}</span>
                    </div>
                  ))}
                </div>

                {/* Progress bar */}
                <div style={{ height: 5, background: '#e5e7eb', borderRadius: 3, overflow: 'hidden' }}>
                  <div style={{ height: '100%', width: `${calledPct}%`, background: 'linear-gradient(90deg, #6366f1, #ec4899)', borderRadius: 3, transition: 'width 0.4s' }} />
                </div>

                {/* View Leads button */}
                <div>
                  <button onClick={() => handleViewCampaign(campaign)}
                    style={{ background: 'none', border: 'none', color: '#9ca3af', cursor: 'pointer', fontSize: 13, fontWeight: 600, padding: 0, fontFamily: 'inherit' }}>
                    View Leads →
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      <CampaignModals
        showCreateModal={showCreateModal}
        setShowCreateModal={setShowCreateModal}
        createForm={createForm}
        setCreateForm={setCreateForm}
        handleCreateCampaign={handleCreateCampaign}
        loading={loading}
        createError={createError}
        setCreateError={setCreateError}
        orgProducts={orgProducts}
        orgExotelAccounts={orgExotelAccounts}
        selectedTemplate={selectedTemplate}
        setSelectedTemplate={setSelectedTemplate}
        showAddLeadsModal={false}
        setShowAddLeadsModal={setShowAddLeadsModal}
        availableLeads={availableLeads}
        selectedLeadIds={selectedLeadIds}
        toggleLeadSelection={toggleLeadSelection}
        handleAddLeads={handleAddLeads}
        showCsvImportModal={false}
        setShowCsvImportModal={setShowCsvImportModal}
        csvFile={csvFile}
        setCsvFile={setCsvFile}
        handleCsvImport={handleCsvImport}
        editLead={null}
        setEditLead={setEditLead}
        editForm={editForm}
        setEditForm={setEditForm}
        handleSaveEdit={handleSaveEdit}
        showEditCampaignModal={showEditCampaignModal}
        setShowEditCampaignModal={setShowEditCampaignModal}
        editCampaignForm={editCampaignForm}
        setEditCampaignForm={setEditCampaignForm}
        handleSaveEditCampaign={handleSaveEditCampaign}
        editCampaignError={editCampaignError}
        setEditCampaignError={setEditCampaignError}
      />

    </div>
  );
}
