import React, { useState, useEffect, useRef } from 'react';
import { useToast, useConfirm } from '../contexts/UIContext';
import { useAuth } from '../contexts/AuthContext';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const inputStyle = (hasError) => ({
  padding: '9px 13px', borderRadius: 8, fontSize: 13,
  border: `1px solid ${hasError ? T.red : T.border}`,
  background: T.card, color: T.text, fontFamily: T.font, outline: 'none',
});

const labelStyle = {
  fontSize: 10, fontWeight: 700, color: T.muted,
  textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 8,
};

function SourceBadge({ source }) {
  const colors = {
    manual:           { bg: 'rgba(148,163,184,0.15)', color: '#64748b' },
    ndnc:             { bg: 'rgba(239,68,68,0.1)',   color: T.red },
    customer_request: { bg: 'rgba(245,158,11,0.1)',  color: T.amber },
  };
  const s = colors[source] || colors.manual;
  return (
    <span style={{
      fontSize: 11, fontWeight: 600, padding: '3px 10px', borderRadius: 20,
      background: s.bg, color: s.color,
    }}>{source || 'manual'}</span>
  );
}

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const inputStyle = (hasError) => ({
  padding: '9px 13px', borderRadius: 8, fontSize: 13,
  border: `1px solid ${hasError ? T.red : T.border}`,
  background: T.card, color: T.text, fontFamily: T.font, outline: 'none',
});

const labelStyle = {
  fontSize: 10, fontWeight: 700, color: T.muted,
  textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 8,
};

function SourceBadge({ source }) {
  const colors = {
    manual:           { bg: 'rgba(148,163,184,0.15)', color: '#64748b' },
    ndnc:             { bg: 'rgba(239,68,68,0.1)',   color: T.red },
    customer_request: { bg: 'rgba(245,158,11,0.1)',  color: T.amber },
  };
  const s = colors[source] || colors.manual;
  return (
    <span style={{
      fontSize: 11, fontWeight: 600, padding: '3px 10px', borderRadius: 20,
      background: s.bg, color: s.color,
    }}>{source || 'manual'}</span>
  );
}

export default function DndPage({ apiFetch, API_URL }) {
  const toast = useToast();
  const confirm = useConfirm();
  const { currentUser } = useAuth();
  const [numbers, setNumbers] = useState([]);
  const [totalCount, setTotalCount] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [addPhone, setAddPhone] = useState('');
  const [addSource, setAddSource] = useState('');
  const [addError, setAddError] = useState('');
  const [checkPhone, setCheckPhone] = useState('');
  const [checkResult, setCheckResult] = useState(null);
  const [importing, setImporting] = useState(false);
  const [importMsg, setImportMsg] = useState('');
  const perPage = 50;

  const isValidPhone = (p) => /^\d{10}$/.test(p);

  const addErrTimer = useRef(null);
  const checkErrTimer = useRef(null);
  const flashAddError = (msg) => {
    setAddError(msg);
    if (addErrTimer.current) clearTimeout(addErrTimer.current);
    addErrTimer.current = setTimeout(() => setAddError(''), 2500);
  };
  const flashCheckError = (msg) => {
    setCheckResult({ error: msg });
    if (checkErrTimer.current) clearTimeout(checkErrTimer.current);
    checkErrTimer.current = setTimeout(() => setCheckResult(null), 2500);
  };
  useEffect(() => () => {
    if (addErrTimer.current) clearTimeout(addErrTimer.current);
    if (checkErrTimer.current) clearTimeout(checkErrTimer.current);
  }, []);

  const fetchNumbers = async (p = page) => {
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/dnd?page=${p}&per_page=${perPage}`);
      const data = await res.json();
      setNumbers(data.numbers || data.items || data.data || []);
      setTotalCount(data.total || data.total_count || 0);
    } catch (e) { console.error('Failed to fetch DND list', e); }
    setLoading(false);
  };

  // eslint-disable-next-line react-hooks/set-state-in-effect, react-hooks/exhaustive-deps
  useEffect(() => { if (currentUser?.role === 'Admin') fetchNumbers(page); }, [page, currentUser?.role]);

  const handleAdd = async () => {
    const phone = addPhone.trim();
    if (!phone) { setAddError('Phone number is required'); return; }
    if (!isValidPhone(phone)) { setAddError('Phone must be exactly 10 digits'); return; }
    setAddError('');
    try {
      const body = { phone };
      if (addSource.trim()) body.source = addSource.trim();
      const res = await apiFetch(`${API_URL}/dnd`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setAddError(data.error || `Add failed (${res.status})`);
        return;
      }
      setAddPhone(''); setAddSource('');
      fetchNumbers(1); setPage(1);
    } catch (e) { setAddError('Failed to add number: ' + e.message); }
  };

  const handleRemove = async (phone) => {
    if (!await confirm({ message: `Remove ${phone} from DND list?` })) return;
    try {
      await apiFetch(`${API_URL}/dnd/${encodeURIComponent(phone)}`, { method: 'DELETE' });
      fetchNumbers(page);
    } catch (e) { toast('Failed to remove: ' + e.message); }
  };

  const handleCheck = async () => {
    const phone = checkPhone.trim();
    if (!phone) { setCheckResult({ error: 'Phone number is required' }); return; }
    if (!isValidPhone(phone)) { setCheckResult({ error: 'Phone must be exactly 10 digits' }); return; }
    try {
      const res = await apiFetch(`${API_URL}/dnd/check/${encodeURIComponent(phone)}`);
      const data = await res.json();
      setCheckResult(data);
    } catch { setCheckResult({ error: 'Check failed'  }); }
  };

  const handleImport = async (e) => {
    const file = e.target.files[0];
    if (!file) return;
    setImporting(true); setImportMsg('');
    try {
      const formData = new FormData();
      formData.append('file', file);
      const res = await apiFetch(`${API_URL}/dnd/import`, { method: 'POST', body: formData });
      const data = await res.json();
      setImportMsg(data.message || `Imported ${data.imported || 0} numbers`);
      fetchNumbers(1); setPage(1);
    } catch (e) { setImportMsg('Import failed: ' + e.message); }
    setImporting(false);
    e.target.value = '';
  };

  const totalPages = Math.ceil(totalCount / perPage);

  if (currentUser?.role !== 'Admin') {
    return (
      <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>
        <div style={{ ...card, padding: '3rem', textAlign: 'center', color: T.muted }}>
          <div style={{ fontSize: 16, fontWeight: 600, color: T.text, marginBottom: 6 }}>Access Restricted</div>
          <div style={{ fontSize: 13 }}>DND management is available to Admins only.</div>
        </div>
      </div>
    );
  }

  const thStyle = {
    fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase',
    letterSpacing: '0.07em', padding: '0 0 12px', textAlign: 'left',
    borderBottom: `1px solid ${T.border}`,
  };
  const tdStyle = {
    fontSize: 13, color: T.sub, padding: '14px 0',
    borderBottom: `1px solid ${T.border}`, verticalAlign: 'middle',
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title */}
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>DND Management</h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Manage Do-Not-Disturb numbers to exclude from all campaigns.
        </p>
      </div>

      {/* Controls card */}
      <div style={{ ...card, padding: '24px 28px', marginBottom: 16 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '32px', alignItems: 'flex-start' }}>

          {/* Add Number */}
          <div style={{ flex: '1 1 320px' }}>
            <div style={labelStyle}>Add Number to DND</div>
            <div style={{ display: 'flex', gap: 8 }}>
              <input
                type="text" inputMode="numeric" maxLength={10}
                placeholder="10-digit phone (e.g. 9876543210)" value={addPhone}
                onChange={e => {
                  const raw = e.target.value;
                  const digits = raw.replace(/\D/g, '').slice(0, 10);
                  setAddPhone(digits);
                  if (raw !== digits && raw.length > digits.length) flashAddError('Only digits allowed (0-9)');
                  else if (addError) setAddError('');
                }}
                onKeyDown={e => e.key === 'Enter' && handleAdd()}
                style={{ ...inputStyle(!!addError), flex: 1 }}
              />
              <input
                type="text" placeholder="Source (optional)" value={addSource}
                onChange={e => setAddSource(e.target.value)}
                style={{ ...inputStyle(false), width: 150 }}
              />
              <button onClick={handleAdd} style={{
                padding: '9px 18px', borderRadius: 8, border: 'none', fontSize: 13,
                fontWeight: 600, background: T.accent, color: '#fff', cursor: 'pointer',
                fontFamily: T.font, whiteSpace: 'nowrap',
              }}>Add</button>
            </div>
            {addError && (
              <div role="alert" aria-live="polite" style={{ fontSize: 12, color: T.red, marginTop: 5, fontWeight: 600 }}>
                {addError}
              </div>
            )}
          </div>

          {/* Import CSV */}
          <div>
            <div style={labelStyle}>Import CSV</div>
            <label style={{
              display: 'inline-flex', alignItems: 'center', gap: 6,
              padding: '9px 18px', borderRadius: 8, fontSize: 13, fontWeight: 600,
              border: `1px solid ${T.accent}`, color: T.accent,
              background: 'rgba(99,102,241,0.06)', cursor: 'pointer', fontFamily: T.font,
            }}>
              <span>↑</span> {importing ? 'Importing...' : 'Upload CSV'}
              <input type="file" accept=".csv" onChange={handleImport} style={{ display: 'none' }} disabled={importing} />
            </label>
            {importMsg && <div style={{ fontSize: 12, color: T.muted, marginTop: 5 }}>{importMsg}</div>}
          </div>

          {/* Check Number */}
          <div>
            <div style={labelStyle}>Check Number</div>
            <div style={{ display: 'flex', gap: 8 }}>
              <input
                type="text" inputMode="numeric" maxLength={10}
                placeholder="10-digit phone" value={checkPhone}
                onChange={e => {
                  const raw = e.target.value;
                  const digits = raw.replace(/\D/g, '').slice(0, 10);
                  setCheckPhone(digits);
                  if (raw !== digits && raw.length > digits.length) flashCheckError('Only digits allowed (0-9)');
                  else setCheckResult(null);
                }}
                onKeyDown={e => e.key === 'Enter' && handleCheck()}
                style={{ ...inputStyle(checkResult?.error), width: 180 }}
              />
              <button onClick={handleCheck} style={{
                padding: '9px 18px', borderRadius: 8, fontSize: 13, fontWeight: 600,
                cursor: 'pointer', border: `1px solid ${T.border}`,
                background: T.card, color: T.sub, fontFamily: T.font,
              }}>Check</button>
            </div>
            {checkResult && (
              <div aria-live="polite" style={{
                fontSize: 12, marginTop: 5, fontWeight: 600,
                color: checkResult.is_dnd ? T.red : checkResult.error ? T.amber : T.green,
              }}>
                {checkResult.error ? checkResult.error : checkResult.is_dnd ? 'On DND list' : 'Not on DND list'}
              </div>
            )}
          </div>

        </div>
      </div>

      {/* Table card */}
      <div style={{ ...card, padding: '24px 28px' }}>
        <h3 style={{ margin: '0 0 20px', fontSize: 15, fontWeight: 700, color: T.text }}>
          DND Numbers{' '}
          <span style={{ fontSize: 13, fontWeight: 400, color: T.muted }}>({totalCount} total)</span>
        </h3>

        {loading ? (
          <div style={{ textAlign: 'center', padding: '2.5rem 0', color: T.muted, fontSize: 14 }}>Loading...</div>
        ) : numbers.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '2.5rem 0', color: T.muted, fontSize: 14 }}>No DND numbers found.</div>
        ) : (
          <>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  <th style={thStyle}>Phone Number</th>
                  <th style={thStyle}>Source</th>
                  <th style={thStyle}>Added</th>
                  <th style={{ ...thStyle, textAlign: 'right' }}>Action</th>
                </tr>
              </thead>
              <tbody>
                {numbers.map((n, i) => {
                  const isLast = i === numbers.length - 1;
                  const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                  return (
                    <tr key={n.phone}>
                      <td style={{ ...rowTd, fontFamily: T.mono, fontWeight: 600, color: T.text, paddingRight: 16 }}>
                        {n.phone}
                      </td>
                      <td style={{ ...rowTd, paddingRight: 16 }}>
                        <SourceBadge source={n.source} />
                      </td>
                      <td style={{ ...rowTd, color: T.muted, paddingRight: 16 }}>
                        {n.created_at ? new Date(n.created_at).toLocaleDateString() : '-'}
                      </td>
                      <td style={{ ...rowTd, textAlign: 'right' }}>
                        <button onClick={() => handleRemove(n.phone)} style={{
                          padding: '4px 12px', borderRadius: 20, fontSize: 11, fontWeight: 600,
                          cursor: 'pointer', border: `1px solid rgba(239,68,68,0.3)`,
                          background: 'rgba(239,68,68,0.08)', color: T.red, fontFamily: T.font,
                        }}>Remove</button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>

            {totalPages > 1 && (
              <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', gap: 12, marginTop: 20 }}>
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page <= 1}
                  style={{
                    padding: '7px 16px', borderRadius: 8, fontSize: 13, fontWeight: 600,
                    cursor: page <= 1 ? 'default' : 'pointer', fontFamily: T.font,
                    border: `1px solid ${T.border}`, background: T.card,
                    color: page <= 1 ? T.muted : T.sub,
                  }}>Previous</button>
                <span style={{ fontSize: 13, color: T.muted }}>Page {page} of {totalPages}</span>
                <button
                  onClick={() => setPage(p => Math.min(totalPages, p + 1))} disabled={page >= totalPages}
                  style={{
                    padding: '7px 16px', borderRadius: 8, fontSize: 13, fontWeight: 600,
                    cursor: page >= totalPages ? 'default' : 'pointer', fontFamily: T.font,
                    border: `1px solid ${T.border}`, background: T.card,
                    color: page >= totalPages ? T.muted : T.sub,
                  }}>Next</button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
