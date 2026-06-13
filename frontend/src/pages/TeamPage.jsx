import { useState, useEffect, useCallback } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { useToast, useConfirm, usePrompt } from '../contexts/UIContext';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif",
};

const cardStyle = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
  padding: '24px 28px',
};

const inputStyle = {
  background: '#f9fafb', border: `1px solid ${T.border}`,
  borderRadius: 8, color: T.text, padding: '10px 14px', fontSize: 13,
  outline: 'none', width: '100%', boxSizing: 'border-box', fontFamily: T.font,
};

const thStyle = {
  textAlign: 'left', padding: '0 12px 12px', color: T.muted,
  fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.07em',
  borderBottom: `1px solid ${T.border}`,
};

const tdStyle = {
  padding: '12px', color: T.sub, fontSize: 13, borderBottom: `1px solid ${T.border}`,
  verticalAlign: 'middle',
};

export default function TeamPage({ apiFetch, API_URL }) {
  const { currentUser } = useAuth();
  const toast = useToast();
  const confirmDialog = useConfirm();
  const promptInline = usePrompt();
  const [members, setMembers] = useState([]);
  const [pendingInvites, setPendingInvites] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showInvite, setShowInvite] = useState(false);
  const [inviteForm, setInviteForm] = useState({ email: '', full_name: '', role: 'Agent' });
  const [inviteError, setInviteError] = useState('');
  const [inviteSuccess, setInviteSuccess] = useState('');
  const [inviteLoading, setInviteLoading] = useState(false);
  const [copiedInviteId, setCopiedInviteId] = useState(null);

  // API keys keyed by member user_id (encoded in the key name as "team:<user_id>:...").
  // Only the most-recently-issued key per user is surfaced — older orphaned rows
  // are ignored since the raw secret is unrecoverable.
  const [apiKeysByUser, setApiKeysByUser] = useState({});
  // After Generate, hold the raw key long enough for the Admin to copy it.
  // Cleared when the modal closes.
  const [newKey, setNewKey] = useState(null); // { user_id, email, key }
  const [keyBusyUserId, setKeyBusyUserId] = useState(null);

  const fetchTeam = useCallback(async () => {
    setLoading(true);
    try {
      const [mRes, iRes, kRes] = await Promise.all([
        apiFetch(`${API_URL}/team`),
        apiFetch(`${API_URL}/team/invites`),
        // Admin-only; non-admins get 403 — render "—" in the API Key column.
        apiFetch(`${API_URL}/api-keys`),
      ]);
      if (mRes.ok) setMembers(await mRes.json());
      if (iRes.ok) setPendingInvites(await iRes.json());
      if (kRes.ok) {
        const keys = await kRes.json();
        const byUser = {};
        for (const k of (keys || [])) {
          const m = /^team:(\d+)/.exec(k.name || '');
          if (!m) continue;
          const uid = Number(m[1]);
          const prev = byUser[uid];
          if (!prev || k.id > prev.id) byUser[uid] = k;
        }
        setApiKeysByUser(byUser);
      }
    } catch (e) { console.error('Team fetch error:', e); }
    setLoading(false);
  }, [apiFetch, API_URL]);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { fetchTeam(); }, [fetchTeam]);

  const isAdminMember = (m) => m && m.role === 'Admin';

  const handleGenerateKey = async (member) => {
    setKeyBusyUserId(member.id);
    try {
      const res = await apiFetch(`${API_URL}/api-keys`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: `team:${member.id}:${member.email}` }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast(data.error || data.detail || 'Failed to generate key', 'error');
        setKeyBusyUserId(null);
        return;
      }
      setNewKey({ user_id: member.id, email: member.email, key: data.key });
      fetchTeam();
    } catch { toast('Network error', 'error');  }
    setKeyBusyUserId(null);
  };

  const handleRevokeKey = async (member, key, makeActive) => {
    const verb = makeActive ? 'Reactivate' : 'Revoke';
    const ok = await confirmDialog({
      title: `${verb} API key`,
      message: makeActive
        ? `Reactivate this API key for ${member.email}? It will start accepting requests again.`
        : `Revoke this API key for ${member.email}? Calls using it will start returning 403 immediately.`,
      okText: verb,
      danger: !makeActive,
    });
    if (!ok) return;
    setKeyBusyUserId(member.id);
    try {
      const res = await apiFetch(`${API_URL}/api-keys/${key.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ is_active: makeActive }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast(data.error || data.detail || `Failed to ${verb.toLowerCase()} key`, 'error');
      } else {
        fetchTeam();
      }
    } catch { toast('Network error', 'error');  }
    setKeyBusyUserId(null);
  };

  const handleDeleteKey = async (member, key) => {
    const ok = await confirmDialog({
      title: 'Delete API key',
      message: `Permanently delete this API key for ${member.email}? This cannot be undone.`,
      okText: 'Delete',
      danger: true,
    });
    if (!ok) return;
    setKeyBusyUserId(member.id);
    try {
      const res = await apiFetch(`${API_URL}/api-keys/${key.id}`, { method: 'DELETE' });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast(data.error || data.detail || 'Failed to delete key', 'error');
      } else {
        fetchTeam();
      }
    } catch { toast('Network error', 'error');  }
    setKeyBusyUserId(null);
  };

  const copyNewKey = async () => {
    if (!newKey) return;
    try {
      await navigator.clipboard.writeText(newKey.key);
      toast('API key copied to clipboard', 'success');
    } catch { await promptInline({
        title: 'Copy API key',
        message: 'Clipboard access was blocked — select and copy manually.',
        defaultValue: newKey.key,
        okText: 'Done',
       });
    }
  };

  const closeInvite = () => {
    setShowInvite(false);
    setInviteForm({ email: '', full_name: '', role: 'Agent' });
    setInviteError('');
    setInviteSuccess('');
  };

  const handleInvite = async (e) => {
    e.preventDefault();
    setInviteError('');
    setInviteSuccess('');
    setInviteLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/team/invite`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(inviteForm),
      });
      const data = await res.json().catch(() => ({}));
      if (res.ok) {
        setInviteSuccess(data.message || `Invite email sent to ${inviteForm.email}.`);
        setInviteForm({ email: '', full_name: '', role: 'Agent' });
        fetchTeam();
      } else {
        setInviteError(data.error || data.detail || 'Failed to send invite');
      }
    } catch { setInviteError('Network error');
     }
    setInviteLoading(false);
  };

  const handleCopyInviteLink = async (inviteId) => {
    try {
      const res = await apiFetch(`${API_URL}/team/invites/${inviteId}/link`);
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast(data.error || data.detail || 'Failed to fetch invite link', 'error');
        return;
      }
      try {
        await navigator.clipboard.writeText(data.invite_link);
        setCopiedInviteId(inviteId);
        setTimeout(() => setCopiedInviteId(prev => prev === inviteId ? null : prev), 2000);
      } catch { await promptInline({
          title: 'Copy invite link',
          message: 'Clipboard access was blocked — select and copy manually.',
          defaultValue: data.invite_link,
          okText: 'Done',
         });
      }
    } catch { toast('Network error', 'error');  }
  };

  const handleCancelInvite = async (invite) => {
    const ok = await confirmDialog({
      title: 'Cancel invite',
      message: `Cancel the invite for ${invite.email}? They won't be able to use the link anymore.`,
      okText: 'Cancel invite',
      cancelText: 'Keep it',
      danger: true,
    });
    if (!ok) return;
    try {
      const res = await apiFetch(`${API_URL}/team/invites/${invite.id}`, { method: 'DELETE' });
      if (res.ok) {
        fetchTeam();
      } else {
        let msg = `Failed to cancel invite (HTTP ${res.status})`;
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch { /* ignore */ }
        toast(msg, 'error');
      }
    } catch { toast('Network error', 'error');  }
  };

  const handleRoleChange = async (userId, newRole) => {
    try {
      const res = await apiFetch(`${API_URL}/team/${userId}/role`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ role: newRole }),
      });
      if (res.ok) fetchTeam();
      else {
        const data = await res.json();
        toast(data.detail || 'Failed to update role', 'error');
      }
    } catch { toast('Network error', 'error');  }
  };

  const handleDelete = async (member) => {
    const label = member.full_name || member.email;
    const ok = await confirmDialog({
      title: 'Remove team member',
      message: `Remove ${label} from the team? They'll lose access immediately.`,
      okText: 'Remove',
      danger: true,
    });
    if (!ok) return;
    try {
      const res = await apiFetch(`${API_URL}/team/${member.id}`, { method: 'DELETE' });
      if (res.ok) {
        fetchTeam();
      } else {
        let msg = `Failed to remove user (HTTP ${res.status})`;
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch { /* ignore */ }
        toast(msg, 'error');
      }
    } catch { toast('Network error', 'error');  }
  };

  const roleBadge = (role) => {
    const colors = {
      Admin:  { bg: 'rgba(99,102,241,0.1)',  color: T.accent, border: 'rgba(99,102,241,0.3)' },
      Agent:  { bg: 'rgba(16,185,129,0.1)',  color: T.green,  border: 'rgba(16,185,129,0.3)' },
      Viewer: { bg: 'rgba(245,158,11,0.1)',  color: T.amber,  border: 'rgba(245,158,11,0.3)' },
    };
    const c = colors[role] || colors.Agent;
    return (
      <span style={{
        padding: '2px 10px', borderRadius: 12, fontSize: 11, fontWeight: 600,
        background: c.bg, color: c.color, border: `1px solid ${c.border}`,
      }}>{role}</span>
    );
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
          <span style={{ color: T.accent }}>Team</span> Members
        </h2>
        <button
          onClick={() => setShowInvite(true)}
          style={{
            background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
            borderRadius: 8, color: '#fff', padding: '10px 20px', cursor: 'pointer',
            fontWeight: 700, fontSize: 13, fontFamily: T.font,
          }}>
          + Invite Member
        </button>
      </div>

      {/* Invite Modal */}
      {showInvite && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000,
        }} onClick={closeInvite}>
          <div style={{ ...cardStyle, width: 440, maxWidth: '90vw' }} onClick={e => e.stopPropagation()}>
            <h3 style={{ margin: '0 0 6px', fontSize: 16, fontWeight: 700, color: T.text }}>Invite Team Member</h3>
            <p style={{ margin: '0 0 18px', color: T.muted, fontSize: 13 }}>
              They'll get an email with a link to set their own password — no password is set here.
            </p>
            <form onSubmit={handleInvite}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <input
                  placeholder="Full Name" required value={inviteForm.full_name}
                  onChange={e => setInviteForm({ ...inviteForm, full_name: e.target.value })}
                  style={inputStyle}
                />
                <input
                  placeholder="Email" type="email" required value={inviteForm.email}
                  onChange={e => setInviteForm({ ...inviteForm, email: e.target.value })}
                  style={inputStyle}
                />
                <select
                  value={inviteForm.role}
                  onChange={e => setInviteForm({ ...inviteForm, role: e.target.value })}
                  style={{ ...inputStyle, cursor: 'pointer' }}
                >
                  <option value="Admin">Admin</option>
                  <option value="Agent">Agent</option>
                  <option value="Viewer">Viewer</option>
                </select>
                {inviteError && (
                  <div style={{ color: T.red, fontSize: 13, background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 8, padding: '8px 12px' }}>
                    {inviteError}
                  </div>
                )}
                {inviteSuccess && (
                  <div style={{ color: T.green, fontSize: 13, background: 'rgba(16,185,129,0.08)', border: '1px solid rgba(16,185,129,0.25)', borderRadius: 8, padding: '8px 12px' }}>
                    {inviteSuccess}
                  </div>
                )}
                <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end' }}>
                  <button type="button" onClick={closeInvite}
                    style={{ background: T.bg, border: `1px solid ${T.border}`, borderRadius: 8, color: T.sub, padding: '8px 16px', cursor: 'pointer', fontFamily: T.font, fontWeight: 600, fontSize: 13 }}>
                    {inviteSuccess ? 'Done' : 'Cancel'}
                  </button>
                  <button type="submit" disabled={inviteLoading}
                    style={{
                      background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
                      borderRadius: 8, color: '#fff', padding: '8px 20px', cursor: inviteLoading ? 'not-allowed' : 'pointer',
                      fontWeight: 700, fontSize: 13, fontFamily: T.font, opacity: inviteLoading ? 0.7 : 1,
                    }}>
                    {inviteLoading ? 'Sending...' : 'Send Invite'}
                  </button>
                </div>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Newly-generated key modal — shown once only */}
      {newKey && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000,
        }} onClick={() => setNewKey(null)}>
          <div style={{ ...cardStyle, width: 520, maxWidth: '92vw' }} onClick={e => e.stopPropagation()}>
            <h3 style={{ margin: '0 0 6px', fontSize: 16, fontWeight: 700, color: T.text }}>
              API key for {newKey.email}
            </h3>
            <p style={{ margin: '0 0 14px', color: T.amber, fontSize: 13, background: 'rgba(245,158,11,0.08)', border: '1px solid rgba(245,158,11,0.3)', borderRadius: 8, padding: '8px 12px' }}>
              Copy this key now — it cannot be shown again. Use it in the <code>X-API-Key</code> header.
            </p>
            <div style={{
              background: '#f9fafb', border: `1px solid ${T.border}`,
              borderRadius: 8, padding: '10px 14px', color: T.text, fontFamily: 'monospace',
              fontSize: 13, wordBreak: 'break-all', marginBottom: 16,
              userSelect: 'all',
            }}>{newKey.key}</div>
            <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end' }}>
              <button onClick={copyNewKey}
                style={{ background: 'rgba(99,102,241,0.1)', border: '1px solid rgba(99,102,241,0.3)', borderRadius: 8, color: T.accent, padding: '8px 16px', cursor: 'pointer', fontWeight: 600, fontSize: 13, fontFamily: T.font }}>
                Copy
              </button>
              <button onClick={() => setNewKey(null)}
                style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none', borderRadius: 8, color: '#fff', padding: '8px 20px', cursor: 'pointer', fontWeight: 700, fontSize: 13, fontFamily: T.font }}>
                Done
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Pending Invites */}
      {pendingInvites.length > 0 && (
        <div style={{ ...cardStyle, marginBottom: 16 }}>
          <h3 style={{ margin: '0 0 16px', fontSize: 15, fontWeight: 700, color: T.text }}>
            Pending Invites{' '}
            <span style={{ color: T.muted, fontWeight: 400, fontSize: 13 }}>({pendingInvites.length})</span>
          </h3>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Email</th>
                <th style={thStyle}>Role</th>
                <th style={thStyle}>Invited By</th>
                <th style={thStyle}>Expires</th>
                <th style={{ ...thStyle, textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {pendingInvites.map((inv, i) => {
                const isLast = i === pendingInvites.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                return (
                  <tr key={inv.id}>
                    <td style={rowTd}>{inv.full_name || '-'}</td>
                    <td style={rowTd}>{inv.email}</td>
                    <td style={rowTd}>{roleBadge(inv.role)}</td>
                    <td style={rowTd}>{inv.invited_by || '-'}</td>
                    <td style={{ ...rowTd, color: T.muted }}>
                      {inv.expires_at ? new Date(inv.expires_at).toLocaleString() : '-'}
                    </td>
                    <td style={{ ...rowTd, textAlign: 'right' }}>
                      <span style={{ display: 'inline-flex', gap: 6, justifyContent: 'flex-end' }}>
                        <button onClick={() => handleCopyInviteLink(inv.id)}
                          title="Copy invite link to clipboard"
                          style={{
                            background: copiedInviteId === inv.id ? 'rgba(16,185,129,0.08)' : 'rgba(99,102,241,0.08)',
                            border: `1px solid ${copiedInviteId === inv.id ? 'rgba(16,185,129,0.3)' : 'rgba(99,102,241,0.25)'}`,
                            borderRadius: 6, color: copiedInviteId === inv.id ? T.green : T.accent,
                            padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                          }}>
                          {copiedInviteId === inv.id ? '✓ Copied' : '🔗 Copy link'}
                        </button>
                        <button onClick={() => handleCancelInvite(inv)}
                          style={{ background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 6, color: T.red, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
                          Cancel Invite
                        </button>
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Team Table */}
      <div style={cardStyle}>
        {loading ? (
          <div style={{ textAlign: 'center', color: T.muted, padding: '40px' }}>Loading team...</div>
        ) : members.length === 0 ? (
          <div style={{ textAlign: 'center', color: T.muted, padding: '40px' }}>No team members found.</div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Email</th>
                <th style={thStyle}>Role</th>
                <th style={thStyle}>Joined</th>
                <th style={thStyle}>API Key</th>
                <th style={{ ...thStyle, textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {members.map((m, i) => {
                const isSelf = currentUser && currentUser.id === m.id;
                const isLast = i === members.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                const key = apiKeysByUser[m.id];
                const busy = keyBusyUserId === m.id;
                return (
                  <tr key={m.id}>
                    <td style={{ ...rowTd, fontWeight: 600, color: T.text }}>
                      {m.full_name || '-'}
                      {isSelf && (
                        <span style={{ marginLeft: 8, fontSize: 11, color: T.accent, fontWeight: 600 }}>(you)</span>
                      )}
                    </td>
                    <td style={rowTd}>{m.email}</td>
                    <td style={rowTd}>
                      <select
                        value={m.role}
                        disabled={isSelf}
                        title={isSelf ? 'You cannot change your own role' : undefined}
                        onChange={e => handleRoleChange(m.id, e.target.value)}
                        style={{
                          background: T.bg, border: `1px solid ${T.border}`,
                          borderRadius: 6, color: isSelf ? T.muted : T.sub,
                          padding: '4px 8px', fontSize: 12, fontFamily: T.font,
                          cursor: isSelf ? 'not-allowed' : 'pointer',
                          opacity: isSelf ? 0.6 : 1,
                        }}
                      >
                        <option value="Admin">Admin</option>
                        <option value="Agent">Agent</option>
                        <option value="Viewer">Viewer</option>
                      </select>
                    </td>
                    <td style={{ ...rowTd, color: T.muted }}>
                      {m.created_at ? new Date(m.created_at).toLocaleDateString() : '-'}
                    </td>
                    <td style={rowTd}>
                      {!isAdminMember(m) ? (
                        // API keys are Admin-only — Agents/Viewers shouldn't mint
                        // org-scoped keys that bypass their role restrictions.
                        <span style={{ color: T.muted }}>—</span>
                      ) : !key ? (
                        <button
                          onClick={() => handleGenerateKey(m)}
                          disabled={busy}
                          style={{
                            background: 'rgba(99,102,241,0.1)', border: '1px solid rgba(99,102,241,0.3)',
                            borderRadius: 6, color: T.accent, padding: '3px 10px',
                            cursor: busy ? 'wait' : 'pointer', fontSize: 12, fontWeight: 600,
                            fontFamily: T.font, opacity: busy ? 0.6 : 1,
                          }}>
                          {busy ? 'Generating...' : '+ Generate'}
                        </button>
                      ) : (
                        <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                          <code style={{
                            background: '#f3f4f6', border: `1px solid ${T.border}`,
                            borderRadius: 4, padding: '2px 6px', color: T.text, fontSize: 12,
                          }}>
                            {key.key_prefix}…
                          </code>
                          {key.is_active ? (
                            <span style={{ fontSize: 11, color: T.green, fontWeight: 600 }}>active</span>
                          ) : (
                            <span style={{ fontSize: 11, color: T.red, fontWeight: 600 }}>revoked</span>
                          )}
                          {key.is_active ? (
                            <button
                              onClick={() => handleRevokeKey(m, key, false)}
                              disabled={busy}
                              style={{
                                background: 'rgba(245,158,11,0.1)', border: '1px solid rgba(245,158,11,0.3)',
                                borderRadius: 4, color: T.amber, padding: '2px 8px',
                                cursor: busy ? 'wait' : 'pointer', fontSize: 11, fontFamily: T.font,
                              }}>Revoke</button>
                          ) : (
                            <button
                              onClick={() => handleRevokeKey(m, key, true)}
                              disabled={busy}
                              style={{
                                background: 'rgba(16,185,129,0.1)', border: '1px solid rgba(16,185,129,0.3)',
                                borderRadius: 4, color: T.green, padding: '2px 8px',
                                cursor: busy ? 'wait' : 'pointer', fontSize: 11, fontFamily: T.font,
                              }}>Reactivate</button>
                          )}
                          <button
                            onClick={() => handleDeleteKey(m, key)}
                            disabled={busy}
                            style={{
                              background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)',
                              borderRadius: 4, color: T.red, padding: '2px 8px',
                              cursor: busy ? 'wait' : 'pointer', fontSize: 11, fontFamily: T.font,
                            }}>Delete</button>
                        </div>
                      )}
                    </td>
                    <td style={{ ...rowTd, textAlign: 'right' }}>
                      {isSelf ? (
                        <span style={{ color: T.muted, fontSize: 13 }}>—</span>
                      ) : (
                        <button onClick={() => handleDelete(m)}
                          style={{ background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 6, color: T.red, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
                          Remove
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
