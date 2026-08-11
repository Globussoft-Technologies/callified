import React, { useState, useEffect, useCallback } from 'react';
import { useToast } from '../contexts/UIContext';
import { useCall } from '../contexts/CallContext';
import { isValidPhone, normalizePhone, PHONE_VALIDATION_MESSAGE } from '../utils/phone';
import PageHeader from '../components/PageHeader';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "Quicksand", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const inputStyle = (hasError) => ({
  padding: '9px 13px', borderRadius: 8, fontSize: 13,
  border: `1px solid ${hasError ? T.red : T.border}`,
  background: T.card, color: T.text, fontFamily: T.font, outline: 'none',
});

const btnPrimary = {
  background: T.accent, color: '#fff', border: 'none',
  borderRadius: 8, padding: '8px 18px', fontWeight: 600,
  cursor: 'pointer', fontFamily: T.font, fontSize: '0.9rem',
};



function maskPhone(phone) {
  if (!phone) return '-';
  const digits = String(phone).replace(/\D/g, '');
  if (digits.length <= 5) return digits;
  return digits.slice(0, 5) + 'X'.repeat(digits.length - 5);
}

export default function ManualDialPage({ apiFetch, API_URL, campaigns = [] }) {
  const toast = useToast();
  const { triggerBrowserCall, browserCallDialing } = useCall();

  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedCampaignId, setSelectedCampaignId] = useState('');
  const [callingId, setCallingId] = useState(null);

  const [newName, setNewName] = useState('');
  const [newPhone, setNewPhone] = useState('');
  const [newCompany, setNewCompany] = useState('');
  const [newError, setNewError] = useState({});
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (campaigns.length > 0 && !selectedCampaignId) {
      setSelectedCampaignId(String(campaigns[0].id));
    }
  }, [campaigns, selectedCampaignId]);

  const search = useCallback(async (q = query) => {
    const trimmed = q.trim();
    if (!trimmed) {
      setResults([]);
      return;
    }
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/leads/search?q=${encodeURIComponent(trimmed)}`);
      if (!res.ok) throw new Error('Search failed');
      const data = await res.json();
      setResults(data || []);
    } catch {
      toast('Failed to search leads');
      setResults([]);
    }
    setLoading(false);
  }, [apiFetch, API_URL, query, toast]);

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') search();
  };

  const handleAIDial = async (lead) => {
    setCallingId(lead.id);
    try {
      const res = await apiFetch(`${API_URL}/dial/${lead.id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ campaign_id: 0 }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || `Dial failed (HTTP ${res.status})`);
      }
      toast('Dialing customer…');
    } catch (e) {
      toast(`AI dial failed: ${e.message}`);
    }
    setCallingId(null);
  };

  const handleBrowserCall = async (lead) => {
    const campaignId = parseInt(selectedCampaignId, 10);
    if (!campaignId) {
      toast('Please select a campaign for the browser call');
      return;
    }
    triggerBrowserCall(lead, campaignId);
  };

  const validateNewLead = () => {
    const errors = {};
    const name = newName.trim();
    if (!name) errors.name = 'Name is required';
    else if (!/[A-Za-z]/.test(name)) errors.name = 'Name must contain at least one letter';
    if (!newPhone.trim()) errors.phone = 'Phone is required';
    else if (!isValidPhone(newPhone)) errors.phone = PHONE_VALIDATION_MESSAGE;
    return errors;
  };

  const createAndCall = async (mode) => {
    const errors = validateNewLead();
    if (Object.keys(errors).length > 0) {
      setNewError(errors);
      return;
    }
    const campaignId = parseInt(selectedCampaignId, 10);
    if (mode === 'browser' && !campaignId) {
      toast('Please select a campaign for the browser call');
      return;
    }

    setCreating(true);
    let leadId;
    try {
      const res = await apiFetch(`${API_URL}/leads`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          first_name: newName.trim(),
          phone: normalizePhone(newPhone),
          company: newCompany.trim(),
          source: 'Manual Dial',
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        if (data.fields) {
          setNewError(data.fields);
          throw new Error('Validation failed');
        }
        throw new Error(data.error || `Create failed (HTTP ${res.status})`);
      }
      leadId = data.id;
      if (campaignId) {
        await apiFetch(`${API_URL}/campaigns/${campaignId}/leads`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ lead_ids: [leadId] }),
        });
      }
      toast('Lead created');
    } catch (e) {
      toast(`Failed to create lead: ${e.message}`);
      setCreating(false);
      return;
    }

    // Re-fetch the lead so we have a full lead object.
    try {
      const res = await apiFetch(`${API_URL}/leads/${leadId}`);
      const lead = await res.json();
      if (mode === 'ai') {
        await handleAIDial(lead);
      } else {
        handleBrowserCall(lead);
      }
      // Refresh search results so the new lead appears.
      search();
      setNewName('');
      setNewPhone('');
      setNewCompany('');
      setNewError({});
    } catch {
      toast('Lead created but could not start call');
    }
    setCreating(false);
  };

  const canDial = !browserCallDialing && !callingId && !creating;

  return (
    <div style={{ padding: '24px', maxWidth: 960, margin: '0 auto', fontFamily: T.font }}>
      <PageHeader
        icon="manualDial"
        title="Manual Dial"
        subtitle="Search customers and place calls."
      />

      <div style={{ ...card, padding: '1.25rem', marginBottom: 20 }}>
        <div style={{ display: 'flex', alignItems: 'flex-end', gap: 12, flexWrap: 'wrap' }}>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 220, flex: 1 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>Search by name, phone or company</span>
            <input
              type="text"
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="e.g. Rahul, 98765 or Acme..."
              style={{ ...inputStyle(false), width: '100%', boxSizing: 'border-box' }}
            />
          </label>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 220 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>Campaign (for browser call)</span>
            <select
              value={selectedCampaignId}
              onChange={e => setSelectedCampaignId(e.target.value)}
              style={{ ...inputStyle(false), width: '100%', boxSizing: 'border-box', height: 38 }}
            >
              {campaigns.length === 0 && <option value="">No campaigns</option>}
              {campaigns.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </label>
          <button onClick={() => search()} disabled={loading} style={{ ...btnPrimary, opacity: loading ? 0.6 : 1 }}>
            {loading ? 'Searching…' : 'Search'}
          </button>
        </div>
      </div>

      {results.length > 0 && (
        <div style={{ ...card, overflow: 'hidden', marginBottom: 20 }}>
          <div style={{ padding: '14px 18px', borderBottom: `1px solid ${T.border}`, fontWeight: 700, color: T.text, fontSize: 15 }}>
            Search Results ({results.length})
          </div>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Name', 'Phone', 'Company', 'Source', 'Status', 'Action'].map(h => (
                  <th key={h} style={{ padding: '10px 16px', textAlign: 'left', fontSize: 12, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: '0.05em', borderBottom: `1px solid ${T.border}` }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {results.map(lead => (
                <tr key={lead.id}>
                  <td style={{ padding: '12px 16px', fontWeight: 600, color: T.text }}>{lead.first_name} {lead.last_name}</td>
                  <td style={{ padding: '12px 16px', fontFamily: T.mono, color: T.sub }}>{maskPhone(lead.phone)}</td>
                  <td style={{ padding: '12px 16px', color: T.sub }}>{lead.company || '-'}</td>
                  <td style={{ padding: '12px 16px' }}><span style={{ fontSize: 11, fontWeight: 600, padding: '3px 10px', borderRadius: 20, color: T.accent, background: `${T.accent}15` }}>{lead.source || '-'}</span></td>
                  <td style={{ padding: '12px 16px' }}><span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>{lead.status || 'New'}</span></td>
                  <td style={{ padding: '12px 16px' }}>
                    <div style={{ display: 'flex', gap: 8 }}>
                      <button
                        onClick={() => handleAIDial(lead)}
                        disabled={!canDial}
                        style={{ ...btnPrimary, background: T.green, padding: '6px 14px', fontSize: '0.8rem', opacity: !canDial ? 0.6 : 1 }}>
                        {callingId === lead.id ? 'Dialing…' : '🤖 AI Dial'}
                      </button>
                      <button
                        onClick={() => handleBrowserCall(lead)}
                        disabled={!canDial || !selectedCampaignId}
                        style={{ ...btnPrimary, padding: '6px 14px', fontSize: '0.8rem', opacity: (!canDial || !selectedCampaignId) ? 0.6 : 1 }}>
                        {browserCallDialing ? 'Calling…' : '🎙 Browser Call'}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {!loading && query.trim() && results.length === 0 && (
        <div style={{ ...card, padding: '1.25rem', marginBottom: 20, textAlign: 'center', color: T.muted }}>
          No customer found for “{query.trim()}”. Add them below to place a call.
        </div>
      )}

      <div style={{ ...card, padding: '1.25rem' }}>
        <div style={{ fontWeight: 700, color: T.text, fontSize: 15, marginBottom: 14 }}>➕ Add New Customer & Call</div>
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12, flexWrap: 'wrap' }}>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 180, flex: 1 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>Name</span>
            <input
              type="text"
              value={newName}
              onChange={e => { setNewName(e.target.value); setNewError(prev => ({ ...prev, name: '' })); }}
              placeholder="Customer name"
              style={{ ...inputStyle(!!newError.name), width: '100%', boxSizing: 'border-box' }}
            />
            {newError.name && <span style={{ color: T.red, fontSize: '0.75rem' }}>{newError.name}</span>}
          </label>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 180, flex: 1 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>Phone</span>
            <input
              type="tel"
              value={newPhone}
              onChange={e => { setNewPhone(e.target.value); setNewError(prev => ({ ...prev, phone: '' })); }}
              placeholder="9876543210"
              style={{ ...inputStyle(!!newError.phone), width: '100%', boxSizing: 'border-box' }}
            />
            {newError.phone && <span style={{ color: T.red, fontSize: '0.75rem' }}>{newError.phone}</span>}
          </label>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 180, flex: 1 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>Company <span style={{ fontWeight: 400, color: T.muted }}>(Optional)</span></span>
            <input
              type="text"
              value={newCompany}
              onChange={e => setNewCompany(e.target.value)}
              placeholder="e.g. Acme Inc."
              style={{ ...inputStyle(false), width: '100%', boxSizing: 'border-box' }}
            />
          </label>
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start', paddingTop: 22 }}>
            <button
              onClick={() => createAndCall('ai')}
              disabled={!canDial}
              style={{ ...btnPrimary, background: T.green, opacity: !canDial ? 0.6 : 1 }}>
              {creating ? 'Creating…' : '🤖 Create & AI Dial'}
            </button>
            <button
              onClick={() => createAndCall('browser')}
              disabled={!canDial || !selectedCampaignId}
              style={{ ...btnPrimary, opacity: (!canDial || !selectedCampaignId) ? 0.6 : 1 }}>
              {creating ? 'Creating…' : '🎙 Create & Browser Call'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
