import React, { useState, useEffect, useRef, useCallback } from 'react';

// DevPage — paginated user list with one-click impersonation. Modeled after
// the aMember reference. Backend gates /api/dev/* by the DEVELOPER_EMAILS env
// allowlist; non-developers get a 404 on the data fetch and see an empty
// state. The /dev nav link in TopHeader is also hidden for non-developers.
export default function DevPage({ apiFetch, API_URL }) {
  const [users, setUsers] = useState([]);
  const [meta, setMeta] = useState({ page: 1, limit: 25, total: 0, total_pages: 0 });
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(25);
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState(null);

  const searchDebounce = useRef(null);

  const fetchUsers = useCallback(async (overrides = {}) => {
    setLoading(true); setError('');
    const params = new URLSearchParams({
      page: String(overrides.page ?? page),
      limit: String(overrides.limit ?? limit),
    });
    const s = overrides.search ?? search;
    const st = overrides.status ?? status;
    if (s) params.set('search', s);
    if (st) params.set('status', st);
    try {
      const res = await apiFetch(`${API_URL}/dev/users?${params.toString()}`);
      if (res.status === 404) {
        setError('Developer surface is disabled or your account is not allowlisted.');
        setUsers([]); setMeta({ page: 1, limit, total: 0, total_pages: 0 });
        return;
      }
      if (!res.ok) {
        setError(`HTTP ${res.status}`);
        return;
      }
      const data = await res.json();
      setUsers(data.users || []);
      setMeta({ page: data.page, limit: data.limit, total: data.total, total_pages: data.total_pages });
    } catch (e) {
      setError(`network error: ${e.message || e}`);
    } finally {
      setLoading(false);
    }
  }, [apiFetch, API_URL, page, limit, search, status]);

  useEffect(() => { fetchUsers(); /* initial */ }, []); // eslint-disable-line

  // Debounced search: typing resets to page 1 once the user pauses 300ms.
  useEffect(() => {
    if (searchDebounce.current) clearTimeout(searchDebounce.current);
    searchDebounce.current = setTimeout(() => {
      setPage(1);
      fetchUsers({ page: 1, search });
    }, 300);
    return () => searchDebounce.current && clearTimeout(searchDebounce.current);
  }, [search]); // eslint-disable-line

  const onStatusChange = (v) => {
    setStatus(v);
    setPage(1);
    fetchUsers({ page: 1, status: v });
  };
  const onLimitChange = (v) => {
    const n = parseInt(v, 10) || 25;
    setLimit(n);
    setPage(1);
    fetchUsers({ page: 1, limit: n });
  };

  const changePage = (delta) => {
    const next = Math.max(1, page + delta);
    if (meta.total_pages && next > meta.total_pages) return;
    setPage(next);
    fetchUsers({ page: next });
  };

  const loginAs = async (user) => {
    setBusyId(user.id);
    try {
      const res = await apiFetch(`${API_URL}/dev/impersonate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: user.id }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        alert(`Impersonation failed: ${body.error || `HTTP ${res.status}`}`);
        return;
      }
      const { key } = await res.json();
      const target = `/sso/exchange?key=${encodeURIComponent(key)}&next=/crm`;
      const opened = window.open(target, '_blank', 'noopener,noreferrer');
      if (!opened) {
        alert('Popup blocked. Allow popups for this site and try again.');
      }
    } catch (e) {
      alert(`Network error: ${e.message || e}`);
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div style={S.page}>
      <h1 style={S.title}>Developer Dashboard</h1>
      <p style={S.subtitle}>
        Internal break-glass tool — log into any user's account without their
        password for support, debugging, or demo. Every impersonation is
        logged on the server with your email as the actor.
      </p>

      <div style={S.panel}>
        <div style={S.grid}>
          <div>
            <label style={S.label}>Search (email / name)</label>
            <input style={S.input} value={search} onChange={(e) => setSearch(e.target.value)} placeholder="optional" />
          </div>
          <div>
            <label style={S.label}>Status</label>
            <select style={S.input} value={status} onChange={(e) => onStatusChange(e.target.value)}>
              <option value="">All</option>
              <option value="active">Active (password set)</option>
              <option value="inactive">Inactive (SSO-only)</option>
            </select>
          </div>
          <div>
            <label style={S.label}>Page size</label>
            <select style={S.input} value={limit} onChange={(e) => onLimitChange(e.target.value)}>
              <option value="25">25</option>
              <option value="50">50</option>
              <option value="100">100</option>
            </select>
          </div>
          <div style={{ display: 'flex', alignItems: 'flex-end' }}>
            <button style={S.btnGhost} onClick={() => fetchUsers()}>Refresh</button>
          </div>
        </div>
      </div>

      {error && <div style={S.errBox}>{error}</div>}

      <div style={S.panel}>
        <div style={S.meta}>
          <span>Page <b>{meta.page}</b> of <b>{meta.total_pages || 1}</b></span>
          <span>Showing <b>{users.length}</b> of <b>{meta.total}</b> users</span>
          {loading && <span style={{ color: '#a5b4fc' }}>loading…</span>}
        </div>

        <div style={{ overflowX: 'auto' }}>
          <table style={S.table}>
            <thead>
              <tr>
                <th style={S.th}>ID</th>
                <th style={S.th}>Email</th>
                <th style={S.th}>Name</th>
                <th style={S.th}>Role</th>
                <th style={S.th}>Org</th>
                <th style={S.th}>Created</th>
                <th style={S.th}>Status</th>
                <th style={S.th}>Action</th>
              </tr>
            </thead>
            <tbody>
              {users.length === 0 && !loading && (
                <tr><td colSpan={8} style={{ ...S.td, textAlign: 'center', color: '#64748b', padding: '24px' }}>No users.</td></tr>
              )}
              {users.map(u => (
                <tr key={u.id} style={S.tr}>
                  <td style={S.td}>{u.id}</td>
                  <td style={S.td}>{u.email}</td>
                  <td style={S.td}>{u.full_name || <span style={{ color: '#64748b' }}>—</span>}</td>
                  <td style={S.td}><span style={S.badge(u.role)}>{u.role}</span></td>
                  <td style={S.td}>{u.org_name || <span style={{ color: '#64748b' }}>—</span>} <span style={{ color: '#64748b' }}>{u.org_id ? `(#${u.org_id})` : ''}</span></td>
                  <td style={S.td}>{u.created_at ? u.created_at.slice(0, 10) : '—'}</td>
                  <td style={S.td}>
                    {u.status === 'active'
                      ? <span style={{ color: '#22c55e' }}>active</span>
                      : <span style={{ color: '#f59e0b' }}>inactive</span>}
                  </td>
                  <td style={S.td}>
                    <button
                      onClick={() => loginAs(u)}
                      disabled={busyId === u.id}
                      style={S.btnPrimary(busyId === u.id)}
                      title="Open a new tab signed in as this user. Audit-logged."
                    >
                      {busyId === u.id ? 'opening…' : 'Login as →'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div style={S.pagination}>
          <button style={S.btnGhost} onClick={() => changePage(-1)} disabled={page <= 1}>← Prev</button>
          <span style={{ color: '#94a3b8', fontSize: 13 }}>Page {meta.page}{meta.total_pages ? ` of ${meta.total_pages}` : ''}</span>
          <button style={S.btnGhost} onClick={() => changePage(1)} disabled={meta.total_pages && page >= meta.total_pages}>Next →</button>
        </div>
      </div>
    </div>
  );
}

const S = {
  page: { padding: '24px', maxWidth: '1280px', margin: '0 auto', color: '#e2e8f0' },
  title: { margin: 0, fontSize: 22 },
  subtitle: { margin: '6px 0 18px', color: '#94a3b8', fontSize: 13, maxWidth: 720 },
  panel: { background: '#1e293b', border: '1px solid #334155', borderRadius: 10, padding: 16, marginBottom: 16 },
  grid: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 },
  label: { display: 'block', fontSize: 11, color: '#94a3b8', textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: 4 },
  input: {
    width: '100%', padding: '8px 10px', background: '#0f172a',
    border: '1px solid #334155', borderRadius: 6, color: '#e2e8f0', fontSize: 14,
  },
  meta: { display: 'flex', gap: 16, flexWrap: 'wrap', fontSize: 13, color: '#94a3b8', marginBottom: 12 },
  table: { width: '100%', borderCollapse: 'collapse', fontSize: 13 },
  th: {
    textAlign: 'left', padding: '8px 10px',
    borderBottom: '1px solid #334155',
    background: '#0f172a', color: '#94a3b8',
    fontWeight: 600, textTransform: 'uppercase', fontSize: 11, letterSpacing: 0.5,
  },
  tr: { transition: 'background 0.1s' },
  td: { padding: '8px 10px', borderBottom: '1px solid #1f2937' },
  badge: (role) => {
    const colors = { Admin: '#a5b4fc', Agent: '#86efac', Viewer: '#fcd34d' };
    return {
      padding: '2px 8px', borderRadius: 4, fontSize: 11, fontWeight: 700,
      color: colors[role] || '#cbd5e1',
      background: 'rgba(255,255,255,0.04)', border: '1px solid #334155',
    };
  },
  btnPrimary: (busy) => ({
    padding: '6px 12px', borderRadius: 6, cursor: busy ? 'wait' : 'pointer',
    background: busy ? '#334155' : 'linear-gradient(135deg, #6366f1, #22d3ee)',
    border: 'none', color: '#fff', fontSize: 12, fontWeight: 700,
    boxShadow: busy ? 'none' : '0 4px 12px rgba(99,102,241,0.3)',
  }),
  btnGhost: {
    padding: '7px 14px', borderRadius: 6, cursor: 'pointer',
    background: 'rgba(255,255,255,0.04)', border: '1px solid #334155',
    color: '#cbd5e1', fontSize: 13, fontWeight: 600,
  },
  pagination: { display: 'flex', gap: 12, alignItems: 'center', marginTop: 14 },
  errBox: {
    background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
    color: '#fca5a5', padding: '10px 14px', borderRadius: 6, marginBottom: 14, fontSize: 13,
  },
};
