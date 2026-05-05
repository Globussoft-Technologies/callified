import React from 'react';

// AI Receptionist (separate Go service running on :8000) embedded as an
// iframe. The receptionist's browser demo (mic + TTS + REST endpoints
// /start-call, /process-input, /end-call) is self-contained, so loading the
// iframe directly from its own origin keeps its absolute fetch paths working
// without needing a path-rewriting proxy.
//
// Dev: http://localhost:8000 (the go-receptionist binary running on the host).
// Prod: configure RECEPTIONIST_URL via Vite env (VITE_RECEPTIONIST_URL).
const RECEPTIONIST_URL = import.meta.env.VITE_RECEPTIONIST_URL || 'http://localhost:8000';

export default function ReceptionistPage() {
  return (
    <div style={{ padding: 16, height: 'calc(100vh - 80px)' }}>
      <iframe
        src={RECEPTIONIST_URL}
        title="AI Receptionist"
        style={{
          width: '100%',
          height: '100%',
          border: '1px solid #334155',
          borderRadius: 12,
          background: '#0f172a',
        }}
        // Mic access for the receptionist's voice demo.
        allow="microphone; autoplay; clipboard-read; clipboard-write"
      />
    </div>
  );
}
