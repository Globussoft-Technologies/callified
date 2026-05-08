import React, { useState, useEffect, useRef } from 'react';
import { useAuth } from './contexts/AuthContext';

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

export default function KnowledgeBase({ apiUrl }) {
  const { apiFetch } = useAuth();
  const [files, setFiles] = useState([]);
  const [uploading, setUploading] = useState(false);
  const [statusMsg, setStatusMsg] = useState('');
  const [confirmDeleteId, setConfirmDeleteId] = useState(null);
  const fileInputRef = useRef(null);

  const fetchFiles = async () => {
    try {
      const authToken = localStorage.getItem('authToken');
      const res = await fetch(`${apiUrl}/knowledge`, {
        headers: { 'Authorization': `Bearer ${authToken}` }
      });
      const data = await res.json();
      if (Array.isArray(data)) setFiles(data);
    } catch(e) { console.error(e); }
  };

  useEffect(() => {
    fetchFiles();
    // Poll every 5 seconds since FAISS processes in the background
    const interval = setInterval(fetchFiles, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleUpload = async (e) => {
    e.preventDefault();
    const file = fileInputRef.current?.files[0];
    if (!file) return;
    const formData = new FormData();
    formData.append('file', file);
    setUploading(true);
    setStatusMsg('Vectorizing and Embedding PDF using internal local FAISS Engine...');
    try {
      const authToken = localStorage.getItem('authToken');
      const res = await fetch(`${apiUrl}/knowledge/upload`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${authToken}` },
        body: formData
      });
      const data = await res.json();
      if (data.status === 'success') {
        setStatusMsg('✅ File uploaded! Background worker is currently extracting and mapping chunks.');
        fetchFiles();
        if (fileInputRef.current) fileInputRef.current.value = '';
      } else {
        setStatusMsg(`❌ Error: ${data.message || data.detail}`);
      }
    } catch (e) { setStatusMsg(`❌ Upload failed: ${e.message}`); }
    setUploading(false);
  };

  const handleDelete = async (fileId, filename) => {
    try {
      const authToken = localStorage.getItem('authToken');
      await fetch(`${apiUrl}/knowledge/${fileId}?filename=${encodeURIComponent(filename)}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${authToken}` }
      });
      fetchFiles();
    } catch(e) {}
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title */}
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
          🧠 RAG Knowledge Base
        </h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Upload company PDFs, product sheets, and manuals. The AI will instantly search and read these during live phone calls to eliminate hallucinations.
        </p>
      </div>

      <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', alignItems: 'flex-start' }}>

        {/* Upload card */}
        <div style={{ ...card, padding: '24px 28px', flex: '1 1 300px' }}>
          <h3 style={{ margin: '0 0 20px', fontSize: 15, fontWeight: 700, color: T.text }}>Upload Document</h3>
          <form onSubmit={handleUpload} style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <input
              type="file" accept=".pdf" ref={fileInputRef} required
              style={{
                padding: '9px 13px', borderRadius: 8, fontSize: 13,
                border: `1px solid ${T.border}`, background: T.card,
                color: T.sub, fontFamily: T.font, outline: 'none', cursor: 'pointer',
              }}
            />
            <button type="submit" disabled={uploading} style={{
              padding: '10px 18px', borderRadius: 8, border: 'none',
              fontWeight: 600, fontSize: 13, fontFamily: T.font,
              background: uploading ? T.muted : T.accent,
              color: '#fff', cursor: uploading ? 'not-allowed' : 'pointer',
            }}>
              {uploading ? 'Processing Vector Embeddings...' : '☁️ Upload & Embed PDF'}
            </button>
            {statusMsg && (
              <p style={{ margin: 0, fontSize: 13, color: statusMsg.includes('❌') ? T.red : T.green }}>
                {statusMsg}
              </p>
            )}
          </form>
        </div>

        {/* Active Vector Memory card */}
        <div style={{ ...card, padding: '24px 28px', flex: '2 1 400px' }}>
          <h3 style={{ margin: '0 0 16px', fontSize: 15, fontWeight: 700, color: T.text }}>Active Vector Memory</h3>
          <div style={{ background: T.bg, borderRadius: 8, padding: '16px', minHeight: 200 }}>
            {files.length === 0 ? (
              <p style={{ color: T.muted, textAlign: 'center', marginTop: '3rem', fontSize: 14 }}>
                No documents in the vector database yet.
              </p>
            ) : (
              <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 10 }}>
                {files.map((f, i) => {
                  const handleOpen = async (e) => {
                    e.preventDefault();
                    try {
                      const res = await apiFetch(`${apiUrl}/knowledge/${f.id}/download`);
                      if (!res.ok) { alert(`Download failed (HTTP ${res.status})`); return; }
                      const blob = await res.blob();
                      const objURL = URL.createObjectURL(blob);
                      window.open(objURL, '_blank', 'noopener,noreferrer');
                      setTimeout(() => URL.revokeObjectURL(objURL), 60_000);
                    } catch (err) { alert('Download failed: ' + (err?.message || 'network error')); }
                  };
                  const isActive = f.status === 'Active';
                  return (
                    <li key={i} style={{
                      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                      background: T.card, padding: '12px 14px', borderRadius: 8,
                      border: `1px solid ${T.border}`,
                      borderLeft: `3px solid ${isActive ? T.green : T.amber}`,
                    }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                        <span style={{ fontSize: 18 }}>📄</span>
                        <div>
                          <a href="#" onClick={handleOpen}
                            style={{ color: T.accent, fontWeight: 600, fontSize: 13, textDecoration: 'none', cursor: 'pointer' }}
                            onMouseEnter={e => e.currentTarget.style.textDecoration = 'underline'}
                            onMouseLeave={e => e.currentTarget.style.textDecoration = 'none'}
                            title="Open document">
                            {f.filename}
                          </a>
                          <div style={{ fontSize: 12, color: T.muted, marginTop: 2 }}>
                            {f.status === 'Processing' ? '⚙️ Synthesizing...' : `✅ Active (${f.chunk_count} FAISS Chunks)`}
                          </div>
                        </div>
                      </div>
                      {confirmDeleteId === f.id ? (
                        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                          <span style={{ color: T.amber, fontSize: 12 }}>Delete?</span>
                          <button onClick={() => { setConfirmDeleteId(null); handleDelete(f.id, f.filename); }}
                            style={{
                              background: 'rgba(239,68,68,0.08)', border: `1px solid rgba(239,68,68,0.3)`,
                              color: T.red, borderRadius: 6, padding: '3px 10px',
                              cursor: 'pointer', fontSize: 11, fontWeight: 600, fontFamily: T.font,
                            }}>Confirm</button>
                          <button onClick={() => setConfirmDeleteId(null)}
                            style={{
                              background: T.card, border: `1px solid ${T.border}`,
                              color: T.muted, borderRadius: 6, padding: '3px 10px',
                              cursor: 'pointer', fontSize: 11, fontFamily: T.font,
                            }}>Cancel</button>
                        </div>
                      ) : (
                        <button onClick={() => setConfirmDeleteId(f.id)}
                          style={{ background: 'transparent', border: 'none', color: T.red, cursor: 'pointer', fontSize: 16 }}
                          title="Delete">🗑️</button>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </div>

      </div>
    </div>
  );
}
