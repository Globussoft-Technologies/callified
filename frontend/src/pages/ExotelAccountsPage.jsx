import React, { useState, useEffect, useCallback } from 'react';
import { API_URL } from '../constants/api';
import { useAuth } from '../contexts/AuthContext';

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
  name: '',
  // Exotel fields
  api_key: '', api_token: '', account_sid: '', caller_id: '', app_id: '', app_type: 'exoml',
  // Twilio-only
  api_secret: '',
};

const PROVIDER_BADGE = {
  exotel: { label: 'Exotel', bg: 'rgba(99,102,241,0.08)', color: '#6366f1', border: 'rgba(99,102,241,0.25)' },
  twilio: { label: 'Twilio', bg: 'rgba(239,68,68,0.06)', color: '#ef4444', border: 'rgba(239,68,68,0.2)' },
};

export default function ExotelAccountsPage() {
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

  const openAdd = () => { setForm(EMPTY_FORM); setEditingId(null); setError(''); setShowForm(true); setShowApiKey(false); setShowApiToken(false); };
  const openEdit = (a) => {
    setForm({
      provider: a.provider || 'exotel',
      name: a.name,
      api_key: a.api_key,
      api_token: a.api_token,
      api_secret: a.api_secret || '',
      account_sid: a.account_sid,
      caller_id: a.caller_id,
      app_id: a.app_id || '',
      app_type: a.app_type || 'exoml',
    });
    setEditingId(a.id);
    setError('');
    setShowForm(true);
    setShowApiKey(false);
    setShowApiToken(false);
  };
  const closeForm = () => { setShowForm(false); setEditingId(null); setForm(EMPTY_FORM); setError(''); setShowApiKey(false); setShowApiToken(false); };

  const setField = (key, val) => setForm(f => ({ ...f, [key]: val }));

  const handleSave = async (e) => {
    e.preventDefault();
    const { provider, name, api_key, api_token, api_secret, account_sid, caller_id } = form;
    if (!name.trim()) { setError('Account name is required.'); return; }
    if (provider === 'twilio') {
      if (!account_sid || !api_key || !api_token || !api_secret || !caller_id) {
        setError('Account SID, Auth Token, API Key SID, API Secret and Phone Number are required for Twilio.');
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
      const method = editingId ? 'PUT' : 'POST';
      const url = editingId
        ? `${API_URL}/exotel-accounts/${editingId}`
        : `${API_URL}/exotel-accounts`;
      const res = await apiFetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
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
    if (!window.confirm('Delete this provider account?')) return;
    try {
      await apiFetch(`${API_URL}/exotel-accounts/${id}`, { method: 'DELETE' });
      fetchAccounts();
    } catch (e) {
      console.error(e);
    }
  };

  const isExotel = form.provider === 'exotel';

  return (
    <div style={{ padding: '28px 32px', maxWidth: 900, margin: '0 auto', fontFamily: T.font, background: T.bg, minHeight: '100%' }}>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '1.5rem' }}>
        <div>
          <h2 style={{ margin: 0, color: T.text, fontSize: '1.4rem', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>
            📞 Provider Accounts
          </h2>
          <p style={{ margin: '4px 0 0', color: T.muted, fontSize: '0.85rem' }}>
            Save Exotel credentials and select one per campaign.
          </p>
        </div>
        <button onClick={openAdd}
          style={{
            background: 'linear-gradient(135deg, #6366f1, #8b5cf6)',
            border: 'none', borderRadius: 8, color: '#fff',
            padding: '9px 18px', cursor: 'pointer', fontSize: 13,
            fontWeight: 600, fontFamily: T.font, whiteSpace: 'nowrap',
          }}>
          + Add Account
        </button>
      </div>

      {/* Add / Edit Form */}
      {showForm && (
        <div style={{ ...card, padding: '1.5rem', marginBottom: '1.5rem' }}>
          <h3 style={{ margin: '0 0 1.2rem', color: T.text, fontSize: '1rem', fontWeight: 700 }}>
            {editingId ? 'Edit Account' : 'Add New Account'}
          </h3>
          <form onSubmit={handleSave}>

            {/* Provider selector */}
            <div style={{ marginBottom: '1.2rem' }}>
              <label style={labelStyle}>Provider <span style={{ color: T.red }}>*</span></label>
              <div style={{ display: 'flex', gap: 8 }}>
                {['exotel' /* , 'twilio' */].map(p => (
                  <button key={p} type="button"
                    onClick={() => setField('provider', p)}
                    style={{
                      padding: '7px 22px', borderRadius: 8, fontSize: 13, fontWeight: 600,
                      fontFamily: T.font, cursor: 'pointer',
                      border: form.provider === p ? `2px solid ${T.accent}` : `1px solid ${T.border}`,
                      background: form.provider === p ? 'rgba(99,102,241,0.08)' : '#fff',
                      color: form.provider === p ? T.accent : T.sub,
                    }}>
                    {p === 'exotel' ? 'Exotel' : 'Twilio'}
                  </button>
                ))}
              </div>
            </div>

            {/* Account Name */}
            <div style={{ marginBottom: '1rem' }}>
              <label style={labelStyle}>Account Name <span style={{ color: T.red }}>*</span></label>
              <input style={inputStyle} placeholder="e.g. Main Caller, Sales India"
                value={form.name} onChange={e => setField('name', e.target.value)} />
            </div>

            {/* Exotel fields */}
            {isExotel && (
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: '12px', marginBottom: '1rem' }}>
                <div>
                  <label style={labelStyle}>API Key <span style={{ color: T.red }}>*</span></label>
                  <div style={{ position: 'relative' }}>
                    <input style={{ ...inputStyle, paddingRight: '2.5rem' }} placeholder="API Key" type={showApiKey ? 'text' : 'password'}
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
                  <input style={inputStyle} placeholder="e.g. globussoft3"
                    value={form.account_sid} onChange={e => setField('account_sid', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>Caller ID <span style={{ color: T.red }}>*</span></label>
                  <input style={inputStyle} placeholder="e.g. 09513886363"
                    value={form.caller_id} onChange={e => setField('caller_id', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>App ID</label>
                  <input style={inputStyle} placeholder="e.g. 1244808"
                    value={form.app_id} onChange={e => setField('app_id', e.target.value)} />
                </div>
                <div>
                  <label style={labelStyle}>App Type</label>
                  <select
                    style={inputStyle}
                    value={form.app_type || 'exoml'}
                    onChange={e => setField('app_type', e.target.value)}
                  >
                    <option value="exoml">Legacy ExoML (XML)</option>
                    <option value="voicebot">AgentStream Voicebot (JSON)</option>
                  </select>
                  <div style={{ color: T.muted, fontSize: 11, marginTop: 4 }}>
                    Use AgentStream for modern Exotel Voicebot flows.
                  </div>
                </div>
              </div>
            )}

            {/* Twilio fields */}
            {!isExotel && (
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
                      value={form.api_key} onChange={e => setField('api_key', e.target.value)} />
                    <button type="button" onClick={() => setShowApiKey(v => !v)}
                      aria-label={showApiKey ? 'Hide Auth Token' : 'Show Auth Token'}
                      style={{ position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)', background: 'transparent', border: 'none', cursor: 'pointer', color: T.muted, fontSize: 16, padding: 2 }}>
                      {showApiKey ? '🙈' : '👁️'}
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
              <button type="submit" disabled={saving}
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

      {/* Accounts list */}
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
          {accounts.map(a => {
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
                  </div>
                  <div style={{ color: T.muted, fontSize: '0.78rem', display: 'flex', gap: '16px', flexWrap: 'wrap' }}>
                    <span>Account: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.account_sid}</span></span>
                    <span>Caller: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.caller_id}</span></span>
                    {a.app_id && <span>App: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.app_id}</span></span>}
                    {a.provider !== 'twilio' && <span>Type: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.app_type || 'exoml'}</span></span>}
                    <span>Key: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.api_key.slice(0, 8)}…</span></span>
                  </div>
                </div>
                <div style={{ display: 'flex', gap: '8px', flexShrink: 0 }}>
                  <button onClick={() => openEdit(a)}
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
  );
}
