import React, { useState, useEffect } from 'react';
import { formatDateTime } from '../utils/dateFormat';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

export default function ScheduledCallsPage({ apiFetch, API_URL, orgTimezone }) {
  const [scheduledCalls, setScheduledCalls] = useState([]);
  const [loading, setLoading] = useState(true);
  const [confirmCancelId, setConfirmCancelId] = useState(null);

  const fetchScheduledCalls = async () => {
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/scheduled-calls`);
      setScheduledCalls(await res.json());
    } catch (e) { console.error('Failed to fetch scheduled calls', e); }
    setLoading(false);
  };

  // eslint-disable-next-line react-hooks/set-state-in-effect, react-hooks/exhaustive-deps
  useEffect(() => { fetchScheduledCalls(); }, []);

  const handleCancel = async (id) => {
    try {
      await apiFetch(`${API_URL}/scheduled-calls/${id}`, { method: 'DELETE' });
      fetchScheduledCalls();
    } catch { alert('Failed to cancel');  }
  };

  const statusStyle = (status) => {
    const map = {
      pending:   { color: '#f59e0b', bg: 'rgba(245,158,11,0.1)', border: 'rgba(245,158,11,0.3)' },
      dialing:   { color: '#6366f1', bg: 'rgba(99,102,241,0.1)', border: 'rgba(99,102,241,0.3)' },
      completed: { color: '#10b981', bg: 'rgba(16,185,129,0.1)',  border: 'rgba(16,185,129,0.3)' },
      failed:    { color: '#ef4444', bg: 'rgba(239,68,68,0.1)',  border: 'rgba(239,68,68,0.3)' },
      cancelled: { color: '#9ca3af', bg: 'rgba(156,163,175,0.1)', border: 'rgba(156,163,175,0.3)' },
    };
    return map[status] || map.pending;
  };

  const thStyle = {
    fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase',
    letterSpacing: '0.07em', padding: '0 12px 12px', textAlign: 'left',
    borderBottom: `1px solid ${T.border}`,
  };
  const tdStyle = {
    fontSize: 13, color: T.sub, padding: '13px 12px',
    borderBottom: `1px solid ${T.border}`, verticalAlign: 'middle',
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
          <span style={{ color: T.accent }}>Scheduled</span> Calls
        </h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Upcoming and past scheduled calls. Schedule calls from the CRM or campaign pages.
        </p>
      </div>

      {loading ? (
        <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>
          Loading scheduled calls...
        </div>
      ) : scheduledCalls.length === 0 ? (
        <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>
          No scheduled calls. Schedule calls from the CRM or campaign pages.
        </div>
      ) : (
        <div style={{ ...card, overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={thStyle}>Scheduled Time</th>
                <th style={thStyle}>Lead Name</th>
                <th style={thStyle}>Phone</th>
                <th style={thStyle}>Status</th>
                <th style={{ ...thStyle, width: 260, minWidth: 260 }}>Action</th>
              </tr>
            </thead>
            <tbody>
              {scheduledCalls.map((call, i) => {
                const sc = statusStyle(call.status);
                const isLast = i === scheduledCalls.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                return (
                  <tr key={call.id}>
                    <td style={{ ...rowTd, color: T.sub }}>
                      {formatDateTime(call.scheduled_time, orgTimezone)}
                    </td>
                    <td style={{ ...rowTd, fontWeight: 600, color: T.text }}>
                      {call.lead_name || call.first_name || '-'}
                    </td>
                    <td style={{ ...rowTd, fontFamily: T.mono, fontSize: 12, color: T.muted }}>
                      {call.phone}
                    </td>
                    <td style={rowTd}>
                      <span style={{
                        padding: '3px 10px', borderRadius: 12, fontSize: 11, fontWeight: 600,
                        color: sc.color, background: sc.bg, border: `1px solid ${sc.border}`,
                      }}>
                        {call.status}
                      </span>
                    </td>
                    <td style={{ ...rowTd, width: 260, minWidth: 260 }}>
                      {call.status === 'pending' && (
                        confirmCancelId === call.id ? (
                          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                            <span style={{ color: T.amber, fontSize: 12 }}>Cancel call?</span>
                            <button onClick={() => { setConfirmCancelId(null); handleCancel(call.id); }}
                              style={{
                                background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.3)',
                                color: T.red, borderRadius: 6, padding: '4px 10px',
                                cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                              }}>Confirm</button>
                            <button onClick={() => setConfirmCancelId(null)}
                              style={{
                                background: 'transparent', border: `1px solid ${T.border}`,
                                color: T.muted, borderRadius: 6, padding: '4px 10px',
                                cursor: 'pointer', fontSize: 12, fontFamily: T.font,
                              }}>Keep</button>
                          </div>
                        ) : (
                          <button onClick={() => setConfirmCancelId(call.id)}
                            style={{
                              background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.25)',
                              color: T.red, borderRadius: 6, padding: '4px 12px',
                              cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                            }}>Cancel</button>
                        )
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
