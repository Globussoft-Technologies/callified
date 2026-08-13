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

function toLocalInputValue(isoString) {
  const d = new Date(isoString);
  if (isNaN(d.getTime())) return '';
  const pad = n => String(n).padStart(2, '0');
  const offsetMs = d.getTimezoneOffset() * 60000;
  const local = new Date(d.getTime() - offsetMs);
  return `${local.getFullYear()}-${pad(local.getMonth() + 1)}-${pad(local.getDate())}T${pad(local.getHours())}:${pad(local.getMinutes())}`;
}

export default function ScheduledCallbackPreview({ call, onStart, onDismiss, onRescheduled, apiFetch, API_URL, orgTimezone, currentUser, toast }) {
  const [lead, setLead] = useState(null);
  const [transcripts, setTranscripts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [secondsLeft, setSecondsLeft] = useState(AUTO_START_SECONDS);
  const [autoStarting, setAutoStarting] = useState(false);
  const [rescheduling, setRescheduling] = useState(false);
  const [newScheduleAt, setNewScheduleAt] = useState(() => toLocalInputValue(call.scheduled_time));
  const [newScheduleMode, setNewScheduleMode] = useState(call.mode || 'manual');
  const [newScheduleNotes, setNewScheduleNotes] = useState(call.notes || '');
  const [rescheduleSaving, setRescheduleSaving] = useState(false);
  const [rescheduleError, setRescheduleError] = useState('');
  const [dismissing, setDismissing] = useState(false);
  const [dismissError, setDismissError] = useState('');

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
    if (loading || error || !lead || autoStarting || rescheduling) return;
    if (secondsLeft <= 0) {
      setAutoStarting(true);
      onStart();
      return;
    }
    const id = setInterval(() => {
      setSecondsLeft(s => s - 1);
    }, 1000);
    return () => clearInterval(id);
  }, [loading, error, lead, secondsLeft, autoStarting, rescheduling, onStart]);

  const handleManualStart = () => {
    setAutoStarting(true);
    onStart();
  };

  const handleDismiss = async () => {
    if (!call?.id) {
      onDismiss();
      return;
    }
    setDismissing(true);
    setDismissError('');
    try {
      const res = await apiFetch(`${API_URL}/scheduled-calls/${call.id}`, { method: 'DELETE' });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setDismissError(data.error || `Failed to cancel (${res.status})`);
        return;
      }
      toast('Callback cancelled');
      onDismiss();
    } catch {
      setDismissError('Network error while cancelling.');
    } finally {
      setDismissing(false);
    }
  };

  return (
    <div
      className="modal-overlay"
      onClick={e => { if (e.target === e.currentTarget) handleDismiss(); }}
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
          <button onClick={handleDismiss} disabled={dismissing} style={{ background: 'transparent', border: 'none', color: T.muted, fontSize: '1.2rem', cursor: 'pointer' }}>✕</button>
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
          <button onClick={() => setRescheduling(true)} disabled={autoStarting || rescheduling} style={{ ...btnGhost, opacity: (autoStarting || rescheduling) ? 0.6 : 1 }}>
            📅 Reschedule
          </button>
          <button onClick={handleDismiss} disabled={autoStarting || rescheduling || dismissing} style={{ ...btnGhost, opacity: (autoStarting || rescheduling || dismissing) ? 0.6 : 1 }}>{dismissing ? 'Cancelling…' : 'Dismiss'}</button>
          <button onClick={handleManualStart} disabled={loading || autoStarting || rescheduling} style={{ ...btnPrimary, background: T.green, opacity: (loading || autoStarting || rescheduling) ? 0.6 : 1 }}>
            {autoStarting ? 'Starting…' : '▶ Start Call'}
          </button>
        </div>
      </div>

      {rescheduling && (
        <div
          className="modal-overlay"
          onClick={e => { if (e.target === e.currentTarget) { setRescheduling(false); setRescheduleError(''); } }}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}
          style={{
            position: 'fixed', inset: 0, background: 'rgba(2,6,23,0.6)',
            backdropFilter: 'blur(4px)', display: 'flex', alignItems: 'center',
            justifyContent: 'center', zIndex: 10002, padding: '1rem'
          }}
        >
          <div
            onClick={e => e.stopPropagation()}
            style={{ ...card, maxWidth: 440, width: '100%', padding: '1.5rem', fontFamily: T.font }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
              <h3 style={{ margin: 0, color: T.text, fontSize: 18, fontWeight: 700 }}>📅 Reschedule Callback</h3>
              <button
                onClick={() => { setRescheduling(false); setRescheduleError(''); }}
                style={{ background: 'transparent', border: 'none', color: T.muted, fontSize: '1.2rem', cursor: 'pointer' }}>
                ✕
              </button>
            </div>
            <p style={{ color: T.muted, fontSize: '0.85rem', marginBottom: '1.25rem' }}>
              {lead?.first_name || call.first_name} {lead?.last_name || ''} — {lead?.phone || call.phone}
            </p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <label style={{ fontSize: '0.8rem', color: T.sub, fontWeight: 600 }}>
                Date &amp; Time
                <input
                  type="datetime-local"
                  value={newScheduleAt}
                  onChange={e => { setNewScheduleAt(e.target.value); setRescheduleError(''); }}
                  min={(() => {
                    const d = new Date();
                    const pad = n => String(n).padStart(2, '0');
                    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
                  })()}
                  style={{
                    width: '100%', marginTop: 6, padding: '8px 10px',
                    border: `1px solid ${T.border}`, borderRadius: 8,
                    fontSize: 13, fontFamily: T.font, color: T.text, boxSizing: 'border-box'
                  }}
                />
              </label>
              <label style={{ fontSize: '0.8rem', color: T.sub, fontWeight: 600 }}>
                Callback mode
                <select
                  value={newScheduleMode}
                  onChange={e => setNewScheduleMode(e.target.value)}
                  style={{
                    width: '100%', marginTop: 6, padding: '8px 10px',
                    border: `1px solid ${T.border}`, borderRadius: 8,
                    fontSize: 13, fontFamily: T.font, color: T.text, boxSizing: 'border-box',
                    height: 38, background: '#fff'
                  }}
                >
                  <option value="manual">Manual / Browser Callback (auto-connect for you)</option>
                  <option value="ai">AI Dial</option>
                </select>
              </label>
              <label style={{ fontSize: '0.8rem', color: T.sub, fontWeight: 600 }}>
                Notes (optional)
                <textarea
                  value={newScheduleNotes}
                  onChange={e => setNewScheduleNotes(e.target.value)}
                  rows={3}
                  placeholder="e.g. follow-up on pricing discussion"
                  style={{
                    width: '100%', marginTop: 6, padding: '8px 10px',
                    border: `1px solid ${T.border}`, borderRadius: 8,
                    fontSize: 13, fontFamily: T.font, color: T.text, boxSizing: 'border-box',
                    resize: 'vertical'
                  }}
                />
              </label>
            </div>
            {(rescheduleError || dismissError) && (
              <div style={{
                marginTop: '1rem', padding: '8px 12px', borderRadius: 8, fontSize: '0.8rem',
                background: '#fee2e2', border: '1px solid #fca5a5', color: T.red
              }}>
                ⚠️ {rescheduleError || dismissError}
              </div>
            )}
            <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: '1.25rem' }}>
              <button
                onClick={() => { setRescheduling(false); setRescheduleError(''); }}
                disabled={rescheduleSaving}
                style={{ ...btnGhost, opacity: rescheduleSaving ? 0.6 : 1 }}>
                Cancel
              </button>
              <button
                disabled={rescheduleSaving || !newScheduleAt}
                onClick={async () => {
                  if (!newScheduleAt) return;
                  if (new Date(newScheduleAt).getTime() <= Date.now()) {
                    setRescheduleError('Please pick a future date and time.');
                    return;
                  }
                  setRescheduleError('');
                  setRescheduleSaving(true);
                  try {
                    const serverTime = new Date(newScheduleAt).toISOString();
                    const res = await apiFetch(`${API_URL}/scheduled-calls`, {
                      method: 'POST',
                      headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({
                        lead_id: call.lead_id,
                        campaign_id: call.campaign_id,
                        scheduled_at: serverTime,
                        notes: newScheduleNotes,
                        mode: newScheduleMode,
                        executive_id: call.executive_id || 0,
                        scheduled_by_user_id: currentUser?.id || 0,
                      }),
                    });
                    const data = await res.json().catch(() => ({}));
                    if (!res.ok) {
                      setRescheduleError(data.error || data.detail || `Failed to reschedule (${res.status})`);
                    } else {
                      toast('Callback rescheduled');
                      setRescheduling(false);
                      onDismiss();
                      if (onRescheduled) onRescheduled(data.id);
                    }
                  } catch {
                    setRescheduleError('Network error while rescheduling.');
                  }
                  setRescheduleSaving(false);
                }}
                style={{ ...btnPrimary, opacity: (rescheduleSaving || !newScheduleAt) ? 0.6 : 1 }}>
                {rescheduleSaving ? 'Saving…' : 'Save New Time'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
