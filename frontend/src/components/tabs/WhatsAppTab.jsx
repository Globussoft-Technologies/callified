import React, { useState, useEffect, useRef, useCallback } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { formatDateTime, formatTime } from '../../utils/dateFormat';

const PROVIDERS = [
  { value: 'gupshup', label: 'Gupshup' },
  { value: 'wati', label: 'Wati' },
  { value: 'aisensei', label: 'AiSensei' },
  { value: 'interakt', label: 'Interakt' },
  { value: 'meta', label: 'Meta (Cloud API)' },
  { value: 'wasender', label: 'WaSender' },
];

// Field `key` values are the JSON keys posted to /api/wa/config; backend
// reads body.Credentials["api_key" / "app_id" / "phone_number" / "webhook_url"]
// and maps each into its own column. The user-facing `label` stays
// human-readable; only the wire-format key matches the backend's expected
// names. Renaming `app_name` → `app_id` and `source_phone` → `phone_number`
// closes the silent persistence gap where users filled the form, hit Save,
// reloaded, and watched their values disappear because no one was reading
// those keys server-side.
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
  meta: [
    { key: 'access_token', label: 'Access Token', type: 'password' },
    { key: 'phone_number_id', label: 'Phone Number ID', type: 'text' },
    { key: 'app_secret', label: 'App Secret', type: 'password' },
    { key: 'verify_token', label: 'Verify Token', type: 'text' },
  ],
  wasender: [
    { key: 'api_key', label: 'API Key', type: 'password' },
    { key: 'phone_number', label: 'Source Phone', type: 'text' },
    // webhook_secret is the shared secret WaSender echoes in the
    // X-Webhook-Signature header on every inbound delivery. Backend
    // rejects mismatched hits when set, so leaving it blank keeps the
    // legacy "accept anything" behaviour. Marked optional so the Save
    // button doesn't gate on it.
    { key: 'webhook_secret', label: 'Webhook Secret (recommended)', type: 'password', optional: true },
    { key: 'base_url', label: 'Base URL (optional)', type: 'text', optional: true },
  ],
};

/* ─── Secret input with show/hide toggle ─── */
// Used for any credential field (API key, bearer token, app secret). Masks
// the value by default to prevent shoulder-surfing during screen-share, with
// an eye toggle for the user to verify what they pasted. autoComplete and
// the data-* attributes opt out of browser/password-manager autofill so the
// user's personal saved logins don't accidentally get suggested into a
// third-party API-key field.
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
        style={{ ...inputStyle, paddingRight: '52px' }}
      />
      <button
        type="button"
        onClick={() => setReveal(!reveal)}
        aria-label={reveal ? 'Hide value' : 'Show value'}
        style={{
          position: 'absolute', right: '6px', top: '50%', transform: 'translateY(-50%)',
          background: 'none', border: 'none', color: '#94a3b8',
          cursor: 'pointer', fontSize: '0.75rem', padding: '4px 8px',
        }}>
        {reveal ? 'Hide' : 'Show'}
      </button>
    </div>
  );
}

/* ─── Config Modal ─── */
function ConfigModal({ show, onClose, apiFetch, API_URL, orgProducts, selectedOrg }) {
  const [provider, setProvider] = useState('gupshup');
  const [creds, setCreds] = useState({});
  const [defaultProduct, setDefaultProduct] = useState('');
  const [autoReply, setAutoReply] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (!show || !selectedOrg) return;
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
        setLoaded(true);
      })
      .catch(() => setLoaded(true));
  }, [show, selectedOrg]);

  // Required = every field shown for the current provider. The list in
  // PROVIDER_FIELDS is intentionally minimal (no optional fields) so any blank
  // entry in the modal means the provider integration won't actually work.
  const fields = PROVIDER_FIELDS[provider] || [];
  const missingField = fields.find(f => !f.optional && !(creds[f.key] || '').trim());
  const canSave = !saving && fields.length > 0 && !missingField;

  const handleSave = async () => {
    setError('');
    if (missingField) {
      setError(`${missingField.label} is required`);
      return;
    }
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
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch (_) { }
        setError(msg);
        setSaving(false);
        return;
      }
      onClose();
    } catch (e) {
      setError('Network error — could not reach server');
    }
    setSaving(false);
  };

  if (!show) return null;

  const webhookUrl = `https://testgo1.callified.ai/wa/webhook/${provider}`;

  return (
    <div style={overlayStyle} onClick={onClose}>
      <div className="glass-panel" style={modalStyle} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.2rem' }}>
          <h3 style={{ margin: 0, color: '#e2e8f0' }}>WhatsApp Channel Config</h3>
          <button onClick={onClose} style={closeBtnStyle}>&times;</button>
        </div>

        {error && (
          <div style={{ background: 'rgba(239,68,68,0.15)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#fca5a5', fontSize: '0.85rem' }}>
            {error}
          </div>
        )}

        <label style={labelStyle}>Provider</label>
        <select value={provider} onChange={e => { setProvider(e.target.value); setCreds({}); setError(''); }} style={selectStyle}>
          {PROVIDERS.map(p => <option key={p.value} value={p.value}>{p.label}</option>)}
        </select>

        {fields.map(f => (
          <div key={f.key}>
            <label style={labelStyle}>{f.label}</label>
            {f.type === 'password' ? (
              <SecretField
                value={creds[f.key] || ''}
                onChange={(v) => { setCreds({ ...creds, [f.key]: v }); if (error) setError(''); }}
                placeholder={f.label}
              />
            ) : (
              <input type={f.type} value={creds[f.key] || ''}
                onChange={e => { setCreds({ ...creds, [f.key]: e.target.value }); if (error) setError(''); }}
                style={inputStyle} placeholder={f.label} />
            )}
          </div>
        ))}

        <label style={labelStyle}>Default Product</label>
        <select value={defaultProduct} onChange={e => setDefaultProduct(e.target.value)} style={selectStyle}>
          <option value="">— None —</option>
          {(orgProducts || []).map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>

        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', margin: '1rem 0' }}>
          <label style={{ ...labelStyle, margin: 0 }}>Auto-Reply</label>
          <button onClick={() => setAutoReply(!autoReply)}
            style={{ ...toggleStyle, background: autoReply ? '#25D366' : '#4a5568' }}>
            {autoReply ? 'ON' : 'OFF'}
          </button>
        </div>

        <div style={{ background: 'rgba(37,211,102,0.08)', border: '1px solid rgba(37,211,102,0.2)', borderRadius: '8px', padding: '0.75rem', marginBottom: '1rem' }}>
          <label style={{ ...labelStyle, fontSize: '0.7rem', color: '#25D366' }}>Webhook URL — configure in your provider dashboard</label>
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <code style={{ flex: 1, color: '#e2e8f0', fontSize: '0.78rem', wordBreak: 'break-all' }}>{webhookUrl}</code>
            <button onClick={() => navigator.clipboard.writeText(webhookUrl)}
              style={{ ...btnSmallStyle, background: 'rgba(37,211,102,0.15)', color: '#25D366', border: '1px solid rgba(37,211,102,0.3)' }}>
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

  // syncWebhook pushes this backend's public webhook URL + secret up to
  // WaSender so the provider knows where to POST inbound messages. The
  // backend does this automatically inside connect() too — this button
  // is a manual re-sync for the case where the session was scanned
  // before the auto-sync existed, or the PUBLIC_SERVER_URL changed.
  // Without a successful sync, real-WhatsApp messages never reach our
  // /wa/webhook/wasender endpoint and the AI auto-reply silently doesn't
  // fire.
  const [synced, setSynced] = useState(false);
  const syncWebhook = async () => {
    if (!session?.id || busy) return;
    setBusy(true);
    setError('');
    setSynced(false);
    try {
      const res = await apiFetch(`${API_URL}/wa/session/${session.id}/sync-webhook`, { method: 'POST' });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setError(data?.message || `Sync failed (HTTP ${res.status})`);
        return;
      }
      setSynced(true);
      setTimeout(() => setSynced(false), 4000);
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
    } catch (_) { /* ignore — next tick retries */ }
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
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '0.75rem 1rem', borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
        <h3 style={{ margin: 0, color: '#25D366', fontSize: '0.95rem' }}>📱 WhatsApp Session</h3>
        <button onClick={onClose} style={{ ...btnSmallStyle, background: 'rgba(255,255,255,0.06)', color: '#94a3b8', border: '1px solid rgba(255,255,255,0.1)' }} title="Close">×</button>
      </div>
      <div style={{ padding: '1rem', flex: 1, overflowY: 'auto' }}>
        {loading && <div style={{ color: '#64748b', fontSize: '0.85rem' }}>Loading…</div>}

        {error && (
          <div style={{ background: 'rgba(239,68,68,0.15)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#fca5a5', fontSize: '0.8rem' }}>
            {error}
          </div>
        )}

        {!loading && needsPAT && (
          // Friendly fallback for accounts that only have a per-session
          // API key (good for sending) but no PAT (required for QR /
          // session-management endpoints). Sending and inbound webhooks
          // still work — only the in-dashboard scan flow is unavailable.
          <div style={{
            background: 'rgba(99,102,241,0.10)', border: '1px solid rgba(99,102,241,0.25)',
            borderRadius: '8px', padding: '12px', color: '#a5b4fc', fontSize: '0.82rem', lineHeight: 1.5,
          }}>
            <div style={{ fontWeight: 700, color: '#e2e8f0', marginBottom: '6px' }}>📱 Session view unavailable</div>
            Your WaSender plan exposes a per-session API key (used for sending) but not a Personal Access Token (needed to fetch QR codes from this dashboard).
            <div style={{ marginTop: '8px' }}>
              You can still scan and manage sessions on <a href="https://wasenderapi.com" target="_blank" rel="noreferrer" style={{ color: '#a5b4fc', textDecoration: 'underline' }}>wasenderapi.com</a> — and inbound/outbound messages will work normally here.
            </div>
          </div>
        )}

        {!loading && !session && !error && !needsPAT && (
          <div style={{ color: '#94a3b8', fontSize: '0.85rem' }}>
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
              <div style={{ color: '#94a3b8', fontSize: '0.65rem', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '4px' }}>Linked Device</div>
              <div style={{ color: '#fff', fontSize: '1.15rem', fontWeight: 700, fontFamily: 'monospace', marginBottom: '6px' }}>
                {session.phone_number || '—'}
              </div>
              <div style={{ color: '#94a3b8', fontSize: '0.78rem', marginBottom: '10px' }}>
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
            <div style={{ background: 'rgba(37,211,102,0.06)', border: '1px solid rgba(37,211,102,0.2)', borderRadius: '8px', padding: '0.6rem 0.75rem', color: '#bbf7d0', fontSize: '0.78rem', lineHeight: 1.5 }}>
              ✅ Send and receive WhatsApp messages. New incoming chats will appear in the inbox automatically.
            </div>
            {/* Sync webhook — tells WaSender to start POSTing inbound
                messages to this backend. Without it, real-WhatsApp
                "hlo" never reaches us and AI auto-reply doesn't fire. */}
            <button onClick={syncWebhook} disabled={busy}
              style={{
                width: '100%', marginTop: '0.6rem',
                background: synced ? 'rgba(37,211,102,0.18)' : 'transparent',
                color: synced ? '#25D366' : '#a5b4fc',
                border: `1px solid ${synced ? 'rgba(37,211,102,0.5)' : 'rgba(165,180,252,0.4)'}`,
                borderRadius: '8px',
                padding: '8px', fontSize: '0.78rem', fontWeight: 600,
                cursor: busy ? 'not-allowed' : 'pointer', opacity: busy ? 0.5 : 1,
              }}>
              {busy ? 'Syncing…' : synced ? '✓ Webhook synced' : 'Sync webhook to WaSender'}
            </button>
            {/* Disconnect button — recovers from stale "connected" state
                where the phone unlinked but WaSender hasn't caught up.
                Opens a themed confirmation modal (rendered below) so the
                user can't fat-finger this irreversible action. */}
            <button onClick={() => setConfirmDisconnect(true)} disabled={busy}
              style={{
                width: '100%', marginTop: '0.4rem',
                background: 'transparent', color: '#fbbf24',
                border: '1px solid rgba(234,179,8,0.4)', borderRadius: '8px',
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
              <div style={{ color: '#94a3b8', fontSize: '0.7rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Session</div>
              <div style={{ color: '#e2e8f0', fontSize: '0.9rem', fontWeight: 600 }}>{session.name || `#${session.id}`}</div>
            </div>
            <div style={{ marginBottom: '0.75rem' }}>
              <div style={{ color: '#94a3b8', fontSize: '0.7rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Phone</div>
              <div style={{ color: '#e2e8f0', fontSize: '0.85rem', fontFamily: 'monospace' }}>{session.phone_number || '—'}</div>
            </div>
            <div style={{ marginBottom: '1rem' }}>
              <div style={{ color: '#94a3b8', fontSize: '0.7rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Status</div>
              <div>
                <span style={{
                  display: 'inline-block', padding: '3px 10px', borderRadius: '10px',
                  fontSize: '0.72rem', fontWeight: 700,
                  background: 'rgba(234,179,8,0.18)', color: '#fbbf24',
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
              <div style={{ background: 'rgba(255,255,255,0.04)', border: '1px dashed rgba(255,255,255,0.12)', borderRadius: '8px', padding: '2rem', textAlign: 'center', color: '#64748b', fontSize: '0.8rem', marginBottom: '0.75rem' }}>
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
              background: '#1a2236',
              border: '1px solid rgba(255,255,255,0.08)',
              boxShadow: '0 20px 60px rgba(0,0,0,0.5)',
            }}
            onClick={e => e.stopPropagation()}
          >
            <h3 style={{ margin: '0 0 0.75rem 0', color: '#e2e8f0' }}>Disconnect WhatsApp?</h3>
            <p style={{ color: '#94a3b8', fontSize: '0.85rem', lineHeight: 1.5, marginBottom: '1.2rem' }}>
              This will drop the link to <span style={{ color: '#e2e8f0', fontFamily: 'monospace' }}>{session?.phone_number || 'this device'}</span>. You'll need to scan a fresh QR code to reconnect.
            </p>
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
              <button onClick={() => setConfirmDisconnect(false)}
                style={{
                  background: 'rgba(255,255,255,0.06)', color: '#94a3b8',
                  border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px',
                  padding: '8px 16px', fontSize: '0.85rem', fontWeight: 600,
                  cursor: 'pointer',
                }}>
                Cancel
              </button>
              <button onClick={confirmDisconnectAction}
                style={{
                  background: 'rgba(234,179,8,0.18)', color: '#fbbf24',
                  border: '1px solid rgba(234,179,8,0.4)', borderRadius: '8px',
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
    } catch (e) {
      setError('Network error — could not reach server');
      setBusy(false);
    }
  };

  return (
    <div style={overlayStyle} onClick={onClose}>
      <div className="glass-panel" style={{ ...modalStyle, maxWidth: '440px' }} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h3 style={{ margin: 0, color: '#e2e8f0' }}>Start New Chat</h3>
          <button onClick={onClose} style={closeBtnStyle}>&times;</button>
        </div>

        {error && (
          <div style={{ background: 'rgba(239,68,68,0.15)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#fca5a5', fontSize: '0.85rem' }}>
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
export default function WhatsAppTab({ apiFetch, API_URL, orgProducts, selectedOrg, orgTimezone }) {
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
  // ensuredPhonesRef tracks which phones we've already auto-created
  // conversations for in this browser session, so we don't hammer the
  // backend on every render — but the server-side handler is idempotent
  // either way, so a stale ref is harmless.
  const ensuredPhonesRef = useRef(new Set());

  // Post-scan effect: whenever we have a connected session with a known
  // phone, ensure a conversation row exists for it so the operator sees
  // their linked number in the left-side inbox immediately. Idempotent
  // on both sides — the server uses GetOrCreateWAConversation, and we
  // memoize per-phone here so subsequent renders are no-ops.
  useEffect(() => {
    if (!sessionConnected || !sessionPhone) return;
    const key = sessionPhone.replace(/^\+/, '');
    if (ensuredPhonesRef.current.has(key)) return;
    ensuredPhonesRef.current.add(key);

    setShowSession(true);
    apiFetch(`${API_URL}/wa/conversations/ensure`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ phone: sessionPhone, provider: 'wasender' }),
    }).then(() => {
      // Refresh inbox list so the new empty conversation appears
      // without waiting for the next poll tick.
      fetchConversations?.();
    }).catch(() => {
      // On failure, allow a retry next render so a transient network
      // hiccup doesn't permanently block the auto-create.
      ensuredPhonesRef.current.delete(key);
    });
    // fetchConversations is intentionally omitted from deps — using it
    // would cycle the effect on every poll-driven re-render. The
    // optional-chain call below tolerates first-mount ordering.
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
        // Build AI-enabled map
        const map = {};
        convos.forEach(c => { map[c.phone || c.contact_phone] = c.ai_active !== false; });
        setAiEnabled(map);
      }
    } catch (e) { console.error('Failed to fetch conversations', e); }
  }, [apiFetch, API_URL, showArchived]);

  useEffect(() => { fetchConversations(); }, [fetchConversations]);

  /* ── Fetch messages for selected conversation ── */
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

  /* ── Poll every 5s ── */
  useEffect(() => {
    if (pollRef.current) clearInterval(pollRef.current);
    pollRef.current = setInterval(() => {
      fetchConversations();
      if (selectedPhone) fetchMessages();
    }, 5000);
    return () => clearInterval(pollRef.current);
  }, [fetchConversations, fetchMessages, selectedPhone]);

  /* ── Auto-scroll ── */
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

  /* ── Send message ──
     The "AI Auto-Reply" toggle above the chat decides what Send does:
       • ON  → /wa/send-ai. The text is treated as a simulated inbound
               from the customer and the AI generates an outbound reply.
               Nothing leaves the system; this is the in-dashboard AI
               test/preview path.
       • OFF → /wa/send. Real outbound to the customer via the configured
               WhatsApp provider — the operator-typing-to-customer flow.
     Both endpoints save messages so the chat view refreshes the same way. */
  const handleSend = async () => {
    if (!messageText.trim() || !selectedPhone || sending) return;
    setSending(true);
    try {
      const endpoint = aiActive ? '/wa/send-ai' : '/wa/send';
      await apiFetch(`${API_URL}${endpoint}`, {
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

  /* ── Toggle AI ── */
  const toggleAi = async () => {
    if (!selectedPhone) return;
    const current = aiEnabled[selectedPhone] !== false;
    try {
      await apiFetch(`${API_URL}/wa/toggle-ai/${encodeURIComponent(selectedPhone)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !current }),
      });
      setAiEnabled(prev => ({ ...prev, [selectedPhone]: !current }));
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
    <div style={{ display: 'flex', height: 'calc(100vh - 140px)', gap: '0' }}>
      {/* ── LEFT PANEL: Conversation List ── */}
      <div className="glass-panel" style={leftPanelStyle}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '0.75rem 1rem', borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
          <h3 style={{ margin: 0, color: '#25D366', fontSize: '1rem' }}>
            <span style={{ marginRight: '6px' }}>💬</span>WhatsApp Inbox
          </h3>
          <div style={{ display: 'flex', gap: '6px' }}>
            <button onClick={() => setShowNewChat(true)}
              style={{ ...btnSmallStyle, background: 'rgba(37,211,102,0.18)', color: '#25D366', border: '1px solid rgba(37,211,102,0.3)', fontWeight: 700 }}
              title="Start a new chat">
              + New
            </button>
            <button onClick={() => setShowSession(s => !s)}
              style={{ ...btnSmallStyle, background: showSession ? 'rgba(37,211,102,0.18)' : 'rgba(255,255,255,0.06)', color: showSession ? '#25D366' : '#94a3b8', border: '1px solid rgba(255,255,255,0.1)' }}
              title="WhatsApp Session / QR">
              📱
            </button>
            <button onClick={() => setShowConfig(true)}
              style={{ ...btnSmallStyle, background: 'rgba(255,255,255,0.06)', color: '#94a3b8', border: '1px solid rgba(255,255,255,0.1)' }}
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
              background: showArchived ? 'rgba(99,102,241,0.18)' : 'rgba(255,255,255,0.06)',
              color: showArchived ? '#a5b4fc' : '#94a3b8',
              border: '1px solid rgba(255,255,255,0.1)',
            }}>
            {showArchived ? '📂' : '📁'}
          </button>
        </div>

        {/* Conversations */}
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {filtered.length === 0 ? (
            <div style={{ padding: '2rem 1rem', textAlign: 'center', color: '#64748b', fontSize: '0.85rem' }}>
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
            const linkedNow = sessionPhone && noPlus === sessionPhone.replace(/^\+/, '');
            const everLinked = ensuredPhonesRef.current.has(noPlus);
            const isLinkedDevice = linkedNow || everLinked;
            return (
            <div key={conv.phone} onClick={() => setSelectedPhone(conv.phone)}
              style={{
                padding: '0.7rem 1rem', cursor: 'pointer', borderBottom: '1px solid rgba(255,255,255,0.04)',
                background: selectedPhone === conv.phone ? 'rgba(37,211,102,0.08)' : 'transparent',
                transition: 'background 0.15s',
              }}
              onMouseEnter={e => { if (selectedPhone !== conv.phone) e.currentTarget.style.background = 'rgba(255,255,255,0.03)'; }}
              onMouseLeave={e => { if (selectedPhone !== conv.phone) e.currentTarget.style.background = 'transparent'; }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flex: 1, minWidth: 0 }}>
                  {isLinkedDevice && <span title={linkedNow ? 'Currently linked WhatsApp device' : 'Previously linked device'} style={{ fontSize: '0.85rem', flexShrink: 0 }}>📱</span>}
                  {!isLinkedDevice && conv.ai_active && <span style={greenDotStyle} title="AI Auto-Reply active" />}
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ color: '#e2e8f0', fontWeight: 600, fontSize: '0.85rem', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {conv.name || conv.phone}
                      {linkedNow && <span style={{ marginLeft: '6px', fontSize: '0.62rem', color: '#25D366', fontWeight: 700, letterSpacing: '0.05em' }}>● LIVE</span>}
                    </div>
                    <div style={{ fontFamily: 'monospace', color: '#64748b', fontSize: '0.72rem' }}>{conv.phone}</div>
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
                    <span style={unreadBadgeStyle}>{conv.unread_count}</span>
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
                      color: '#94a3b8', cursor: 'pointer', fontSize: '1.1rem',
                      padding: '0 4px', lineHeight: 1,
                    }}>
                    ⋮
                  </button>
                  {openMenu === conv.phone && (
                    <div onClick={(e) => e.stopPropagation()}
                      style={{
                        position: 'absolute', top: '100%', right: 0, marginTop: '4px',
                        background: '#1a2236', border: '1px solid rgba(255,255,255,0.1)',
                        borderRadius: '8px', boxShadow: '0 8px 24px rgba(0,0,0,0.5)',
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
                      <div style={{ height: '1px', background: 'rgba(255,255,255,0.06)', margin: '4px 0' }} />
                      <button onClick={() => { setOpenMenu(null); setConfirmAction({ type: 'delete', phone: conv.phone, name: conv.name || conv.phone }); }}
                        style={{ ...menuItemStyle, color: '#fca5a5' }}>
                        🗑 Delete
                      </button>
                    </div>
                  )}
                </div>
              </div>
              {conv.last_message && (
                <div style={{ color: '#94a3b8', fontSize: '0.78rem', marginTop: '4px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {conv.last_message.length > 40 ? conv.last_message.substring(0, 40) + '...' : conv.last_message}
                </div>
              )}
            </div>
            );
          })}
        </div>
      </div>

      {/* ── RIGHT PANEL: Chat Window ── */}
      <div style={rightPanelStyle}>
        {!selectedPhone ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#64748b', fontSize: '0.95rem' }}>
            Select a conversation to start chatting
          </div>
        ) : (
          <>
            {/* Chat Header */}
            <div className="glass-panel" style={chatHeaderStyle}>
              <div style={{ flex: 1 }}>
                <div style={{ color: '#e2e8f0', fontWeight: 700, fontSize: '0.95rem' }}>
                  {selectedConv?.name || selectedPhone}
                </div>
                <div style={{ fontFamily: 'monospace', color: '#64748b', fontSize: '0.78rem' }}>{selectedPhone}</div>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span style={{ color: '#94a3b8', fontSize: '0.78rem' }}>AI Auto-Reply</span>
                <button onClick={toggleAi}
                  style={{
                    ...toggleStyle,
                    background: aiActive ? '#25D366' : '#ef4444',
                    minWidth: '48px',
                  }}>
                  {aiActive ? 'ON' : 'OFF'}
                </button>
              </div>
            </div>

            {/* Messages */}
            <div style={messagesAreaStyle}>
              {messages.length === 0 ? (
                <div style={{ textAlign: 'center', color: '#64748b', marginTop: '3rem', fontSize: '0.85rem' }}>No messages yet</div>
              ) : messages.map((msg, i) => {
                const isOutbound = msg.direction === 'outbound';
                return (
                  <div key={msg.id || i} style={{ display: 'flex', justifyContent: isOutbound ? 'flex-end' : 'flex-start', marginBottom: '8px' }}>
                    <div style={{
                      maxWidth: '70%', padding: '8px 12px', borderRadius: '12px',
                      background: isOutbound ? '#25D366' : '#2d3748',
                      color: isOutbound ? '#fff' : '#e2e8f0',
                      fontSize: '0.85rem', lineHeight: '1.45',
                      borderTopRightRadius: isOutbound ? '4px' : '12px',
                      borderTopLeftRadius: isOutbound ? '12px' : '4px',
                    }}>
                      {msg.ai_generated && <span title="AI-generated" style={{ marginRight: '4px' }}>🤖</span>}
                      <span>{msg.message_text || msg.text || msg.body || msg.content}</span>
                      <div style={{ fontSize: '0.65rem', color: isOutbound ? 'rgba(255,255,255,0.7)' : '#64748b', marginTop: '4px', textAlign: 'right' }}>
                        {formatTime(msg.created_at || msg.timestamp, orgTimezone)}
                      </div>
                    </div>
                  </div>
                );
              })}
              <div ref={messagesEndRef} />
            </div>

            {/* Input Bar */}
            <div style={inputBarStyle}>
              <input type="text" value={messageText}
                onChange={e => setMessageText(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend(); } }}
                placeholder="Type a message..."
                style={{ ...inputStyle, flex: 1, margin: 0, fontSize: '0.85rem', padding: '10px 14px' }} />
              <button onClick={handleSend} disabled={sending || !messageText.trim()}
                style={{
                  ...btnStyle, background: '#25D366', color: '#fff', fontWeight: 700,
                  opacity: (sending || !messageText.trim()) ? 0.5 : 1, padding: '10px 20px',
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
        apiFetch={apiFetch} API_URL={API_URL} orgProducts={orgProducts} selectedOrg={selectedOrg} />

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
              background: '#1a2236',
              border: '1px solid rgba(255,255,255,0.08)',
              boxShadow: '0 20px 60px rgba(0,0,0,0.5)',
            }}
            onClick={e => e.stopPropagation()}
          >
            <h3 style={{ margin: '0 0 0.75rem 0', color: '#e2e8f0' }}>
              {confirmAction.type === 'clear' ? 'Clear chat history?' : 'Delete conversation?'}
            </h3>
            <p style={{ color: '#94a3b8', fontSize: '0.85rem', lineHeight: 1.5, marginBottom: '1.2rem' }}>
              {confirmAction.type === 'clear' ? (
                <>This will permanently delete all messages with <span style={{ color: '#e2e8f0', fontFamily: 'monospace' }}>{confirmAction.name}</span>. The conversation row stays so they can message you again, but the history cannot be recovered.</>
              ) : (
                <>This will permanently delete the conversation with <span style={{ color: '#e2e8f0', fontFamily: 'monospace' }}>{confirmAction.name}</span> and all its messages. This cannot be undone.</>
              )}
            </p>
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
              <button onClick={() => setConfirmAction(null)}
                style={{
                  background: 'rgba(255,255,255,0.06)', color: '#94a3b8',
                  border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px',
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
                  background: 'rgba(239,68,68,0.18)', color: '#fca5a5',
                  border: '1px solid rgba(239,68,68,0.4)', borderRadius: '8px',
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
  background: 'rgba(255,255,255,0.01)', borderRadius: '0 12px 12px 0',
  border: '1px solid rgba(255,255,255,0.06)', borderLeft: 'none',
};

// Conversation kebab-menu items. Inline so we can override per-item
// (the destructive Delete uses red text, the rest neutral). Hover is
// handled via onMouseEnter/Leave inline at use sites — keeps this
// constant cheap and CSS-class-free for consistency with the rest of
// the file's styling approach.
const menuItemStyle = {
  display: 'block', width: '100%', textAlign: 'left',
  background: 'transparent', border: 'none', color: '#e2e8f0',
  padding: '8px 12px', borderRadius: '6px', cursor: 'pointer',
  fontSize: '0.82rem',
};

// Optional third column shown when the user clicks the 📱 button. Fixed
// width so it doesn't squeeze the chat too aggressively; collapses to
// nothing when toggled off.
const sessionPanelStyle = {
  width: '320px', flexShrink: 0,
  display: 'flex', flexDirection: 'column',
  background: 'rgba(255,255,255,0.02)',
  border: '1px solid rgba(255,255,255,0.06)', borderLeft: 'none',
  borderRadius: '0 12px 12px 0',
  marginLeft: '-12px', // sit flush against the chat panel rather than gap
};

const chatHeaderStyle = {
  display: 'flex', alignItems: 'center', padding: '0.75rem 1rem',
  borderRadius: 0, borderBottom: '1px solid rgba(255,255,255,0.06)',
  margin: 0,
};

const messagesAreaStyle = {
  flex: 1, overflowY: 'auto', padding: '1rem',
  background: 'rgba(0,0,0,0.15)',
};

const inputBarStyle = {
  display: 'flex', gap: '8px', padding: '0.75rem 1rem',
  borderTop: '1px solid rgba(255,255,255,0.06)',
  background: 'rgba(255,255,255,0.02)',
};

const inputStyle = {
  width: '100%', boxSizing: 'border-box', padding: '8px 12px', borderRadius: '8px',
  border: '1px solid rgba(255,255,255,0.1)', background: '#1e293b',
  color: '#e2e8f0', fontSize: '0.85rem', outline: 'none',
};

const selectStyle = {
  ...inputStyle,
  appearance: 'auto',
  WebkitAppearance: 'menulist',
  cursor: 'pointer',
};

const labelStyle = {
  display: 'block', color: '#94a3b8', fontSize: '0.75rem', fontWeight: 600,
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
  background: 'none', border: 'none', color: '#94a3b8', fontSize: '1.4rem',
  cursor: 'pointer', padding: '0 4px',
};
