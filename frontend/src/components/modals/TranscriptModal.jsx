import React from 'react';

export default function TranscriptModal({ transcriptLead, setTranscriptLead, transcripts }) {
  if (!transcriptLead) return null;

  return (
    <div className="modal-overlay">
      <div className="modal-content glass-panel" style={{background: 'rgba(15, 23, 42, 0.97)', border: '1px solid rgba(99, 102, 241, 0.2)', maxWidth: '700px', maxHeight: '85vh', display: 'flex', flexDirection: 'column'}}>
        <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem', borderBottom: '1px solid rgba(255,255,255,0.05)', paddingBottom: '1rem'}}>
          <div>
            <h2 style={{marginTop: 0, marginBottom: '4px', color: '#818cf8', display: 'flex', alignItems: 'center', gap: '8px'}}>📋 Call Transcripts</h2>
            <p style={{margin: 0, color: '#94a3b8', fontSize: '0.9rem'}}>{transcriptLead.first_name} — {transcriptLead.phone}</p>
          </div>
          <button className="btn-call" style={{borderColor: 'transparent', color: '#cbd5e1', background: 'transparent', fontSize: '1.2rem'}} onClick={() => setTranscriptLead(null)}>✕</button>
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
                    <span style={{fontSize: '0.8rem', color: '#64748b'}}>{new Date(t.created_at + (t.created_at.endsWith('Z') ? '' : 'Z')).toLocaleString()}</span>
                  </div>
                  {t.call_duration_s > 0 && (
                    <span className="badge" style={{background: 'rgba(99, 102, 241, 0.1)', color: '#818cf8', fontSize: '0.75rem'}}>{Math.round(t.call_duration_s)}s</span>
                  )}
                </div>

                {/* Audio Player */}
                {t.recording_url && (
                  <div style={{marginBottom: '1rem', padding: '10px', background: 'rgba(99, 102, 241, 0.05)', borderRadius: '8px', border: '1px solid rgba(99, 102, 241, 0.15)'}}>
                    <div style={{fontSize: '0.8rem', color: '#818cf8', marginBottom: '6px', fontWeight: 600}}>🔊 Call Recording</div>
                    <audio controls style={{width: '100%', height: '36px'}} src={t.recording_url}>
                      Your browser does not support the audio element.
                    </audio>
                  </div>
                )}

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
