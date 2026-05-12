import React, { useState, useEffect, useRef } from 'react';
import { useAuth } from './contexts/AuthContext';
import { useToast, useConfirm } from './contexts/UIContext';

export default function KnowledgeBase({ apiUrl }) {
  const { apiFetch } = useAuth();
  const toast = useToast();
  const confirmDialog = useConfirm();
  const [files, setFiles] = useState([]);
  const [uploading, setUploading] = useState(false);
  // statusMsg is shown inline below the form and kept terse — only the
  // mid-upload progress text. Outcome (success / error) goes through the
  // toast system so it doesn't stick around after the user moves on.
  const [statusMsg, setStatusMsg] = useState('');
  // hasFile gates the submit button: no file picked → button disabled.
  // Browsers don't fire onChange when the input is reset, so we also
  // toggle this back to false after a successful upload.
  const [hasFile, setHasFile] = useState(false);
  const fileInputRef = useRef(null);

  const fetchFiles = async () => {
    try {
      const authToken = localStorage.getItem('authToken');
      const res = await fetch(`${apiUrl}/knowledge`, {
        headers: { 'Authorization': `Bearer ${authToken}` }
      });
      const data = await res.json();
      if (Array.isArray(data)) setFiles(data);
    } catch(e) {
      console.error(e);
    }
  };

  useEffect(() => {
    fetchFiles();
    // Poll every 5 seconds since FAISS processes in the background!
    const interval = setInterval(fetchFiles, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleUpload = async (e) => {
    e.preventDefault();
    const file = fileInputRef.current?.files[0];
    if (!file) return;

    const formData = new FormData();
    formData.append("file", file);

    setUploading(true);
    setStatusMsg('Vectorizing and Embedding PDF using internal local FAISS Engine...');
    try {
      const authToken = localStorage.getItem('authToken');
      const res = await fetch(`${apiUrl}/knowledge/upload`, {
        method: "POST",
        headers: { 'Authorization': `Bearer ${authToken}` },
        body: formData
      });
      const data = await res.json();
      if (data.status === 'success') {
        // Outcome is ephemeral — toast it instead of pinning to the form.
        // The Active Vector Memory list (refreshed below) is the durable
        // record of what's been uploaded.
        toast('File uploaded — background worker is extracting chunks now.', 'success');
        setStatusMsg('');
        fetchFiles();
        if (fileInputRef.current) fileInputRef.current.value = "";
        setHasFile(false);
      } else {
        const msg = data.message || data.detail || 'Upload failed';
        toast(msg, 'error');
        setStatusMsg('');
      }
    } catch (e) {
      toast('Upload failed: ' + e.message, 'error');
      setStatusMsg('');
    }
    setUploading(false);
  };

  const handleDelete = async (fileId, filename) => {
    const ok = await confirmDialog({
      title: 'Delete document',
      message: `Delete "${filename}" from the knowledge base? This removes its FAISS chunks too.`,
      okText: 'Delete',
      danger: true,
    });
    if (!ok) return;
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
    <div className="glass-panel" style={{padding: '2rem'}}>
      <h2 style={{marginTop: 0, marginBottom: '0.5rem', color: '#f8fafc'}}>🧠 RAG Knowledge Base</h2>
      <p style={{color: '#94a3b8', marginBottom: '2rem'}}>Upload company PDFs, product sheets, and manuals. The AI will instantly search and read these during live phone calls to eliminate hallucinations.</p>
      
      <div style={{display: 'flex', gap: '2rem', flexWrap: 'wrap'}}>
        <div style={{flex: '1 1 300px', background: 'rgba(15, 23, 42, 0.6)', border: '1px solid rgba(255,255,255,0.05)', borderRadius: '8px', padding: '1.5rem', height: 'fit-content'}}>
          <h3 style={{marginTop: 0, color: '#e2e8f0'}}>Upload Document</h3>
          <form onSubmit={handleUpload} style={{display: 'flex', flexDirection: 'column', gap: '1rem', marginTop: '1rem'}}>
            <input
              type="file"
              accept=".pdf"
              ref={fileInputRef}
              className="form-input"
              style={{background: 'rgba(0,0,0,0.3)', color: '#94a3b8', padding: '10px'}}
              onChange={(e) => setHasFile(!!e.target.files?.length)}
              required
            />
            <button
              type="submit"
              className="btn-primary"
              disabled={uploading || !hasFile}
              style={{
                opacity: (uploading || !hasFile) ? 0.5 : 1,
                cursor: (uploading || !hasFile) ? 'not-allowed' : 'pointer',
              }}
            >
              {uploading ? 'Processing Vector Embeddings...' : '☁️ Upload & Embed PDF'}
            </button>
            {statusMsg && <p style={{fontSize: '0.9rem', color: '#4ade80'}}>{statusMsg}</p>}
          </form>
        </div>

        <div style={{flex: '2 1 400px'}}>
          <h3 style={{marginTop: 0, color: '#e2e8f0'}}>Active Vector Memory</h3>
          <div style={{background: 'rgba(0,0,0,0.3)', borderRadius: '8px', padding: '1rem', minHeight: '200px'}}>
            {files.length === 0 ? (
              <p style={{color: '#64748b', textAlign: 'center', marginTop: '3rem'}}>No documents in the vector database yet.</p>
            ) : (
              <ul style={{listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: '10px'}}>
                {files.map((f, i) => {
                  // /api/knowledge/{id}/download is auth-gated. We can't put
                  // the JWT in the URL (issue #80), so fetch as blob via the
                  // Authorization header and open the resulting object URL.
                  const handleOpen = async (e) => {
                    e.preventDefault();
                    try {
                      const res = await apiFetch(`${apiUrl}/knowledge/${f.id}/download`);
                      if (!res.ok) { toast(`Download failed (HTTP ${res.status})`, 'error'); return; }
                      const blob = await res.blob();
                      const objURL = URL.createObjectURL(blob);
                      window.open(objURL, '_blank', 'noopener,noreferrer');
                      // Revoke after a beat so the new tab can finish loading.
                      setTimeout(() => URL.revokeObjectURL(objURL), 60_000);
                    } catch (err) { toast('Download failed: ' + (err?.message || 'network error'), 'error'); }
                  };
                  return (
                  <li key={i} style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', background: 'rgba(255,255,255,0.03)', padding: '12px', borderRadius: '6px', borderLeft: f.status === 'Active' ? '3px solid #4ade80' : '3px solid #f59e0b'}}>
                    <div style={{display: 'flex', alignItems: 'center', gap: '10px'}}>
                      <span style={{fontSize: '1.2rem'}}>📄</span>
                      <div>
                        <a href="#" onClick={handleOpen}
                          style={{color: '#93c5fd', fontWeight: 500, textDecoration: 'none', cursor: 'pointer'}}
                          onMouseEnter={e => e.currentTarget.style.textDecoration = 'underline'}
                          onMouseLeave={e => e.currentTarget.style.textDecoration = 'none'}
                          title="Open document">
                          {f.filename}
                        </a>
                        <div style={{fontSize: '0.8rem', color: '#94a3b8'}}>
                          {f.status === 'Processing' ? '⚙️ Synthesizing...' : `✅ Active (${f.chunk_count} FAISS Chunks)`}
                        </div>
                      </div>
                    </div>
                    <button
                      onClick={() => handleDelete(f.id, f.filename)}
                      style={{background: 'transparent', border: 'none', color: '#ef4444', cursor: 'pointer'}}
                      title="Delete"
                    >🗑️</button>
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
