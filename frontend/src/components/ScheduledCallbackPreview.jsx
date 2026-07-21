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
  borderRadius: 12, boxShadow: '0 8px 40px rgba(0,0,0,0.12)',
};

const btnPrimary = {
  background: T.accent, border: 'none', color: '#fff',
  borderRadius: 8, padding: '8px 18px', cursor: 'pointer',
  fontSize: 13, fontWeight: 600, fontFamily: T.font,
};

const btnGhost = {
  background: '#fff', border: `1px solid ${T.border}`, color: T.sub,
  borderRadius: 8, padding: '8px 18px', cursor: 'pointer',
  fontSize: 13, fontWeight: 600, fontFamily: T.font,
};

function outcomeFor(t) {
  const duration = parseFloat(t.call_duration_s || 0);
  if (duration > 30) return 'Completed';
  if (duration > 5) return 'Connected';
  return 'No Answer';
}

const AUTO_START_SECONDS = 5;

export default function ScheduledCallbackPreview({ call, onStart, onDismiss, apiFetch, API_URL, orgTimezone }) {
  const [lead, setLead] = useState(null);
  const [transcripts, setTranscripts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [secondsLeft, setSecondsLeft] = useState(AUTO_START_SECONDS);
  const [autoStarting, setAutoStarting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    setSecondsLeft(AUTO_START_SECONDS);
    setAutoStarting(false);
    (async () => {
      try {
        const [leadRes, txRes] = await Promise.all([
          apiFetch(`${API_URL}/leads/${call.lead_id}`),
          apiFetch(`${API_URL}/leads/${call.lead_id}/transcripts`),
        ]);
        if (!leadRes.ok || !txRes.ok) throw new Error('Failed to load preview');
        const leadData = await leadRes.json();
        const txData = await txRes.json();
        if (!cancelled) {
          setLead(leadData);
          setTranscripts(Array.isArray(txData) ? txData : []);
        }
      } catch (e) {
        if (!cancelled) setError(e.message || 'Failed to load preview');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [call.lead_id, apiFetch, API_URL]);

  // Countdown to auto-start the browser call once the preview is loaded.
  useEffect(() => {
    if (loading || error || !lead || autoStarting) return;
    if (secondsLeft <= 0) {
      setAutoStarting(true);
      onStart();
      return;
    }
    const id = setInterval(() => {
      setSecondsLeft(s => s - 1);
    }, 1000);
    return () => clearInterval(id);
  }, [loading, error, lead, secondsLeft, autoStarting, onStart]);

  const handleManualStart = () => {
    setAutoStarting(true);
    onStart();
  };

  return (
    <div
      className="modal-overlay"
      onClick={e => { if (e.target === e.currentTarget) onDismiss(); }}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}
      style={{
        position: 'fixed', inset: 0, background: 'rgba(2,6,23,0.6)',
        backdropFilter: 'blur(4px)', display: 'flex', alignItems: 'center',
        justifyContent: 'center', zIndex: 10001, padding: '1rem'
      }}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{ ...card, maxWidth: 560, width: '100%', maxHeight: '85vh', overflow: 'auto', padding: '1.5rem', fontFamily: T.font }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h3 style={{ margin: 0, color: T.text, fontSize: 18, fontWeight: 700 }}>📅 Scheduled Callback Ready</h3>
          <button onClick={onDismiss} style={{ background: 'transparent', border: 'none', color: T.muted, fontSize: '1.2rem', cursor: 'pointer' }}>✕</button>
        </div>

        {loading ? (
          <div style={{ color: T.muted, padding: '1rem 0' }}>Loading customer details…</div>
        ) : error ? (
          <div style={{ color: T.red, padding: '1rem 0' }}>{error}</div>
        ) : (
          <>
            <div style={{ background: '#f8fafc', border: `1px solid ${T.border}`, borderRadius: 10, padding: '1rem', marginBottom: '1rem' }}>
              <div style={{ fontSize: '0.75rem', color: T.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: 6 }}>Customer</div>
              <div style={{ fontSize: '1.1rem', fontWeight: 700, color: T.text }}>
                {lead?.first_name} {lead?.last_name}
              </div>
              <div style={{ fontFamily: T.mono, fontSize: '0.9rem', color: T.sub, marginTop: 4 }}>
                {lead?.phone || call.phone}
              </div>
              {(lead?.source || lead?.status) && (
                <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
                  {lead?.source && <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: T.accent, background: `${T.accent}15` }}>{lead.source}</span>}
                  {lead?.status && <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: T.sub, background: '#e5e7eb' }}>{lead.status}</span>}
                </div>
              )}
            </div>

            {(lead?.follow_up_note || call.notes) && (
              <div style={{ background: 'rgba(245,158,11,0.06)', border: `1px solid rgba(245,158,11,0.25)`, borderRadius: 10, padding: '1rem', marginBottom: '1rem' }}>
                <div style={{ fontSize: '0.75rem', color: T.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: 6 }}>Remarks</div>
                {lead?.follow_up_note && <div style={{ fontSize: '0.9rem', color: T.text, marginBottom: 6 }}>{lead.follow_up_note}</div>}
                {call.notes && <div style={{ fontSize: '0.85rem', color: T.sub }}><em>Schedule note:</em> {call.notes}</div>}
              </div>
            )}

            <div style={{ marginBottom: '1rem' }}>
              <div style={{ fontSize: '0.75rem', color: T.muted, fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: 8 }}>Previous Call History ({transcripts.length})</div>
              {transcripts.length === 0 ? (
                <div style={{ color: T.muted, fontSize: '0.85rem' }}>No previous calls found.</div>
              ) : (
                <div style={{ border: `1px solid ${T.border}`, borderRadius: 10, overflow: 'hidden' }}>
                  <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
                    <thead>
                      <tr style={{ background: T.bg }}>
                        <th style={{ padding: '8px 10px', textAlign: 'left', color: T.muted, fontWeight: 600 }}>Time</th>
                        <th style={{ padding: '8px 10px', textAlign: 'left', color: T.muted, fontWeight: 600 }}>Outcome</th>
                        <th style={{ padding: '8px 10px', textAlign: 'left', color: T.muted, fontWeight: 600 }}>Recording</th>
                      </tr>
                    </thead>
                    <tbody>
                      {transcripts.map(t => (
                        <tr key={t.id} style={{ borderTop: `1px solid ${T.border}` }}>
                          <td style={{ padding: '8px 10px', color: T.sub }}>{formatDateTime(t.created_at, orgTimezone)}</td>
                          <td style={{ padding: '8px 10px', color: T.sub }}>{outcomeFor(t)}</td>
                          <td style={{ padding: '8px 10px' }}>
                            {t.recording_url ? (
                              <a href={t.recording_url} target="_blank" rel="noreferrer" style={{ color: T.accent, textDecoration: 'underline' }}>Play</a>
                            ) : (
                              <span style={{ color: T.muted }}>-</span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </>
        )}

        <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', alignItems: 'center', marginTop: '1rem' }}>
          {!loading && !error && (
            <div style={{ fontSize: 13, color: T.muted, marginRight: 'auto' }}>
              {autoStarting ? 'Connecting…' : `Auto-starting call in ${secondsLeft}s`}
            </div>
          )}
          <button onClick={onDismiss} disabled={autoStarting} style={{ ...btnGhost, opacity: autoStarting ? 0.6 : 1 }}>Dismiss</button>
          <button onClick={handleManualStart} disabled={loading || autoStarting} style={{ ...btnPrimary, background: T.green, opacity: (loading || autoStarting) ? 0.6 : 1 }}>
            {autoStarting ? 'Starting…' : '▶ Start Call'}
          </button>
        </div>
      </div>
    </div>
  );
}
