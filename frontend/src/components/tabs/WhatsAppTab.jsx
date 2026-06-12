import React, { useState, useEffect, useRef, useCallback } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { formatTime } from '../../utils/dateFormat';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
  wa: '#25D366',
};

const PROVIDERS = [
  { value: 'gupshup', label: 'Gupshup' },
  { value: 'wati', label: 'Wati' },
  { value: 'aisensei', label: 'AiSensei' },
  { value: 'interakt', label: 'Interakt' },
  { value: 'meta', label: 'Meta (Cloud API)' },
  { value: 'wasender', label: 'WaSender' },
];

const PROVIDER_FIELDS = {
  gupshup: [
    { key: 'api_key', label: 'API Key', type: 'password' },
    { key: 'app_id', label: 'App Name', type: 'text' },
    { key: 'phone_number', label: 'Source Phone', type: 'text' },
  ],
  wati: [
    { key: 'bearer_token', label: 'Bearer Token', type: 'password' },
    { key: 'tenant_url', label: 'Tenant URL', type: 'text' },
  ],
  aisensei: [
    { key: 'api_key', label: 'API Key', type: 'password' },
    { key: 'base_url', label: 'Base URL', type: 'text' },
  ],
  interakt: [
    { key: 'api_key', label: 'API Key', type: 'password' },
  ],
  meta: [],
  wasender: [
    { key: 'api_key', label: 'Personal Access Token (from wasenderapi.com → Profile)', type: 'password' },
    { key: 'phone_number', label: 'Source Phone', type: 'text' },
    { key: 'webhook_secret', label: 'Webhook Secret (recommended)', type: 'password', optional: true },
    { key: 'base_url', label: 'Base URL (optional)', type: 'text', optional: true },
  ],
};

const inputBase = {
  width: '100%', boxSizing: 'border-box', padding: '9px 13px', borderRadius: 8,
  border: `1px solid ${T.border}`, background: '#f9fafb',
  color: T.text, fontSize: 13, outline: 'none', fontFamily: T.font,
};

function SecretField({ value, onChange, placeholder }) {
  const [reveal, setReveal] = useState(false);
  return (
    <div style={{ position: 'relative' }}>
      <input
        type={reveal ? 'text' : 'password'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
        data-1p-ignore
        data-lpignore="true"
        style={{ ...inputBase, paddingRight: 52 }}
      />
      <button
        type="button"
        onClick={() => setReveal(!reveal)}
        aria-label={reveal ? 'Hide value' : 'Show value'}
        style={{
          position: 'absolute', right: '6px', top: '50%', transform: 'translateY(-50%)',
          background: 'none', border: 'none', color: '#64748b',
          cursor: 'pointer', fontSize: '0.75rem', padding: '4px 8px',
        }}>
        {reveal ? 'Hide' : 'Show'}
      </button>
    </div>
  );
}

/* ─── Meta Embedded Signup Connect Panel ─── */
function MetaConnectPanel({ apiFetch, API_URL, existingPhone, onConnected }) {
  const [appConfig, setAppConfig] = useState(null);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState('');
  const [connectedPhone, setConnectedPhone] = useState(existingPhone || '');
  const sdkReady = useRef(false);

  const loadFBSDK = (appId, version) => {
    if (document.getElementById('facebook-jssdk')) {
      if (window.FB) {
        window.FB.init({ appId, autoLogAppEvents: true, xfbml: true, version });
        sdkReady.current = true;
      }
      return;
    }
    window.fbAsyncInit = function () {
      window.FB.init({ appId, autoLogAppEvents: true, xfbml: true, version });
      sdkReady.current = true;
    };
    const s = document.createElement('script');
    s.id = 'facebook-jssdk';
    s.src = 'https://connect.facebook.net/en_US/sdk.js';
    document.head.appendChild(s);
  };

  useEffect(() => {
    apiFetch(`${API_URL}/wa/meta/app-config`)
      .then(r => r.json())
      .then(cfg => {
        setAppConfig(cfg);
        loadFBSDK(cfg.app_id, cfg.graph_version || 'v25.0');
      })
      .catch(() => setError('Could not load Meta app config from server'));
  }, [apiFetch, API_URL]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setConnectedPhone(existingPhone || '');
  }, [existingPhone]);

  const handleConnect = () => {
    if (!window.FB || !appConfig) {
      setError('Facebook SDK not ready — please wait a moment and try again');
      return;
    }
    setConnecting(true);
    setError('');

    // Reset after 90s in case the popup is blocked or silently closed —
    // FB.login does not always fire the callback when popups are blocked.
    const bail = setTimeout(() => {
      setConnecting(false);
      setError('Timed out. The Facebook popup may have been blocked — allow popups for this site and try again.');
    }, 90000);

    window.FB.login((response) => {
      clearTimeout(bail);
      if (!response.authResponse?.code) {
        setConnecting(false);
        setError('Connection cancelled or popup blocked. Allow popups for this site and try again.');
        return;
      }
      apiFetch(`${API_URL}/wa/onboard/exchange`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code: response.authResponse.code }),
      })
        .then(res => res.json().then(data => ({ res, data })))
        .then(({ res, data }) => {
          if (!res.ok) {
            setError(data.error || 'Connection failed');
            setConnecting(false);
            return;
          }
          const phone = data.phone_display || data.phone_number_id;
          setConnectedPhone(phone);
          onConnected?.(phone, data.phone_number_id);
          setConnecting(false);
        })
        .catch(() => {
          setError('Network error — could not reach server');
          setConnecting(false);
        });
    }, {
      config_id: appConfig.es_config_id,
      response_type: 'code',
      override_default_response_type: true,
      extras: { setup: {}, featuretype: '', sessionInfoVersion: '3' },
    });
  };

  if (connectedPhone) {
    return (
      <div style={{ background: 'rgba(34,197,94,0.08)', border: '1px solid rgba(34,197,94,0.3)', borderRadius: '8px', padding: '12px 14px', marginBottom: '1rem', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div>
          <div style={{ color: '#15803d', fontSize: '0.72rem', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '3px' }}>Connected</div>
          <div style={{ color: '#1e293b', fontFamily: 'monospace', fontSize: '0.9rem', fontWeight: 600 }}>{connectedPhone}</div>
        </div>
        <button onClick={() => setConnectedPhone('')} style={{ background: 'none', border: '1px solid rgba(34,197,94,0.4)', borderRadius: '6px', color: '#15803d', padding: '5px 12px', cursor: 'pointer', fontSize: '0.78rem', fontWeight: 600 }}>
          Reconnect
        </button>
      </div>
    );
  }

  return (
    <div style={{ marginBottom: '1rem' }}>
      {error && (
        <div style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.25)', borderRadius: '8px', padding: '10px 14px', marginBottom: '0.75rem', color: '#dc2626', fontSize: '0.82rem' }}>
          {error}
        </div>
      )}

      {/* Platform credentials option — skips FB popup when server has env credentials */}
      <div style={{ background: 'rgba(37,211,102,0.06)', border: '1px solid rgba(37,211,102,0.25)', borderRadius: '8px', padding: '12px 14px', marginBottom: '0.75rem' }}>
        <div style={{ color: '#15803d', fontSize: '0.72rem', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: '4px' }}>Platform Credentials</div>
        <div style={{ color: '#334155', fontSize: '0.82rem', marginBottom: '10px' }}>
          Use the WhatsApp Business credentials already configured on the server.
        </div>
        <button
          onClick={() => onConnected?.('platform', 'platform')}
          style={{
            width: '100%', background: '#25D366', color: '#fff',
            border: 'none', borderRadius: '8px', padding: '9px 16px',
            fontSize: '0.85rem', fontWeight: 600, cursor: 'pointer',
          }}
        >
          ✅ Use Platform Credentials
        </button>
      </div>

      <div style={{ textAlign: 'center', color: '#94a3b8', fontSize: '0.72rem', margin: '8px 0' }}>— or connect your own account —</div>

      <button
        onClick={handleConnect}
        disabled={connecting || !appConfig}
        style={{
          width: '100%', background: connecting || !appConfig ? '#94a3b8' : '#1877F2',
          color: '#fff', border: 'none', borderRadius: '8px', padding: '11px 16px',
          fontSize: '0.88rem', fontWeight: 600, cursor: connecting || !appConfig ? 'not-allowed' : 'pointer',
          display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '8px',
        }}
      >
        {connecting ? 'Connecting…' : '🔗 Connect with WhatsApp Business'}
      </button>
      <div style={{ color: '#94a3b8', fontSize: '0.74rem', textAlign: 'center', marginTop: '6px' }}>
        A Facebook popup will open — log in and select your WhatsApp Business Account
      </div>
    </div>
  );
}

/* ─── Config Modal ─── */
function ConfigModal({ show, onClose, apiFetch, API_URL }) {
  const [provider, setProvider] = useState('gupshup');
  const [creds, setCreds] = useState({});
  const [defaultProduct, setDefaultProduct] = useState('');
  const [autoReply, setAutoReply] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!show) return;
     
    setError('');
    apiFetch(`${API_URL}/wa/config`)
      .then(r => r.ok ? r.json() : null)
      .then(data => {
        if (data) {
          setProvider(data.provider || 'gupshup');
          setCreds(data.credentials || {});
          setDefaultProduct(data.default_product_id || '');
          setAutoReply(data.auto_reply !== false);
        }
      })
      .catch(() => {});
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [show]);

  const fields = PROVIDER_FIELDS[provider] || [];
  const missingField = fields.find(f => !f.optional && !(creds[f.key] || '').trim());
  const metaConnected = provider === 'meta' && !!(creds.phone_number || creds.phone_display || creds.platform);
  const canSave = !saving && (provider === 'meta' ? metaConnected : (fields.length > 0 && !missingField));

  const handleSave = async () => {
    setError('');
    if (missingField) { setError(`${missingField.label} is required`); return; }
    setSaving(true);
    try {
      const productID = defaultProduct ? Number(defaultProduct) : null;
      const res = await apiFetch(`${API_URL}/wa/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, credentials: creds, default_product_id: productID, auto_reply: autoReply }),
      });
      if (!res.ok) {
        let msg = `Save failed (HTTP ${res.status})`;
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch { /* ignore */ }
        setError(msg);
        setSaving(false);
        return;
      }
      onClose();
    } catch { setError('Network error — could not reach server');
     }
    setSaving(false);
  };

  if (!show) return null;

  const webhookUrl = `${window.location.origin}/wa/webhook/${provider}`;

  return (
    <div style={overlayStyle} onClick={onClose}>
      <div className="glass-panel" style={modalStyle} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.2rem' }}>
          <h3 style={{ margin: 0, color: '#1e293b' }}>WhatsApp Channel Config</h3>
          <button onClick={onClose} style={closeBtnStyle}>&times;</button>
        </div>

        {error && (
          <div style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.25)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#dc2626', fontSize: '0.85rem' }}>
            {error}
          </div>
        )}

        <label style={labelSt}>Provider</label>
        <select value={provider} onChange={e => { setProvider(e.target.value); setCreds({}); setError(''); }}
          style={{ ...inputBase, cursor: 'pointer', appearance: 'auto', WebkitAppearance: 'menulist' }}>
          {PROVIDERS.map(p => <option key={p.value} value={p.value}>{p.label}</option>)}
        </select>

        {provider === 'meta' ? (
          <MetaConnectPanel
            apiFetch={apiFetch}
            API_URL={API_URL}
            existingPhone={creds.phone_number}
            onConnected={(display, phoneNumberID) => setCreds({ ...creds, phone_display: display, phone_number: phoneNumberID || display })}
          />
        ) : fields.map(f => (
          <div key={f.key}>
            <label style={labelSt}>{f.label}</label>
            {f.type === 'password' ? (
              <SecretField value={creds[f.key] || ''} onChange={(v) => { setCreds({ ...creds, [f.key]: v }); if (error) setError(''); }} placeholder={f.label} />
            ) : (
              <input type={f.type} value={creds[f.key] || ''}
                onChange={e => { setCreds({ ...creds, [f.key]: e.target.value }); if (error) setError(''); }}
                style={inputBase} placeholder={f.label} />
            )}
          </div>
        ))}

        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', margin: '1rem 0' }}>
          <label style={{ ...labelStyle, margin: 0 }}>Auto-Reply</label>
          <button onClick={() => setAutoReply(!autoReply)}
            style={{ border: 'none', borderRadius: 12, cursor: 'pointer', padding: '4px 14px', fontSize: 12, fontWeight: 700, color: '#fff', fontFamily: T.font, background: autoReply ? T.wa : T.muted }}>
            {autoReply ? 'ON' : 'OFF'}
          </button>
        </div>

        <div style={{ background: 'rgba(37,211,102,0.08)', border: '1px solid rgba(37,211,102,0.2)', borderRadius: '8px', padding: '0.75rem', marginBottom: '1rem' }}>
          <label style={{ ...labelStyle, fontSize: '0.7rem', color: '#25D366' }}>Webhook URL — configure in your provider dashboard</label>
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <code style={{ flex: 1, color: '#1e293b', fontSize: '0.78rem', wordBreak: 'break-all' }}>{webhookUrl}</code>
            <button onClick={() => navigator.clipboard.writeText(webhookUrl)}
              style={{ border: '1px solid rgba(37,211,102,0.3)', borderRadius: 6, background: 'rgba(37,211,102,0.1)', color: T.wa, padding: '4px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
              Copy
            </button>
          </div>
        </div>

        <button onClick={handleSave} disabled={!canSave}
          title={missingField ? `${missingField.label} is required` : ''}
          style={{
            ...btnStyle, width: '100%', background: '#25D366', color: '#fff', fontWeight: 700,
            opacity: canSave ? 1 : 0.5, cursor: canSave ? 'pointer' : 'not-allowed'
          }}>
          {saving ? 'Saving...' : 'Save Configuration'}
        </button>
      </div>
    </div>
  );
}

// Lower-case "connected" check. WaSender has flipped between "Connected"
// and "connected" historically; case-insensitive matching avoids the
// dashboard getting stuck on "Connecting…" because of capitalization.
const isWaConnected = (s) => (s || '').toLowerCase() === 'connected';

/* ─── useWASession ───
   Shared session-status hook. Polls /api/wa/session at a steady cadence
   so the inbox header and the SessionPanel agree without double-polling.
   Returns the first session (we only support one WaSender session per
   org today; if that changes, callers will need a selector).

   Pacing: 4s while not connected (user is mid-scan, wants snappy
   feedback); 15s while connected (just keeping the badge fresh, no
   reason to hammer WaSender). */
function useWASession(apiFetch, API_URL) {
  const [session, setSession] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const timerRef = useRef(null);

  const load = useCallback(async () => {
    try {
      const res = await apiFetch(`${API_URL}/wa/session`);
      const data = await res.json().catch(() => ({}));
      if (!res.ok || !data?.success) {
        setError(data?.error || data?.message || `Failed to load session (HTTP ${res.status})`);
        setSession(null);
        return;
      }
      setError('');
      setSession(Array.isArray(data.data) ? (data.data[0] || null) : null);
    } catch (e) {
      setError('Network error: ' + (e.message || String(e)));
    } finally {
      setLoading(false);
    }
  }, [apiFetch, API_URL]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    clearInterval(timerRef.current);
    const interval = isWaConnected(session?.status) ? 15000 : 4000;
    timerRef.current = setInterval(load, interval);
    return () => clearInterval(timerRef.current);
  }, [session?.status, load]);

  return { session, loading, error, reload: load };
}

/* ─── WhatsApp Session Panel (right column) ───
   Shows the user's WaSender session: phone, status, and a scannable QR
   when not connected. PAT lives server-side; we never see it here. The
   panel reuses the shared useWASession hook so polling stays in one
   place. QR refreshing remains local because it's only relevant when
   this panel is open. */
function SessionPanel({ apiFetch, API_URL, onClose, session, loading, error: hookError, reloadSession }) {
  const [qr, setQr] = useState('');             // raw QR string from WaSender
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);      // suppress overlapping clicks
  const [confirmDisconnect, setConfirmDisconnect] = useState(false); // themed confirm
  const qrTimerRef = useRef(null);

  const isConnected = isWaConnected;

  // disconnect forces WaSender to drop the current link. Used to recover
  // from stale "connected" state where the user has unlinked the device
  // from their phone but WaSender hasn't noticed yet (force-quit, network
  // loss, unclean logout). After this fires, status flips to NEED_SCAN
  // and the QR-flow path takes over for re-scanning.
  //
  // The button click only opens the themed confirmation modal; the
  // actual API call lives in confirmDisconnectAction below. This avoids
  // the jarring native browser confirm() dialog and keeps the UI on
  // theme.
  const confirmDisconnectAction = async () => {
    setConfirmDisconnect(false);
    if (!session?.id || busy) return;
    setBusy(true);
    setError('');
    try {
      const res = await apiFetch(`${API_URL}/wa/session/${session.id}/disconnect`, { method: 'POST' });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setError(data?.message || `Disconnect failed (HTTP ${res.status})`);
        return;
      }
      setQr(''); // clear stale QR if any
      reloadSession();
    } catch (e) {
      setError('Network error: ' + (e.message || String(e)));
    } finally {
      setBusy(false);
    }
  };

  // Connect kicks off the WaSender session and returns the first QR in
  // the same response. Used by the "Connect / Generate QR" button.
  const connect = async () => {
    if (!session?.id || busy) return;
    setBusy(true);
    setError('');
    try {
      const res = await apiFetch(`${API_URL}/wa/session/${session.id}/connect`, { method: 'POST' });
      const data = await res.json().catch(() => ({}));
      if (!res.ok || !data?.success) {
        setError(data?.message || `Connect failed (HTTP ${res.status})`);
        return;
      }
      if (data?.data?.qrCode) setQr(data.data.qrCode);
      // Nudge the shared hook to re-poll so the status badge updates
      // from "logged_out" to whatever WaSender now reports (NEED_SCAN,
      // connecting, …). Without this the inbox header would lag a few
      // seconds behind the panel's own state.
      reloadSession();
    } catch (e) {
      setError('Network error: ' + (e.message || String(e)));
    } finally {
      setBusy(false);
    }
  };

  // refreshQR is the polling-friendly variant. WaSender QR codes expire
  // after 45s; we re-fetch every 30s so the displayed code is always
  // scannable. Errors here are silent — a stale QR is better than a
  // dashboard full of red banners.
  const refreshQR = useCallback(async () => {
    if (!session?.id) return;
    try {
      const res = await apiFetch(`${API_URL}/wa/session/${session.id}/qr`);
      const data = await res.json().catch(() => ({}));
      if (res.ok && data?.success && data?.data?.qrCode) {
        setQr(data.data.qrCode);
      }
    } catch { /* ignore — next tick retries */  }
  }, [session?.id, apiFetch, API_URL]);

  // QR-only polling: while we're showing a QR (i.e. session is not yet
  // connected), refresh it every 30s so it never goes stale before the
  // user scans. Stops the moment status flips to connected. Status
  // polling itself is handled by the shared useWASession hook.
  useEffect(() => {
    clearInterval(qrTimerRef.current);
    if (!session || isConnected(session.status)) { setQr(''); return; }
    qrTimerRef.current = setInterval(refreshQR, 30000);
    return () => clearInterval(qrTimerRef.current);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.id, session?.status, refreshQR]);

  // Surface hook-level fetch errors inline on the panel — but treat
  // "personal access token required" as a soft state rather than an
  // error: many WaSender plans only ship a per-session key (good for
  // sending) and don't expose a PAT (needed for the session/QR
  // management endpoints). The dashboard still works without it; we
  // just hide the QR/management UI and tell the user clearly instead
  // of dumping a red banner.
  const needsPAT = (hookError || '').toLowerCase().includes('personal access token');
  useEffect(() => {
    if (needsPAT) { setError(''); return; }
    setError(hookError || '');
  }, [hookError, needsPAT]);

  return (
    <div style={sessionPanelStyle}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '0.75rem 1rem', borderBottom: '1px solid #e2e8f0' }}>
        <h3 style={{ margin: 0, color: '#25D366', fontSize: '0.95rem' }}>📱 WhatsApp Session</h3>
        <button onClick={onClose} style={{ ...btnSmallStyle, background: '#f1f5f9', color: '#64748b', border: '1px solid #e2e8f0' }} title="Close">×</button>
      </div>
      <div style={{ padding: '1rem', flex: 1, overflowY: 'auto' }}>
        {loading && <div style={{ color: '#64748b', fontSize: '0.85rem' }}>Loading…</div>}

        {error && (
          <div style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.25)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#dc2626', fontSize: '0.8rem' }}>
            {error}
          </div>
        )}

        {!loading && needsPAT && (
          // Friendly fallback for accounts that only have a per-session
          // API key (good for sending) but no PAT (required for QR /
          // session-management endpoints). Sending and inbound webhooks
          // still work — only the in-dashboard scan flow is unavailable.
          <div style={{
            background: 'rgba(99,102,241,0.06)', border: '1px solid rgba(99,102,241,0.2)',
            borderRadius: '8px', padding: '12px', color: '#4f46e5', fontSize: '0.82rem', lineHeight: 1.5,
          }}>
            <div style={{ fontWeight: 700, color: '#1e293b', marginBottom: '6px' }}>📱 Session view unavailable</div>
            Your WaSender plan exposes a per-session API key (used for sending) but not a Personal Access Token (needed to fetch QR codes from this dashboard).
            <div style={{ marginTop: '8px' }}>
              You can still scan and manage sessions on <a href="https://wasenderapi.com" target="_blank" rel="noreferrer" style={{ color: '#4f46e5', textDecoration: 'underline' }}>wasenderapi.com</a> — and inbound/outbound messages will work normally here.
            </div>
          </div>
        )}

        {!loading && !session && !error && !needsPAT && (
          <div style={{ color: '#64748b', fontSize: '0.85rem' }}>
            No WhatsApp session found. Open WhatsApp Channel Config (gear icon) and save your WaSender API token first.
          </div>
        )}

        {session && isConnected(session.status) && (
          // Connected: a "Linked Device" confirmation card. The phone is
          // the headline (largest text) since that's what the user wants
          // to confirm — "yes, my number is here". The session name is
          // secondary metadata for users who run multiple WaSender
          // sessions later. Followed by a thin help banner explaining
          // what the connection enables.
          <>
            <div style={{
              background: 'linear-gradient(135deg, rgba(37,211,102,0.18) 0%, rgba(37,211,102,0.06) 100%)',
              border: '1px solid rgba(37,211,102,0.4)', borderRadius: '12px',
              padding: '1rem', textAlign: 'center', marginBottom: '0.75rem',
            }}>
              <div style={{ fontSize: '2rem', lineHeight: 1, marginBottom: '0.5rem' }}>📱</div>
              <div style={{ color: '#64748b', fontSize: '0.65rem', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '4px' }}>Linked Device</div>
              <div style={{ color: '#1e293b', fontSize: '1.15rem', fontWeight: 700, fontFamily: 'monospace', marginBottom: '6px' }}>
                {session.phone_number || '—'}
              </div>
              <div style={{ color: '#64748b', fontSize: '0.78rem', marginBottom: '10px' }}>
                {session.name || `Session #${session.id}`}
              </div>
              <span style={{
                display: 'inline-block', padding: '4px 12px', borderRadius: '12px',
                fontSize: '0.7rem', fontWeight: 700, letterSpacing: '0.05em',
                background: 'rgba(37,211,102,0.25)', color: '#25D366',
              }}>
                ● CONNECTED
              </span>
            </div>
            <div style={{ background: 'rgba(37,211,102,0.08)', border: '1px solid rgba(37,211,102,0.25)', borderRadius: '8px', padding: '0.6rem 0.75rem', color: '#166534', fontSize: '0.78rem', lineHeight: 1.5 }}>
              ✅ Send and receive WhatsApp messages. New incoming chats will appear in the inbox automatically.
            </div>
            {/* Disconnect button — recovers from stale "connected" state
                where the phone unlinked but WaSender hasn't caught up.
                Opens a themed confirmation modal (rendered below) so the
                user can't fat-finger this irreversible action. */}
            <button onClick={() => setConfirmDisconnect(true)} disabled={busy}
              style={{
                width: '100%', marginTop: '0.6rem',
                background: 'rgba(234,179,8,0.08)', color: '#92400e',
                border: '1px solid rgba(234,179,8,0.35)', borderRadius: '8px',
                padding: '8px', fontSize: '0.78rem', fontWeight: 600,
                cursor: busy ? 'not-allowed' : 'pointer', opacity: busy ? 0.5 : 1,
              }}>
              {busy ? 'Disconnecting…' : 'Disconnect & Re-scan'}
            </button>
          </>
        )}

        {session && !isConnected(session.status) && (
          // Not connected: surface the QR + scan instructions. Keep the
          // lighter info rows here too, so the user has a status line
          // with the "need_scan" / "logged_out" badge for context.
          <>
            <div style={{ marginBottom: '0.75rem' }}>
              <div style={{ color: '#64748b', fontSize: '0.7rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Session</div>
              <div style={{ color: '#1e293b', fontSize: '0.9rem', fontWeight: 600 }}>{session.name || `#${session.id}`}</div>
            </div>
            <div style={{ marginBottom: '0.75rem' }}>
              <div style={{ color: '#64748b', fontSize: '0.7rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Phone</div>
              <div style={{ color: '#1e293b', fontSize: '0.85rem', fontFamily: 'monospace' }}>{session.phone_number || '—'}</div>
            </div>
            <div style={{ marginBottom: '1rem' }}>
              <div style={{ color: '#64748b', fontSize: '0.7rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Status</div>
              <div>
                <span style={{
                  display: 'inline-block', padding: '3px 10px', borderRadius: '10px',
                  fontSize: '0.72rem', fontWeight: 700,
                  background: 'rgba(234,179,8,0.1)', color: '#92400e',
                }}>
                  {session.status || 'unknown'}
                </span>
              </div>
            </div>
            {qr ? (
              <div style={{ background: '#fff', padding: '12px', borderRadius: '8px', textAlign: 'center', marginBottom: '0.75rem' }}>
                <QRCodeSVG value={qr} size={220} level="M" />
              </div>
            ) : (
              <div style={{ background: '#f1f5f9', border: '1px dashed #cbd5e1', borderRadius: '8px', padding: '2rem', textAlign: 'center', color: '#94a3b8', fontSize: '0.8rem', marginBottom: '0.75rem' }}>
                Click "Generate QR" to start the scan.
              </div>
            )}
            <button onClick={connect} disabled={busy}
              style={{
                width: '100%', background: '#25D366', color: '#fff',
                border: 'none', borderRadius: '8px', padding: '10px',
                fontWeight: 700, fontSize: '0.85rem',
                cursor: busy ? 'not-allowed' : 'pointer', opacity: busy ? 0.6 : 1,
                marginBottom: '0.5rem',
              }}>
              {busy ? 'Working…' : (qr ? 'Refresh QR' : 'Generate QR')}
            </button>
            <div style={{ color: '#64748b', fontSize: '0.72rem', textAlign: 'center' }}>
              Open WhatsApp on your phone → Settings → Linked Devices → Link a Device → scan this code.
            </div>
          </>
        )}
      </div>

      {/* Themed confirm: replaces window.confirm() so the prompt sits in
          the same dark-glass aesthetic as the rest of the dashboard.
          Click on the overlay or Cancel dismisses without action; OK
          fires the disconnect API call. Uses an opaque solid panel
          (not the page-glass class) plus a heavier overlay so the
          background content fully fades out — otherwise the
          translucent "Select a conversation…" hint bleeds through and
          looks broken. */}
      {confirmDisconnect && (
        <div
          style={{ ...overlayStyle, background: 'rgba(0,0,0,0.78)', backdropFilter: 'blur(4px)', WebkitBackdropFilter: 'blur(4px)' }}
          onClick={() => setConfirmDisconnect(false)}
        >
          <div
            style={{
              ...modalStyle, maxWidth: '380px',
              background: '#ffffff',
              border: '1px solid #e2e8f0',
              boxShadow: '0 20px 60px rgba(0,0,0,0.12)',
            }}
            onClick={e => e.stopPropagation()}
          >
            <h3 style={{ margin: '0 0 0.75rem 0', color: '#1e293b' }}>Disconnect WhatsApp?</h3>
            <p style={{ color: '#64748b', fontSize: '0.85rem', lineHeight: 1.5, marginBottom: '1.2rem' }}>
              This will drop the link to <span style={{ color: '#1e293b', fontFamily: 'monospace' }}>{session?.phone_number || 'this device'}</span>. You'll need to scan a fresh QR code to reconnect.
            </p>
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
              <button onClick={() => setConfirmDisconnect(false)}
                style={{
                  background: '#f1f5f9', color: '#64748b',
                  border: '1px solid #e2e8f0', borderRadius: '8px',
                  padding: '8px 16px', fontSize: '0.85rem', fontWeight: 600,
                  cursor: 'pointer',
                }}>
                Cancel
              </button>
              <button onClick={confirmDisconnectAction}
                style={{
                  background: 'rgba(234,179,8,0.1)', color: '#92400e',
                  border: '1px solid rgba(234,179,8,0.35)', borderRadius: '8px',
                  padding: '8px 16px', fontSize: '0.85rem', fontWeight: 700,
                  cursor: 'pointer',
                }}>
                Disconnect
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/* ─── New Chat Modal ─── */
// Lets the operator start a conversation with a number that has never
// messaged us before. Phone normalisation matches the rest of the app:
// strip everything but digits, then require country code (no implicit
// India default — too easy to dial the wrong country silently). On
// submit we send the first message via /api/wa/send, which also creates
// the conversation row server-side, then jump the parent into that
// conversation so the operator can keep typing.
function NewChatModal({ show, onClose, apiFetch, API_URL, onStarted }) {
  const [phone, setPhone] = useState('');
  const [name, setName] = useState('');
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // Reset every time the modal opens so previous (failed) input doesn't
  // leak back in if the user cancelled and reopened.
  useEffect(() => {
    if (!show) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPhone(''); setName(''); setText(''); setError(''); setBusy(false);
  }, [show]);

  if (!show) return null;

  // Normalize: keep digits + leading +. WaSender's API requires E.164
  // with the +, so we add it if missing — but ONLY when the input has at
  // least 11 digits (country code + national number). Shorter inputs
  // probably mean the user forgot a digit, so we surface that as an
  // error instead of silently mangling the number.
  const normalizePhone = (raw) => {
    const digits = (raw || '').replace(/[^\d]/g, '');
    if (digits.length < 10) return null;
    return digits.startsWith('+') ? raw.trim() : '+' + digits;
  };

  const submit = async () => {
    setError('');
    const normalized = normalizePhone(phone);
    if (!normalized) { setError('Enter a valid phone with country code (e.g. +919876543210)'); return; }
    if (!text.trim()) { setError('First message is required'); return; }
    setBusy(true);
    try {
      const res = await apiFetch(`${API_URL}/wa/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ contact_phone: normalized, text: text.trim() }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok || data?.sent !== true) {
        setError(data?.error || data?.detail || `Send failed (HTTP ${res.status})`);
        setBusy(false);
        return;
      }
      // Use the phone shape the server stored (no leading +), so the
      // parent's selectedPhone matches what the conversations list will
      // emit on its next poll.
      const stored = data.phone || normalized.replace(/^\+/, '');
      onStarted(stored, name.trim());
      onClose();
    } catch { setError('Network error — could not reach server');
      setBusy(false);
     }
  };

  return (
    <div style={overlayStyle} onClick={onClose}>
      <div className="glass-panel" style={{ ...modalStyle, maxWidth: '440px' }} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h3 style={{ margin: 0, color: '#1e293b' }}>Start New Chat</h3>
          <button onClick={onClose} style={closeBtnStyle}>&times;</button>
        </div>

        {error && (
          <div style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.25)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#dc2626', fontSize: '0.85rem' }}>
            {error}
          </div>
        )}

        <label style={labelStyle}>Phone (with country code)</label>
        <input type="tel" value={phone} onChange={e => setPhone(e.target.value)}
          placeholder="+919876543210" style={inputStyle} autoFocus />

        <label style={labelStyle}>Name (optional)</label>
        <input type="text" value={name} onChange={e => setName(e.target.value)}
          placeholder="Acme Corp" style={inputStyle} />

        <label style={labelStyle}>First message</label>
        <textarea value={text} onChange={e => setText(e.target.value)}
          placeholder="Hi! Thanks for your interest in EmpMonitor..."
          rows={3}
          style={{ ...inputStyle, resize: 'vertical', minHeight: '70px', fontFamily: 'inherit' }} />

        <button onClick={submit} disabled={busy}
          style={{
            width: '100%', background: '#25D366', color: '#fff',
            border: 'none', borderRadius: '8px', padding: '10px',
            fontWeight: 700, fontSize: '0.9rem',
            cursor: busy ? 'not-allowed' : 'pointer', opacity: busy ? 0.6 : 1,
            marginTop: '0.5rem',
          }}>
          {busy ? 'Sending…' : 'Start Chat'}
        </button>

        <div style={{ marginTop: '0.75rem', color: '#64748b', fontSize: '0.7rem', lineHeight: 1.5 }}>
          Note: WhatsApp's policy discourages messaging users who haven't opted in. Use this for warm leads or replying to known contacts.
        </div>
      </div>
    </div>
  );
}

/* ─── Main Component ─── */
export default function WhatsAppTab({ apiFetch, API_URL, orgTimezone }) {
  const [conversations, setConversations] = useState([]);
  const [selectedPhone, setSelectedPhone] = useState(null);
  const [messages, setMessages] = useState([]);
  const [search, setSearch] = useState('');
  const [messageText, setMessageText] = useState('');
  const [sending, setSending] = useState(false);
  const [showConfig, setShowConfig] = useState(false);
  const [showSession, setShowSession] = useState(false);
  const [showNewChat, setShowNewChat] = useState(false);
  const [aiEnabled, setAiEnabled] = useState({});
  // Conversation management state. openMenu holds the phone whose
  // kebab dropdown is currently open (only one at a time). showArchived
  // flips the inbox listing to include is_archived rows so the operator
  // can review or unarchive them. confirmAction is the in-app modal
  // payload for destructive ops (clear / delete) — null while idle.
  const [openMenu, setOpenMenu] = useState(null);
  const [showArchived, setShowArchived] = useState(false);
  const [confirmAction, setConfirmAction] = useState(null);
  const messagesEndRef = useRef(null);
  const pollRef = useRef(null);

  // Shared session state. Drives the SessionPanel content and the
  // post-scan auto-actions below.
  const sessionState = useWASession(apiFetch, API_URL);
  const sessionConnected = isWaConnected(sessionState.session?.status);
  const sessionPhone = sessionState.session?.phone_number || '';
  // ensuredPhonesRef persists in sessionStorage so page reloads don't
  // re-trigger the ensure-conversation flow and force the session panel
  // open every time the operator refreshes while already connected.
  const ensuredPhonesRef = useRef(
    new Set(JSON.parse(sessionStorage.getItem('wa_ensured_phones') || '[]'))
  );
  // prevConnected tracks the previous connection state so we only
  // auto-open the session panel on a genuine disconnect→connect transition
  // (i.e. after a QR scan), not on every page load when already connected.
  const prevConnectedRef = useRef(null);

  // Post-scan effect: create a conversation row for the linked device
  // phone so it appears in the inbox. Auto-opens the session panel only
  // on a fresh connection, not on every page load.
  useEffect(() => {
    if (!sessionConnected || !sessionPhone) {
      prevConnectedRef.current = false;
      return;
    }
    const key = sessionPhone.replace(/^\+/, '');
    const alreadyEnsured = ensuredPhonesRef.current.has(key);
    const wasAlreadyConnected = prevConnectedRef.current === true;
    prevConnectedRef.current = true;

    if (alreadyEnsured) return;
    ensuredPhonesRef.current.add(key);
    try {
      sessionStorage.setItem('wa_ensured_phones',
        JSON.stringify([...ensuredPhonesRef.current]));
    } catch { /* ignore */ }

    // Only auto-open session panel on a fresh scan (transition),
    // not when the page loads and the session is already connected.
    if (!wasAlreadyConnected) setShowSession(true);

    apiFetch(`${API_URL}/wa/conversations/ensure`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ phone: sessionPhone, provider: 'wasender' }),
    }).then(() => {
      fetchConversations?.();
    }).catch(() => {
      ensuredPhonesRef.current.delete(key);
      try {
        sessionStorage.setItem('wa_ensured_phones',
          JSON.stringify([...ensuredPhonesRef.current]));
      } catch { /* ignore */ }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionConnected, sessionPhone, apiFetch, API_URL]);

  /* ── Fetch conversations ──
     `archived=1` flips the backend filter so archived rows surface for
     the "Show archived" view. Both views poll on the same 5s cadence —
     the toggle just changes which set is currently rendered. */
  const fetchConversations = useCallback(async () => {
    try {
      const url = `${API_URL}/wa/conversations${showArchived ? '?archived=1' : ''}`;
      const res = await apiFetch(url);
      if (res.ok) {
        const data = await res.json();
        const convos = Array.isArray(data) ? data : (data.conversations || []);
        setConversations(convos);
        // AI is "on" when conversation is NOT muted — is_muted is the actual
        // gate checked by the webhook handler.
        const map = {};
        convos.forEach(c => { map[c.phone || c.contact_phone] = !c.is_muted; });
        setAiEnabled(map);
      }
    } catch (e) { console.error('Failed to fetch conversations', e); }
  }, [apiFetch, API_URL, showArchived]);

  useEffect(() => { fetchConversations(); }, [fetchConversations]);

  const fetchMessages = useCallback(async () => {
    if (!selectedPhone) return;
    try {
      const res = await apiFetch(`${API_URL}/wa/conversations/${encodeURIComponent(selectedPhone)}/messages`);
      if (res.ok) {
        const data = await res.json();
        setMessages(Array.isArray(data) ? data : (data.messages || []));
      }
    } catch (e) { console.error('Failed to fetch messages', e); }
  }, [apiFetch, API_URL, selectedPhone]);

  useEffect(() => { fetchMessages(); }, [fetchMessages]);

  useEffect(() => {
    if (pollRef.current) clearInterval(pollRef.current);
    pollRef.current = setInterval(() => {
      fetchConversations();
      if (selectedPhone) fetchMessages();
    }, 5000);
    return () => clearInterval(pollRef.current);
  }, [fetchConversations, fetchMessages, selectedPhone]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  /* ── Close kebab menu when the user clicks anywhere else ──
     Without this the menu would stay open as the user navigates around
     the inbox, which is confusing and lets accidental presses on hidden
     items fire. We listen at the document level and close on any click
     not handled by the menu's own stopPropagation. */
  useEffect(() => {
    if (!openMenu) return;
    const close = () => setOpenMenu(null);
    document.addEventListener('click', close);
    return () => document.removeEventListener('click', close);
  }, [openMenu]);

  /* ── Send message ── */
  const handleSend = async () => {
    if (!messageText.trim() || !selectedPhone || sending) return;
    setSending(true);
    try {
      await apiFetch(`${API_URL}/wa/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ contact_phone: selectedPhone, text: messageText }),
      });
      setMessageText('');
      fetchMessages();
      fetchConversations();
    } catch (e) { console.error(e); }
    setSending(false);
  };

  /* ── Conversation management actions ──
     All four scope by phone (URL-encoded so a leading + survives the
     hop). After each action, refresh the inbox list so the UI catches
     up to the new state without waiting for the 5s poll. Clicking the
     active conversation's row out (e.g. after delete) prevents stale
     selection lingering in the right pane. */
  const muteConv = async (phone, muted) => {
    setOpenMenu(null);
    try {
      await apiFetch(`${API_URL}/wa/conversations/${encodeURIComponent(phone)}/mute`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ muted }),
      });
      // Keep aiEnabled state in sync so the chat header toggle reflects immediately
      setAiEnabled(prev => ({ ...prev, [phone]: !muted }));
      fetchConversations();
    } catch (e) { console.error(e); }
  };
  const archiveConv = async (phone, archived) => {
    setOpenMenu(null);
    try {
      await apiFetch(`${API_URL}/wa/conversations/${encodeURIComponent(phone)}/archive`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ archived }),
      });
      // Archive flips visibility — drop selection if the archived row
      // is currently open, so the right pane doesn't show a thread the
      // user can no longer see in the list.
      if (archived && selectedPhone === phone) setSelectedPhone(null);
      fetchConversations();
    } catch (e) { console.error(e); }
  };
  const clearConv = async (phone) => {
    try {
      await apiFetch(`${API_URL}/wa/conversations/${encodeURIComponent(phone)}/clear`, { method: 'POST' });
      if (selectedPhone === phone) fetchMessages();
      fetchConversations();
    } catch (e) { console.error(e); }
  };
  const deleteConv = async (phone) => {
    try {
      await apiFetch(`${API_URL}/wa/conversations/${encodeURIComponent(phone)}`, { method: 'DELETE' });
      if (selectedPhone === phone) setSelectedPhone(null);
      fetchConversations();
    } catch (e) { console.error(e); }
  };

  /* ── Toggle AI (per-conversation) ──
     Uses the mute endpoint — the webhook handler checks is_muted to gate
     AI replies, so muted=true means AI off and muted=false means AI on.
     The old toggle-ai endpoint updated a different column that was never
     checked, so we route through mute here for correctness. */
  const toggleAi = async () => {
    if (!selectedPhone) return;
    const aiOn = aiEnabled[selectedPhone] !== false;
    const newMuted = aiOn; // turning AI off = muting
    try {
      await apiFetch(`${API_URL}/wa/conversations/${encodeURIComponent(selectedPhone)}/mute`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ muted: newMuted }),
      });
      setAiEnabled(prev => ({ ...prev, [selectedPhone]: !aiOn }));
      setConversations(prev => prev.map(c =>
        c.phone === selectedPhone ? { ...c, is_muted: newMuted } : c
      ));
    } catch (e) { console.error(e); }
  };

  /* ── Filter conversations ──
     Three stages: (1) drop rows whose phone is missing/empty — these
     are usually orphaned from a malformed webhook event (presence
     update, status ack, etc.) and aren't real chats; backend skips
     them now but we filter here too so existing rows in the DB don't
     keep showing in the inbox. (2) hide archived rows when the
     showArchived toggle is off; (3) text search by name or phone. */
  const filtered = conversations.filter(c => {
    const phone = (c.phone || '').trim();
    if (!phone) return false;
    if (!showArchived && c.is_archived) return false;
    if (!search) return true;
    const q = search.toLowerCase();
    return (c.name || '').toLowerCase().includes(q) || phone.includes(q);
  });

  const selectedConv = conversations.find(c => c.phone === selectedPhone);
  const aiActive = selectedPhone ? aiEnabled[selectedPhone] !== false : false;

  return (
    <div style={{ display: 'flex', height: 'calc(100vh - 56px)', fontFamily: T.font }}>

      {/* LEFT: Conversation list */}
      <div style={{
        width: 300, flexShrink: 0, display: 'flex', flexDirection: 'column',
        background: T.card, borderRight: `1px solid ${T.border}`,
      }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '0.75rem 1rem', borderBottom: '1px solid #e2e8f0' }}>
          <h3 style={{ margin: 0, color: '#25D366', fontSize: '1rem' }}>
            <span style={{ marginRight: '6px' }}>💬</span>WhatsApp Inbox
          </h3>
          <div style={{ display: 'flex', gap: '6px' }}>
            <button onClick={() => setShowNewChat(true)}
              style={{ ...btnSmallStyle, background: 'rgba(37,211,102,0.15)', color: '#16a34a', border: '1px solid rgba(37,211,102,0.3)', fontWeight: 700 }}
              title="Start a new chat">
              + New
            </button>
            <button onClick={() => setShowSession(s => !s)}
              style={{ ...btnSmallStyle, background: showSession ? 'rgba(37,211,102,0.15)' : '#f1f5f9', color: showSession ? '#16a34a' : '#64748b', border: '1px solid #e2e8f0' }}
              title="WhatsApp Session / QR">
              📱
            </button>
            <button onClick={() => setShowConfig(true)}
              style={{ ...btnSmallStyle, background: '#f1f5f9', color: '#64748b', border: '1px solid #e2e8f0' }}
              title="Channel Configuration">
              ⚙️
            </button>
          </div>
        </div>

        {/* Search + Show archived toggle.
            The toggle sits on the same row as the search so it doesn't
            steal vertical space; clicking it flips the list between
            "active" and "archived" — same poll, different filter. */}
        <div style={{ padding: '0.5rem 0.75rem', display: 'flex', gap: '6px', alignItems: 'center' }}>
          <input type="text" placeholder="Search by name or phone..." value={search} onChange={e => setSearch(e.target.value)}
            style={{ ...inputStyle, margin: 0, fontSize: '0.8rem', padding: '6px 10px', flex: 1 }} />
          <button onClick={() => setShowArchived(v => !v)}
            title={showArchived ? 'Showing archived — click to show active' : 'Show archived conversations'}
            style={{
              ...btnSmallStyle, flexShrink: 0,
              background: showArchived ? 'rgba(99,102,241,0.1)' : '#f1f5f9',
              color: showArchived ? '#4f46e5' : '#64748b',
              border: '1px solid #e2e8f0',
            }}>
            {showArchived ? '📂' : '📁'}
          </button>
        </div>

        {/* Conversations */}
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {filtered.length === 0 ? (
            <div style={{ padding: '2rem 1rem', textAlign: 'center', color: T.muted, fontSize: 13 }}>
              No WhatsApp conversations yet
            </div>
          ) : filtered.map(conv => {
            // Mark linked-device rows so the operator can tell their own
            // scanned phones apart from inbound customer chats. A conv
            // is "linked" when it matches the currently-active session
            // phone, OR when our local memo says we ensured a row for
            // that phone in this browser session (covers previously-
            // linked devices that have since been replaced by a new
            // scan, so their entry stays distinguishable from customers).
            const noPlus = (conv.phone || '').replace(/^\+/, '');
            const linkedNow = sessionConnected && sessionPhone && noPlus === sessionPhone.replace(/^\+/, '');
            const isLinkedDevice = linkedNow;
            return (
            <div key={conv.phone} onClick={() => setSelectedPhone(conv.phone)}
              style={{
                padding: '0.7rem 1rem', cursor: 'pointer', borderBottom: '1px solid #f1f5f9',
                background: selectedPhone === conv.phone ? 'rgba(37,211,102,0.08)' : 'transparent',
                transition: 'background 0.15s',
              }}
              onMouseEnter={e => { if (selectedPhone !== conv.phone) e.currentTarget.style.background = '#f8fafc'; }}
              onMouseLeave={e => { if (selectedPhone !== conv.phone) e.currentTarget.style.background = 'transparent'; }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flex: 1, minWidth: 0 }}>
                  {isLinkedDevice && <span title="Currently linked WhatsApp device" style={{ fontSize: '0.85rem', flexShrink: 0 }}>📱</span>}
                  {!isLinkedDevice && conv.ai_active && <span style={greenDotStyle} title="AI Auto-Reply active" />}
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ color: '#1e293b', fontWeight: 600, fontSize: '0.85rem', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {conv.name || conv.phone}
                      {linkedNow && <span style={{ marginLeft: '6px', fontSize: '0.62rem', color: '#25D366', fontWeight: 700, letterSpacing: '0.05em' }}>● LIVE</span>}
                    </div>
                    <div style={{ fontFamily: T.mono, color: T.muted, fontSize: 11 }}>{conv.phone}</div>
                  </div>
                </div>
                <div style={{ textAlign: 'right', flexShrink: 0, marginLeft: '8px', position: 'relative' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '4px', justifyContent: 'flex-end' }}>
                    {conv.is_muted && <span title="Muted — AI auto-reply skipped" style={{ fontSize: '0.7rem' }}>🔇</span>}
                    {conv.is_archived && <span title="Archived" style={{ fontSize: '0.7rem' }}>📂</span>}
                    {/* Timestamp uses updated_at (the API field that
                        actually exists on whatsapp_conversations); the
                        old code referenced last_message_at which is
                        always undefined, so formatTime returned "—".
                        Skip the render entirely when no timestamp is
                        available — a row with no activity yet shouldn't
                        flaunt a placeholder dash. */}
                    {(conv.updated_at || conv.last_message_at) && (
                      <div style={{ color: '#64748b', fontSize: '0.68rem' }}>
                        {formatTime(conv.updated_at || conv.last_message_at, orgTimezone)}
                      </div>
                    )}
                  </div>
                  {conv.unread_count > 0 && (
                    <span style={{ display: 'inline-block', background: T.wa, color: '#fff', borderRadius: 10, padding: '1px 7px', fontSize: 11, fontWeight: 700, marginTop: 2 }}>
                      {conv.unread_count}
                    </span>
                  )}
                  {/* Kebab opens the per-conversation action menu.
                      stopPropagation prevents the row click from also
                      firing (which would jump into the chat). The menu
                      itself is absolutely-positioned so it overlays
                      neighbouring rows without disturbing layout. */}
                  <button onClick={(e) => { e.stopPropagation(); setOpenMenu(openMenu === conv.phone ? null : conv.phone); }}
                    title="More actions"
                    style={{
                      marginTop: '4px', background: 'transparent', border: 'none',
                      color: '#64748b', cursor: 'pointer', fontSize: '1.1rem',
                      padding: '0 4px', lineHeight: 1,
                    }}>
                    ⋮
                  </button>
                  {openMenu === conv.phone && (
                    <div onClick={(e) => e.stopPropagation()}
                      style={{
                        position: 'absolute', top: '100%', right: 0, marginTop: '4px',
                        background: '#ffffff', border: '1px solid #e2e8f0',
                        borderRadius: '8px', boxShadow: '0 8px 24px rgba(0,0,0,0.1)',
                        minWidth: '160px', zIndex: 50,
                        padding: '4px',
                      }}>
                      <button onClick={() => muteConv(conv.phone, !conv.is_muted)} style={menuItemStyle}>
                        {conv.is_muted ? '🔊 Unmute' : '🔇 Mute'}
                      </button>
                      <button onClick={() => archiveConv(conv.phone, !conv.is_archived)} style={menuItemStyle}>
                        {conv.is_archived ? '📥 Unarchive' : '📂 Archive'}
                      </button>
                      <button onClick={() => { setOpenMenu(null); setConfirmAction({ type: 'clear', phone: conv.phone, name: conv.name || conv.phone }); }} style={menuItemStyle}>
                        🧹 Clear chat
                      </button>
                      <div style={{ height: '1px', background: '#e2e8f0', margin: '4px 0' }} />
                      <button onClick={() => { setOpenMenu(null); setConfirmAction({ type: 'delete', phone: conv.phone, name: conv.name || conv.phone }); }}
                        style={{ ...menuItemStyle, color: '#dc2626' }}>
                        🗑 Delete
                      </button>
                    </div>
                  )}
                </div>
              </div>
              {conv.last_message && (
                <div style={{ color: '#64748b', fontSize: '0.78rem', marginTop: '4px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {conv.last_message.length > 40 ? conv.last_message.substring(0, 40) + '...' : conv.last_message}
                </div>
              )}
            </div>
            );
          })}
        </div>
      </div>

      {/* RIGHT: Chat window */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', background: T.bg }}>
        {!selectedPhone ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: T.muted, fontSize: 14 }}>
            Select a conversation to start chatting
          </div>
        ) : (
          <>
            {/* Chat header */}
            <div style={{ display: 'flex', alignItems: 'center', padding: '12px 20px', background: T.card, borderBottom: `1px solid ${T.border}` }}>
              <div style={{ flex: 1 }}>
                <div style={{ color: '#1e293b', fontWeight: 700, fontSize: '0.95rem' }}>
                  {selectedConv?.name || selectedPhone}
                </div>
                <div style={{ fontFamily: 'monospace', color: '#64748b', fontSize: '0.78rem' }}>{selectedPhone}</div>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span style={{ color: '#64748b', fontSize: '0.78rem' }}>AI Auto-Reply</span>
                <button onClick={toggleAi}
                  style={{
                    border: 'none', borderRadius: 12, cursor: 'pointer',
                    padding: '4px 14px', fontSize: 12, fontWeight: 700, color: '#fff', fontFamily: T.font,
                    background: aiActive ? T.wa : T.red, minWidth: 48,
                  }}>
                  {aiActive ? 'ON' : 'OFF'}
                </button>
              </div>
            </div>

            {/* Messages — reversed so oldest appears at top, newest at bottom */}
            <div style={messagesAreaStyle}>
              {messages.length === 0 ? (
                <div style={{ textAlign: 'center', color: '#64748b', marginTop: '3rem', fontSize: '0.85rem' }}>No messages yet</div>
              ) : [...messages].reverse().map((msg, i) => {
                const isOutbound = msg.direction === 'outbound';
                return (
                  <div key={msg.id || i} style={{ display: 'flex', justifyContent: isOutbound ? 'flex-end' : 'flex-start' }}>
                    <div style={{
                      maxWidth: '70%', padding: msg.message_type === 'image' ? '4px' : '8px 12px', borderRadius: '12px',
                      background: isOutbound ? '#25D366' : '#e8edf2',
                      color: isOutbound ? '#fff' : '#1e293b',
                      fontSize: '0.85rem', lineHeight: '1.45',
                      borderTopRightRadius: isOutbound ? '4px' : '12px',
                      borderTopLeftRadius: isOutbound ? '12px' : '4px',
                      overflow: 'hidden',
                    }}>
                      {msg.message_type === 'image' ? (
                        <div>
                          <img
                            src={msg.message_text || msg.text}
                            alt="Sent image"
                            style={{ display: 'block', maxWidth: '260px', maxHeight: '260px', width: '100%', borderRadius: '8px', objectFit: 'cover', cursor: 'pointer' }}
                            onClick={() => window.open(msg.message_text || msg.text, '_blank')}
                            onError={e => { e.target.style.display = 'none'; e.target.nextSibling.style.display = 'block'; }}
                          />
                          <span style={{ display: 'none', padding: '8px 12px', fontSize: '0.78rem', color: isOutbound ? 'rgba(255,255,255,0.8)' : '#64748b' }}>
                            📷 Image
                          </span>
                          <div style={{ fontSize: '0.65rem', color: isOutbound ? 'rgba(255,255,255,0.7)' : '#64748b', padding: '2px 8px 4px', textAlign: 'right' }}>
                            {formatTime(msg.created_at || msg.timestamp, orgTimezone)}
                          </div>
                        </div>
                      ) : (
                        <>
                          {msg.ai_generated && <span title="AI-generated" style={{ marginRight: '4px' }}>🤖</span>}
                          <span>{msg.message_text || msg.text || msg.body || msg.content}</span>
                          <div style={{ fontSize: '0.65rem', color: isOutbound ? 'rgba(255,255,255,0.7)' : '#64748b', marginTop: '4px', textAlign: 'right' }}>
                            {formatTime(msg.created_at || msg.timestamp, orgTimezone)}
                          </div>
                        </>
                      )}
                    </div>
                  </div>
                );
              })}
              <div ref={messagesEndRef} />
            </div>

            {/* Input bar */}
            <div style={{ display: 'flex', gap: 8, padding: '12px 16px', background: T.card, borderTop: `1px solid ${T.border}` }}>
              <input type="text" value={messageText}
                onChange={e => setMessageText(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend(); } }}
                placeholder="Type a message..."
                style={{ ...inputBase, flex: 1, fontSize: 13, padding: '10px 14px' }} />
              <button onClick={handleSend} disabled={sending || !messageText.trim()}
                style={{
                  border: 'none', borderRadius: 10, padding: '10px 22px',
                  background: T.wa, color: '#fff', fontWeight: 700, fontSize: 13, fontFamily: T.font,
                  opacity: (sending || !messageText.trim()) ? 0.5 : 1,
                  cursor: (sending || !messageText.trim()) ? 'not-allowed' : 'pointer',
                }}>
                {sending ? '...' : 'Send'}
              </button>
            </div>
          </>
        )}
      </div>

      {/* ── SESSION PANEL: QR + status (toggle via 📱 button) ── */}
      {showSession && (
        <SessionPanel
          apiFetch={apiFetch} API_URL={API_URL}
          onClose={() => setShowSession(false)}
          session={sessionState.session} loading={sessionState.loading}
          error={sessionState.error} reloadSession={sessionState.reload} />
      )}

      {/* Config Modal */}
      <ConfigModal show={showConfig} onClose={() => setShowConfig(false)}
        apiFetch={apiFetch} API_URL={API_URL} />

      {/* New Chat Modal — onStarted jumps the inbox to the new
          conversation so the operator can keep typing without finding it
          in the list. fetchConversations refreshes the list so the new
          row shows up on the next poll tick (or immediately). */}
      <NewChatModal show={showNewChat} onClose={() => setShowNewChat(false)}
        apiFetch={apiFetch} API_URL={API_URL}
        onStarted={(phone /*, name */) => {
          setSelectedPhone(phone);
          fetchConversations();
        }} />

      {/* Themed confirm for destructive conversation actions (clear/
          delete). Shared between both because the dialog shape is
          identical — only the title/body/action differs. Solid bg with
          backdrop blur so the inbox underneath fades out cleanly. */}
      {confirmAction && (
        <div
          style={{ ...overlayStyle, background: 'rgba(0,0,0,0.78)', backdropFilter: 'blur(4px)', WebkitBackdropFilter: 'blur(4px)' }}
          onClick={() => setConfirmAction(null)}
        >
          <div
            style={{
              ...modalStyle, maxWidth: '400px',
              background: '#ffffff',
              border: '1px solid #e2e8f0',
              boxShadow: '0 20px 60px rgba(0,0,0,0.12)',
            }}
            onClick={e => e.stopPropagation()}
          >
            <h3 style={{ margin: '0 0 0.75rem 0', color: '#1e293b' }}>
              {confirmAction.type === 'clear' ? 'Clear chat history?' : 'Delete conversation?'}
            </h3>
            <p style={{ color: '#64748b', fontSize: '0.85rem', lineHeight: 1.5, marginBottom: '1.2rem' }}>
              {confirmAction.type === 'clear' ? (
                <>This will permanently delete all messages with <span style={{ color: '#1e293b', fontFamily: 'monospace' }}>{confirmAction.name}</span>. The conversation row stays so they can message you again, but the history cannot be recovered.</>
              ) : (
                <>This will permanently delete the conversation with <span style={{ color: '#1e293b', fontFamily: 'monospace' }}>{confirmAction.name}</span> and all its messages. This cannot be undone.</>
              )}
            </p>
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
              <button onClick={() => setConfirmAction(null)}
                style={{
                  background: '#f1f5f9', color: '#64748b',
                  border: '1px solid #e2e8f0', borderRadius: '8px',
                  padding: '8px 16px', fontSize: '0.85rem', fontWeight: 600,
                  cursor: 'pointer',
                }}>
                Cancel
              </button>
              <button onClick={() => {
                  const a = confirmAction;
                  setConfirmAction(null);
                  if (a.type === 'clear') clearConv(a.phone);
                  else deleteConv(a.phone);
                }}
                style={{
                  background: 'rgba(239,68,68,0.08)', color: '#dc2626',
                  border: '1px solid rgba(239,68,68,0.3)', borderRadius: '8px',
                  padding: '8px 16px', fontSize: '0.85rem', fontWeight: 700,
                  cursor: 'pointer',
                }}>
                {confirmAction.type === 'clear' ? 'Clear' : 'Delete'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/* ─── Inline Styles ─── */
const leftPanelStyle = {
  width: '30%', minWidth: '280px', display: 'flex', flexDirection: 'column',
  borderRadius: '12px 0 0 12px', overflow: 'hidden',
};

const rightPanelStyle = {
  flex: 1, display: 'flex', flexDirection: 'column',
  background: '#f8fafc', borderRadius: '0 12px 12px 0',
  border: '1px solid #e2e8f0', borderLeft: 'none',
};

// Conversation kebab-menu items. Inline so we can override per-item
// (the destructive Delete uses red text, the rest neutral). Hover is
// handled via onMouseEnter/Leave inline at use sites — keeps this
// constant cheap and CSS-class-free for consistency with the rest of
// the file's styling approach.
const menuItemStyle = {
  display: 'block', width: '100%', textAlign: 'left',
  background: 'transparent', border: 'none', color: '#1e293b',
  padding: '8px 12px', borderRadius: '6px', cursor: 'pointer',
  fontSize: '0.82rem',
};

// Optional third column shown when the user clicks the 📱 button. Fixed
// width so it doesn't squeeze the chat too aggressively; collapses to
// nothing when toggled off.
const sessionPanelStyle = {
  width: '320px', flexShrink: 0,
  display: 'flex', flexDirection: 'column',
  background: '#f8fafc',
  border: '1px solid #e2e8f0', borderLeft: 'none',
  borderRadius: '0 12px 12px 0',
  marginLeft: '-12px', // sit flush against the chat panel rather than gap
};

const chatHeaderStyle = {
  display: 'flex', alignItems: 'center', padding: '0.75rem 1rem',
  borderRadius: 0, borderBottom: '1px solid #e2e8f0',
  margin: 0,
};

const messagesAreaStyle = {
  flex: 1, overflowY: 'auto', padding: '1rem',
  background: '#f0f4f8',
};

const inputBarStyle = {
  display: 'flex', gap: '8px', padding: '0.75rem 1rem',
  borderTop: '1px solid #e2e8f0',
  background: '#f8fafc',
};

const inputStyle = {
  width: '100%', boxSizing: 'border-box', padding: '8px 12px', borderRadius: '8px',
  border: '1px solid #e2e8f0', background: '#ffffff',
  color: '#1e293b', fontSize: '0.85rem', outline: 'none',
};

const selectStyle = {
  ...inputStyle,
  appearance: 'auto',
  WebkitAppearance: 'menulist',
  cursor: 'pointer',
};

const labelStyle = {
  display: 'block', color: '#64748b', fontSize: '0.75rem', fontWeight: 600,
  marginBottom: '4px', marginTop: '0.75rem',
};

const btnStyle = {
  border: 'none', borderRadius: '8px', cursor: 'pointer',
  padding: '8px 16px', fontSize: '0.85rem', transition: 'opacity 0.15s',
};

const btnSmallStyle = {
  border: 'none', borderRadius: '6px', cursor: 'pointer',
  padding: '4px 10px', fontSize: '0.75rem',
};

const toggleStyle = {
  border: 'none', borderRadius: '12px', cursor: 'pointer',
  padding: '4px 12px', fontSize: '0.72rem', fontWeight: 700, color: '#fff',
  transition: 'background 0.2s',
};

const greenDotStyle = {
  width: '8px', height: '8px', borderRadius: '50%', background: '#25D366',
  display: 'inline-block', flexShrink: 0,
};

const unreadBadgeStyle = {
  display: 'inline-block', background: '#25D366', color: '#fff',
  borderRadius: '10px', padding: '1px 7px', fontSize: '0.68rem', fontWeight: 700,
  marginTop: '2px',
};

const overlayStyle = {
  position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
  background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center',
  zIndex: 1000,
};

const modalStyle = {
  // overflowX: 'hidden' is a defence-in-depth in case any future field still
  // overflows the content area; the box-sizing fix on inputStyle is the
  // primary cause of the historical horizontal scrollbar.
  width: '480px', maxHeight: '85vh', overflowY: 'auto', overflowX: 'hidden',
  padding: '1.5rem', borderRadius: '12px', boxSizing: 'border-box',
};

const closeBtnStyle = {
  background: 'none', border: 'none', color: '#64748b', fontSize: '1.4rem',
  cursor: 'pointer', padding: '0 4px',
};
