import React, { useEffect, useState } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { API_URL } from '../constants/api';

export default function ReceptionistInstructions({ orgId, active }) {
  const { apiFetch } = useAuth();
  const [settings, setSettings] = useState({ greeting: '', instructions: '' });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  const [loadFailed, setLoadFailed] = useState(false);
  useEffect(() => {
    let cancelled = false;
    setSettings({ greeting: '', instructions: '' });
    setMessage('');
    setLoading(true);
    setLoadFailed(false);
    if (!orgId) return () => { cancelled = true; };
    apiFetch(`${API_URL}/organizations/${orgId}/receptionist-settings`)
      .then(async res => {
        if (!res.ok) throw new Error('Could not load incoming instructions.');
        return res.json();
      })
      .then(data => { if (!cancelled) setSettings(data); })
      .catch(error => { if (!cancelled) { setMessage(error.message); setLoadFailed(true); } })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [orgId, apiFetch]);

  const save = async () => {
    setSaving(true);
    setMessage('');
    try {
      const res = await apiFetch(`${API_URL}/organizations/${orgId}/receptionist-settings`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(settings),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Could not save instructions.');
      setMessage('Saved. These instructions apply to new incoming calls.');
    } catch (error) { setMessage(error.message); }
    finally { setSaving(false); }
  };
  const fieldStyle = { display: 'block', width: '100%', boxSizing: 'border-box', marginTop: 8, padding: 12, border: '1px solid #d1d5db', borderRadius: 6, font: 'inherit', lineHeight: 1.6, resize: 'vertical' };
  return (
    <details style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 18 }}>
      <summary style={{ cursor: 'pointer', fontSize: 16, fontWeight: 700 }}>Incoming Call Instructions</summary>
      <p style={{ color: '#6b7280', fontSize: 13 }}>Set the greeting and tell the AI how to handle incoming phone calls and browser test calls. Include your business details, questions, offers and closing instructions.</p>
      <label style={{ display: 'block', fontWeight: 700 }}>Opening greeting
        <textarea value={settings.greeting} onChange={e => setSettings(prev => ({ ...prev, greeting: e.target.value }))} disabled={loading || saving || active || loadFailed} maxLength={4000} rows={3} placeholder="Enter the exact greeting in the language you want spoken." style={fieldStyle} />
      </label>
      <label style={{ display: 'block', marginTop: 16, fontWeight: 700 }}>AI instructions
        <textarea value={settings.instructions} onChange={e => setSettings(prev => ({ ...prev, instructions: e.target.value }))} disabled={loading || saving || active || loadFailed} maxLength={20000} rows={14} placeholder="Describe your business, conversation steps, information to collect, answers to questions, active offers and how to end the call." style={fieldStyle} />
      </label>
      <button onClick={save} disabled={!orgId || loading || saving || active || loadFailed || !settings.greeting.trim() || !settings.instructions.trim()} style={{ marginTop: 14, padding: '9px 16px', border: 0, borderRadius: 6, background: '#2563eb', color: '#fff', fontWeight: 700 }}>{loading ? 'Loading...' : saving ? 'Saving...' : 'Save Instructions'}</button>
      {message && <p role="status" style={{ fontSize: 13 }}>{message}</p>}
    </details>
  );
}
