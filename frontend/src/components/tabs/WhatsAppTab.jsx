import React, { useState, useEffect, useRef, useCallback } from 'react';
import { formatDateTime, formatTime } from '../../utils/dateFormat';

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
  meta: [
    { key: 'access_token', label: 'Access Token', type: 'password' },
    { key: 'phone_number_id', label: 'Phone Number ID', type: 'text' },
    { key: 'app_secret', label: 'App Secret', type: 'password' },
    { key: 'verify_token', label: 'Verify Token', type: 'text' },
  ],
  wasender: [
    { key: 'api_key', label: 'API Key', type: 'password' },
    { key: 'base_url', label: 'Base URL (optional)', type: 'text' },
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
          position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)',
          background: 'none', border: 'none', color: T.accent,
          cursor: 'pointer', fontSize: 12, fontWeight: 600, padding: '2px 6px', fontFamily: T.font,
        }}>
        {reveal ? 'Hide' : 'Show'}
      </button>
    </div>
  );
}

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

  const fields = PROVIDER_FIELDS[provider] || [];
  const missingField = fields.find(f => !(creds[f.key] || '').trim());
  const canSave = !saving && fields.length > 0 && !missingField;

  const handleSave = async () => {
    setError('');
    if (missingField) { setError(`${missingField.label} is required`); return; }
    setSaving(true);
    try {
      const res = await apiFetch(`${API_URL}/wa/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, credentials: creds, default_product_id: defaultProduct || null, auto_reply: autoReply }),
      });
      if (!res.ok) {
        let msg = `Save failed (HTTP ${res.status})`;
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch (_) {}
        setError(msg); setSaving(false); return;
      }
      onClose();
    } catch (e) { setError('Network error — could not reach server'); }
    setSaving(false);
  };

  if (!show) return null;

  const webhookUrl = `https://test.callified.ai/wa/webhook/${provider}`;
  const labelSt = { display: 'block', color: T.sub, fontSize: 12, fontWeight: 600, marginBottom: 4, marginTop: 12, fontFamily: T.font };

  return (
    <div style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }} onClick={onClose}>
      <div style={{
        background: T.card, border: `1px solid ${T.border}`, borderRadius: 16,
        boxShadow: '0 8px 32px rgba(0,0,0,0.12)', padding: '28px 32px',
        width: 480, maxHeight: '85vh', overflowY: 'auto', overflowX: 'hidden',
        boxSizing: 'border-box', fontFamily: T.font,
      }} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
          <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: T.text }}>WhatsApp Channel Config</h3>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: T.muted, fontSize: 22, cursor: 'pointer', padding: '0 4px' }}>&times;</button>
        </div>

        {error && (
          <div style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.25)', borderRadius: 8, padding: '10px 14px', marginBottom: 16, color: T.red, fontSize: 13 }}>
            {error}
          </div>
        )}

        <label style={labelSt}>Provider</label>
        <select value={provider} onChange={e => { setProvider(e.target.value); setCreds({}); setError(''); }}
          style={{ ...inputBase, cursor: 'pointer', appearance: 'auto', WebkitAppearance: 'menulist' }}>
          {PROVIDERS.map(p => <option key={p.value} value={p.value}>{p.label}</option>)}
        </select>

        {fields.map(f => (
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

        <label style={labelSt}>Default Product</label>
        <select value={defaultProduct} onChange={e => setDefaultProduct(e.target.value)}
          style={{ ...inputBase, cursor: 'pointer', appearance: 'auto', WebkitAppearance: 'menulist' }}>
          <option value="">— None —</option>
          {(orgProducts || []).map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>

        <div style={{ display: 'flex', alignItems: 'center', gap: 10, margin: '16px 0' }}>
          <label style={{ ...labelSt, margin: 0 }}>Auto-Reply</label>
          <button onClick={() => setAutoReply(!autoReply)}
            style={{ border: 'none', borderRadius: 12, cursor: 'pointer', padding: '4px 14px', fontSize: 12, fontWeight: 700, color: '#fff', fontFamily: T.font, background: autoReply ? T.wa : T.muted }}>
            {autoReply ? 'ON' : 'OFF'}
          </button>
        </div>

        <div style={{ background: 'rgba(37,211,102,0.06)', border: '1px solid rgba(37,211,102,0.2)', borderRadius: 8, padding: 12, marginBottom: 16 }}>
          <label style={{ ...labelSt, marginTop: 0, fontSize: 11, color: T.wa }}>Webhook URL — configure in your provider dashboard</label>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <code style={{ flex: 1, color: T.sub, fontSize: 12, wordBreak: 'break-all', fontFamily: T.mono }}>{webhookUrl}</code>
            <button onClick={() => navigator.clipboard.writeText(webhookUrl)}
              style={{ border: '1px solid rgba(37,211,102,0.3)', borderRadius: 6, background: 'rgba(37,211,102,0.1)', color: T.wa, padding: '4px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
              Copy
            </button>
          </div>
        </div>

        <button onClick={handleSave} disabled={!canSave}
          title={missingField ? `${missingField.label} is required` : ''}
          style={{
            width: '100%', border: 'none', borderRadius: 10, padding: '12px 18px',
            background: T.wa, color: '#fff', fontWeight: 700, fontSize: 14, fontFamily: T.font,
            opacity: canSave ? 1 : 0.5, cursor: canSave ? 'pointer' : 'not-allowed',
          }}>
          {saving ? 'Saving...' : 'Save Configuration'}
        </button>
      </div>
    </div>
  );
}

export default function WhatsAppTab({ apiFetch, API_URL, orgProducts, selectedOrg, orgTimezone }) {
  const [conversations, setConversations] = useState([]);
  const [selectedPhone, setSelectedPhone] = useState(null);
  const [messages, setMessages] = useState([]);
  const [search, setSearch] = useState('');
  const [messageText, setMessageText] = useState('');
  const [sending, setSending] = useState(false);
  const [showConfig, setShowConfig] = useState(false);
  const [aiEnabled, setAiEnabled] = useState({});
  const messagesEndRef = useRef(null);
  const pollRef = useRef(null);

  const fetchConversations = useCallback(async () => {
    try {
      const res = await apiFetch(`${API_URL}/wa/conversations`);
      if (res.ok) {
        const data = await res.json();
        const convos = Array.isArray(data) ? data : (data.conversations || []);
        setConversations(convos);
        const map = {};
        convos.forEach(c => { map[c.phone || c.contact_phone] = c.ai_active !== false; });
        setAiEnabled(map);
      }
    } catch (e) { console.error('Failed to fetch conversations', e); }
  }, [apiFetch, API_URL]);

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

  const filtered = conversations.filter(c => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (c.name || '').toLowerCase().includes(q) || (c.phone || '').includes(q);
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
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '14px 16px', borderBottom: `1px solid ${T.border}` }}>
          <h3 style={{ margin: 0, color: T.wa, fontSize: 14, fontWeight: 700, display: 'flex', alignItems: 'center', gap: 6 }}>
            💬 WhatsApp Inbox
          </h3>
          <button onClick={() => setShowConfig(true)}
            title="Channel Configuration"
            style={{ background: T.bg, border: `1px solid ${T.border}`, borderRadius: 8, color: T.muted, padding: '4px 8px', cursor: 'pointer', fontSize: 14 }}>
            ⚙️
          </button>
        </div>

        {/* Search */}
        <div style={{ padding: '8px 12px', borderBottom: `1px solid ${T.border}` }}>
          <input type="text" placeholder="Search by name or phone..." value={search}
            onChange={e => setSearch(e.target.value)}
            style={{ ...inputBase, fontSize: 12, padding: '7px 10px' }} />
        </div>

        {/* Conversations */}
        <div style={{ flex: 1, overflowY: 'auto' }}>
          {filtered.length === 0 ? (
            <div style={{ padding: '2rem 1rem', textAlign: 'center', color: T.muted, fontSize: 13 }}>
              No WhatsApp conversations yet
            </div>
          ) : filtered.map(conv => (
            <div key={conv.phone} onClick={() => setSelectedPhone(conv.phone)}
              style={{
                padding: '10px 14px', cursor: 'pointer', borderBottom: `1px solid ${T.border}`,
                background: selectedPhone === conv.phone ? 'rgba(37,211,102,0.06)' : 'transparent',
                transition: 'background 0.15s',
              }}
              onMouseEnter={e => { if (selectedPhone !== conv.phone) e.currentTarget.style.background = T.bg; }}
              onMouseLeave={e => { if (selectedPhone !== conv.phone) e.currentTarget.style.background = 'transparent'; }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 7, flex: 1, minWidth: 0 }}>
                  {conv.ai_active && (
                    <span style={{ width: 7, height: 7, borderRadius: '50%', background: T.wa, display: 'inline-block', flexShrink: 0 }} title="AI Auto-Reply active" />
                  )}
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ color: T.text, fontWeight: 600, fontSize: 13, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {conv.name || conv.phone}
                    </div>
                    <div style={{ fontFamily: T.mono, color: T.muted, fontSize: 11 }}>{conv.phone}</div>
                  </div>
                </div>
                <div style={{ textAlign: 'right', flexShrink: 0, marginLeft: 8 }}>
                  <div style={{ color: T.muted, fontSize: 11 }}>{formatTime(conv.last_message_at, orgTimezone)}</div>
                  {conv.unread_count > 0 && (
                    <span style={{ display: 'inline-block', background: T.wa, color: '#fff', borderRadius: 10, padding: '1px 7px', fontSize: 11, fontWeight: 700, marginTop: 2 }}>
                      {conv.unread_count}
                    </span>
                  )}
                </div>
              </div>
              {conv.last_message && (
                <div style={{ color: T.muted, fontSize: 12, marginTop: 3, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {conv.last_message.length > 40 ? conv.last_message.substring(0, 40) + '...' : conv.last_message}
                </div>
              )}
            </div>
          ))}
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
                <div style={{ color: T.text, fontWeight: 700, fontSize: 14 }}>{selectedConv?.name || selectedPhone}</div>
                <div style={{ fontFamily: T.mono, color: T.muted, fontSize: 12 }}>{selectedPhone}</div>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={{ color: T.muted, fontSize: 12 }}>AI Auto-Reply</span>
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

            {/* Messages */}
            <div style={{ flex: 1, overflowY: 'auto', padding: '16px 20px', display: 'flex', flexDirection: 'column', gap: 8 }}>
              {messages.length === 0 ? (
                <div style={{ textAlign: 'center', color: T.muted, marginTop: '3rem', fontSize: 13 }}>No messages yet</div>
              ) : messages.map((msg, i) => {
                const isOutbound = msg.direction === 'outbound';
                return (
                  <div key={msg.id || i} style={{ display: 'flex', justifyContent: isOutbound ? 'flex-end' : 'flex-start' }}>
                    <div style={{
                      maxWidth: '70%', padding: '9px 13px', borderRadius: 14, lineHeight: 1.45,
                      background: isOutbound ? T.wa : T.card,
                      color: isOutbound ? '#fff' : T.text,
                      fontSize: 13,
                      border: isOutbound ? 'none' : `1px solid ${T.border}`,
                      boxShadow: isOutbound ? 'none' : '0 1px 2px rgba(0,0,0,0.04)',
                      borderTopRightRadius: isOutbound ? 4 : 14,
                      borderTopLeftRadius: isOutbound ? 14 : 4,
                    }}>
                      {msg.ai_generated && <span title="AI-generated" style={{ marginRight: 4 }}>🤖</span>}
                      <span>{msg.text || msg.body}</span>
                      <div style={{ fontSize: 11, color: isOutbound ? 'rgba(255,255,255,0.7)' : T.muted, marginTop: 4, textAlign: 'right' }}>
                        {formatTime(msg.created_at || msg.timestamp, orgTimezone)}
                      </div>
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

      <ConfigModal show={showConfig} onClose={() => setShowConfig(false)}
        apiFetch={apiFetch} API_URL={API_URL} orgProducts={orgProducts} selectedOrg={selectedOrg} />
    </div>
  );
}
