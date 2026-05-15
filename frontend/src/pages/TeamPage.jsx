import React, { useState, useEffect } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { useToast, usePrompt, useConfirm } from '../contexts/UIContext';

export default function TeamPage({ apiFetch, API_URL }) {
  const { currentUser } = useAuth();
  const toast = useToast();
  const promptInline = usePrompt();
  const confirmDialog = useConfirm();
  const [members, setMembers] = useState([]);
  const [pendingInvites, setPendingInvites] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showInvite, setShowInvite] = useState(false);
  const [inviteForm, setInviteForm] = useState({ email: '', full_name: '', role: 'Agent' });
  const [inviteError, setInviteError] = useState('');
  const [inviteSuccess, setInviteSuccess] = useState('');
  const [inviteLoading, setInviteLoading] = useState(false);
  const [copiedInviteId, setCopiedInviteId] = useState(null);
  // API keys keyed by member user_id (we encode that in the key's name as
  // "team:<user_id>" since the api_keys table has no user_id column). The
  // map only ever holds the most-recently-issued key per user — if an Admin
  // generates twice, the older row is left orphaned in the DB but no longer
  // surfaced in the UI. Showing it would be misleading since the raw secret
  // for that older row is unrecoverable.
  const [apiKeysByUser, setApiKeysByUser] = useState({});
  // After Generate, we hold the raw key in state just long enough for the
  // Admin to copy it. Cleared as soon as the modal closes.
  const [newKey, setNewKey] = useState(null); // { user_id, email, key }
  const [keyBusyUserId, setKeyBusyUserId] = useState(null);

  useEffect(() => { fetchTeam(); }, []);

  const fetchTeam = async () => {
    setLoading(true);
    try {
      const [mRes, iRes, kRes] = await Promise.all([
        apiFetch(`${API_URL}/team`),
        apiFetch(`${API_URL}/team/invites`),
        // /api/api-keys is admin-only; non-admins get 403 here and we just
        // render "—" in the API Key column. Don't toast — that's expected.
        apiFetch(`${API_URL}/api-keys`),
      ]);
      if (mRes.ok) setMembers(await mRes.json());
      if (iRes.ok) setPendingInvites(await iRes.json());
      if (kRes.ok) {
        const keys = await kRes.json();
        // Bucket by the embedded user_id. We keep only the highest-id row
        // per user (most recent) so Generate-after-Delete-after-Generate
        // doesn't show stale entries.
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
  };

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
        return;
      }
      setNewKey({ user_id: member.id, email: member.email, key: data.key });
      // Re-fetch so the new key appears in the row with its prefix.
      fetchTeam();
    } catch (e) { toast('Network error', 'error'); }
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
    } catch (e) { toast('Network error', 'error'); }
    setKeyBusyUserId(null);
  };

  const handleDeleteKey = async (member, key) => {
    const ok = await confirmDialog({
      title: 'Delete API key',
      message: `Permanently delete this API key for ${member.email}? This cannot be undone — generate a new one if you change your mind.`,
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
    } catch (e) { toast('Network error', 'error'); }
    setKeyBusyUserId(null);
  };

  const copyNewKey = async () => {
    if (!newKey) return;
    try {
      await navigator.clipboard.writeText(newKey.key);
      toast('API key copied to clipboard', 'success');
    } catch (_) {
      // Fall back to inline prompt when clipboard write is blocked.
      await promptInline({
        title: 'Copy API key',
        message: 'Select and copy the key — clipboard access was blocked.',
        defaultValue: newKey.key,
        okText: 'Done',
        cancelText: 'Close',
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
    } catch (e) {
      setInviteError('Network error');
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
      // navigator.clipboard requires a secure context (https / localhost),
      // which both dev and prod satisfy. Fall back to a read-only inline
      // prompt if it throws (e.g. permission denied / non-secure origin).
      try {
        await navigator.clipboard.writeText(data.invite_link);
        setCopiedInviteId(inviteId);
        setTimeout(() => setCopiedInviteId(prev => prev === inviteId ? null : prev), 2000);
      } catch (_) {
        await promptInline({
          title: 'Copy invite link',
          message: 'Select and copy the link below — clipboard access was blocked.',
          defaultValue: data.invite_link,
          okText: 'Done',
          cancelText: 'Close',
        });
      }
    } catch (e) { toast('Network error', 'error'); }
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
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch (_) {}
        toast(msg, 'error');
      }
    } catch (e) { toast('Network error', 'error'); }
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
    } catch (e) { toast('Network error', 'error'); }
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
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch (_) {}
        toast(msg, 'error');
      }
    } catch (e) { toast('Network error', 'error'); }
  };

  const roleBadge = (role) => {
    const colors = {
      Admin: { bg: 'rgba(99,102,241,0.2)', color: '#a5b4fc', border: 'rgba(99,102,241,0.4)' },
      Agent: { bg: 'rgba(34,197,94,0.2)', color: '#4ade80', border: 'rgba(34,197,94,0.4)' },
      Viewer: { bg: 'rgba(234,179,8,0.2)', color: '#fde047', border: 'rgba(234,179,8,0.4)' },
    };
    const c = colors[role] || colors.Agent;
    return (
      <span style={{
        padding: '2px 10px', borderRadius: '12px', fontSize: '0.75rem', fontWeight: 600,
        background: c.bg, color: c.color, border: `1px solid ${c.border}`,
      }}>{role}</span>
    );
  };

  const cardStyle = {
    background: 'rgba(30,41,59,0.7)', border: '1px solid rgba(148,163,184,0.1)',
    borderRadius: '12px', padding: '24px',
  };

  return (
    <div style={{ padding: '24px', maxWidth: '900px', margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
        <h2 style={{ margin: 0, color: '#f1f5f9' }}>Team Members</h2>
        <button
          onClick={() => setShowInvite(true)}
          style={{
            background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
            borderRadius: '8px', color: '#fff', padding: '10px 20px', cursor: 'pointer',
            fontWeight: 600, fontSize: '0.85rem',
          }}
        >+ Invite Member</button>
      </div>

      {/* Invite Modal */}
      {showInvite && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000,
        }} onClick={closeInvite}>
          <div style={{ ...cardStyle, width: '440px', maxWidth: '90vw' }} onClick={e => e.stopPropagation()}>
            <h3 style={{ margin: '0 0 6px', color: '#f1f5f9' }}>Invite Team Member</h3>
            <p style={{ margin: '0 0 16px', color: '#94a3b8', fontSize: '0.8rem' }}>
              They'll get an email with a link to set their own password — no password is set here.
            </p>
            <form onSubmit={handleInvite}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
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
                  style={inputStyle}
                >
                  <option value="Admin">Admin</option>
                  <option value="Agent">Agent</option>
                  <option value="Viewer">Viewer</option>
                </select>
                {inviteError && <div style={{ color: '#fca5a5', fontSize: '0.85rem' }}>{inviteError}</div>}
                {inviteSuccess && <div style={{ color: '#86efac', fontSize: '0.85rem', background: 'rgba(34,197,94,0.12)', border: '1px solid rgba(34,197,94,0.3)', borderRadius: '6px', padding: '8px 12px' }}>{inviteSuccess}</div>}
                <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
                  <button type="button" onClick={closeInvite}
                    style={{ background: 'rgba(148,163,184,0.15)', border: '1px solid rgba(148,163,184,0.2)', borderRadius: '6px', color: '#94a3b8', padding: '8px 16px', cursor: 'pointer' }}>
                    {inviteSuccess ? 'Done' : 'Cancel'}
                  </button>
                  <button type="submit" disabled={inviteLoading}
                    style={{
                      background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
                      borderRadius: '6px', color: '#fff', padding: '8px 20px', cursor: 'pointer', fontWeight: 600,
                      opacity: inviteLoading ? 0.6 : 1,
                    }}>
                    {inviteLoading ? 'Sending...' : 'Send Invite'}
                  </button>
                </div>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Pending Invites */}
      {pendingInvites.length > 0 && (
        <div style={{ ...cardStyle, marginBottom: '20px' }}>
          <h3 style={{ margin: '0 0 14px', color: '#f1f5f9', fontSize: '1rem' }}>
            Pending Invites <span style={{ color: '#94a3b8', fontWeight: 400, fontSize: '0.85rem' }}>({pendingInvites.length})</span>
          </h3>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid rgba(148,163,184,0.15)' }}>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Email</th>
                <th style={thStyle}>Role</th>
                <th style={thStyle}>Invited By</th>
                <th style={thStyle}>Expires</th>
                <th style={{ ...thStyle, textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {pendingInvites.map(inv => (
                <tr key={inv.id} style={{ borderBottom: '1px solid rgba(148,163,184,0.08)' }}>
                  <td style={tdStyle}>{inv.full_name || '-'}</td>
                  <td style={tdStyle}>{inv.email}</td>
                  <td style={tdStyle}>{roleBadge(inv.role)}</td>
                  <td style={tdStyle}>{inv.invited_by || '-'}</td>
                  <td style={tdStyle}>
                    {inv.expires_at ? new Date(inv.expires_at).toLocaleString() : '-'}
                  </td>
                  <td style={{ ...tdStyle, textAlign: 'right' }}>
                    <span style={{ display: 'inline-flex', gap: '6px', justifyContent: 'flex-end' }}>
                      <button onClick={() => handleCopyInviteLink(inv.id)}
                        title="Copy invite link to clipboard — useful when SMTP isn't configured or to resend out-of-band"
                        style={{ background: copiedInviteId === inv.id ? 'rgba(34,197,94,0.15)' : 'rgba(99,102,241,0.1)', border: `1px solid ${copiedInviteId === inv.id ? 'rgba(34,197,94,0.4)' : 'rgba(99,102,241,0.25)'}`, borderRadius: '4px', color: copiedInviteId === inv.id ? '#86efac' : '#a5b4fc', padding: '3px 10px', cursor: 'pointer', fontSize: '0.75rem', fontWeight: 600 }}>
                        {copiedInviteId === inv.id ? '✓ Copied' : '🔗 Copy link'}
                      </button>
                      <button onClick={() => handleCancelInvite(inv)}
                        style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: '4px', color: '#fca5a5', padding: '3px 10px', cursor: 'pointer', fontSize: '0.75rem' }}>
                        Cancel Invite
                      </button>
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Newly-generated key modal — shown once, never again */}
      {newKey && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000,
        }} onClick={() => setNewKey(null)}>
          <div style={{ ...cardStyle, width: '520px', maxWidth: '92vw' }} onClick={e => e.stopPropagation()}>
            <h3 style={{ margin: '0 0 6px', color: '#f1f5f9' }}>API key for {newKey.email}</h3>
            <p style={{ margin: '0 0 14px', color: '#fde047', fontSize: '0.8rem' }}>
              Copy this key now — it cannot be shown again. Use it in the <code>X-API-Key</code> header.
            </p>
            <div style={{
              background: 'rgba(15,23,42,0.9)', border: '1px solid rgba(148,163,184,0.2)',
              borderRadius: '6px', padding: '10px 12px', color: '#e2e8f0', fontFamily: 'monospace',
              fontSize: '0.8rem', wordBreak: 'break-all', marginBottom: '14px',
            }}>{newKey.key}</div>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button onClick={copyNewKey}
                style={{ background: 'rgba(99,102,241,0.15)', border: '1px solid rgba(99,102,241,0.35)', borderRadius: '6px', color: '#a5b4fc', padding: '8px 16px', cursor: 'pointer', fontWeight: 600 }}>
                Copy
              </button>
              <button onClick={() => setNewKey(null)}
                style={{ background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none', borderRadius: '6px', color: '#fff', padding: '8px 20px', cursor: 'pointer', fontWeight: 600 }}>
                Done
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Team Table */}
      <div style={cardStyle}>
        {loading ? (
          <div style={{ textAlign: 'center', color: '#94a3b8', padding: '40px' }}>Loading team...</div>
        ) : members.length === 0 ? (
          <div style={{ textAlign: 'center', color: '#94a3b8', padding: '40px' }}>No team members found.</div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid rgba(148,163,184,0.15)' }}>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Email</th>
                <th style={thStyle}>Role</th>
                <th style={thStyle}>Joined</th>
                <th style={thStyle}>API Key</th>
                <th style={{ ...thStyle, textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {members.map(m => {
                const isSelf = currentUser && currentUser.id === m.id;
                const key = apiKeysByUser[m.id];
                const busy = keyBusyUserId === m.id;
                return (
                <tr key={m.id} style={{ borderBottom: '1px solid rgba(148,163,184,0.08)' }}>
                  <td style={tdStyle}>
                    {m.full_name || '-'}
                    {isSelf && (
                      <span style={{ marginLeft: '8px', fontSize: '0.7rem', color: '#a78bfa', fontWeight: 600 }}>(you)</span>
                    )}
                  </td>
                  <td style={tdStyle}>{m.email}</td>
                  <td style={tdStyle}>
                    <select
                      value={m.role}
                      disabled={isSelf}
                      title={isSelf ? "You cannot change your own role" : undefined}
                      onChange={e => handleRoleChange(m.id, e.target.value)}
                      style={{
                        background: 'rgba(30,41,59,0.9)', border: '1px solid rgba(148,163,184,0.2)',
                        borderRadius: '6px', color: isSelf ? '#64748b' : '#e2e8f0', padding: '4px 8px', fontSize: '0.8rem',
                        cursor: isSelf ? 'not-allowed' : 'pointer',
                        opacity: isSelf ? 0.6 : 1,
                      }}
                    >
                      <option value="Admin">Admin</option>
                      <option value="Agent">Agent</option>
                      <option value="Viewer">Viewer</option>
                    </select>
                  </td>
                  <td style={tdStyle}>
                    {m.created_at ? new Date(m.created_at).toLocaleDateString() : '-'}
                  </td>
                  <td style={tdStyle}>
                    {!isAdminMember(m) ? (
                      // API keys are intentionally Admin-only. Agents/Viewers
                      // shouldn't be able to mint org-scoped keys that would
                      // bypass their own role restrictions when called from a
                      // partner integration.
                      <span style={{ color: '#64748b' }}>—</span>
                    ) : !key ? (
                      <button
                        onClick={() => handleGenerateKey(m)}
                        disabled={busy}
                        style={{
                          background: 'rgba(99,102,241,0.15)', border: '1px solid rgba(99,102,241,0.35)',
                          borderRadius: '4px', color: '#a5b4fc', padding: '3px 10px', cursor: busy ? 'wait' : 'pointer',
                          fontSize: '0.75rem', fontWeight: 600, opacity: busy ? 0.6 : 1,
                        }}
                      >
                        {busy ? 'Generating...' : '+ Generate'}
                      </button>
                    ) : (
                      <div style={{ display: 'inline-flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                        <code style={{ background: 'rgba(15,23,42,0.7)', border: '1px solid rgba(148,163,184,0.15)', borderRadius: '4px', padding: '2px 6px', color: '#cbd5e1', fontSize: '0.75rem' }}>
                          {key.key_prefix}…
                        </code>
                        {key.is_active ? (
                          <span style={{ fontSize: '0.7rem', color: '#86efac', fontWeight: 600 }}>active</span>
                        ) : (
                          <span style={{ fontSize: '0.7rem', color: '#fca5a5', fontWeight: 600 }}>revoked</span>
                        )}
                        {key.is_active ? (
                          <button
                            onClick={() => handleRevokeKey(m, key, false)}
                            disabled={busy}
                            style={{
                              background: 'rgba(234,179,8,0.12)', border: '1px solid rgba(234,179,8,0.3)',
                              borderRadius: '4px', color: '#fde047', padding: '2px 8px', cursor: busy ? 'wait' : 'pointer',
                              fontSize: '0.7rem',
                            }}
                          >Revoke</button>
                        ) : (
                          <button
                            onClick={() => handleRevokeKey(m, key, true)}
                            disabled={busy}
                            style={{
                              background: 'rgba(34,197,94,0.1)', border: '1px solid rgba(34,197,94,0.3)',
                              borderRadius: '4px', color: '#86efac', padding: '2px 8px', cursor: busy ? 'wait' : 'pointer',
                              fontSize: '0.7rem',
                            }}
                          >Reactivate</button>
                        )}
                        <button
                          onClick={() => handleDeleteKey(m, key)}
                          disabled={busy}
                          style={{
                            background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.2)',
                            borderRadius: '4px', color: '#fca5a5', padding: '2px 8px', cursor: busy ? 'wait' : 'pointer',
                            fontSize: '0.7rem',
                          }}
                        >Delete</button>
                      </div>
                    )}
                  </td>
                  <td style={{ ...tdStyle, textAlign: 'right' }}>
                    {currentUser && currentUser.id === m.id ? (
                      // No Remove button on the caller's own row — self-removal
                      // would lock them out (and could lock the org out if
                      // they're the only admin). Backend rejects it anyway.
                      // Issue #54.
                      <span style={{ color: '#64748b', fontSize: '0.75rem' }}>—</span>
                    ) : (
                      <button onClick={() => handleDelete(m)}
                        style={{
                          background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.2)',
                          borderRadius: '4px', color: '#fca5a5', padding: '3px 10px', cursor: 'pointer',
                          fontSize: '0.75rem',
                        }}>Remove</button>
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

const inputStyle = {
  background: 'rgba(15,23,42,0.8)', border: '1px solid rgba(148,163,184,0.2)',
  borderRadius: '8px', color: '#e2e8f0', padding: '10px 14px', fontSize: '0.9rem',
  outline: 'none', width: '100%', boxSizing: 'border-box',
};

const thStyle = {
  textAlign: 'left', padding: '10px 12px', color: '#94a3b8',
  fontSize: '0.75rem', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.5px',
};

const tdStyle = {
  padding: '12px', color: '#e2e8f0', fontSize: '0.85rem',
};
