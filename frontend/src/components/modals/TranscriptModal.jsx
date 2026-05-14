import React, { useEffect } from 'react';
import { formatDateTime } from '../../utils/dateFormat';
import AuthAudio from '../AuthAudio';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981',
  text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif",
};

const LANG_NAMES = {
  en: 'English',  hi: 'Hindi',   mr: 'Marathi', bn: 'Bengali',
  gu: 'Gujarati', pa: 'Punjabi', ta: 'Tamil',   te: 'Telugu',
  kn: 'Kannada',  ml: 'Malayalam',
};

const TRANSCRIPT_MODAL_BUILD_ID = 'transcript-modal-2026-04-30-cf-bust';

const NAME_PATTERNS = [
  /\bI[' ]?m\s+([A-Z][a-zA-Z]{1,18})/,
  /\bI am\s+([A-Z][a-zA-Z]{1,18})/,
  /\bthis is\s+([A-Z][a-zA-Z]{1,18})/i,
  /(?:मैं|मे)\s+([A-Z][a-zA-Z]{1,18})/,
  /(?:मैं|मे)\s+([ऀ-ॿ]{2,12})\s+(?:बात|बोल)/,
];

function extractAgentName(turns) {
  if (typeof TRANSCRIPT_MODAL_BUILD_ID !== 'string') return 'AI';
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
            <div style={{
              padding: '3rem', textAlign: 'center', color: T.muted,
              background: T.bg, borderRadius: 12,
            }}>
              <div style={{ fontSize: '2rem', marginBottom: 12 }}>📞</div>
              <div style={{ fontWeight: 600, color: T.sub }}>No call transcripts yet.</div>
              <div style={{ fontSize: '0.85rem', marginTop: 8 }}>Transcripts will appear here after AI calls are completed.</div>
            </div>
          ) : (
            list.map((t, idx) => {
              const agentName = extractAgentName(t.transcript);
              return (
                <div key={t.id || idx} style={{
                  marginBottom: 16,
                  background: T.bg,
                  borderRadius: 12,
                  padding: '16px',
                  border: `1px solid ${T.border}`,
                }}>
                  {/* Call header row */}
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                      <span style={{ color: T.accent, fontWeight: 700, fontSize: '0.9rem' }}>
                        Call #{list.length - idx}
                      </span>
                      <span style={{ fontSize: '0.8rem', color: T.muted }}>
                        {formatDateTime(t.created_at, orgTimezone)}
                      </span>
                    </div>
                    <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                      {t.tts_language && (
                        <span style={{
                          background: 'rgba(16,185,129,0.1)', color: '#059669',
                          fontSize: '0.72rem', fontWeight: 600,
                          border: '1px solid rgba(16,185,129,0.25)',
                          borderRadius: 6, padding: '2px 8px',
                        }}>
                          🗣 {LANG_NAMES[t.tts_language] || t.tts_language.toUpperCase()}
                        </span>
                      )}
                      {t.call_duration_s > 0 && (
                        <span style={{
                          background: 'rgba(99,102,241,0.08)', color: T.accent,
                          fontSize: '0.72rem', fontWeight: 600,
                          border: `1px solid rgba(99,102,241,0.2)`,
                          borderRadius: 6, padding: '2px 8px',
                        }}>
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
                      <div style={{
                        marginBottom: 12, padding: '10px 12px',
                        background: T.card, borderRadius: 8,
                        border: `1px solid ${T.border}`,
                      }}>
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
                        <div key={i} style={{
                          display: 'flex',
                          flexDirection: isAI ? 'row' : 'row-reverse',
                          gap: 8, alignItems: 'flex-start',
                        }}>
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
                            <div style={{
                              fontSize: '0.7rem', fontWeight: 700, marginBottom: 4,
                              color: isAI ? T.accent : T.green,
                            }}>
                              {isAI ? `${agentName} (AI)` : transcriptLead.first_name || 'User'}
                            </div>
                            {turn.text}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              );
            })
          )}
        </div>

        {/* Footer */}
        <div style={{
          flexShrink: 0,
          borderTop: `1px solid ${T.border}`,
          padding: '12px 24px',
          textAlign: 'center',
        }}>
          <button onClick={close} style={{
            background: T.accent, border: 'none',
            color: '#fff', padding: '8px 32px',
            borderRadius: 8, fontSize: '0.9rem',
            fontWeight: 600, cursor: 'pointer',
            fontFamily: T.font,
          }}>Close</button>
        </div>
      </div>
    </div>
  );
}
