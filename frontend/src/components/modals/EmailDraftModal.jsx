import React from 'react';

export default function EmailDraftModal({ emailDraft, setEmailDraft }) {
  if (!emailDraft) return null;

  return (
    <div className="modal-overlay">
      <div className="modal-content glass-panel" style={{background: 'rgba(15, 23, 42, 0.95)', border: '1px solid rgba(245, 158, 11, 0.2)'}}>
        <h2 style={{marginTop: 0, color: '#f59e0b', display: 'flex', alignItems: 'center', gap: '8px'}}>✨ GenAI Drafted Email</h2>
        
        <div style={{background: 'rgba(0,0,0,0.3)', padding: '15px', borderRadius: '8px', marginBottom: '15px', border: '1px solid rgba(255,255,255,0.05)'}}>
          <div style={{marginBottom: '10px', fontWeight: 'bold'}}>Subject: <span style={{fontWeight: 'normal', color: '#e2e8f0'}}>{emailDraft.subject}</span></div>
          <div style={{whiteSpace: 'pre-wrap', color: '#94a3b8', lineHeight: '1.5'}}>{emailDraft.body}</div>
        </div>

        <div style={{display: 'flex', gap: '10px', justifyContent: 'flex-end'}}>
          <button className="btn-secondary" onClick={() => setEmailDraft(null)}>Close</button>
          <button className="btn-primary" style={{background: 'linear-gradient(135deg, #f59e0b, #dc2626)'}} onClick={() => {
            navigator.clipboard.writeText(`Subject: ${emailDraft.subject}\n\n${emailDraft.body}`);
            alert("Copied directly to clipboard!");
          }}>
            📋 Copy to Clipboard
          </button>
        </div>
      </div>
    </div>
  );
}
