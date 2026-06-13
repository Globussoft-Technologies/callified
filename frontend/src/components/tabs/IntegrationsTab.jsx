import React, { useState } from 'react';
import { formatDateTime } from '../../utils/dateFormat';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 16, boxShadow: '0 2px 8px rgba(0,0,0,0.06), 0 8px 24px rgba(0,0,0,0.06)',
};

const FALLBACK_FIELDS = [
  { key: 'api_key', label: 'API Key / Token', type: 'password' },
  { key: 'base_url', label: 'REST API Base URL', type: 'text' },
];

function SecretField({ value, onChange, placeholder, ariaInvalid }) {
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
        aria-invalid={ariaInvalid}
        style={{
          width: '100%', padding: '11px 52px 11px 14px', borderRadius: 10, fontSize: 13,
          border: `1px solid ${ariaInvalid ? T.red : T.border}`,
          background: '#f9fafb', color: T.text, fontFamily: T.font, outline: 'none',
          boxSizing: 'border-box',
        }}
      />
      <button
        type="button"
        onClick={() => setReveal(!reveal)}
        aria-label={reveal ? 'Hide value' : 'Show value'}
        style={{
          position: 'absolute', right: 10, top: '50%', transform: 'translateY(-50%)',
          background: 'none', border: 'none', color: T.accent,
          cursor: 'pointer', fontSize: 12, fontWeight: 600, padding: '2px 4px',
          fontFamily: T.font,
        }}>
        {reveal ? 'Hide' : 'Show'}
      </button>
    </div>
  );
}

function connectionStatus(intg) {
  if (!intg.is_active) return { label: 'Disabled', bg: 'rgba(148,163,184,0.15)', color: T.muted };
  if (!intg.last_synced_at) return { label: 'Pending first sync', bg: 'rgba(245,158,11,0.1)', color: T.amber };
  return { label: 'Active Sync', bg: 'rgba(16,185,129,0.1)', color: T.green };
}

export default function IntegrationsTab({
  handleCreateIntegration, intFormData, setIntFormData, CRM_SCHEMAS, loading, integrations, orgTimezone,
  fieldErrors = {}, setFieldErrors = () => {}
}) {
  const [submitAttempted, setSubmitAttempted] = useState(false);

  const fields = CRM_SCHEMAS[intFormData.provider] || FALLBACK_FIELDS;
  const missingKeys = fields.map(f => f.key).filter(k => !(intFormData.credentials[k] || '').trim());
  const isFormValid = missingKeys.length === 0;

  const onSubmit = (e) => {
    setSubmitAttempted(true);
    if (!isFormValid) { e.preventDefault(); return; }
    setSubmitAttempted(false);
    handleCreateIntegration(e);
  };

  const labelStyle = { fontSize: 14, fontWeight: 600, color: T.text, marginBottom: 8, display: 'block', fontFamily: T.font };
  const inputStyle = (hasError) => ({
    width: '100%', padding: '11px 14px', borderRadius: 10, fontSize: 13,
    border: `1px solid ${hasError ? T.red : T.border}`,
    background: '#f9fafb', color: T.text, fontFamily: T.font, outline: 'none',
    boxSizing: 'border-box',
  });

  const thStyle = {
    fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase',
    letterSpacing: '0.07em', padding: '0 0 12px', textAlign: 'left',
    borderBottom: `1px solid ${T.border}`,
  };
  const tdStyle = {
    fontSize: 13, color: T.sub, padding: '13px 0',
    borderBottom: `1px solid ${T.border}`, verticalAlign: 'middle',
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title */}
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
          <span style={{ color: T.accent }}>CRM</span> Integrations
        </h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Connect external CRM platforms to pull leads automatically and push call outcomes back.
        </p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(300px, 380px) 1fr', gap: 16, alignItems: 'stretch' }}>

        {/* Add Connection card */}
        <div style={{ ...card, padding: '24px 28px' }}>
          <h3 style={{ margin: '0 0 20px', fontSize: 16, fontWeight: 700, color: T.text }}>Add New Connection</h3>
          <form onSubmit={onSubmit} noValidate style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

            {/* Provider */}
            <div>
              <label style={labelStyle}>Provider</label>
              <select
                value={intFormData.provider}
                onChange={e => setIntFormData({ provider: e.target.value, credentials: {} })}
                style={{ ...inputStyle(false), cursor: 'pointer' }}>
                {Object.keys(CRM_SCHEMAS).map(p => (
                  <option key={p} value={p}>{p}</option>
                ))}
              </select>
            </div>

            {/* Dynamic fields */}
            {fields.map(field => {
              const value = intFormData.credentials[field.key] || '';
              const showError = submitAttempted && !value.trim();
              return (
                <div key={field.key}>
                  <label style={labelStyle}>
                    {field.label} <span style={{ color: T.red }}>*</span>
                  </label>
                  {field.type === 'password' ? (
                    <SecretField
                      value={value}
                      ariaInvalid={showError}
                      onChange={(v) => setIntFormData({ ...intFormData, credentials: { ...intFormData.credentials, [field.key]: v } })}
                      placeholder={field.label + '...'}
                    />
                  ) : (
                    <input
                      type={field.type}
                      value={value}
                      required
                      autoComplete="off"
                      aria-invalid={showError}
                      onChange={e => setIntFormData({ ...intFormData, credentials: { ...intFormData.credentials, [field.key]: e.target.value } })}
                      placeholder={field.label + '...'}
                      style={inputStyle(showError)}
                    />
                  )}
                  {showError && (
                    <div style={{ marginTop: 4, color: T.red, fontSize: 12, fontWeight: 600 }}>
                      {field.label} is required
                    </div>
                  )}
                </div>
              );
            })}

            <button
              type="submit"
              disabled={loading}
              title={!isFormValid ? `Fill in: ${missingKeys.join(', ')}` : undefined}
              style={{
                marginTop: 4, padding: '12px 18px', borderRadius: 8, border: 'none',
                fontWeight: 700, fontSize: 14, fontFamily: T.font,
                background: loading || !isFormValid
                  ? T.muted
                  : 'linear-gradient(135deg, #6366f1, #ec4899)',
                color: '#fff',
                cursor: loading || !isFormValid ? 'not-allowed' : 'pointer',
                width: '100%',
              }}>
              {loading ? 'Connecting...' : '⚡ Save Connection'}
            </button>
          </form>
        </div>

        {/* Active Connections card */}
        <div style={{ ...card, padding: '24px 28px', overflowX: 'auto' }}>
          <h3 style={{ margin: '0 0 20px', fontSize: 15, fontWeight: 700, color: T.text }}>Active Connections</h3>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={thStyle}>Provider</th>
                <th style={thStyle}>API Key (Masked)</th>
                <th style={thStyle}>Status</th>
                <th style={{ ...thStyle, textAlign: 'right' }}>Last Synced</th>
              </tr>
            </thead>
            <tbody>
              {integrations.length === 0 ? (
                <tr>
                  <td colSpan="4" style={{ textAlign: 'center', padding: '2.5rem 0', color: T.muted, fontSize: 14 }}>
                    No integrations connected yet.
                  </td>
                </tr>
              ) : integrations.map((intg, i) => {
                const isLast = i === integrations.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                const s = connectionStatus(intg);
                return (
                  <tr key={intg.id}>
                    <td style={{ ...rowTd, fontWeight: 600, color: T.text, paddingRight: 16 }}>{intg.provider}</td>
                    <td style={{ ...rowTd, fontFamily: T.mono, fontSize: 12, color: T.muted, paddingRight: 16 }}>
                      {Object.keys(intg.credentials || {}).map(k => (
                        <div key={k}>{k}: ••••••••</div>
                      ))}
                    </td>
                    <td style={{ ...rowTd, paddingRight: 16 }}>
                      <span style={{
                        fontSize: 11, fontWeight: 600, padding: '3px 10px', borderRadius: 20,
                        background: s.bg, color: s.color,
                      }}>{s.label}</span>
                    </td>
                    <td style={{ ...rowTd, textAlign: 'right', color: T.muted }}>
                      {intg.last_synced_at ? formatDateTime(intg.last_synced_at, orgTimezone) : 'Never'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

      </div>
    </div>
  );
}
