import { useEffect, useState, useCallback } from 'react';
import { formatDateTime } from '../../utils/dateFormat';
import AuthAudio from '../AuthAudio';
import { useAuth } from '../../contexts/AuthContext';
import { API_URL } from '../../constants/api';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981',
  text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif",
};

const SENTIMENT_STYLE = {
  positive: { color: '#15803d', bg: 'rgba(34,197,94,0.12)',  border: 'rgba(34,197,94,0.3)',  emoji: '😊' },
  neutral:  { color: '#6b7280', bg: 'rgba(148,163,184,0.12)', border: 'rgba(148,163,184,0.3)', emoji: '😐' },
  negative: { color: '#dc2626', bg: 'rgba(239,68,68,0.12)',   border: 'rgba(239,68,68,0.3)',  emoji: '☹️' },
  annoyed:  { color: '#c2410c', bg: 'rgba(251,146,60,0.12)',  border: 'rgba(251,146,60,0.3)', emoji: '😤' },
};

const LANG_NAMES = {
  en: 'English', hi: 'Hindi',  mr: 'Marathi', bn: 'Bengali',
  gu: 'Gujarati', pa: 'Punjabi', ta: 'Tamil', te: 'Telugu',
  kn: 'Kannada', ml: 'Malayalam',
};

const NAME_PATTERNS = [
  /\bI[' ]?m\s+([A-Z][a-zA-Z]{1,18})/,
  /\bI am\s+([A-Z][a-zA-Z]{1,18})/,
  /\bthis is\s+([A-Z][a-zA-Z]{1,18})/i,
  /(?:मैं|मे)\s+([A-Z][a-zA-Z]{1,18})/,
  /(?:मैं|मे)\s+([ऀ-ॿ]{2,12})\s+(?:बात|बोल)/,
];

function extractAgentName(turns) {
  if (!Array.isArray(turns)) return 'AI';
  for (const t of turns) {
    if ((t.role || '').toLowerCase() !== 'ai' && (t.role || '').toLowerCase() !== 'model') continue;
    const text = (t.text || t.Text || '').slice(0, 240);
    if (!text) continue;
    for (const re of NAME_PATTERNS) {
      const m = text.match(re);
      if (m && m[1]) return m[1].trim();
    }
  }
  return 'AI';
}

// ConclusionCard — fetches AI conclusion lazily per transcript.
// POST /api/transcripts/{id}/conclusion (cached; ?force=1 to regenerate).
function ConclusionCard({ transcriptId, turns }) {
  const { apiFetch } = useAuth();
  const [state, setState] = useState({ status: 'idle', review: null, error: '' });

  const turnCount = Array.isArray(turns) ? turns.length : 0;
  const interactionHappened = turnCount >= 1;

  const fetchConclusion = useCallback((force = false) => {
    if (!transcriptId) return;
    setState((s) => ({ ...s, status: 'loading', error: '' }));
    const qs = force ? '?force=1' : '';
    apiFetch(`${API_URL}/transcripts/${transcriptId}/conclusion${qs}`, { method: 'POST' })
      .then(async (res) => {
        if (res.status === 204) { setState({ status: 'empty', review: null, error: '' }); return; }
        const body = await res.json().catch(() => ({}));
        if (!res.ok) { setState({ status: 'error', review: null, error: body?.error || `HTTP ${res.status}` }); return; }
        setState({ status: 'ready', review: body, error: '' });
      })
      .catch((e) => setState({ status: 'error', review: null, error: e?.message || 'network error' }));
  }, [apiFetch, transcriptId]);

  useEffect(() => {
    if (!transcriptId || !interactionHappened) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    fetchConclusion(false);
  }, [fetchConclusion, transcriptId, interactionHappened]);

  if (!interactionHappened) return null;

  const wrap = (children) => (
    <div style={{
      marginTop: 14, padding: '14px 16px', borderRadius: 10,
      background: '#ffffff', border: '1px solid #c4b5fd',
      boxShadow: '0 1px 3px rgba(0,0,0,0.06)',
    }}>
      <div style={{ fontSize: '0.82rem', fontWeight: 700, color: '#7c3aed', marginBottom: 8, display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 6 }}>
        <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          ✨ AI Conclusion
          <span style={{ fontSize: '0.7rem', fontWeight: 500, color: '#6b7280' }}>(Gemini)</span>
        </span>
        <button type="button" onClick={() => fetchConclusion(true)} disabled={state.status === 'loading'}
          title="Regenerate"
          style={{
            background: 'transparent', border: '1px solid rgba(139,92,246,0.3)',
            color: '#7c3aed', borderRadius: 6, padding: '3px 10px',
            fontSize: '0.72rem', cursor: state.status === 'loading' ? 'wait' : 'pointer',
            fontWeight: 600,
          }}>↻ Regenerate</button>
      </div>
      {children}
    </div>
  );

  if (state.status === 'loading' || state.status === 'idle')
    return wrap(<div style={{ color: '#6b7280', fontSize: '0.85rem' }}>Generating conclusion…</div>);
  if (state.status === 'error')
    return wrap(<div style={{ color: '#dc2626', fontSize: '0.82rem' }}>Could not generate conclusion: {state.error}</div>);
  if (state.status === 'empty')
    return wrap(<div style={{ color: '#6b7280', fontSize: '0.85rem' }}>No transcript turns to analyse.</div>);

  const r = state.review || {};
  const summary       = (r.summary || '').trim();
  const wentWell      = (r.what_went_well || '').trim();
  const wentWrong     = (r.what_went_wrong || '').trim();
  const failureReason = (r.failure_reason || '').trim();
  const suggestion    = (r.prompt_improvement_suggestion || '').trim();
  const insights      = (r.insights || '').trim();

  const score = Math.max(0, Math.min(5, Math.round(Number(r.quality_score) || 0)));
  const stars = '★'.repeat(score) + '☆'.repeat(5 - score);
  const rawSent = (r.customer_sentiment || r.sentiment || '').toLowerCase();
  const sStyle = SENTIMENT_STYLE[rawSent] || null;

  return wrap(
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10, fontSize: '0.85rem', color: '#1f2937', lineHeight: 1.5 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
        {score > 0 && (
          <span style={{ background: '#fef9c3', color: '#a16207', border: '1px solid #fde047', fontSize: '0.75rem', padding: '2px 8px', borderRadius: 12, fontWeight: 600 }}>
            {stars} {score}/5
          </span>
        )}
        {sStyle && (
          <span style={{ background: sStyle.bg, color: sStyle.color, border: `1px solid ${sStyle.border}`, fontSize: '0.75rem', padding: '2px 8px', borderRadius: 12, fontWeight: 600 }}>
            {sStyle.emoji} {rawSent}
          </span>
        )}
        <span style={{
          background: r.appointment_booked ? '#dcfce7' : '#f3f4f6',
          color: r.appointment_booked ? '#15803d' : '#6b7280',
          border: `1px solid ${r.appointment_booked ? '#86efac' : '#e5e7eb'}`,
          fontSize: '0.75rem', padding: '2px 8px', borderRadius: 12, fontWeight: 600,
        }}>
          {r.appointment_booked ? '✅ Appointment booked' : '❌ No appointment'}
        </span>
      </div>
      {summary       && <div><span style={{ color: '#7c3aed', fontWeight: 700 }}>Summary: </span>{summary}</div>}
      {wentWell      && <div><span style={{ color: '#15803d', fontWeight: 700 }}>What went well: </span>{wentWell}</div>}
      {wentWrong     && <div><span style={{ color: '#dc2626', fontWeight: 700 }}>What went wrong: </span>{wentWrong}</div>}
      {failureReason && !r.appointment_booked && <div><span style={{ color: '#c2410c', fontWeight: 700 }}>Why no booking: </span>{failureReason}</div>}
      {suggestion    && <div><span style={{ color: '#7c3aed', fontWeight: 700 }}>Suggested next step: </span>{suggestion}</div>}
      {!suggestion && insights && <div><span style={{ color: '#7c3aed', fontWeight: 700 }}>Coaching insight: </span>{insights}</div>}
    </div>
  );
}

export default function TranscriptModal({ transcriptLead, setTranscriptLead, transcripts, orgTimezone }) {
  const list = Array.isArray(transcripts) ? transcripts : [];

  useEffect(() => {
    if (!transcriptLead) return;
    const onKey = (e) => { if (e.key === 'Escape') setTranscriptLead(null); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [transcriptLead, setTranscriptLead]);

  if (!transcriptLead) return null;

  const close = () => setTranscriptLead(null);

  return (
    <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) close(); }}>
      <div style={{
        position: 'relative',
        background: T.card,
        border: `1px solid ${T.border}`,
        borderRadius: 16,
        boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
        maxWidth: 700,
        width: '90%',
        maxHeight: '85vh',
        display: 'flex',
        flexDirection: 'column',
        fontFamily: T.font,
      }}>

        {/* Header */}
        <div style={{
          flexShrink: 0,
          display: 'flex', justifyContent: 'space-between', alignItems: 'center',
          padding: '20px 24px 16px',
          borderBottom: `1px solid ${T.border}`,
        }}>
          <div>
            <h2 style={{ margin: 0, fontSize: '1.1rem', fontWeight: 700, color: T.text, display: 'flex', alignItems: 'center', gap: 8 }}>
              📋 Call Transcripts
            </h2>
            <p style={{ margin: '4px 0 0', color: T.muted, fontSize: '0.85rem' }}>
              {transcriptLead.first_name} — {transcriptLead.phone}
            </p>
          </div>
          <button onClick={close} style={{
            background: 'none', border: 'none', cursor: 'pointer',
            color: T.muted, fontSize: '1.2rem', lineHeight: 1, padding: 4,
          }}>✕</button>
        </div>

        {/* Body */}
        <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '16px 24px' }}>
          {list.length === 0 ? (
            <div style={{ padding: '3rem', textAlign: 'center', color: T.muted, background: T.bg, borderRadius: 12 }}>
              <div style={{ fontSize: '2rem', marginBottom: 12 }}>📞</div>
              <div style={{ fontWeight: 600, color: T.sub }}>No call transcripts yet.</div>
              <div style={{ fontSize: '0.85rem', marginTop: 8 }}>Transcripts will appear here after AI calls are completed.</div>
            </div>
          ) : (
            list.map((t, idx) => {
              const agentName = extractAgentName(t.transcript);
              return (
                <div key={t.id || idx} style={{
                  marginBottom: 16, background: T.bg, borderRadius: 12,
                  padding: '16px', border: `1px solid ${T.border}`,
                }}>
                  {/* Call header */}
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                      <span style={{ color: T.accent, fontWeight: 700, fontSize: '0.9rem' }}>Call #{list.length - idx}</span>
                      <span style={{ fontSize: '0.8rem', color: T.muted }}>{formatDateTime(t.created_at, orgTimezone)}</span>
                    </div>
                    <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                      {t.tts_language && (
                        <span style={{ background: 'rgba(16,185,129,0.1)', color: '#059669', fontSize: '0.72rem', fontWeight: 600, border: '1px solid rgba(16,185,129,0.25)', borderRadius: 6, padding: '2px 8px' }}>
                          🗣 {LANG_NAMES[t.tts_language] || t.tts_language.toUpperCase()}
                        </span>
                      )}
                      {t.call_duration_s > 0 && (
                        <span style={{ background: 'rgba(99,102,241,0.08)', color: T.accent, fontSize: '0.72rem', fontWeight: 600, border: `1px solid rgba(99,102,241,0.2)`, borderRadius: 6, padding: '2px 8px' }}>
                          {Math.round(t.call_duration_s)}s
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Audio player */}
                  {t.recording_url && (() => {
                    const url = t.recording_url || '';
                    const isWav = url.endsWith('.wav');
                    const isMp3 = url.endsWith('.mp3');
                    const isWebm = url.endsWith('.webm');
                    const sourceLabel = isWav ? '🖥️ Server Recording (Stereo)' : isMp3 ? '📞 Exotel Recording' : isWebm ? '🌐 Browser Recording' : '🔊 Recording';
                    const color = isWav ? '#0891b2' : isMp3 ? '#059669' : isWebm ? '#7c3aed' : T.accent;
                    return (
                      <div style={{ marginBottom: 12, padding: '10px 12px', background: T.card, borderRadius: 8, border: `1px solid ${T.border}` }}>
                        <div style={{ fontSize: '0.78rem', color, fontWeight: 600, marginBottom: 6 }}>{sourceLabel}</div>
                        <AuthAudio style={{ width: '100%', height: 36 }} src={url} />
                      </div>
                    );
                  })()}

                  {/* Turn-by-turn transcript */}
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    {(Array.isArray(t.transcript) ? t.transcript : []).map((turn, i) => {
                      const isAI = turn.role === 'AI';
                      return (
                        <div key={i} style={{ display: 'flex', flexDirection: isAI ? 'row' : 'row-reverse', gap: 8, alignItems: 'flex-start' }}>
                          <div style={{
                            width: 28, height: 28, borderRadius: '50%', flexShrink: 0,
                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                            fontSize: '0.75rem', fontWeight: 700,
                            background: isAI ? 'rgba(99,102,241,0.1)' : 'rgba(16,185,129,0.1)',
                            color: isAI ? T.accent : T.green,
                            border: `1px solid ${isAI ? 'rgba(99,102,241,0.25)' : 'rgba(16,185,129,0.25)'}`,
                          }}>
                            {isAI ? '🤖' : '👤'}
                          </div>
                          <div style={{
                            maxWidth: '75%', padding: '10px 14px', borderRadius: 12,
                            background: isAI ? 'rgba(99,102,241,0.06)' : '#ffffff',
                            border: `1px solid ${isAI ? 'rgba(99,102,241,0.15)' : T.border}`,
                            color: T.text, fontSize: '0.88rem', lineHeight: 1.55,
                          }}>
                            <div style={{ fontSize: '0.7rem', fontWeight: 700, marginBottom: 4, color: isAI ? T.accent : T.green }}>
                              {isAI ? `${agentName} (AI)` : transcriptLead.first_name || 'User'}
                            </div>
                            {turn.text}
                          </div>
                        </div>
                      );
                    })}
                  </div>

                  {/* AI Conclusion */}
                  {t.id && (
                    <ConclusionCard
                      transcriptId={t.id}
                      turns={Array.isArray(t.transcript) ? t.transcript : []}
                    />
                  )}
                </div>
              );
            })
          )}
        </div>

        {/* Footer */}
        <div style={{ flexShrink: 0, borderTop: `1px solid ${T.border}`, padding: '12px 24px', textAlign: 'center' }}>
          <button onClick={close} style={{
            background: T.accent, border: 'none', color: '#fff',
            padding: '8px 32px', borderRadius: 8, fontSize: '0.9rem',
            fontWeight: 600, cursor: 'pointer', fontFamily: T.font,
          }}>Close</button>
        </div>
      </div>
    </div>
  );
}
