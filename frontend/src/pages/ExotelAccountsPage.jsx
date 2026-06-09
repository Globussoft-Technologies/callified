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

const EMPTY_FORM = { name: '', api_key: '', api_token: '', account_sid: '', caller_id: '', app_id: '' };

export default function ExotelAccountsPage() {
  const { apiFetch } = useAuth();
  const [accounts, setAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState(EMPTY_FORM);
  const [editingId, setEditingId] = useState(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [showForm, setShowForm] = useState(false);

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

  const openAdd = () => { setForm(EMPTY_FORM); setEditingId(null); setError(''); setShowForm(true); };
  const openEdit = (a) => {
    setForm({ name: a.name, api_key: a.api_key, api_token: a.api_token,
              account_sid: a.account_sid, caller_id: a.caller_id, app_id: a.app_id || '' });
    setEditingId(a.id);
    setError('');
    setShowForm(true);
  };
  const closeForm = () => { setShowForm(false); setEditingId(null); setForm(EMPTY_FORM); setError(''); };

  const handleSave = async (e) => {
    e.preventDefault();
    if (!form.name.trim() || !form.api_key || !form.api_token || !form.account_sid || !form.caller_id) {
      setError('Name, API Key, API Token, Account SID and Caller ID are required.');
      return;
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
    if (!window.confirm('Delete this Exotel account?')) return;
    try {
      await apiFetch(`${API_URL}/exotel-accounts/${id}`, { method: 'DELETE' });
      fetchAccounts();
    } catch (e) {
      console.error(e);
    }
  };

  return (
    <div style={{ padding: '28px 32px', maxWidth: 900, margin: '0 auto', fontFamily: T.font, background: T.bg, minHeight: '100%' }}>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '1.5rem' }}>
        <div>
          <h2 style={{ margin: 0, color: T.text, fontSize: '1.4rem', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>
            📞 Exotel Accounts
          </h2>
          <p style={{ margin: '4px 0 0', color: T.muted, fontSize: '0.85rem' }}>
            Save multiple Exotel credentials and select one per campaign.
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
            <div style={{ marginBottom: '1rem' }}>
              <label style={labelStyle}>Account Name <span style={{ color: T.red }}>*</span></label>
              <input style={inputStyle} placeholder="e.g. Main Caller, Sales India"
                value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: '12px', marginBottom: '1rem' }}>
              <div>
                <label style={labelStyle}>API Key <span style={{ color: T.red }}>*</span></label>
                <input style={inputStyle} placeholder="API Key" type="password"
                  value={form.api_key} onChange={e => setForm({ ...form, api_key: e.target.value })} />
              </div>
              <div>
                <label style={labelStyle}>API Token <span style={{ color: T.red }}>*</span></label>
                <input style={inputStyle} placeholder="API Token" type="password"
                  value={form.api_token} onChange={e => setForm({ ...form, api_token: e.target.value })} />
              </div>
              <div>
                <label style={labelStyle}>Account SID <span style={{ color: T.red }}>*</span></label>
                <input style={inputStyle} placeholder="e.g. globussoft3"
                  value={form.account_sid} onChange={e => setForm({ ...form, account_sid: e.target.value })} />
              </div>
              <div>
                <label style={labelStyle}>Caller ID <span style={{ color: T.red }}>*</span></label>
                <input style={inputStyle} placeholder="e.g. 09513886363"
                  value={form.caller_id} onChange={e => setForm({ ...form, caller_id: e.target.value })} />
              </div>
              <div>
                <label style={labelStyle}>App ID</label>
                <input style={inputStyle} placeholder="e.g. 1244808"
                  value={form.app_id} onChange={e => setForm({ ...form, app_id: e.target.value })} />
              </div>
            </div>
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
          No Exotel accounts saved yet. Click <strong style={{ color: T.accent }}>+ Add Account</strong> to get started.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          {accounts.map(a => (
            <div key={a.id} style={{ ...card, padding: '14px 18px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12 }}>
              <div>
                <div style={{ color: T.text, fontWeight: 700, fontSize: '0.95rem', marginBottom: 4 }}>
                  {a.name}
                </div>
                <div style={{ color: T.muted, fontSize: '0.78rem', display: 'flex', gap: '16px', flexWrap: 'wrap' }}>
                  <span>Account: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.account_sid}</span></span>
                  <span>Caller: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.caller_id}</span></span>
                  {a.app_id && <span>App: <span style={{ color: T.sub, fontFamily: T.mono }}>{a.app_id}</span></span>}
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
          ))}
        </div>
      )}
    </div>
  );
}
