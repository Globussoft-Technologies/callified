import React, { useState, useEffect, useRef } from 'react';
import { formatDateTime } from '../utils/dateFormat';

export default function ScheduledCallsPage({ apiFetch, API_URL, orgTimezone }) {
  const [scheduledCalls, setScheduledCalls] = useState([]);
  const [loading, setLoading] = useState(true);
  const [confirmCancelId, setConfirmCancelId] = useState(null);
  const intervalRef = useRef(null);

  const fetchScheduledCalls = async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/scheduled-calls`);
      setScheduledCalls(await res.json());
    } catch (e) { console.error('Failed to fetch scheduled calls', e); }
    if (!silent) setLoading(false);
  };

  useEffect(() => {
    fetchScheduledCalls();

    // Poll every 10s so statuses update in real-time (pending → dialing → completed/failed)
    intervalRef.current = setInterval(() => fetchScheduledCalls(true), 10000);
    return () => clearInterval(intervalRef.current);
  }, []);

  const handleCancel = async (id) => {
    try {
      await apiFetch(`${API_URL}/scheduled-calls/${id}`, { method: 'DELETE' });
      setConfirmCancelId(null);
      fetchScheduledCalls(true);
    } catch (e) { alert('Failed to cancel'); }
  };

  const statusStyle = (status) => {
    const map = {
      pending:   { color: '#f59e0b', bg: 'rgba(245,158,11,0.1)', border: 'rgba(245,158,11,0.3)' },
      dialing:   { color: '#60a5fa', bg: 'rgba(96,165,250,0.1)', border: 'rgba(96,165,250,0.3)' },
      completed: { color: '#22c55e', bg: 'rgba(34,197,94,0.1)',  border: 'rgba(34,197,94,0.3)' },
      failed:    { color: '#ef4444', bg: 'rgba(239,68,68,0.1)',  border: 'rgba(239,68,68,0.3)' },
      cancelled: { color: '#94a3b8', bg: 'rgba(148,163,184,0.1)', border: 'rgba(148,163,184,0.3)' },
    };
    return map[status] || map.pending;
  };

  if (loading) {
    return (
      <div className="page-container">
        <div className="glass-panel" style={{padding: '2rem', textAlign: 'center', color: '#94a3b8'}}>
          Loading scheduled calls...
        </div>
      </div>
    );
  }

  const hasActive = scheduledCalls.some(c => c.status === 'pending' || c.status === 'dialing');

  return (
    <div className="page-container">
      <div style={{display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '1.5rem'}}>
        <h2 style={{margin: 0, color: '#e2e8f0'}}>Scheduled Calls</h2>
        {hasActive && (
          <span style={{fontSize: '0.72rem', color: '#60a5fa', background: 'rgba(96,165,250,0.1)',
            border: '1px solid rgba(96,165,250,0.25)', borderRadius: '20px', padding: '2px 10px', fontWeight: 600}}>
            ● live
          </span>
        )}
        <button onClick={() => fetchScheduledCalls(true)}
          style={{marginLeft: 'auto', background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.1)',
            color: '#94a3b8', borderRadius: '6px', padding: '4px 12px', cursor: 'pointer', fontSize: '0.78rem'}}>
          ↻ Refresh
        </button>
      </div>

      {scheduledCalls.length === 0 ? (
        <div className="glass-panel" style={{padding: '2rem', textAlign: 'center', color: '#64748b'}}>
          No scheduled calls. Schedule calls from the CRM or campaign pages.
        </div>
      ) : (
        <div className="glass-panel" style={{overflowX: 'auto'}}>
          <table className="leads-table" style={{width: '100%'}}>
            <thead>
              <tr>
                <th>Scheduled Time</th>
                <th>Lead Name</th>
                <th>Phone</th>
                <th>Status</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              {scheduledCalls.map(call => {
                const sc = statusStyle(call.status);
                return (
                  <React.Fragment key={call.id}>
                    <tr>
                      <td style={{fontSize: '0.85rem', color: '#e2e8f0'}}>
                        {formatDateTime(call.scheduled_time || call.scheduled_at, orgTimezone)}
                      </td>
                      <td style={{fontWeight: 600}}>{call.lead_name || call.first_name || '-'}</td>
                      <td style={{fontFamily: 'SFMono-Regular, Consolas, monospace', color: '#cbd5e1', fontSize: '0.85rem'}}>
                        {call.phone || '-'}
                      </td>
                      <td>
                        <span style={{
                          padding: '3px 10px', borderRadius: '12px', fontSize: '0.75rem', fontWeight: 600,
                          color: sc.color, background: sc.bg, border: `1px solid ${sc.border}`,
                        }}>
                          {call.status === 'dialing' ? '⏳ dialing' : call.status}
                        </span>
                      </td>
                      <td>
                        {call.status === 'pending' && (
                          <button onClick={() => setConfirmCancelId(confirmCancelId === call.id ? null : call.id)}
                            style={{
                              background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
                              color: '#fca5a5', borderRadius: '6px', padding: '4px 12px',
                              cursor: 'pointer', fontSize: '0.75rem', fontWeight: 600,
                            }}>
                            Cancel
                          </button>
                        )}
                      </td>
                    </tr>
                    {confirmCancelId === call.id && (
                      <tr>
                        <td colSpan={5} style={{padding: 0}}>
                          <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                            background: 'rgba(239,68,68,0.1)', borderTop: '1px solid rgba(239,68,68,0.2)',
                            padding: '10px 14px', gap: '10px'}}>
                            <span style={{color: '#fca5a5', fontSize: '0.85rem'}}>
                              Cancel this scheduled call for <strong>{call.lead_name || call.first_name}</strong>?
                            </span>
                            <div style={{display: 'flex', gap: '8px', flexShrink: 0}}>
                              <button onClick={() => setConfirmCancelId(null)}
                                style={{padding: '4px 14px', borderRadius: '6px', border: '1px solid rgba(255,255,255,0.15)',
                                  background: 'transparent', color: '#94a3b8', cursor: 'pointer', fontSize: '0.82rem'}}>
                                Keep
                              </button>
                              <button onClick={() => handleCancel(call.id)}
                                style={{padding: '4px 14px', borderRadius: '6px', border: 'none',
                                  background: '#ef4444', color: '#fff', cursor: 'pointer', fontSize: '0.82rem', fontWeight: 600}}>
                                Yes, Cancel
                              </button>
                            </div>
                          </div>
                        </td>
                      </tr>
                    )}
                  </React.Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
