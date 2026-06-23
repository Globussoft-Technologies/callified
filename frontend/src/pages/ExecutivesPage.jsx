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

const EMPTY_FORM = { name: '', email: '', phone: '' };

export default function ExecutivesPage() {
  const { apiFetch } = useAuth();
  const [executives, setExecutives] = useState([]);
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState(EMPTY_FORM);
  const [editingId, setEditingId] = useState(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [showForm, setShowForm] = useState(false);

  const fetchExecutives = useCallback(async () => {
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/executives`);
      const data = await res.json();
      setExecutives(Array.isArray(data) ? data : []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [apiFetch]);

  useEffect(() => { fetchExecutives(); }, [fetchExecutives]);

  const openAdd = () => { setForm(EMPTY_FORM); setEditingId(null); setError(''); setShowForm(true); };
  const openEdit = (e) => {
    setForm({ name: e.name || '', email: e.email || '', phone: e.phone || '' });
    setEditingId(e.id);
    setError('');
    setShowForm(true);
  };
  const closeForm = () => { setShowForm(false); setEditingId(null); setForm(EMPTY_FORM); setError(''); };

  const setField = (key, val) => setForm(f => ({ ...f, [key]: val }));

  const handleSave = async (e) => {
    e.preventDefault();
    if (!form.name.trim()) { setError('Executive name is required.'); return; }
    setSaving(true);
    setError('');
    try {
      const method = editingId ? 'PUT' : 'POST';
      const url = editingId ? `${API_URL}/executives/${editingId}` : `${API_URL}/executives`;
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
      fetchExecutives();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id) => {
    if (!window.confirm('Delete this executive? Leads assigned to them will become unassigned.')) return;
    try {
      await apiFetch(`${API_URL}/executives/${id}`, { method: 'DELETE' });
      fetchExecutives();
    } catch (e) {
      console.error(e);
    }
  };

  return (
    <div style={{ padding: '28px 32px', maxWidth: 900, margin: '0 auto', fontFamily: T.font, background: T.bg, minHeight: '100%' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '1.5rem' }}>
        <div>
          <h2 style={{ margin: 0, color: T.text, fontSize: '1.4rem', fontWeight: 700, display: 'flex', alignItems: 'center', gap: 8 }}>
            🧑‍💼 Executives
          </h2>
          <p style={{ margin: '4px 0 0', color: T.muted, fontSize: '0.85rem' }}>
            Create sales/ops executives and assign them to campaigns and leads.
          </p>
        </div>
        <button onClick={openAdd}
          style={{
            background: 'linear-gradient(135deg, #6366f1, #8b5cf6)',
            border: 'none', borderRadius: 8, color: '#fff',
            padding: '9px 18px', cursor: 'pointer', fontSize: 13,
            fontWeight: 600, fontFamily: T.font, whiteSpace: 'nowrap',
          }}>
          + Add Executive
        </button>
      </div>

      {showForm && (
        <div style={{ ...card, padding: '1.5rem', marginBottom: '1.5rem' }}>
          <h3 style={{ margin: '0 0 1.2rem', color: T.text, fontSize: '1rem', fontWeight: 700 }}>
            {editingId ? 'Edit Executive' : 'Add New Executive'}
          </h3>
          <form onSubmit={handleSave}>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: '12px', marginBottom: '1rem' }}>
              <div>
                <label style={labelStyle}>Name <span style={{ color: T.red }}>*</span></label>
                <input style={inputStyle} placeholder="e.g. Rahul Sharma"
                  value={form.name} onChange={e => setField('name', e.target.value)} />
              </div>
              <div>
                <label style={labelStyle}>Email</label>
                <input style={inputStyle} placeholder="rahul@company.com" type="email"
                  value={form.email} onChange={e => setField('email', e.target.value)} />
              </div>
              <div>
                <label style={labelStyle}>Phone</label>
                <input style={inputStyle} placeholder="10-digit mobile"
                  value={form.phone} onChange={e => setField('phone', e.target.value.replace(/\D/g, '').slice(0, 10))} />
              </div>
            </div>
            {error && (
              <div style={{ marginBottom: '1rem', padding: '10px 14px', borderRadius: 8,
                background: '#fee2e2', border: '1px solid #fca5a5', color: T.red, fontSize: '0.85rem' }}>
                {error}
              </div>
            )}
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button type="button" onClick={closeForm}
                style={{ background: '#fff', border: `1px solid ${T.border}`, color: T.sub, padding: '8px 16px', borderRadius: 8, cursor: 'pointer', fontWeight: 600 }}>
                Cancel
              </button>
              <button type="submit" className="btn-primary" disabled={saving}
                style={{ background: T.accent, border: 'none', color: '#fff', padding: '8px 18px', borderRadius: 8, cursor: 'pointer', fontWeight: 600 }}>
                {saving ? 'Saving...' : (editingId ? 'Save Changes' : 'Add Executive')}
              </button>
            </div>
          </form>
        </div>
      )}

      <div style={{ ...card, overflow: 'hidden' }}>
        {loading ? (
          <div style={{ padding: '2rem', textAlign: 'center', color: T.muted }}>Loading...</div>
        ) : executives.length === 0 ? (
          <div style={{ padding: '2rem', textAlign: 'center', color: T.muted }}>
            No executives yet. Add one to start assigning leads.
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: '#f9fafb' }}>
                {['Name', 'Email', 'Phone', 'Actions'].map(h => (
                  <th key={h} style={{ padding: '12px 16px', textAlign: 'left', fontSize: 12, fontWeight: 700, color: T.sub, borderBottom: `1px solid ${T.border}` }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {executives.map(e => (
                <tr key={e.id} style={{ borderBottom: `1px solid ${T.border}` }}>
                  <td style={{ padding: '12px 16px', fontWeight: 600, color: T.text, fontSize: 13 }}>{e.name}</td>
                  <td style={{ padding: '12px 16px', color: T.sub, fontSize: 13 }}>{e.email || '-'}</td>
                  <td style={{ padding: '12px 16px', color: T.sub, fontSize: 13, fontFamily: T.mono }}>{e.phone || '-'}</td>
                  <td style={{ padding: '12px 16px' }}>
                    <div style={{ display: 'flex', gap: 8 }}>
                      <button onClick={() => openEdit(e)}
                        style={{ fontSize: 12, padding: '5px 12px', cursor: 'pointer', background: 'rgba(245,158,11,0.08)', color: '#92400e', border: '1px solid rgba(245,158,11,0.25)', borderRadius: 6, fontWeight: 600 }}>
                        Edit
                      </button>
                      <button onClick={() => handleDelete(e.id)}
                        style={{ fontSize: 12, padding: '5px 12px', cursor: 'pointer', background: '#fee2e2', color: T.red, border: '1px solid #fca5a5', borderRadius: 6, fontWeight: 600 }}>
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
