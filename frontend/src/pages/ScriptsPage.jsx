import React, { useEffect, useMemo, useState } from 'react';
import { useToast } from '../contexts/UIContext';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', cyan: '#0891b2', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const inputStyle = {
  width: '100%', padding: '9px 13px', borderRadius: 8, fontSize: 13,
  border: `1px solid ${T.border}`, background: T.card,
  color: T.text, fontFamily: T.font, outline: 'none', boxSizing: 'border-box',
};

const labelStyle = {
  display: 'block', marginBottom: 6, fontSize: 12, fontWeight: 600,
  color: T.muted, textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: T.font,
};

const LANGUAGES = [
  { code: 'en', label: 'English' },
  { code: 'hi', label: 'Hindi' },
  { code: 'mr', label: 'Marathi' },
  { code: 'bn', label: 'Bengali' },
  { code: 'gu', label: 'Gujarati' },
  { code: 'pa', label: 'Punjabi' },
  { code: 'ta', label: 'Tamil' },
  { code: 'te', label: 'Telugu' },
  { code: 'kn', label: 'Kannada' },
  { code: 'ml', label: 'Malayalam' },
];

export default function ScriptsPage({ apiFetch, API_URL }) {
  const toast = useToast();
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState({ name: '', language: 'en', template_type: 'voice', script_body: '' });

  const fetchTemplates = async () => {
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/templates`);
      const data = await res.json();
      setTemplates(data.templates || []);
    } catch (e) {
      toast('Failed to load scripts');
    }
    setLoading(false);
  };

  useEffect(() => { fetchTemplates(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const grouped = useMemo(() => {
    const map = {};
    templates.forEach(t => {
      const key = `${t.name}|${t.language}`;
      if (!map[key] || t.version > map[key].version) map[key] = t;
    });
    return Object.values(map).sort((a, b) => a.name.localeCompare(b.name));
  }, [templates]);

  const handleSave = async () => {
    if (!form.name.trim() || !form.script_body.trim()) {
      toast('Name and script body are required');
      return;
    }
    try {
      const url = editing ? `${API_URL}/templates/${editing.id}` : `${API_URL}/templates`;
      const method = editing ? 'PUT' : 'POST';
      const res = await apiFetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      });
      if (!res.ok) throw new Error((await res.text()) || 'save failed');
      toast(editing ? 'Script updated' : 'Script created');
      setEditing(null);
      setForm({ name: '', language: 'en', template_type: 'voice', script_body: '' });
      fetchTemplates();
    } catch (e) {
      toast('Save failed: ' + e.message);
    }
  };

  const startEdit = (t) => {
    setEditing(t);
    setForm({ name: t.name, language: t.language, template_type: t.template_type, script_body: t.script_body });
  };

  const seedPanora = async () => {
    try {
      const res = await apiFetch(`${API_URL}/templates/seed-panora`, { method: 'POST' });
      if (!res.ok) throw new Error(await res.text());
      toast('Panora templates seeded');
      fetchTemplates();
    } catch (e) {
      toast('Seed failed: ' + e.message);
    }
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
          <span style={{ color: T.cyan }}>Call</span> Scripts
        </h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Create reusable AI voice scripts. Products and campaigns can pick a script to override the default prompt.
        </p>
      </div>

      <div style={{ ...card, padding: '24px 28px', marginBottom: 16 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
          <h3 style={{ margin: 0, fontSize: 15, fontWeight: 700 }}>{editing ? 'Edit Script' : 'New Script'}</h3>
          <button onClick={seedPanora} style={{
            padding: '8px 14px', borderRadius: 8, border: 'none',
            background: T.amber, color: '#fff', fontWeight: 600, fontSize: 13,
            cursor: 'pointer', fontFamily: T.font,
          }}>✨ Seed Panora Templates</button>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 140px', gap: 12, marginBottom: 12 }}>
          <div>
            <label style={labelStyle}>Name</label>
            <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} style={inputStyle} placeholder="e.g. Wholesale Qualification" />
          </div>
          <div>
            <label style={labelStyle}>Language</label>
            <select value={form.language} onChange={e => setForm({ ...form, language: e.target.value })} style={inputStyle}>
              {LANGUAGES.map(l => <option key={l.code} value={l.code}>{l.label}</option>)}
            </select>
          </div>
          <div>
            <label style={labelStyle}>Type</label>
            <select value={form.template_type} onChange={e => setForm({ ...form, template_type: e.target.value })} style={inputStyle}>
              <option value="voice">Voice</option>
              <option value="whatsapp">WhatsApp</option>
            </select>
          </div>
        </div>
        <div style={{ marginBottom: 12 }}>
          <label style={labelStyle}>Script Body</label>
          <textarea
            value={form.script_body}
            onChange={e => setForm({ ...form, script_body: e.target.value })}
            rows={10}
            style={{ ...inputStyle, resize: 'vertical', minHeight: 200, lineHeight: 1.6, fontFamily: T.mono }}
            placeholder="[LANG:en]&#10;&#10;You are {{PersonaName}}...&#10;&#10;Available variables: {{CompanyName}}, {{ProductName}}, {{PersonaName}}, {{LeadName}}, {{Language}}, {{CampaignName}}"
          />
        </div>
        <div style={{ display: 'flex', gap: 10 }}>
          <button onClick={handleSave} style={{
            padding: '9px 18px', borderRadius: 8, border: 'none',
            background: T.green, color: '#fff', fontWeight: 600, fontSize: 13,
            cursor: 'pointer', fontFamily: T.font,
          }}>{editing ? '💾 Save New Version' : '➕ Create Script'}</button>
          {editing && (
            <button onClick={() => { setEditing(null); setForm({ name: '', language: 'en', template_type: 'voice', script_body: '' }); }} style={{
              padding: '9px 14px', borderRadius: 8, border: `1px solid ${T.border}`,
              background: T.card, color: T.muted, fontWeight: 600, fontSize: 13,
              cursor: 'pointer', fontFamily: T.font,
            }}>Cancel</button>
          )}
        </div>
      </div>

      <div style={{ ...card, padding: '24px 28px' }}>
        <h3 style={{ margin: '0 0 16px', fontSize: 15, fontWeight: 700 }}>Existing Scripts</h3>
        {loading ? (
          <div style={{ color: T.muted }}>Loading...</div>
        ) : grouped.length === 0 ? (
          <div style={{ color: T.muted }}>No scripts yet. Create one above or seed Panora templates.</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {grouped.map(t => (
              <div key={t.id} style={{ padding: '12px 16px', border: `1px solid ${T.border}`, borderRadius: 8, background: T.bg }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div>
                    <div style={{ fontWeight: 700, fontSize: 14, color: T.text }}>{t.name}</div>
                    <div style={{ fontSize: 12, color: T.muted, marginTop: 2 }}>
                      {t.language} · {t.template_type} · v{t.version} · {t.org_id ? 'Org' : 'Global'}
                    </div>
                  </div>
                  <button onClick={() => startEdit(t)} style={{
                    padding: '6px 12px', borderRadius: 6, border: `1px solid ${T.border}`,
                    background: T.card, color: T.cyan, fontWeight: 600, fontSize: 12,
                    cursor: 'pointer', fontFamily: T.font,
                  }}>✏️ Edit</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
