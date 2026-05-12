import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { API_URL } from '../constants/api';

// SsoExchange handles the developer impersonation handoff. The dev-side
// /dev page received an opaque key from /api/dev/impersonate and opened
// /sso/exchange?key=<k>&next=/crm in a new tab. We POST the key, get back
// the impersonation JWT, store it in **sessionStorage** (so the dev's main
// tab is unaffected), then navigate the SPA to ?next=.
//
// This is the POST-body sibling of SsoReturn.jsx (which takes ?token= in the
// URL). We can't reuse SsoReturn because:
//   1. The long-lived JWT must never appear in URL/Referer/history.
//   2. Impersonation tabs must write to sessionStorage, not localStorage.
//
// Public route — must be reachable without an existing session, so it sits
// before App.jsx's authToken gate.
export default function SsoExchange() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { loginWithToken } = useAuth();
  const [error, setError] = useState('');

  useEffect(() => {
    const key = params.get('key');
    const next = params.get('next') || '/crm';
    if (!key) { setError('missing_key'); return; }

    // Mark this tab as session-scoped BEFORE loginWithToken runs so the
    // storage-aware helper in AuthContext writes the impersonated JWT to
    // sessionStorage, not localStorage. Keeps the dev's main tab intact.
    try {
      sessionStorage.setItem('authMode', 'impersonation');
    } catch {
      setError('session_storage_blocked');
      return;
    }

    fetch(`${API_URL}/dev/impersonate/exchange`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key }),
    })
      .then(r => r.ok ? r.json() : Promise.reject(r))
      .then(({ token }) => loginWithToken(token))
      .then(() => navigate(next, { replace: true }))
      .catch((e) => {
        try { sessionStorage.removeItem('authMode'); } catch {}
        setError(typeof e === 'object' && e?.status ? `http_${e.status}` : 'exchange_failed');
      });
  }, []); // run once on mount

  return (
    <div style={{ padding: '3rem', textAlign: 'center', fontFamily: 'system-ui' }}>
      {error ? (
        <>
          <h2 style={{ color: '#ef4444' }}>Impersonation handoff failed</h2>
          <p style={{ color: '#94a3b8' }}>code: {error}</p>
          <p style={{ color: '#64748b', fontSize: '0.85rem', marginTop: '0.5rem' }}>
            The handoff key is single-use and expires in 60 seconds. Try again
            from the developer dashboard.
          </p>
          <p style={{ marginTop: '1.5rem' }}>
            <a href="/dev" style={{ color: '#60a5fa' }}>Back to developer dashboard</a>
          </p>
        </>
      ) : (
        <p style={{ color: '#94a3b8' }}>Signing you in…</p>
      )}
    </div>
  );
}
