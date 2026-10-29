import React, { useState, useEffect, useCallback, useRef } from 'react';
import { API_URL } from '../constants/api';
import { useAuth } from '../contexts/AuthContext';
import { useConfirm } from '../contexts/UIContext';
import AppIcon from '../components/common/AppIcon';
import AppSelect from '../components/common/AppSelect';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', red: '#ef4444', green: '#10b981',
  text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.04)',
};

const inputStyle = {
  padding: '8px 12px', border: `1px solid ${T.border}`, borderRadius: 8,
  fontSize: 13, fontFamily: T.font, color: T.text, background: '#fff',
  outline: 'none', width: '100%', boxSizing: 'border-box',
};

const labelStyle = {
  display: 'block', color: T.sub, fontSize: 12,
  fontWeight: 600, marginBottom: 4,
};

const EMPTY_FORM = {
  provider: 'exotel',
  direction: 'outbound',
  name: '',
  // Exotel fields
  api_key: '', api_token: '', account_sid: '', caller_id: '', app_id: '', app_type: 'exoml',
  region: '', subdomain: '',
  // Twilio-only
  api_secret: '',
};

const PROVIDER_BADGE = {
  plivo: { label: 'Plivo', bg: 'rgba(16,185,129,0.08)', color: '#059669', border: 'rgba(16,185,129,0.25)' },
  exotel: { label: 'Exotel', bg: 'rgba(99,102,241,0.08)', color: '#6366f1', border: 'rgba(99,102,241,0.25)' },
  twilio: { label: 'Twilio', bg: 'rgba(239,68,68,0.06)', color: '#ef4444', border: 'rgba(239,68,68,0.2)' },
  tata: { label: 'Tata Tele', bg: 'rgba(16,185,129,0.08)', color: '#059669', border: 'rgba(16,185,129,0.25)' },
};

const PROVIDER_LABELS = {
  plivo: 'Plivo',
  exotel: 'Exotel',
  tata: 'Tata Tele',
  twilio: 'Twilio',
};

export default function ExotelAccountsPage({ tourFormMode = null, tourActive = false }) {
  const { apiFetch } = useAuth();
  const [accounts, setAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState(EMPTY_FORM);
  const [editingId, setEditingId] = useState(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [showApiKey, setShowApiKey] = useState(false);
  const [showApiToken, setShowApiToken] = useState(false);
  const tourOwnedForm = useRef(false);

  const confirmDialog = useConfirm();
  const [streamVerified, setStreamVerified] = useState(null);
  const [secretAccount, setSecretAccount] = useState(null);
  const [streamSetup, setStreamSetup] = useState(null);
  const [rotatingSecret, setRotatingSecret] = useState(false);
  const [streamSetupError, setStreamSetupError] = useState('');
  const rotateStreamSecret = async account => {
    const confirmed = await confirmDialog({
      title: 'Generate stream secret?',
      message: 'The previous secret will stop working. Update the matching Tata endpoint before placing calls.',
      okText: 'Generate secret',
      cancelText: 'Cancel',
    });
    if (!confirmed) return;
    setStreamSetupError('');
    setRotatingSecret(true);
    try {
      const response = await apiFetch(`${API_URL}/exotel-accounts/${account.id}/stream-secret`, { method: 'POST' });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Could not generate stream secret');
      setStreamSetup({ ...data, name: account.name });
      setStreamVerified(false);
    } catch (err) { setStreamSetupError(err.message); }
    finally { setRotatingSecret(false); }
  };
  const checkStreamVerification = async () => {
    try {
      const response = await apiFetch(`${API_URL}/exotel-accounts/${secretAccount.id}/stream-readiness`);
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Could not check verification');
      setStreamVerified(data.verified);
    } catch (err) { setStreamSetupError(err.message); }
  };
  const hideStreamSecret = () => {
    setStreamSetup(previous => previous ? { ...previous, secret: null } : previous);
  };
  const copyStreamSecret = async () => {
    if (!streamSetup?.secret) return;
    try {
      await navigator.clipboard.writeText(streamSetup.secret);
      hideStreamSecret();
      setStreamSetupError('');
    } catch {
      setStreamSetupError('Could not copy. Copy the secret manually, then click Hide.');
    }
  };
  const fetchAccounts = useCallback(async () => {
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/exotel-accounts`);
      const data = await res.json();
      setAccounts(Array.isArray(data) ? data : []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [apiFetch]);

  useEffect(() => { fetchAccounts(); }, [fetchAccounts]);

  const openAdd = (direction = 'outbound') => {
    setForm({ ...EMPTY_FORM, provider: direction === 'inbound' ? 'tata' : 'exotel', direction });
    setEditingId(null);
    setError('');
    setShowForm(true);
    setShowApiKey(false);
    setShowApiToken(false);
  };
  const openEdit = (a) => {
    setForm({
      provider: a.provider || 'exotel',
      direction: a.direction || 'outbound',
      name: a.name,
      api_key: a.api_key,
      api_token: a.api_token,
      api_secret: a.api_secret || '',
      account_sid: a.account_sid,
      caller_id: a.caller_id,
      app_id: a.provider === 'tata' ? (a.app_id || a.account_sid || '') : (a.app_id || ''),
      app_type: a.app_type || 'exoml',
      region: a.region || '',
      subdomain: a.subdomain || '',
    });
    setEditingId(a.id);
    setError('');
    setShowForm(true);
    setShowApiKey(false);
    setShowApiToken(false);
  };
  const closeForm = () => { setShowForm(false); setEditingId(null); setForm({ ...EMPTY_FORM }); setError(''); setShowApiKey(false); setShowApiToken(false); };

  useEffect(() => {
    if (tourFormMode) {
      tourOwnedForm.current = true;
      setForm({ ...EMPTY_FORM, provider: tourFormMode });
      setEditingId(null);
      setError('');
      setShowForm(true);
      setShowApiKey(false);
      setShowApiToken(false);
    } else if (tourOwnedForm.current) {
      tourOwnedForm.current = false;
      setShowForm(false);
      setForm({ ...EMPTY_FORM });
    }
  }, [tourFormMode]);

  const setField = (key, val) => setForm(f => ({ ...f, [key]: val }));

  const handleSave = async (e) => {
    e.preventDefault();
    if (tourActive) return;
    const { provider, name, api_key, api_token, api_secret, account_sid, caller_id } = form;
    if (!name.trim()) { setError('Account name is required.'); return; }
    if (provider === 'twilio') {
      if (!account_sid || !api_key || !api_token || !api_secret || !caller_id) {
        setError('Account SID, Auth Token, API Key SID, API Secret and Phone Number are required for Twilio.');
        return;
      }
    } else if (provider === 'plivo') {
      if (!account_sid || !api_token || !caller_id) { setError('Auth ID, Auth Token and Caller Number are required for Plivo.'); return; }
    } else if (provider === 'tata') {
      if (form.direction === 'inbound') {
        if (!api_key || !caller_id) {
          setError('API Token and DID are required for inbound Tata Tele.');
          return;
        }
      } else if (!api_key || !api_token || !caller_id || !form.app_id) {
        setError('Click-to-Call Support API Key, Smartflo API Token, Caller ID and Agent Number are required for Tata Tele.');
        return;
      }
    } else {
      if (!api_key || !api_token || !account_sid || !caller_id) {
        setError('API Key, API Token, Account SID and Caller ID are required for Exotel.');
        return;
      }
    }
    setSaving(true);
    setError('');
    try {
      const payload = form.provider === 'tata'
        ? { ...form, api_token: form.api_token, account_sid: form.account_sid || form.app_id }
        : form.provider === 'plivo'
          ? { ...form, api_key: '', api_secret: '', app_id: '', app_type: 'exoml', region: '', subdomain: '' }
          : form;
      const method = editingId ? 'PUT' : 'POST';
      const url = editingId
        ? `${API_URL}/exotel-accounts/${editingId}`
        : `${API_URL}/exotel-accounts`;
      const res = await apiFetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        throw new Error(d.error || 'Save failed');
      }
      closeForm();
      fetchAccounts();
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id) => {
    if (tourActive) return;
    if (!window.confirm('Delete this provider account?')) return;
    try {
      await apiFetch(`${API_URL}/exotel-accounts/${id}`, { method: 'DELETE' });
      fetchAccounts();
    } catch (e) {
      console.error(e);
    }
  };

  const isExotel = form.provider === 'exotel';
  const isTata = form.provider === 'tata';
  const isTwilio = form.provider === 'twilio';
  const isInbound = form.direction === 'inbound';

  return (
    <div style={{ padding: '28px 32px', maxWidth: 900, margin: '0 auto', fontFamily: T.font, background: T.bg, minHeight: '100%' }}>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '1.5rem' }}>
        <div>
          <h2 style={{ margin: 0, color: T.text, fontSize: '1.4rem', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>
            <AppIcon name="phone" /> Provider Accounts
          </h2>
          <p style={{ margin: '4px 0 0', color: T.muted, fontSize: '0.85rem' }}>
            Save calling provider credentials and select one per campaign.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          <button data-tour="provider-account-add" onClick={() => openAdd('outbound')}
            style={{
              background: 'linear-gradient(135deg, #6366f1, #8b5cf6)',
              border: 'none', borderRadius: 8, color: '#fff',
              padding: '9px 18px', cursor: 'pointer', fontSize: 13,
              fontWeight: 600, fontFamily: T.font, whiteSpace: 'nowrap',
            }}>
            + Add Account
          </button>
        </div>
      </div>

      {/* Add / Edit Form */}
      {showForm && (
        <div style={{ ...card, padding: '1.5rem', marginBottom: '1.5rem' }}>
          <h3 style={{ margin: '0 0 1.2rem', color: T.text, fontSize: '1rem', fontWeight: 700 }}>
            {editingId ? 'Edit Account' : isInbound ? 'Add Inbound Account' : 'Add New Account'}
          </h3>
          <form onSubmit={handleSave}>

            {/* Provider selector */}
            <div data-tour="provider-account-usage" style={{ marginBottom: '1.2rem' }}>
              <label style={labelStyle}>Account Usage <span style={{ color: T.red }}>*</span></label>
              <div style={{ display: 'flex', gap: 8, marginBottom: '1rem' }}>
                {[
                  { id: 'outbound', label: 'Outbound' },
                  { id: 'inbound', label: 'Inbound' },
                ].map(d => (
                  <button key={d.id} type="button"
                    onClick={() => setForm(f => ({ ...f, direction: d.id, provider: d.id === 'inbound' ? 'tata' : f.provider }))}
                    style={{
                      padding: '7px 18px', borderRadius: 8, fontSize: 13, fontWeight: 700,
                      fontFamily: T.font, cursor: 'pointer',
                      border: form.direction === d.id ? `2px solid ${d.id === 'inbound' ? T.green : T.accent}` : `1px solid ${T.border}`,
                      background: form.direction === d.id ? (d.id === 'inbound' ? 'rgba(16,185,129,0.08)' : 'rgba(99,102,241,0.08)') : '#fff',
                      color: form.direction === d.id ? (d.id === 'inbound' ? '#059669' : T.accent) : T.sub,
                    }}>
                    {d.label}
                  </button>
                ))}
              </div>
              <label style={labelStyle}>Provider <span style={{ color: T.red }}>*</span></label>
              <div style={{ display: 'flex', gap: 8 }}>
                {(isInbound ? ['tata'] : ['exotel', 'tata', 'plivo' /* , 'twilio' */]).map(p => (
                  <button key={p} type="button"
                    onClick={() => setField('provider', p)}
                    style={{
                      padding: '7px 22px', borderRadius: 8, fontSize: 13, fontWeight: 600,
                      fontFamily: T.font, cursor: 'pointer',
                      border: form.provider === p ? `2px solid ${T.accent}` : `1px solid ${T.border}`,
                      background: form.provider === p ? 'rgba(99,102,241,0.08)' : '#fff',
                      color: form.provider === p ? T.accent : T.sub,
                    }}>
                    {PROVIDER_LABELS[p] || p}
                  </button>
                ))}
              </div>
            </div>

            {/* Account Name */}
            <div data-tour="provider-account-name" style={{ marginBottom: '1rem' }}>
              <label style={labelStyle}>Account Name <span style={{ color: T.red }}>*</span></label>
              <input style={inputStyle} placeholder={isInbound ? 'e.g. Tata Inbound Receptionist' : 'e.g. Main Caller, Sales India'}
                value={form.name} onChange={e => setField('name', e.target.value)} />
            </div>

            {/* Exotel fields */}
            {form.provider === 'plivo' && <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 12, marginBottom: 16 }}>
              <div><label style={labelStyle} htmlFor="plivo-auth-id">Auth ID *</label><input id="plivo-auth-id" style={inputStyle} value={form.account_sid} onChange={e => setField('account_sid', e.target.value)} required /></div>
              <div><label style={labelStyle} htmlFor="plivo-auth-token">Auth Token *</label><div style={{ position: 'relative' }}><input id="plivo-auth-token" type={showApiToken ? 'text' : 'password'} autoComplete="new-password" style={{ ...inputStyle, paddingRight: 60 }} value={form.api_token} onChange={e => setField('api_token', e.target.value)} required /><button type="button" aria-label={showApiToken ? 'Hide Auth Token' : 'Show Auth Token'} onClick={() => setShowApiToken(v => !v)} style={{ position: 'absolute', right: 8, top: 8 }}>{showApiToken ? 'Hide' : 'Show'}</button></div></div>
              <div><label style={labelStyle} htmlFor="plivo-caller">Caller Number *</label><input id="plivo-caller" type="tel" placeholder="+91" style={inputStyle} value={form.caller_id} onChange={e => setField('caller_id', e.target.value)} required /></div>
            </div>}
            {isExotel && (
              <div data-tour="provider-account-exotel-fields" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: '12px', marginBottom: '1rem' }}>
                <div>
                  <label style={labelStyle}>API Key <span style={{ color: T.red }}>*</span></label>
                  <div style={{ position: 'relative' }}>
                    <input style={{ ...inputStyle, paddingRight: '2.5rem' }} placeholder="API Key" type={showApiKey ? 'text' : 'password'}
                      autoComplete="new-password"
                      value={form.api_key} onChange={e => setField('api_key', e.target.value)} />
                    <button type="button" onClick={() => setShowApiKey(v => !v)}
                      aria-label={showApiKey ? 'Hide API Key' : 'Show API Key'}
                      style={{ position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)', background: 'transparent', border: 'none', cursor: 'pointer', color: T.accent, fontSize: 12, fontWeight: 600, padding: 2 }}>
                      {showApiKey ? 'Hide' : 'Show'}
                    </button>
                  </div>
                </div>
                <div>
                  <label style={labelStyle}>API Token <span style={{ color: T.red }}>*</span></label>
                  <div style={{ position: 'relative' }}>
                    <input style={{ ...inputStyle, paddingRight: '2.5rem' }} placeholder="API Token" type={showApiToken ? 'text' : 'password'}
                      autoComplete="new-password"
                      value={form.api_token} onChange={e => setField('api_token', e.target.value)} />
                    <button type="button" onClick={() => setShowApiToken(v => !v)}
                      aria-label={showApiToken ? 'Hide API Token' : 'Show API Token'}
                      style={{ position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)', background: 'transparent', border: 'none', cursor: 'pointer', color: T.accent, fontSize: 12, fontWeight: 600, padding: 2 }}>
                      {showApiToken ? 'Hide' : 'Show'}
                    </button>
                  </div>
                </div>
                <div>
                  <label style={labelStyle}>Account SID <span style={{ color: T.red }}>*</span></label>
                  <input style={inputStyle} placeholder="your_account_sid"
                    value={form.account_sid} onChange={e => setField('account_sid', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>Caller ID <span style={{ color: T.red }}>*</span></label>
                  <input style={inputStyle} placeholder="your_caller_id"
                    value={form.caller_id} onChange={e => setField('caller_id', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>App ID</label>
                  <input style={inputStyle} placeholder="your_app_id"
                    value={form.app_id} onChange={e => setField('app_id', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>App Type</label>
                  <AppSelect
                    value={form.app_type || 'exoml'}
                    onChange={value => setField('app_type', value)}
                    options={[
                      { value: 'exoml', label: 'Legacy ExoML (XML)' },
                      { value: 'voicebot', label: 'AgentStream Voicebot (JSON)' },
                    ]}
                  />
                  <div style={{ color: T.muted, fontSize: 11, marginTop: 4 }}>
                    Use AgentStream for modern Exotel Voicebot flows.
                  </div>
                </div>
                <div>
                  <label style={labelStyle}>Account Region</label>
                  <AppSelect
                    value={form.region || ''}
                    onChange={value => setField('region', value)}
                    options={[
                      { value: '', label: 'Global (api.exotel.com)' },
                      { value: 'in', label: 'India (api.in.exotel.com)' },
                      { value: 'us', label: 'US (api.us.exotel.com)' },
                      { value: 'sg', label: 'Singapore (api.sg.exotel.com)' },
                    ]}
                  />
                  <div style={{ color: T.muted, fontSize: 11, marginTop: 4 }}>
                    Pick the Exotel cluster for this account.
                  </div>
                </div>
                <div>
                  <label style={labelStyle}>Subdomain</label>
                  <input style={inputStyle} placeholder="e.g. arysoftglobalservices1m"
                    value={form.subdomain} onChange={e => setField('subdomain', e.target.value)} />
                  <div style={{ color: T.muted, fontSize: 11, marginTop: 4 }}>
                    Overrides region if set (subdomain.exotel.com).
                  </div>
                </div>
              </div>
            )}

            {/* Tata Tele fields */}
            {isTata && (
              <div data-tour="provider-account-tata-fields" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: '12px', marginBottom: '1rem' }}>
                <div>
                  <label style={labelStyle}>{isInbound ? 'API Token' : 'Click-to-Call Support API Key'} <span style={{ color: T.red }}>*</span></label>
                  <div style={{ position: 'relative' }}>
                    <input style={{ ...inputStyle, paddingRight: '2.5rem' }} placeholder={isInbound ? "Tata API token" : "Click-to-Call Support API key"} type={showApiKey ? 'text' : 'password'}
                      autoComplete="new-password"
                      value={form.api_key} onChange={e => setField('api_key', e.target.value)} />
                    <button type="button" onClick={() => setShowApiKey(v => !v)}
                      aria-label={showApiKey ? 'Hide credential' : 'Show credential'}
                      style={{ position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)', background: 'transparent', border: 'none', cursor: 'pointer', color: T.accent, fontSize: 12, fontWeight: 600, padding: 2 }}>
                      {showApiKey ? 'Hide' : 'Show'}
                    </button>
                  </div>
                </div>
                {!isInbound && (
                  <div>
                    <label style={labelStyle}>Smartflo API Token <span style={{ color: T.red }}>*</span></label>
                    <input style={inputStyle} type="password" autoComplete="new-password"
                      placeholder="Account authorization token"
                      value={form.api_token || ''} onChange={e => setField('api_token', e.target.value)} />
                    <div style={{ fontSize: 11, color: T.sub, marginTop: 4 }}>Use the account API token, separate from the Click-to-Call Support key.</div>
                    {editingId && form.api_key && form.api_key === form.api_token && (
                      <div role="alert" style={{ fontSize: 12, color: '#d97706', marginTop: 4 }}>These credentials match. Verify the Smartflo token; older accounts may have copied the support key here.</div>
                    )}
                  </div>
                )}
                <div>
                  <label style={labelStyle}>{isInbound ? 'Inbound DID' : 'Caller ID'} <span style={{ color: T.red }}>*</span></label>
                  <input style={inputStyle} placeholder="e.g. 919240232665"
                    value={form.caller_id} onChange={e => setField('caller_id', e.target.value)} />
                </div>
                {!isInbound && (
                  <div>
                    <label style={labelStyle}>Agent Number <span style={{ color: T.red }}>*</span></label>
                    <input style={inputStyle} placeholder="Smartflo agent/extension number"
                      value={form.app_id} onChange={e => setField('app_id', e.target.value)} />
                  </div>
                )}
                {!isInbound && (
                  <div>
                    <label style={labelStyle}>Click-to-Call Endpoint</label>
                    <input style={inputStyle} placeholder="Optional Tata API URL"
                      value={form.subdomain} onChange={e => setField('subdomain', e.target.value)} />
                    <div style={{ color: T.muted, fontSize: 11, marginTop: 4 }}>
                      Leave blank to use the default Smartflo endpoint.
                    </div>
                  </div>
                )}
                {isInbound && (
                  <div style={{ gridColumn: '1 / -1', color: T.muted, fontSize: 12, lineHeight: 1.5 }}>
                    This inbound account maps the Tata DID to this Callified organization. In Tata, route the DID to the Voice Streaming endpoint for live AI calls.
                  </div>
                )}
              </div>
            )}

            {/* Twilio fields */}
            {isTwilio && (
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: '12px', marginBottom: '1rem' }}>
                <div>
                  <label style={labelStyle}>Account SID <span style={{ color: T.red }}>*</span></label>
                  <input style={inputStyle} placeholder="ACxxxxxxxxxxxxxxxx"
                    value={form.account_sid} onChange={e => setField('account_sid', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>Auth Token <span style={{ color: T.red }}>*</span></label>
                  <div style={{ position: 'relative' }}>
                    <input style={{ ...inputStyle, paddingRight: '2.5rem' }} placeholder="Auth Token" type={showApiKey ? 'text' : 'password'}
                      autoComplete="new-password"
                      value={form.api_key} onChange={e => setField('api_key', e.target.value)} />
                    <button type="button" onClick={() => setShowApiKey(v => !v)}
                      aria-label={showApiKey ? 'Hide Auth Token' : 'Show Auth Token'}
                      style={{ position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)', background: 'transparent', border: 'none', cursor: 'pointer', color: T.muted, fontSize: 16, padding: 2 }}>
                      <AppIcon name={showApiKey ? 'hide' : 'show'} />
                    </button>
                  </div>
                </div>
                <div>
                  <label style={labelStyle}>API Key SID <span style={{ color: T.red }}>*</span></label>
                  <input style={inputStyle} placeholder="SKxxxxxxxxxxxxxxxx"
                    value={form.api_token} onChange={e => setField('api_token', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>API Secret <span style={{ color: T.red }}>*</span></label>
                  <input style={inputStyle} placeholder="API Secret" type="password"
                    autoComplete="new-password"
                    value={form.api_secret} onChange={e => setField('api_secret', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>Phone Number <span style={{ color: T.red }}>*</span></label>
                  <input style={inputStyle} placeholder="+1xxxxxxxxxx"
                    value={form.caller_id} onChange={e => setField('caller_id', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>TwiML App SID</label>
                  <input style={inputStyle} placeholder="APxxxxxxxxxxxxxxxx"
                    value={form.app_id} onChange={e => setField('app_id', e.target.value)} />
                </div>
              </div>
            )}

            {error && (
              <div style={{
                marginBottom: '1rem', padding: '8px 12px', borderRadius: 8,
                background: 'rgba(239,68,68,0.06)', border: `1px solid rgba(239,68,68,0.25)`,
                color: T.red, fontSize: '0.83rem',
              }}>{error}</div>
            )}
            <div style={{ display: 'flex', gap: '10px' }}>
              <button data-tour="provider-account-save" type="submit" disabled={saving}
                style={{
                  background: 'linear-gradient(135deg, #6366f1, #8b5cf6)',
                  border: 'none', borderRadius: 8, color: '#fff',
                  padding: '8px 20px', cursor: saving ? 'wait' : 'pointer',
                  fontSize: 13, fontWeight: 600, fontFamily: T.font,
                  opacity: saving ? 0.6 : 1,
                }}>
                {saving ? 'Saving…' : 'Save'}
              </button>
              <button type="button" onClick={closeForm}
                style={{
                  background: '#fff', border: `1px solid ${T.border}`,
                  borderRadius: 8, color: T.sub,
                  padding: '8px 16px', cursor: 'pointer',
                  fontSize: 13, fontFamily: T.font,
                }}>
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {secretAccount && <div style={{ position: 'fixed', inset: 0, background: 'rgba(15,23,42,0.45)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
        <section role="dialog" aria-modal="true" aria-labelledby="tata-secret-title" style={{ ...card, background: 'var(--surface-raised)', color: 'var(--text-primary)', border: '1px solid var(--border)', boxShadow: '0 24px 64px rgba(15,23,42,0.24)', fontFamily: T.font, fontSize: 14, lineHeight: 1.6, padding: 24, width: '100%', maxWidth: 640, maxHeight: '85vh', overflowY: 'auto' }}>
          <h3 id="tata-secret-title" style={{ marginTop: 0 }}>Stream secret — {secretAccount.name}</h3>
          <p>Generating a new secret replaces the previous one and blocks outbound calls until the new secret is verified.</p>
          <p>Save the new header in Tata, then run Tata’s endpoint test. An authenticated request verifies the secret; sample-number errors can still fail the routing test. Verification confirms the header, not the entire call routing.</p>
          <button type="button" onClick={checkStreamVerification}>Check verification</button>
          {streamVerified !== null && <p role="status">{streamVerified ? 'Current secret verified. Outbound dialing is enabled.' : 'Current secret not verified. Outbound dialing is blocked.'}</p>}
          {streamSetupError && <p role="alert" style={{ color: T.red }}>{streamSetupError}</p>}
          {streamSetup && <>
            <p>Copy this secret now; it is shown only once. In Tata Voice Bot, select Dynamic and POST.</p>
            <label style={{ ...labelStyle, color: 'var(--text-secondary)' }}>Dynamic URL</label><input style={{ ...inputStyle, background: 'var(--surface-muted)', color: 'var(--text-primary)', borderColor: 'var(--border)', fontFamily: T.mono }} readOnly value={streamSetup.url} />
            <label style={{ ...labelStyle, color: 'var(--text-secondary)', marginTop: 12 }}>Header: {streamSetup.header}</label><input aria-label="Stream secret" type={streamSetup.secret ? 'text' : 'password'} autoComplete="off" spellCheck={false} style={{ ...inputStyle, background: 'var(--surface-muted)', color: 'var(--text-primary)', borderColor: 'var(--border)', fontFamily: T.mono }} readOnly value={streamSetup.secret || '••••••••••••••••'} />
            {streamSetup.secret ? <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
              <button type="button" onClick={copyStreamSecret} className="uip-btn uip-btn-primary">Copy and hide</button>
              <button type="button" onClick={hideStreamSecret} className="uip-btn uip-btn-secondary">Hide</button>
            </div> : <p role="status">Secret hidden. Generate a new secret if you need another copy.</p>}
            <p>Body: callId = $callId, fromNumber = $fromNumber, toNumber = $toNumber, status = $status.</p>
          </>}
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10, marginTop: 20 }}>
            <button type="button" disabled={rotatingSecret} onClick={() => { setSecretAccount(null); setStreamSetup(null); setStreamSetupError(''); }} style={{ padding: '8px 16px', borderRadius: 6, border: '1px solid var(--border)', background: 'var(--surface-muted)', color: 'var(--text-primary)', fontFamily: T.font, cursor: 'pointer' }}>Close</button>
            <button type="button" disabled={rotatingSecret} onClick={() => rotateStreamSecret(secretAccount)} style={{ padding: '8px 16px', borderRadius: 6, border: 'none', background: T.accent, color: '#fff', fontFamily: T.font, cursor: 'pointer', fontWeight: 600 }}>{rotatingSecret ? 'Generating…' : streamSetup ? 'Regenerate secret' : 'Generate stream secret'}</button>
          </div>
        </section>
      </div>}
      {/* Accounts list */}
      <div data-tour="provider-account-list">
      {loading ? (
        <div style={{ color: T.muted, textAlign: 'center', padding: '2rem' }}>Loading…</div>
      ) : accounts.length === 0 ? (
        <div style={{
          ...card, color: T.muted, textAlign: 'center', padding: '3rem',
          border: `1px dashed ${T.border}`,
        }}>
          No provider accounts saved yet. Click <strong style={{ color: T.accent }}>+ Add Account</strong> to get started.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          {accounts.map((a, index) => {
            const badge = PROVIDER_BADGE[a.provider] || PROVIDER_BADGE.exotel;
            return (
              <div key={a.id} style={{ ...card, padding: '14px 18px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12 }}>
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                    <span style={{ color: T.text, fontWeight: 700, fontSize: '0.95rem' }}>{a.name}</span>
                    <span style={{
                      fontSize: 11, fontWeight: 700, padding: '2px 8px', borderRadius: 20,
                      background: badge.bg, color: badge.color, border: `1px solid ${badge.border}`,
                    }}>{badge.label}</span>
                    <span style={{
                      fontSize: 11, fontWeight: 700, padding: '2px 8px', borderRadius: 20,
                      background: (a.direction || 'outbound') === 'inbound' ? 'rgba(16,185,129,0.08)' : 'rgba(99,102,241,0.08)',
                      color: (a.direction || 'outbound') === 'inbound' ? '#059669' : T.accent,
                      border: `1px solid ${(a.direction || 'outbound') === 'inbound' ? 'rgba(16,185,129,0.25)' : 'rgba(99,102,241,0.25)'}`,
                    }}>{(a.direction || 'outbound') === 'inbound' ? 'Inbound' : 'Outbound'}</span>
                  </div>
                  {a.provider === 'tata' && (a.direction || 'outbound') !== 'inbound' && (!a.api_token || a.api_token === a.api_key) && (
                    <div role="status" style={{ color: '#d97706', fontSize: 12, marginTop: 6 }}>Review Smartflo credentials before outbound calling.</div>
                  )}
                  <div style={{ color: T.muted, fontSize: '0.78rem', display: 'flex', gap: '16px', flexWrap: 'wrap' }}>
                    {a.provider !== 'tata' && <span>Account: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.account_sid}</span></span>}
                    <span>{a.provider === 'tata' ? ((a.direction || 'outbound') === 'inbound' ? 'Inbound DID' : 'DID') : 'Caller'}: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.caller_id}</span></span>
                    {a.app_id && <span>App: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.app_id}</span></span>}
                    {a.provider === 'tata' && (a.direction || 'outbound') !== 'inbound' && a.app_id && <span>Agent: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.app_id}</span></span>}
                    {a.provider === 'exotel' && <span>Type: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.app_type || 'exoml'}</span></span>}
                    {a.provider === 'tata' && a.subdomain && <span>Endpoint: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.subdomain}</span></span>}
                    {a.provider === 'exotel' && (a.region || a.subdomain) && <span>Cluster: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.subdomain ? a.subdomain + '.exotel.com' : (a.region ? 'api.' + a.region + '.exotel.com' : 'api.exotel.com')}</span></span>}
                    <span>Key: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.api_key.slice(0, 8)}…</span></span>
                  </div>
                </div>
                <div style={{ display: 'flex', gap: '8px', flexShrink: 0 }}>
                  {a.provider === 'tata' && <button type="button" onClick={() => { setSecretAccount(a); setStreamVerified(null); setStreamSetup(null); setStreamSetupError(''); }} style={{ background: 'rgba(99,102,241,0.08)', border: '1px solid rgba(99,102,241,0.25)', borderRadius: 6, color: T.accent, padding: '5px 14px', cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font }}>Secret</button>}
                  <button data-tour={index === 0 ? 'provider-account-edit' : undefined} onClick={() => openEdit(a)}
                    style={{
                      background: 'rgba(99,102,241,0.08)', border: `1px solid rgba(99,102,241,0.25)`,
                      borderRadius: 6, color: T.accent, padding: '5px 14px',
                      cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                    }}>Edit</button>
                  <button onClick={() => handleDelete(a.id)}
                    style={{
                      background: 'rgba(239,68,68,0.06)', border: `1px solid rgba(239,68,68,0.2)`,
                      borderRadius: 6, color: T.red, padding: '5px 14px',
                      cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                    }}>Delete</button>
                </div>
              </div>
            );
          })}
        </div>
      )}
      </div>
    </div>
  );
}
