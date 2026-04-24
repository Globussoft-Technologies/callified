import React, { useEffect } from 'react';
import { formatDateTime } from '../../utils/dateFormat';

export default function TranscriptModal({ transcriptLead, setTranscriptLead, transcripts, orgTimezone }) {
  // Esc-to-close so there's always a keyboard escape even if the ✕ is
  // somehow hidden by an overflow/scroll edge case.
  useEffect(() => {
    if (!transcriptLead) return;
    const onKey = (e) => { if (e.key === 'Escape') setTranscriptLead(null); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [transcriptLead, setTranscriptLead]);

  if (!transcriptLead) return null;

  const close = () => setTranscriptLead(null);

  return (
    // Clicking the dark backdrop closes — instinctive escape.
    <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) close(); }}>
      {/*
        VIEWPORT-FIXED close button: position: fixed anchors it to the
        browser window, NOT the modal panel — so it cannot be clipped by
        any flex/overflow/scroll edge-case on the panel. Large, red, and
        pinned to the top-right of the browser tab. Impossible to miss,
        impossible to hide. Sits above the modal at z-index 100.
      */}
      <button
        type="button"
        aria-label="Close transcripts"
        onClick={close}
        style={{
          position: 'fixed', top: '16px', right: '16px', zIndex: 100,
          width: '48px', height: '48px', borderRadius: '50%',
          border: '2px solid #fff',
          background: '#ef4444', color: '#fff',
          fontSize: '1.4rem', fontWeight: 700, lineHeight: 1,
          cursor: 'pointer', boxShadow: '0 4px 16px rgba(0,0,0,0.5)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}
      >✕</button>

      <div className="modal-content glass-panel" style={{position: 'relative', background: 'rgba(15, 23, 42, 0.97)', border: '1px solid rgba(99, 102, 241, 0.2)', maxWidth: '700px', maxHeight: '85vh', display: 'flex', flexDirection: 'column'}}>

        <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem', borderBottom: '1px solid rgba(255,255,255,0.05)', paddingBottom: '1rem', paddingRight: '44px'}}>
          <div>
            <h2 style={{marginTop: 0, marginBottom: '4px', color: '#818cf8', display: 'flex', alignItems: 'center', gap: '8px'}}>📋 Call Transcripts</h2>
            <p style={{margin: 0, color: '#94a3b8', fontSize: '0.9rem'}}>{transcriptLead.first_name} — {transcriptLead.phone}</p>
          </div>
        </div>

        {/* Bottom-of-modal close bar — belt-and-braces in case the fixed
            top-right button is obstructed by a browser extension or custom
            toolbar. Sticks to the bottom of the panel, always in view. */}
        <div style={{order: 99, borderTop: '1px solid rgba(255,255,255,0.05)', paddingTop: '12px', marginTop: '12px', textAlign: 'center'}}>
          <button
            type="button"
            onClick={close}
            style={{
              background: 'rgba(99, 102, 241, 0.15)',
              border: '1px solid rgba(99, 102, 241, 0.3)',
              color: '#cbd5e1',
              padding: '8px 24px',
              borderRadius: '8px',
              fontSize: '0.9rem',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >Close</button>
        </div>

        <div style={{flex: 1, overflowY: 'auto', paddingRight: '8px'}}>
          {transcripts.length === 0 ? (
            <div style={{padding: '3rem', textAlign: 'center', color: '#64748b', background: 'rgba(0,0,0,0.2)', borderRadius: '12px'}}>
              <div style={{fontSize: '2rem', marginBottom: '12px'}}>📞</div>
              <div>No call transcripts yet.</div>
              <div style={{fontSize: '0.85rem', marginTop: '8px'}}>Transcripts will appear here after AI calls are completed.</div>
            </div>
          ) : (
            transcripts.map((t, idx) => (
              <div key={t.id || idx} style={{marginBottom: '1.5rem', background: 'rgba(0,0,0,0.2)', borderRadius: '12px', padding: '1.25rem', border: '1px solid rgba(255,255,255,0.05)'}}>
                <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem'}}>
                  <div style={{display: 'flex', alignItems: 'center', gap: '10px'}}>
                    <span style={{color: '#818cf8', fontWeight: 600}}>Call #{transcripts.length - idx}</span>
                    <span style={{fontSize: '0.8rem', color: '#64748b'}}>{formatDateTime(t.created_at, orgTimezone)}</span>
                  </div>
                  {t.call_duration_s > 0 && (
                    <span className="badge" style={{background: 'rgba(99, 102, 241, 0.1)', color: '#818cf8', fontSize: '0.75rem'}}>{Math.round(t.call_duration_s)}s</span>
                  )}
                </div>

                {/* Audio Player — color-coded by source */}
                {t.recording_url && (() => {
                  const url = t.recording_url || '';
                  const isWav = url.endsWith('.wav');
                  const isMp3 = url.endsWith('.mp3');
                  const isWebm = url.endsWith('.webm');
                  const sourceLabel = isWav ? '🖥️ Server Recording (Stereo)' : isMp3 ? '📞 Exotel Recording' : isWebm ? '🌐 Browser Recording' : '🔊 Recording';
                  const color = isWav ? '#22d3ee' : isMp3 ? '#22c55e' : isWebm ? '#a855f7' : '#818cf8';
                  const bg = isWav ? 'rgba(34,211,238,0.05)' : isMp3 ? 'rgba(34,197,94,0.05)' : isWebm ? 'rgba(168,85,247,0.05)' : 'rgba(99,102,241,0.05)';
                  const border = isWav ? 'rgba(34,211,238,0.2)' : isMp3 ? 'rgba(34,197,94,0.2)' : isWebm ? 'rgba(168,85,247,0.2)' : 'rgba(99,102,241,0.15)';
                  // /api/recordings/* is auth-gated on the Go backend; the <audio>
                  // tag can't set an Authorization header, so append the JWT as
                  // a ?token= query param (backend accepts it for audio/SSE).
                  // External URLs (e.g. Exotel-hosted) are left untouched.
                  let audioSrc = url;
                  if (url.startsWith('/api/recordings/')) {
                    const token = localStorage.getItem('authToken');
                    if (token) {
                      const sep = url.includes('?') ? '&' : '?';
                      audioSrc = `${url}${sep}token=${encodeURIComponent(token)}`;
                    }
                  }
                  return (
                    <div style={{marginBottom: '1rem', padding: '10px', background: bg, borderRadius: '8px', border: `1px solid ${border}`}}>
                      <div style={{fontSize: '0.8rem', color, marginBottom: '6px', fontWeight: 600}}>{sourceLabel}</div>
                      <audio controls style={{width: '100%', height: '36px'}} src={audioSrc}>
                        Your browser does not support the audio element.
                      </audio>
                    </div>
                  );
                })()}

                {/* Turn-by-turn transcript */}
                <div style={{display: 'flex', flexDirection: 'column', gap: '8px'}}>
                  {(Array.isArray(t.transcript) ? t.transcript : []).map((turn, i) => (
                    <div key={i} style={{
                      display: 'flex',
                      flexDirection: turn.role === 'AI' ? 'row' : 'row-reverse',
                      gap: '8px',
                      alignItems: 'flex-start'
                    }}>
                      <div style={{
                        width: '28px', height: '28px', borderRadius: '50%', flexShrink: 0,
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: '0.75rem', fontWeight: 700,
                        background: turn.role === 'AI' ? 'rgba(99, 102, 241, 0.2)' : 'rgba(34, 197, 94, 0.2)',
                        color: turn.role === 'AI' ? '#818cf8' : '#4ade80',
                        border: `1px solid ${turn.role === 'AI' ? 'rgba(99, 102, 241, 0.3)' : 'rgba(34, 197, 94, 0.3)'}`
                      }}>
                        {turn.role === 'AI' ? '🤖' : '👤'}
                      </div>
                      <div style={{
                        maxWidth: '75%', padding: '10px 14px', borderRadius: '12px',
                        background: turn.role === 'AI' ? 'rgba(99, 102, 241, 0.08)' : 'rgba(34, 197, 94, 0.08)',
                        border: `1px solid ${turn.role === 'AI' ? 'rgba(99, 102, 241, 0.15)' : 'rgba(34, 197, 94, 0.15)'}`,
                        color: '#e2e8f0', fontSize: '0.9rem', lineHeight: '1.5'
                      }}>
                        <div style={{fontSize: '0.7rem', fontWeight: 600, marginBottom: '4px', color: turn.role === 'AI' ? '#818cf8' : '#4ade80'}}>
                          {turn.role === 'AI' ? 'Arjun (AI)' : transcriptLead.first_name || 'User'}
                        </div>
                        {turn.text}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
