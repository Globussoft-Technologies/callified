import { useState, useEffect, useCallback, useRef } from 'react';
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

const permissionDisplayOverrides = {
  'calls.dial': {
    label: 'AI Dial',
    action: 'Can AI dial',
    description: 'Show and use the single-lead AI Dial button.',
  },
  'calls.dial_all': {
    label: 'AI All Dials',
    action: 'Can bulk AI dial',
    description: 'Show and use AI All Dials / AI All New Dials campaign actions.',
  },
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
  const [inviteForm, setInviteForm] = useState({ email: '', full_name: '', password: '', role: 'Agent' });
  const [inviteError, setInviteError] = useState('');
  const [inviteSuccess, setInviteSuccess] = useState('');
  const [inviteLoading, setInviteLoading] = useState(false);
  const [copiedInviteId, setCopiedInviteId] = useState(null);
  const [bulkLoading, setBulkLoading] = useState(false);
  const [resetMember, setResetMember] = useState(null);
  const [resetForm, setResetForm] = useState({ password: '', confirm: '' });
  const [showInvitePassword, setShowInvitePassword] = useState(false);
  const [showResetPassword, setShowResetPassword] = useState(false);
  const [showResetConfirm, setShowResetConfirm] = useState(false);
  const [resetError, setResetError] = useState('');
  const [resetLoading, setResetLoading] = useState(false);
  const [permissionMember, setPermissionMember] = useState(null);
  const [permissionDefs, setPermissionDefs] = useState([]);
  const [permissionValues, setPermissionValues] = useState([]);
  const [permissionCustom, setPermissionCustom] = useState(false);
  const [permissionLoading, setPermissionLoading] = useState(false);
  const [permissionSaving, setPermissionSaving] = useState(false);
  const [permissionError, setPermissionError] = useState('');
  const [providerAccounts, setProviderAccounts] = useState([]);
  const [selectedProviderAccountIds, setSelectedProviderAccountIds] = useState([]);
  const fileInputRef = useRef(null);

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

  const copyStoredKey = async (key) => {
    if (!key) return;
    const copyValue = key.key_plaintext || '';
    if (!copyValue) {
      toast('Full key is unavailable for older keys. Generate a new key to enable full-key copy.', 'error');
      return;
    }
    try {
      await navigator.clipboard.writeText(copyValue);
      toast('Full API key copied to clipboard', 'success');
    } catch {
      await promptInline({
        title: 'Copy API key',
        message: 'Clipboard access was blocked — select and copy manually.',
        defaultValue: copyValue,
        okText: 'Done',
      });
    }
  };

  const closeInvite = () => {
    setShowInvite(false);
    setInviteForm({ email: '', full_name: '', password: '', role: 'Agent' });
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
        setInviteSuccess(data.message || `Team member ${inviteForm.email} created.`);
        setInviteForm({ email: '', full_name: '', password: '', role: 'Agent' });
        fetchTeam();
      } else {
        setInviteError(data.error || data.detail || 'Failed to send invite');
      }
    } catch { setInviteError('Network error');
     }
    setInviteLoading(false);
  };

  const handleDownloadTemplate = async () => {
    try {
      const res = await apiFetch(`${API_URL}/team/template-csv`);
      if (!res.ok) {
        toast('Failed to download template', 'error');
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'team_members_template.csv';
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch {
      toast('Network error', 'error');
    }
  };

  const handleBulkUpload = async (e) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    const form = new FormData();
    form.append('file', file);
    setBulkLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/team/import-csv`, {
        method: 'POST',
        body: form,
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast(data.error || data.detail || 'Failed to import team members', 'error');
      } else {
        const failed = Array.isArray(data.errors) ? data.errors.length : 0;
        toast(`Created ${data.created || 0} member${data.created === 1 ? '' : 's'}${failed ? `, ${failed} skipped` : ''}`, failed ? 'error' : 'success');
        fetchTeam();
      }
    } catch {
      toast('Network error', 'error');
    }
    setBulkLoading(false);
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
        const data = await res.json().catch(() => ({}));
        toast(data.error || data.detail || `Failed to update role (HTTP ${res.status})`, 'error');
      }
    } catch { toast('Network error', 'error');  }
  };

  const openPermissions = async (member) => {
    setPermissionMember(member);
    setPermissionDefs([]);
    setPermissionValues([]);
    setPermissionCustom(false);
    setPermissionError('');
    setProviderAccounts([]);
    setSelectedProviderAccountIds([]);
    setPermissionLoading(true);
    try {
      const [permsRes, accountsRes] = await Promise.all([
        apiFetch(`${API_URL}/team/${member.id}/permissions`),
        apiFetch(`${API_URL}/team/${member.id}/provider-accounts`),
      ]);
      const data = await permsRes.json().catch(() => ({}));
      if (!permsRes.ok) {
        setPermissionError(data.error || data.detail || 'Failed to load permissions.');
      } else {
        setPermissionDefs(Array.isArray(data.available) ? data.available : []);
        setPermissionValues(Array.isArray(data.permissions) ? data.permissions : []);
        setPermissionCustom(Boolean(data.is_custom));
      }
      const accountsData = await accountsRes.json().catch(() => []);
      if (accountsRes.ok && Array.isArray(accountsData)) {
        setProviderAccounts(accountsData);
        setSelectedProviderAccountIds(accountsData.filter(a => a.allowed).map(a => a.id));
      }
    } catch {
      setPermissionError('Network error');
    }
    setPermissionLoading(false);
  };

  const closePermissions = () => {
    if (permissionSaving) return;
    setPermissionMember(null);
    setPermissionDefs([]);
    setPermissionValues([]);
    setPermissionCustom(false);
    setPermissionError('');
    setProviderAccounts([]);
    setSelectedProviderAccountIds([]);
  };

  const togglePermission = (key) => {
    setPermissionValues(prev => prev.includes(key) ? prev.filter(k => k !== key) : [...prev, key]);
  };

  const savePermissions = async () => {
    if (!permissionMember) return;
    setPermissionSaving(true);
    setPermissionError('');
    try {
      const [permsRes, accountsRes] = await Promise.all([
        apiFetch(`${API_URL}/team/${permissionMember.id}/permissions`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ permissions: permissionValues }),
        }),
        apiFetch(`${API_URL}/team/${permissionMember.id}/provider-accounts`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ account_ids: selectedProviderAccountIds }),
        }),
      ]);
      const data = await permsRes.json().catch(() => ({}));
      if (!permsRes.ok) {
        setPermissionError(data.error || data.detail || 'Failed to save permissions.');
        setPermissionSaving(false);
        return;
      }
      const accountsData = await accountsRes.json().catch(() => ({}));
      if (!accountsRes.ok) {
        setPermissionError(accountsData.error || accountsData.detail || 'Failed to save provider accounts.');
        setPermissionSaving(false);
        return;
      }
      toast(`Permissions saved for ${permissionMember.full_name || permissionMember.email}`, 'success');
      setPermissionSaving(false);
      closePermissions();
      return;
    } catch {
      setPermissionError('Network error');
    }
    setPermissionSaving(false);
  };

  const resetPermissionsToRole = () => {
    setPermissionValues(permissionDefs.map(p => p.key));
    setPermissionCustom(false);
  };

  const groupedPermissions = permissionDefs.reduce((acc, p) => {
    if (!acc[p.module]) acc[p.module] = [];
    acc[p.module].push(p);
    return acc;
  }, {});

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

  const openResetPassword = (member) => {
    setResetMember(member);
    setResetForm({ password: '', confirm: '' });
    setResetError('');
  };

  const closeResetPassword = () => {
    if (resetLoading) return;
    setResetMember(null);
    setResetForm({ password: '', confirm: '' });
    setShowResetPassword(false);
    setShowResetConfirm(false);
    setResetError('');
  };

  const handleResetPassword = async (e) => {
    e.preventDefault();
    if (!resetMember) return;
    const password = resetForm.password;
    if (!password.trim()) {
      setResetError('Password is required.');
      return;
    }
    if (password !== resetForm.confirm) {
      setResetError('Passwords do not match.');
      return;
    }
    setResetError('');
    setResetLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/team/${resetMember.id}/reset-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setResetError(data.error || data.detail || 'Failed to reset password.');
      } else {
        toast(`Password reset for ${resetMember.email}`, 'success');
        setResetLoading(false);
        closeResetPassword();
        return;
      }
    } catch {
      setResetError('Network error');
    }
    setResetLoading(false);
  };

  const roleBadge = (role) => {
    const colors = {
      Admin:  { bg: 'rgba(99,102,241,0.1)',  color: T.accent, border: 'rgba(99,102,241,0.3)' },
      Agent:  { bg: 'rgba(16,185,129,0.1)',  color: T.green,  border: 'rgba(16,185,129,0.3)' },
      Executive: { bg: 'rgba(245,158,11,0.1)',  color: T.amber,  border: 'rgba(245,158,11,0.3)' },
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
        <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          <button
            onClick={handleDownloadTemplate}
            style={{
              background: '#fff', border: `1px solid ${T.border}`,
              borderRadius: 8, color: T.sub, padding: '10px 16px', cursor: 'pointer',
              fontWeight: 700, fontSize: 13, fontFamily: T.font,
            }}>
            Download Template
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".csv,text/csv"
            onChange={handleBulkUpload}
            style={{ display: 'none' }}
          />
          <button
            onClick={() => fileInputRef.current?.click()}
            disabled={bulkLoading}
            style={{
              background: 'rgba(99,102,241,0.1)', border: '1px solid rgba(99,102,241,0.3)',
              borderRadius: 8, color: T.accent, padding: '10px 16px', cursor: bulkLoading ? 'wait' : 'pointer',
              fontWeight: 700, fontSize: 13, fontFamily: T.font, opacity: bulkLoading ? 0.7 : 1,
            }}>
            {bulkLoading ? 'Importing...' : 'Import Members'}
          </button>
          <button
            onClick={() => setShowInvite(true)}
            style={{
              background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
              borderRadius: 8, color: '#fff', padding: '10px 20px', cursor: 'pointer',
              fontWeight: 700, fontSize: 13, fontFamily: T.font,
            }}>
            + Add Member
          </button>
        </div>
      </div>

      {/* Invite Modal */}
      {showInvite && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000,
        }} onClick={closeInvite} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
          <div style={{ ...cardStyle, width: 440, maxWidth: '90vw' }} onClick={e => e.stopPropagation()} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
            <h3 style={{ margin: '0 0 6px', fontSize: 16, fontWeight: 700, color: T.text }}>Add Team Member</h3>
            <p style={{ margin: '0 0 18px', color: T.muted, fontSize: 13 }}>
              Set a password now so the member can sign in immediately.
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
                <PasswordInput
                  placeholder="Password"
                  required
                  value={inviteForm.password}
                  onChange={e => setInviteForm({ ...inviteForm, password: e.target.value })}
                  visible={showInvitePassword}
                  onToggle={() => setShowInvitePassword(v => !v)}
                />
                <select
                  value={inviteForm.role}
                  onChange={e => setInviteForm({ ...inviteForm, role: e.target.value })}
                  style={{ ...inputStyle, cursor: 'pointer' }}
                >
                  <option value="Admin">Admin</option>
                  <option value="Agent">Agent</option>
                  <option value="Executive">Executive</option>
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
                    {inviteLoading ? 'Creating...' : 'Create Member'}
                  </button>
                </div>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Reset Password Modal */}
      {resetMember && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000,
        }} onClick={closeResetPassword} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
          <div style={{ ...cardStyle, width: 420, maxWidth: '90vw' }} onClick={e => e.stopPropagation()} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
            <h3 style={{ margin: '0 0 6px', fontSize: 16, fontWeight: 700, color: T.text }}>Reset Password</h3>
            <p style={{ margin: '0 0 18px', color: T.muted, fontSize: 13 }}>
              Set a new password for {resetMember.full_name || resetMember.email}.
            </p>
            <form onSubmit={handleResetPassword}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <PasswordInput
                  placeholder="New password"
                  required
                  value={resetForm.password}
                  onChange={e => setResetForm({ ...resetForm, password: e.target.value })}
                  visible={showResetPassword}
                  onToggle={() => setShowResetPassword(v => !v)}
                  autoFocus
                />
                <PasswordInput
                  placeholder="Confirm password"
                  required
                  value={resetForm.confirm}
                  onChange={e => setResetForm({ ...resetForm, confirm: e.target.value })}
                  visible={showResetConfirm}
                  onToggle={() => setShowResetConfirm(v => !v)}
                />
                {resetError && (
                  <div style={{ color: T.red, fontSize: 13, background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 8, padding: '8px 12px' }}>
                    {resetError}
                  </div>
                )}
                <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end' }}>
                  <button type="button" onClick={closeResetPassword} disabled={resetLoading}
                    style={{ background: T.bg, border: `1px solid ${T.border}`, borderRadius: 8, color: T.sub, padding: '8px 16px', cursor: resetLoading ? 'not-allowed' : 'pointer', fontFamily: T.font, fontWeight: 600, fontSize: 13 }}>
                    Cancel
                  </button>
                  <button type="submit" disabled={resetLoading}
                    style={{
                      background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
                      borderRadius: 8, color: '#fff', padding: '8px 20px', cursor: resetLoading ? 'not-allowed' : 'pointer',
                      fontWeight: 700, fontSize: 13, fontFamily: T.font, opacity: resetLoading ? 0.7 : 1,
                    }}>
                    {resetLoading ? 'Resetting...' : 'Reset Password'}
                  </button>
                </div>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Permissions Modal */}
      {permissionMember && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000,
        }} onClick={closePermissions} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
          <div style={{ ...cardStyle, width: 760, maxWidth: '94vw', maxHeight: '86vh', overflow: 'hidden', padding: 0 }} onClick={e => e.stopPropagation()} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
            <div style={{ padding: '22px 26px 16px', borderBottom: `1px solid ${T.border}`, display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'flex-start' }}>
              <div>
                <h3 style={{ margin: 0, fontSize: 17, fontWeight: 800, color: T.text }}>Permissions</h3>
                <p style={{ margin: '6px 0 0', color: T.muted, fontSize: 13 }}>
                  {permissionMember.full_name || permissionMember.email} · {permissionMember.role}
                </p>
              </div>
              <button onClick={closePermissions}
                style={{ background: '#fff', border: `1px solid ${T.border}`, borderRadius: 8, color: T.sub, width: 34, height: 34, cursor: 'pointer', fontSize: 18, lineHeight: 1 }}>
                ×
              </button>
            </div>
            <div style={{ padding: '18px 26px', maxHeight: '58vh', overflowY: 'auto' }}>
              {permissionLoading ? (
                <div style={{ color: T.muted, padding: '28px 0', textAlign: 'center' }}>Loading permissions...</div>
              ) : permissionError ? (
                <div style={{ color: T.red, fontSize: 13, background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 8, padding: '10px 12px' }}>
                  {permissionError}
                </div>
              ) : (
                <>
                  <div style={{ marginBottom: 14, color: T.muted, fontSize: 13 }}>
                    Role boundary is enforced automatically. Permissions outside <strong>{permissionMember.role}</strong> are not available here.
                    {permissionCustom && <span style={{ color: T.accent, fontWeight: 700 }}> Custom permissions are active.</span>}
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 12 }}>
                    {Object.entries(groupedPermissions).map(([module, items]) => (
                      <div key={module} style={{ border: `1px solid ${T.border}`, borderRadius: 8, background: '#fff', overflow: 'hidden' }}>
                        <div style={{ padding: '10px 12px', background: '#f9fafb', borderBottom: `1px solid ${T.border}`, color: T.text, fontWeight: 800, fontSize: 13 }}>
                          {module}
                        </div>
                        <div style={{ padding: '8px 10px', display: 'flex', flexDirection: 'column', gap: 6 }}>
                          {items.map(p => {
                            const checked = permissionValues.includes(p.key);
                            const display = permissionDisplayOverrides[p.key] || p;
                            return (
                              <label key={p.key} style={{
                                display: 'grid', gridTemplateColumns: '18px 1fr', gap: 9,
                                alignItems: 'flex-start', padding: '7px 6px', borderRadius: 6,
                                cursor: 'pointer', background: checked ? 'rgba(99,102,241,0.06)' : 'transparent',
                              }}>
                                <input
                                  type="checkbox"
                                  checked={checked}
                                  onChange={() => togglePermission(p.key)}
                                  style={{ marginTop: 2 }}
                                />
                                <span>
                                  <span style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'baseline' }}>
                                    <span style={{ color: T.text, fontWeight: 700, fontSize: 13 }}>{display.label}</span>
                                    <span style={{ color: T.accent, fontWeight: 700, fontSize: 11, whiteSpace: 'nowrap' }}>{display.action}</span>
                                  </span>
                                  <span style={{ display: 'block', color: T.muted, fontSize: 12, lineHeight: 1.35, marginTop: 2 }}>{display.description}</span>
                                </span>
                              </label>
                            );
                          })}
                        </div>
                      </div>
                    ))}
                  </div>

                  {/* Provider Accounts visibility */}
                  {permissionMember && (permissionMember.role === 'Agent' || permissionMember.role === 'Executive') && (
                    <div style={{ border: `1px solid ${T.border}`, borderRadius: 8, background: '#fff', overflow: 'hidden', marginTop: 14 }}>
                      <div style={{ padding: '10px 12px', background: '#f9fafb', borderBottom: `1px solid ${T.border}`, color: T.text, fontWeight: 800, fontSize: 13 }}>
                        Provider Accounts
                      </div>
                      <div style={{ padding: '12px 14px' }}>
                        <p style={{ margin: '0 0 10px', color: T.muted, fontSize: 12, lineHeight: 1.4 }}>
                          Check the accounts this user can see and use for browser calls. Unchecked accounts are hidden from their dropdown.
                        </p>
                        {providerAccounts.length === 0 ? (
                          <p style={{ margin: 0, color: T.muted, fontSize: 13 }}>No org provider accounts available.</p>
                        ) : (
                          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                            {providerAccounts.map(a => {
                              const checked = selectedProviderAccountIds.includes(a.id);
                              return (
                                <label key={a.id} style={{
                                  display: 'grid', gridTemplateColumns: '18px 1fr', gap: 9,
                                  alignItems: 'flex-start', padding: '7px 6px', borderRadius: 6,
                                  cursor: 'pointer', background: checked ? 'rgba(99,102,241,0.06)' : 'transparent',
                                }}>
                                  <input
                                    type="checkbox"
                                    checked={checked}
                                    onChange={() => setSelectedProviderAccountIds(prev =>
                                      prev.includes(a.id) ? prev.filter(id => id !== a.id) : [...prev, a.id]
                                    )}
                                    style={{ marginTop: 2 }}
                                  />
                                  <span>
                                    <span style={{ color: T.text, fontWeight: 700, fontSize: 13 }}>{a.name}</span>
                                    <span style={{ display: 'block', color: T.muted, fontSize: 12 }}>{a.caller_id} · {a.account_sid}</span>
                                  </span>
                                </label>
                              );
                            })}
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </>
              )}
            </div>
            <div style={{ padding: '14px 26px 20px', borderTop: `1px solid ${T.border}`, display: 'flex', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap' }}>
              <button onClick={resetPermissionsToRole} disabled={permissionLoading || permissionSaving}
                style={{ background: '#fff', border: `1px solid ${T.border}`, borderRadius: 8, color: T.sub, padding: '9px 14px', cursor: permissionLoading || permissionSaving ? 'not-allowed' : 'pointer', fontFamily: T.font, fontWeight: 700, fontSize: 13 }}>
                Reset to Role Default
              </button>
              <span style={{ display: 'inline-flex', gap: 10 }}>
                <button onClick={closePermissions} disabled={permissionSaving}
                  style={{ background: T.bg, border: `1px solid ${T.border}`, borderRadius: 8, color: T.sub, padding: '9px 16px', cursor: permissionSaving ? 'not-allowed' : 'pointer', fontFamily: T.font, fontWeight: 700, fontSize: 13 }}>
                  Cancel
                </button>
                <button onClick={savePermissions} disabled={permissionLoading || permissionSaving || Boolean(permissionError)}
                  style={{
                    background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
                    borderRadius: 8, color: '#fff', padding: '9px 18px',
                    cursor: permissionLoading || permissionSaving || permissionError ? 'not-allowed' : 'pointer',
                    fontWeight: 800, fontSize: 13, fontFamily: T.font,
                    opacity: permissionLoading || permissionSaving || permissionError ? 0.65 : 1,
                  }}>
                  {permissionSaving ? 'Saving...' : 'Save Permissions'}
                </button>
              </span>
            </div>
          </div>
        </div>
      )}

      {/* Newly-generated key modal — shown once only */}
      {newKey && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000,
        }} onClick={() => setNewKey(null)} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
          <div style={{ ...cardStyle, width: 520, maxWidth: '92vw' }} onClick={e => e.stopPropagation()} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
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
                <th style={thStyle}>Permissions</th>
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
                        <option value="Executive">Executive</option>
                      </select>
                    </td>
                    <td style={rowTd}>
                      <button
                        onClick={() => openPermissions(m)}
                        disabled={isSelf}
                        title={isSelf ? 'You cannot edit your own permissions' : `Edit permissions for ${m.full_name || m.email}`}
                        style={{
                          background: isSelf ? '#f9fafb' : 'rgba(99,102,241,0.08)',
                          border: `1px solid ${isSelf ? T.border : 'rgba(99,102,241,0.25)'}`,
                          borderRadius: 7, color: isSelf ? T.muted : T.accent,
                          width: 32, height: 30, cursor: isSelf ? 'not-allowed' : 'pointer',
                          fontSize: 14, fontFamily: T.font, fontWeight: 700,
                        }}
                      >
                        🛡
                      </button>
                    </td>
                    <td style={{ ...rowTd, color: T.muted }}>
                      {m.created_at ? new Date(m.created_at).toLocaleDateString() : '-'}
                    </td>
                    <td style={rowTd}>
                      {!isAdminMember(m) ? (
                        // API keys are Admin-only — Agents/Executives shouldn't mint
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
                          <button
                            onClick={() => copyStoredKey(key)}
                            disabled={busy}
                            title={key.key_plaintext ? 'Copy full API key' : 'Full key unavailable for older keys. Generate a new key to copy the full value.'}
                            style={{
                              background: 'rgba(99,102,241,0.08)', border: '1px solid rgba(99,102,241,0.22)',
                              borderRadius: 4, color: T.accent, padding: '2px 8px',
                              cursor: busy ? 'wait' : 'pointer', fontSize: 11, fontFamily: T.font,
                              opacity: key.key_plaintext ? 1 : 0.65,
                            }}>Copy</button>
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
                        <span style={{ display: 'inline-flex', gap: 6, justifyContent: 'flex-end', flexWrap: 'wrap' }}>
                          <button onClick={() => openResetPassword(m)}
                            style={{ background: 'rgba(99,102,241,0.08)', border: '1px solid rgba(99,102,241,0.25)', borderRadius: 6, color: T.accent, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font }}>
                            Reset Password
                          </button>
                          <button onClick={() => handleDelete(m)}
                            style={{ background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 6, color: T.red, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
                            Remove
                          </button>
                        </span>
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

function PasswordInput({ value, onChange, visible, onToggle, placeholder, required = false, autoFocus = false }) {
  return (
    <div style={{ position: 'relative' }}>
      <input
        placeholder={placeholder}
        type={visible ? 'text' : 'password'}
        required={required}
        value={value}
        onChange={onChange}
        style={{ ...inputStyle, paddingRight: 42 }}
        autoFocus={autoFocus}
      />
      <button
        type="button"
        onClick={onToggle}
        aria-label={visible ? 'Hide password' : 'Show password'}
        title={visible ? 'Hide password' : 'Show password'}
        style={{
          position: 'absolute',
          right: 8,
          top: '50%',
          transform: 'translateY(-50%)',
          width: 28,
          height: 28,
          border: 'none',
          borderRadius: 6,
          background: 'transparent',
          color: T.muted,
          cursor: 'pointer',
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: 0,
        }}
      >
        {visible ? (
          <svg width="17" height="17" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M3 3l18 18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
            <path d="M10.6 10.6a2 2 0 0 0 2.8 2.8" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
            <path d="M9.9 5.2A9.7 9.7 0 0 1 12 5c5.2 0 8.5 4.5 9.5 6.3a1.4 1.4 0 0 1 0 1.4 16 16 0 0 1-2.1 2.8" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
            <path d="M6.6 6.8a16.2 16.2 0 0 0-4.1 4.5 1.4 1.4 0 0 0 0 1.4C3.5 14.5 6.8 19 12 19c1.4 0 2.7-.3 3.8-.9" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        ) : (
          <svg width="17" height="17" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M2.5 11.3C3.5 9.5 6.8 5 12 5s8.5 4.5 9.5 6.3a1.4 1.4 0 0 1 0 1.4C20.5 14.5 17.2 19 12 19s-8.5-4.5-9.5-6.3a1.4 1.4 0 0 1 0-1.4Z" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
            <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="2" />
          </svg>
        )}
      </button>
    </div>
  );
}
