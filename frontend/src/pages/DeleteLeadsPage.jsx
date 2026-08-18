import React, { useState, useCallback, useMemo } from 'react';
import { API_URL } from '../constants/api';
import { useAuth } from '../contexts/AuthContext';
import { useToast, useConfirm } from '../contexts/UIContext';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', red: '#ef4444', green: '#10b981',
  text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.04)',
};

const inputStyle = {
  padding: '10px 14px', border: `1px solid ${T.border}`, borderRadius: 8,
  fontSize: 14, fontFamily: T.font, color: T.text, background: '#fff',
  outline: 'none', flex: 1, boxSizing: 'border-box',
};

const btnPrimary = {
  background: T.accent, border: 'none', color: '#fff',
  padding: '10px 18px', borderRadius: 8, cursor: 'pointer',
  fontWeight: 600, fontFamily: T.font, fontSize: 14,
};

const btnDanger = {
  background: '#fee2e2', color: T.red, border: `1px solid #fca5a5`,
  padding: '6px 14px', borderRadius: 6, cursor: 'pointer',
  fontWeight: 600, fontFamily: T.font, fontSize: 13,
};

export default function DeleteLeadsPage() {
  const { apiFetch, hasPermission } = useAuth();
  const toast = useToast();
  const confirm = useConfirm();

  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);
  const [loading, setLoading] = useState(false);
  const [deletingId, setDeletingId] = useState(null);

  const canDelete = hasPermission('crm.delete');

  const handleSearch = useCallback(async () => {
    const q = query.trim();
    if (!q) {
      setResults([]);
      return;
    }
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/leads/search-campaigns?q=${encodeURIComponent(q)}`);
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast(data.error || `Search failed (${res.status})`, 'error');
        setResults([]);
        setLoading(false);
        return;
      }
      const data = await res.json();
      setResults(Array.isArray(data) ? data : []);
    } catch (e) {
      console.error(e);
      toast('Search failed', 'error');
      setResults([]);
    }
    setLoading(false);
  }, [query, apiFetch, toast]);

  const groupedLeads = useMemo(() => {
    const map = new Map();
    for (const row of results) {
      const existing = map.get(row.id);
      const campaignName = row.campaignName || row.campaign_name;
      if (existing) {
        if (campaignName && !existing.campaigns.includes(campaignName)) {
          existing.campaigns.push(campaignName);
        }
      } else {
        map.set(row.id, {
          id: row.id,
          first_name: row.first_name || '',
          last_name: row.last_name || '',
          phone: row.phone || '',
          campaigns: campaignName ? [campaignName] : [],
        });
      }
    }
    return Array.from(map.values());
  }, [results]);

  const handleDelete = async (lead) => {
    if (!canDelete) {
      toast('You do not have permission to delete leads.', 'error');
      return;
    }
    const fullName = `${lead.first_name} ${lead.last_name}`.trim() || 'this lead';
    const confirmed = await confirm({
      title: 'Delete Lead',
      message: `This will delete ${fullName} (${lead.phone || 'no phone'}) from all campaigns. This action cannot be undone.`,
      okText: 'Delete',
      cancelText: 'Cancel',
      danger: true,
    });
    if (!confirmed) return;

    setDeletingId(lead.id);
    try {
      const res = await apiFetch(`${API_URL}/leads/${lead.id}`, { method: 'DELETE' });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast(data.error || `Delete failed (${res.status})`, 'error');
        return;
      }
      toast('Lead deleted from all campaigns');
      handleSearch();
    } catch (e) {
      console.error(e);
      toast('Delete failed', 'error');
    } finally {
      setDeletingId(null);
    }
  };

  const onKeyDown = (e) => {
    if (e.key === 'Enter') handleSearch();
  };

  return (
    <div style={{ padding: '28px 32px', maxWidth: 1100, margin: '0 auto', fontFamily: T.font, background: T.bg, minHeight: '100%' }}>
      <div style={{ marginBottom: '1.5rem' }}>
        <h2 style={{ margin: 0, color: T.text, fontSize: '1.4rem', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>
          🗑️ Delete Leads
        </h2>
        <p style={{ margin: '4px 0 0', color: T.muted, fontSize: '0.85rem' }}>
          Search by name or phone number and permanently delete a lead from all campaigns.
        </p>
      </div>

      <div style={{ ...card, padding: '1.25rem', marginBottom: '1.5rem' }}>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <input
            style={inputStyle}
            placeholder="Search by name or phone number..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
          />
          <button style={btnPrimary} onClick={handleSearch} disabled={loading}>
            {loading ? 'Searching...' : 'Search'}
          </button>
        </div>
      </div>

      <div style={{ ...card, overflow: 'hidden' }}>
        {groupedLeads.length === 0 ? (
          <div style={{ padding: '2rem', textAlign: 'center', color: T.muted }}>
            {query.trim() ? 'No leads found.' : 'Enter a name or phone number to search.'}
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: '#f9fafb' }}>
                {['Name', 'Phone', 'Campaigns', 'Actions'].map((h) => (
                  <th key={h} style={{ padding: '12px 16px', textAlign: 'left', fontSize: 12, fontWeight: 700, color: T.sub, borderBottom: `1px solid ${T.border}` }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {groupedLeads.map((lead) => {
                const fullName = `${lead.first_name} ${lead.last_name}`.trim() || 'Unnamed';
                return (
                  <tr key={lead.id} style={{ borderBottom: `1px solid ${T.border}` }}>
                    <td style={{ padding: '12px 16px', fontWeight: 600, color: T.text, fontSize: 13 }}>{fullName}</td>
                    <td style={{ padding: '12px 16px', color: T.sub, fontSize: 13, fontFamily: T.mono }}>{lead.phone || '-'}</td>
                    <td style={{ padding: '12px 16px', color: T.sub, fontSize: 13 }}>
                      {lead.campaigns.length > 0 ? lead.campaigns.join(', ') : <span style={{ color: T.muted }}>Not in any campaign</span>}
                    </td>
                    <td style={{ padding: '12px 16px' }}>
                      <button
                        style={{ ...btnDanger, opacity: deletingId === lead.id ? 0.6 : 1 }}
                        onClick={() => handleDelete(lead)}
                        disabled={!canDelete || deletingId === lead.id}
                      >
                        {deletingId === lead.id ? 'Deleting...' : 'Delete'}
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
