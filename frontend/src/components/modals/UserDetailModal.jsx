import React, { useEffect, useMemo, useState } from 'react';
import { formatDateTime } from '../../utils/dateFormat';
import AuthAudio from '../AuthAudio';

const T = {
  bg: '#f4f5f9',
  card: '#ffffff',
  border: '#e5e7eb',
  accent: '#6366f1',
  green: '#10b981',
  amber: '#f59e0b',
  red: '#ef4444',
  text: '#111827',
  sub: '#374151',
  muted: '#9ca3af',
  font: "'DM Sans', sans-serif",
  mono: "'DM Mono', monospace",
};

const card = {
  background: T.card,
  border: `1px solid ${T.border}`,
  borderRadius: 12,
  boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const btnGhost = {
  background: '#fff',
  color: T.sub,
  border: `1px solid ${T.border}`,
  borderRadius: 8,
  padding: '8px 14px',
  fontWeight: 700,
  cursor: 'pointer',
  fontFamily: T.font,
  fontSize: 13,
};

const inputStyle = {
  padding: '10px 12px',
  borderRadius: 8,
  fontSize: 13,
  border: `1px solid ${T.border}`,
  background: '#fff',
  color: T.text,
  fontFamily: T.font,
  outline: 'none',
  width: '100%',
  boxSizing: 'border-box',
};

const tabBtn = (active) => ({
  padding: '10px 16px',
  border: 'none',
  borderBottom: `2px solid ${active ? T.accent : 'transparent'}`,
  background: 'transparent',
  color: active ? T.accent : T.sub,
  fontWeight: active ? 800 : 600,
  cursor: 'pointer',
  fontFamily: T.font,
  fontSize: 13,
});

const thStyle = {
  textAlign: 'left',
  padding: '10px 12px',
  color: T.muted,
  fontSize: 10,
  fontWeight: 800,
  textTransform: 'uppercase',
  letterSpacing: '0.04em',
  borderBottom: `1px solid ${T.border}`,
  background: '#fbfcff',
  whiteSpace: 'nowrap',
};

const tdStyle = {
  padding: '12px',
  color: T.sub,
  fontSize: 13,
  borderBottom: `1px solid ${T.border}`,
  verticalAlign: 'middle',
  whiteSpace: 'nowrap',
};

function Badge({ children, color = T.accent }) {
  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      padding: '3px 9px',
      borderRadius: 999,
      fontSize: 10,
      fontWeight: 800,
      color,
      background: `${color}18`,
      border: `1px solid ${color}35`,
    }}>
      {children}
    </span>
  );
}

function formatDuration(s) {
  const sec = Math.round(Number(s) || 0);
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const r = sec % 60;
  if (m < 60) return `${m}m ${r}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m ${r}s`;
}

export default function UserDetailModal({ userId, onClose, apiFetch, API_URL, range }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [activeTab, setActiveTab] = useState('overview');
  const [leadsCampaignId, setLeadsCampaignId] = useState('');
  const [leadsPage, setLeadsPage] = useState(1);
  const [recordingsPage, setRecordingsPage] = useState(1);

  useEffect(() => {
    if (!userId) return;
    const onKey = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [userId, onClose]);

  useEffect(() => {
    if (!userId) return;
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      setError('');
      try {
        const params = new URLSearchParams({
          from: range?.from || '',
          to: range?.to || '',
          leads_page: String(leadsPage),
          recordings_page: String(recordingsPage),
        });
        if (leadsCampaignId) params.set('campaign_id', leadsCampaignId);
        const res = await apiFetch(`${API_URL}/analytics/user-detail/${userId}?${params.toString()}`);
        const body = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
        if (!cancelled) setData(body);
      } catch (e) {
        if (!cancelled) {
          setError(e.message || 'Failed to load user detail');
          setData(null);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    load();
    return () => { cancelled = true; };
  }, [userId, apiFetch, API_URL, range?.from, range?.to, leadsCampaignId, leadsPage, recordingsPage]);

  const campaigns = useMemo(() => Array.isArray(data?.campaigns) ? data.campaigns : [], [data]);
  const leads = useMemo(() => Array.isArray(data?.leads) ? data.leads : [], [data]);
  const recordings = useMemo(() => Array.isArray(data?.recordings) ? data.recordings : [], [data]);
  const stats = data?.stats || {};
  const profile = data?.profile || {};
  const leadPagination = data?.lead_pagination || { page: 1, limit: 20, total: 0 };
  const recordingPagination = data?.recording_pagination || { page: 1, limit: 20, total: 0 };

  const handleCampaignClick = (cid) => {
    setLeadsCampaignId(String(cid));
    setLeadsPage(1);
    setActiveTab('leads');
  };

  const tabs = [
    { key: 'overview', label: 'Overview' },
    { key: 'campaigns', label: 'Campaigns' },
    { key: 'leads', label: 'Leads' },
    { key: 'recordings', label: 'Recordings' },
  ];

  const renderOverview = () => (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 18 }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(140px, 1fr))', gap: 12 }}>
        {[
          { label: 'Assigned Leads', value: stats.assigned_leads || 0 },
          { label: 'Assigned Campaigns', value: stats.assigned_campaigns || 0 },
          { label: 'Total Calls', value: stats.total_calls || 0 },
          { label: 'Recordings', value: stats.recordings || 0 },
          { label: 'Appointments', value: stats.appointments || 0 },
          { label: 'Notes', value: stats.notes || 0 },
        ].map((s) => (
          <div key={s.label} style={{ ...card, padding: 16, textAlign: 'center' }}>
            <div style={{ fontSize: 22, fontWeight: 800, color: T.text }}>{s.value}</div>
            <div style={{ fontSize: 11, color: T.muted, fontWeight: 700, textTransform: 'uppercase', marginTop: 4 }}>{s.label}</div>
          </div>
        ))}
      </div>

      <div>
        <div style={{ fontSize: 14, fontWeight: 800, color: T.text, marginBottom: 10 }}>Assigned campaigns</div>
        {campaigns.length === 0 ? (
          <div style={{ color: T.muted, fontSize: 13, padding: '16px 0' }}>No campaigns assigned.</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {campaigns.map((c) => (
              <button
                key={c.campaign_id}
                onClick={() => handleCampaignClick(c.campaign_id)}
                style={{
                  ...card,
                  padding: 12,
                  textAlign: 'left',
                  cursor: 'pointer',
                  background: '#fff',
                  display: 'grid',
                  gridTemplateColumns: '1fr auto auto auto auto auto auto',
                  gap: 16,
                  alignItems: 'center',
                }}
              >
                <div>
                  <div style={{ fontWeight: 700, color: T.text, fontSize: 13 }}>{c.name}</div>
                  <div style={{ fontSize: 11, color: T.muted }}>Status: {c.status}</div>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <div style={{ fontWeight: 800, fontSize: 13 }}>{c.lead_count || 0}</div>
                  <div style={{ fontSize: 10, color: T.muted }}>Leads</div>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <div style={{ fontWeight: 800, fontSize: 13 }}>{c.total_calls || 0}</div>
                  <div style={{ fontSize: 10, color: T.muted }}>Calls</div>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <div style={{ fontWeight: 800, fontSize: 13 }}>{c.recordings || 0}</div>
                  <div style={{ fontSize: 10, color: T.muted }}>Recs</div>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <div style={{ fontWeight: 800, fontSize: 13 }}>{c.appointments || 0}</div>
                  <div style={{ fontSize: 10, color: T.muted }}>Appts</div>
                </div>
                <div style={{ textAlign: 'center' }}>
                  <div style={{ fontWeight: 800, fontSize: 13 }}>{c.notes || 0}</div>
                  <div style={{ fontSize: 10, color: T.muted }}>Notes</div>
                </div>
                <span style={{ color: T.accent, fontSize: 12, fontWeight: 700 }}>View leads →</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );

  const renderCampaigns = () => (
    <div style={{ overflowX: 'auto' }}>
      <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: 700 }}>
        <thead>
          <tr>
            {['Campaign', 'Status', 'Leads', 'Calls', 'Recordings', 'Appointments', 'Notes'].map((h) => (
              <th key={h} style={thStyle}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {campaigns.map((c, i) => {
            const last = i === campaigns.length - 1;
            const rowTd = { ...tdStyle, borderBottom: last ? 'none' : `1px solid ${T.border}` };
            return (
              <tr
                key={c.campaign_id}
                onClick={() => handleCampaignClick(c.campaign_id)}
                style={{ cursor: 'pointer' }}
                onMouseEnter={(e) => { e.currentTarget.style.background = '#f9fafb'; }}
                onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}
              >
                <td style={{ ...rowTd, color: T.text, fontWeight: 700 }}>{c.name}</td>
                <td style={rowTd}><Badge>{c.status}</Badge></td>
                <td style={rowTd}>{c.lead_count || 0}</td>
                <td style={rowTd}>{c.total_calls || 0}</td>
                <td style={rowTd}>{c.recordings || 0}</td>
                <td style={rowTd}>{c.appointments || 0}</td>
                <td style={rowTd}>{c.notes || 0}</td>
              </tr>
            );
          })}
          {campaigns.length === 0 && (
            <tr><td colSpan="7" style={{ ...tdStyle, textAlign: 'center', padding: 24 }}>No assigned campaigns.</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );

  const renderLeads = () => (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
        <label style={{ display: 'flex', flexDirection: 'column', gap: 4, flex: 1, maxWidth: 300 }}>
          <span style={{ fontSize: 11, fontWeight: 800, color: T.sub, textTransform: 'uppercase' }}>Filter by campaign</span>
          <select
            value={leadsCampaignId}
            onChange={(e) => { setLeadsCampaignId(e.target.value); setLeadsPage(1); }}
            style={{ ...inputStyle, height: 38 }}
          >
            <option value="">All assigned campaigns</option>
            {campaigns.map((c) => (
              <option key={c.campaign_id} value={String(c.campaign_id)}>{c.name}</option>
            ))}
          </select>
        </label>
        {leadsCampaignId && (
          <button onClick={() => { setLeadsCampaignId(''); setLeadsPage(1); }} style={{ ...btnGhost, marginTop: 16 }}>Clear</button>
        )}
      </div>
      <div style={{ overflowX: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: 800 }}>
          <thead>
            <tr>
              {['Name', 'Phone', 'Company', 'Source', 'Status', 'Campaign', 'Created'].map((h) => (
                <th key={h} style={thStyle}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {leads.map((l, i) => {
              const last = i === leads.length - 1;
              const rowTd = { ...tdStyle, borderBottom: last ? 'none' : `1px solid ${T.border}` };
              const name = `${l.first_name || ''} ${l.last_name || ''}`.trim() || '-';
              return (
                <tr key={l.id}>
                  <td style={{ ...rowTd, color: T.text, fontWeight: 700 }}>{name}</td>
                  <td style={rowTd}>{l.phone || '-'}</td>
                  <td style={rowTd}>{l.company || '-'}</td>
                  <td style={rowTd}>{l.source || '-'}</td>
                  <td style={rowTd}><Badge color={l.status === 'new' ? T.amber : T.green}>{l.status || 'new'}</Badge></td>
                  <td style={rowTd}>{l.campaign_name || '-'}</td>
                  <td style={rowTd}>{formatDateTime(l.created_at) || '-'}</td>
                </tr>
              );
            })}
            {leads.length === 0 && (
              <tr><td colSpan="7" style={{ ...tdStyle, textAlign: 'center', padding: 24 }}>No leads found.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {leadPagination.total > leadPagination.limit && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 4 }}>
          <span style={{ fontSize: 12, color: T.muted }}>
            Showing {(leadPagination.page - 1) * leadPagination.limit + 1} - {Math.min(leadPagination.page * leadPagination.limit, leadPagination.total)} of {leadPagination.total}
          </span>
          <div style={{ display: 'flex', gap: 8 }}>
            <button
              disabled={leadPagination.page <= 1}
              onClick={() => setLeadsPage((p) => p - 1)}
              style={{ ...btnGhost, opacity: leadPagination.page <= 1 ? 0.5 : 1 }}
            >Previous</button>
            <button
              disabled={leadPagination.page * leadPagination.limit >= leadPagination.total}
              onClick={() => setLeadsPage((p) => p + 1)}
              style={{ ...btnGhost, opacity: leadPagination.page * leadPagination.limit >= leadPagination.total ? 0.5 : 1 }}
            >Next</button>
          </div>
        </div>
      )}
    </div>
  );

  const renderRecordings = () => (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div style={{ overflowX: 'auto' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: 700 }}>
          <thead>
            <tr>
              {['Date', 'Lead', 'Campaign', 'Duration', 'Status', 'Recording'].map((h) => (
                <th key={h} style={thStyle}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {recordings.map((rec, i) => {
              const last = i === recordings.length - 1;
              const rowTd = { ...tdStyle, borderBottom: last ? 'none' : `1px solid ${T.border}` };
              const recUrl = rec.recording_url || '';
              return (
                <tr key={rec.id || i}>
                  <td style={rowTd}>{formatDateTime(rec.user_call_at || rec.created_at) || '-'}</td>
                  <td style={{ ...rowTd, color: T.text, fontWeight: 700 }}>{rec.lead_name || '-'}</td>
                  <td style={rowTd}>{rec.campaign_name || '-'}</td>
                  <td style={rowTd}>{formatDuration(rec.duration_s)}</td>
                  <td style={rowTd}><Badge>{rec.status || '-'}</Badge></td>
                  <td style={rowTd}>
                    {recUrl ? (
                      <AuthAudio src={recUrl} style={{ width: 240, height: 32, verticalAlign: 'middle' }} />
                    ) : (
                      <span style={{ color: T.muted, fontSize: 12 }}>Unavailable</span>
                    )}
                  </td>
                </tr>
              );
            })}
            {recordings.length === 0 && (
              <tr><td colSpan="6" style={{ ...tdStyle, textAlign: 'center', padding: 24 }}>No recordings found.</td></tr>
            )}
          </tbody>
        </table>
      </div>
      {recordingPagination.total > recordingPagination.limit && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 4 }}>
          <span style={{ fontSize: 12, color: T.muted }}>
            Showing {(recordingPagination.page - 1) * recordingPagination.limit + 1} - {Math.min(recordingPagination.page * recordingPagination.limit, recordingPagination.total)} of {recordingPagination.total}
          </span>
          <div style={{ display: 'flex', gap: 8 }}>
            <button
              disabled={recordingPagination.page <= 1}
              onClick={() => setRecordingsPage((p) => p - 1)}
              style={{ ...btnGhost, opacity: recordingPagination.page <= 1 ? 0.5 : 1 }}
            >Previous</button>
            <button
              disabled={recordingPagination.page * recordingPagination.limit >= recordingPagination.total}
              onClick={() => setRecordingsPage((p) => p + 1)}
              style={{ ...btnGhost, opacity: recordingPagination.page * recordingPagination.limit >= recordingPagination.total ? 0.5 : 1 }}
            >Next</button>
          </div>
        </div>
      )}
    </div>
  );

  const tabContent = {
    overview: renderOverview,
    campaigns: renderCampaigns,
    leads: renderLeads,
    recordings: renderRecordings,
  };

  return (
    <div
      className="modal-overlay"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.45)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
        padding: 20,
      }}
    >
      <div style={{
        background: T.card,
        border: `1px solid ${T.border}`,
        borderRadius: 16,
        boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
        width: '100%',
        maxWidth: 960,
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}>
        <div style={{
          padding: '18px 22px',
          borderBottom: `1px solid ${T.border}`,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-start',
        }}>
          <div>
            <div style={{ fontSize: 18, fontWeight: 800, color: T.text }}>{profile.full_name || profile.email || 'User detail'}</div>
            <div style={{ fontSize: 13, color: T.muted, marginTop: 4 }}>{profile.email}</div>
            <div style={{ marginTop: 8 }}><Badge>{profile.role || '-'}</Badge></div>
          </div>
          <button onClick={onClose} style={{ ...btnGhost, padding: '6px 10px' }}>✕</button>
        </div>

        <div style={{ display: 'flex', borderBottom: `1px solid ${T.border}`, padding: '0 22px' }}>
          {tabs.map((t) => (
            <button key={t.key} onClick={() => setActiveTab(t.key)} style={tabBtn(activeTab === t.key)}>
              {t.label}
            </button>
          ))}
        </div>

        <div style={{ padding: '18px 22px', overflowY: 'auto', flex: 1, minHeight: 0 }}>
          {loading && !data && (
            <div style={{ color: T.muted, fontSize: 13, padding: 24 }}>Loading user detail…</div>
          )}
          {error && (
            <div style={{ color: T.red, fontSize: 13, padding: 16, background: '#fef2f2', borderRadius: 8 }}>{error}</div>
          )}
          {!loading && data && tabContent[activeTab]()}
        </div>
      </div>
    </div>
  );
}
