import React, { useState, useEffect } from 'react';
import { useAuth } from '../contexts/AuthContext';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif",
};

const card = {
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
  const [members, setMembers] = useState([]);
  const [pendingInvites, setPendingInvites] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showInvite, setShowInvite] = useState(false);
  const [inviteForm, setInviteForm] = useState({ email: '', full_name: '', role: 'Agent' });
  const [inviteError, setInviteError] = useState('');
  const [inviteSuccess, setInviteSuccess] = useState('');
  const [inviteLoading, setInviteLoading] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(null);
  const [confirmCancelInvite, setConfirmCancelInvite] = useState(null);
  const [copiedInviteId, setCopiedInviteId] = useState(null);

  useEffect(() => { fetchTeam(); }, []);

  const fetchTeam = async () => {
    setLoading(true);
    try {
      const [mRes, iRes] = await Promise.all([
        apiFetch(`${API_URL}/team`),
        apiFetch(`${API_URL}/team/invites`),
      ]);
      if (mRes.ok) setMembers(await mRes.json());
      if (iRes.ok) setPendingInvites(await iRes.json());
    } catch (e) { console.error('Team fetch error:', e); }
    setLoading(false);
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
        alert(data.error || data.detail || 'Failed to fetch invite link');
        return;
      }
      try {
        await navigator.clipboard.writeText(data.invite_link);
        setCopiedInviteId(inviteId);
        setTimeout(() => setCopiedInviteId(prev => prev === inviteId ? null : prev), 2000);
      } catch (_) {
        prompt('Copy this invite link:', data.invite_link);
      }
    } catch (e) { alert('Network error'); }
  };

  const handleCancelInvite = async (inviteId) => {
    try {
      const res = await apiFetch(`${API_URL}/team/invites/${inviteId}`, { method: 'DELETE' });
      if (res.ok) {
        setConfirmCancelInvite(null);
        fetchTeam();
      } else {
        let msg = `Failed to cancel invite (HTTP ${res.status})`;
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch (_) {}
        alert(msg);
      }
    } catch (e) { alert('Network error'); }
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
        alert(data.detail || 'Failed to update role');
      }
    } catch (e) { alert('Network error'); }
  };

  const handleDelete = async (userId) => {
    try {
      const res = await apiFetch(`${API_URL}/team/${userId}`, { method: 'DELETE' });
      if (res.ok) {
        setConfirmDelete(null);
        fetchTeam();
      } else {
        let msg = `Failed to remove user (HTTP ${res.status})`;
        try { const data = await res.json(); if (data?.error || data?.detail) msg = data.error || data.detail; } catch (_) {}
        alert(msg);
      }
    } catch (e) { alert('Network error'); }
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
        <div>
          <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
            <span style={{ color: T.accent }}>Team</span> Members
          </h2>
          <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
            Manage your organization's users and their roles.
          </p>
        </div>
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
          <div style={{ ...card, width: 440, maxWidth: '90vw' }} onClick={e => e.stopPropagation()}>
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

      {/* Pending Invites */}
      {pendingInvites.length > 0 && (
        <div style={{ ...card, marginBottom: 16 }}>
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
                      {confirmCancelInvite === inv.id ? (
                        <span style={{ display: 'inline-flex', gap: 6, justifyContent: 'flex-end', alignItems: 'center' }}>
                          <span style={{ fontSize: 12, color: T.red }}>Cancel?</span>
                          <button onClick={() => handleCancelInvite(inv.id)}
                            style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: 6, color: T.red, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font }}>Yes</button>
                          <button onClick={() => setConfirmCancelInvite(null)}
                            style={{ background: T.bg, border: `1px solid ${T.border}`, borderRadius: 6, color: T.muted, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>No</button>
                        </span>
                      ) : (
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
                          <button onClick={() => setConfirmCancelInvite(inv.id)}
                            style={{ background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 6, color: T.red, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
                            Cancel Invite
                          </button>
                        </span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Team Table */}
      <div style={card}>
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
                <th style={{ ...thStyle, textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {members.map((m, i) => {
                const isSelf = currentUser && currentUser.id === m.id;
                const isLast = i === members.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
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
                    <td style={{ ...rowTd, textAlign: 'right' }}>
                      {isSelf ? (
                        <span style={{ color: T.muted, fontSize: 13 }}>—</span>
                      ) : confirmDelete === m.id ? (
                        <span style={{ display: 'inline-flex', gap: 6, justifyContent: 'flex-end', alignItems: 'center' }}>
                          <span style={{ fontSize: 12, color: T.red }}>Remove?</span>
                          <button onClick={() => handleDelete(m.id)}
                            style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: 6, color: T.red, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font }}>Yes</button>
                          <button onClick={() => setConfirmDelete(null)}
                            style={{ background: T.bg, border: `1px solid ${T.border}`, borderRadius: 6, color: T.muted, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>No</button>
                        </span>
                      ) : (
                        <button onClick={() => setConfirmDelete(m.id)}
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
