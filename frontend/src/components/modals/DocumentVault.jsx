import React from 'react';
import { formatDateTime } from '../../utils/dateFormat';

export default function DocumentVault({
  activeLeadDocs, setActiveLeadDocs, handleUploadDoc, docFormData, setDocFormData, docs, orgTimezone
}) {
  if (!activeLeadDocs) return null;

  return (
    <div className="modal-overlay" onClick={() => setActiveLeadDocs(null)} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
      <div className="glass-panel modal-content" onClick={e => e.stopPropagation()} style={{maxWidth: '600px'}} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
        <h2 style={{marginTop: 0, marginBottom: '0.5rem'}}>📁 Document Vault</h2>
        <p style={{color: '#94a3b8', marginBottom: '2rem'}}>Client: {activeLeadDocs.first_name} {activeLeadDocs.last_name}</p>
        
        <form onSubmit={handleUploadDoc} style={{display: 'flex', gap: '10px', marginBottom: '2rem', alignItems: 'flex-end'}}>
          <div className="form-group" style={{marginBottom: 0, flexGrow: 1}}>
            <label>Document Name</label>
            <input className="form-input" required value={docFormData.file_name} onChange={e => setDocFormData({...docFormData, file_name: e.target.value})} placeholder="e.g., Aadhar_Card.pdf" />
          </div>
          <div className="form-group" style={{marginBottom: 0, flexGrow: 1}}>
            <label>Mock File URL</label>
            <input className="form-input" required value={docFormData.file_url} onChange={e => setDocFormData({...docFormData, file_url: e.target.value})} placeholder="https://bdrpl.com/vault/..." />
          </div>
          <button type="submit" className="btn-primary" style={{height: '46px', padding: '0 16px'}}>Upload</button>
        </form>

        <h3 style={{fontSize: '1.1rem', marginBottom: '1rem'}}>Secure Uploads</h3>
        <div style={{maxHeight: '300px', overflowY: 'auto'}}>
          {docs.length === 0 ? (
            <div style={{padding: '2rem', textAlign: 'center', color: '#64748b', background: 'rgba(0,0,0,0.2)', borderRadius: '8px'}}>No documents found for this client.</div>
          ) : (
            <div style={{display: 'flex', flexDirection: 'column', gap: '8px'}}>
              {docs.map(doc => (
                <div key={doc.id} style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', background: 'rgba(255,255,255,0.05)', padding: '12px 16px', borderRadius: '8px'}}>
                  <div>
                    <div style={{fontWeight: 600, color: '#e2e8f0'}}>{doc.file_name}</div>
                    <div style={{fontSize: '0.8rem', color: '#94a3b8'}}>{formatDateTime(doc.uploaded_at, orgTimezone)}</div>
                  </div>
                  <a href={doc.file_url} target="_blank" rel="noreferrer" style={{color: '#38bdf8', textDecoration: 'none', fontSize: '0.9rem', fontWeight: 600}}>View &rarr;</a>
                </div>
              ))}
            </div>
          )}
        </div>

        <div style={{marginTop: '2rem', textAlign: 'right'}}>
          <button className="btn-call" style={{borderColor: 'transparent', color: '#cbd5e1', background: 'transparent'}} onClick={() => setActiveLeadDocs(null)}>Close Vault</button>
        </div>
      </div>
    </div>
  );
}
