import React from 'react';
import PageHeader from '../components/PageHeader';

// AI Receptionist (separate Go service running on :8000) embedded as an
// iframe. The receptionist's browser demo (mic + TTS + REST endpoints
// /start-call, /process-input, /end-call) is self-contained, so loading the
// iframe directly from its own origin keeps its absolute fetch paths working
// without needing a path-rewriting proxy.
//
// The receptionist is embedded in audiod, served at /api/receptionist/*.
// Same-origin in every environment — no localhost or sidecar process needed.
// Override only if you ever externalize it again (VITE_RECEPTIONIST_URL).
const RECEPTIONIST_URL = import.meta.env.VITE_RECEPTIONIST_URL || '/api/receptionist/';

export default function ReceptionistPage() {
  return (
    <div style={{
      background: '#f4f5f9', minHeight: '100%',
      display: 'flex', flexDirection: 'column',
    }}>
      <PageHeader
        icon="receptionist"
        title="Receptionist"
        subtitle="Test the AI receptionist by chat or voice."
        style={{ marginBottom: 16 }}
      />
      <div style={{
        flex: 1, background: '#ffffff', border: '1px solid #e5e7eb',
        borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
        overflow: 'hidden', minHeight: 'calc(100vh - 200px)',
      }}>
        <iframe
          src={RECEPTIONIST_URL}
          title="AI Receptionist"
          style={{ width: '100%', height: '100%', border: 'none', minHeight: 'calc(100vh - 200px)' }}
          allow="microphone; autoplay; clipboard-read; clipboard-write"
        />
      </div>
    </div>
  );
}
