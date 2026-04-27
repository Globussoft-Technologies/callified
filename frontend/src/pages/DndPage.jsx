import React, { useState, useEffect, useRef } from 'react';

export default function DndPage({ apiFetch, API_URL }) {
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

  // Toast-style transient warnings auto-dismiss after 2.5s so the red border
  // and "Only digits allowed" message don't stick around when the user stops
  // typing into an empty field (no later onChange fires to clear them).
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

  useEffect(() => { fetchNumbers(page); }, [page]);

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
      setAddPhone('');
      setAddSource('');
      fetchNumbers(1);
      setPage(1);
    } catch (e) { setAddError('Failed to add number: ' + e.message); }
  };

  const handleRemove = async (phone) => {
    if (!window.confirm(`Remove ${phone} from DND list?`)) return;
    try {
      await apiFetch(`${API_URL}/dnd/${encodeURIComponent(phone)}`, { method: 'DELETE' });
      fetchNumbers(page);
    } catch (e) { alert('Failed to remove: ' + e.message); }
  };

  const handleCheck = async () => {
    const phone = checkPhone.trim();
    if (!phone) { setCheckResult({ error: 'Phone number is required' }); return; }
    if (!isValidPhone(phone)) { setCheckResult({ error: 'Phone must be exactly 10 digits' }); return; }
    try {
      const res = await apiFetch(`${API_URL}/dnd/check/${encodeURIComponent(phone)}`);
      const data = await res.json();
      setCheckResult(data);
    } catch (e) { setCheckResult({ error: 'Check failed' }); }
  };

  const handleImport = async (e) => {
    const file = e.target.files[0];
    if (!file) return;
    setImporting(true);
    setImportMsg('');
    try {
      const formData = new FormData();
      formData.append('file', file);
      const res = await apiFetch(`${API_URL}/dnd/import`, {
        method: 'POST',
        body: formData,
      });
      const data = await res.json();
      setImportMsg(data.message || `Imported ${data.imported || 0} numbers`);
      fetchNumbers(1);
      setPage(1);
    } catch (e) { setImportMsg('Import failed: ' + e.message); }
    setImporting(false);
    e.target.value = '';
  };

  const sourceBadge = (source) => {
    const colors = {
      manual: { bg: 'rgba(99,102,241,0.15)', color: '#818cf8' },
      ndnc: { bg: 'rgba(239,68,68,0.15)', color: '#fca5a5' },
      customer_request: { bg: 'rgba(245,158,11,0.15)', color: '#fbbf24' },
    };
    const s = colors[source] || { bg: 'rgba(148,163,184,0.15)', color: '#94a3b8' };
    return (
      <span style={{
        padding: '2px 8px', borderRadius: '4px', fontSize: '0.7rem', fontWeight: 600,
        background: s.bg, color: s.color,
      }}>{source || 'manual'}</span>
    );
  };

  const totalPages = Math.ceil(totalCount / perPage);

  return (
    <div className="page-container">
      <h2 style={{ marginBottom: '1.5rem', color: '#e2e8f0' }}>DND Management</h2>

      {/* Top controls */}
      <div className="glass-panel" style={{ padding: '1.5rem', marginBottom: '1.5rem' }}>
        {/* flex-start (not flex-end) so when the Add column grows because of an
            inline error, neighbouring controls don't shift up/down. */}
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '1rem', alignItems: 'flex-start' }}>
          {/* Add Number */}
          <div style={{ flex: '1 1 300px' }}>
            <div style={{ fontSize: '0.75rem', color: '#64748b', textTransform: 'uppercase', letterSpacing: '1px', marginBottom: '6px' }}>Add Number to DND</div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <input
                type="text" inputMode="numeric" maxLength={10}
                placeholder="10-digit phone (e.g. 9876543210)" value={addPhone}
                onChange={e => {
                  const raw = e.target.value;
                  const digits = raw.replace(/\D/g, '').slice(0, 10);
                  setAddPhone(digits);
                  if (raw !== digits && raw.length > digits.length) {
                    flashAddError('Only digits allowed (0-9)');
                  } else if (addError) {
                    setAddError('');
                  }
                }}
                onKeyDown={e => e.key === 'Enter' && handleAdd()}
                style={{ flex: 1, padding: '8px 12px', borderRadius: '6px', border: `1px solid ${addError ? 'rgba(239,68,68,0.5)' : 'rgba(148,163,184,0.2)'}`, background: 'rgba(15,23,42,0.6)', color: '#e2e8f0', fontSize: '0.85rem' }}
              />
              <input
                type="text" placeholder="Source (optional)" value={addSource}
                onChange={e => setAddSource(e.target.value)}
                style={{ width: '140px', padding: '8px 12px', borderRadius: '6px', border: '1px solid rgba(148,163,184,0.2)', background: 'rgba(15,23,42,0.6)', color: '#e2e8f0', fontSize: '0.85rem' }}
              />
              <button className="btn-primary" onClick={handleAdd} style={{ padding: '8px 16px', fontSize: '0.85rem', whiteSpace: 'nowrap' }}>Add</button>
            </div>
            {addError && (
              <div role="alert" aria-live="polite" style={{ fontSize: '0.75rem', color: '#fca5a5', marginTop: '4px', fontWeight: 600 }}>
                {addError}
              </div>
            )}
          </div>

          {/* Import CSV */}
          <div>
            <div style={{ fontSize: '0.75rem', color: '#64748b', textTransform: 'uppercase', letterSpacing: '1px', marginBottom: '6px' }}>Import CSV</div>
            <label style={{
              display: 'inline-block', padding: '8px 16px', borderRadius: '6px', fontSize: '0.85rem', fontWeight: 600,
              background: 'rgba(34,197,94,0.15)', border: '1px solid rgba(34,197,94,0.3)', color: '#4ade80', cursor: 'pointer',
            }}>
              {importing ? 'Importing...' : 'Upload CSV'}
              <input type="file" accept=".csv" onChange={handleImport} style={{ display: 'none' }} disabled={importing} />
            </label>
            {importMsg && <div style={{ fontSize: '0.75rem', color: '#94a3b8', marginTop: '4px' }}>{importMsg}</div>}
          </div>

          {/* Check Number */}
          <div>
            <div style={{ fontSize: '0.75rem', color: '#64748b', textTransform: 'uppercase', letterSpacing: '1px', marginBottom: '6px' }}>Check Number</div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <input
                type="text" inputMode="numeric" maxLength={10}
                placeholder="10-digit phone" value={checkPhone}
                onChange={e => {
                  const raw = e.target.value;
                  const digits = raw.replace(/\D/g, '').slice(0, 10);
                  setCheckPhone(digits);
                  if (raw !== digits && raw.length > digits.length) {
                    flashCheckError('Only digits allowed (0-9)');
                  } else {
                    setCheckResult(null);
                  }
                }}
                onKeyDown={e => e.key === 'Enter' && handleCheck()}
                style={{ width: '180px', padding: '8px 12px', borderRadius: '6px', border: `1px solid ${checkResult && checkResult.error ? 'rgba(239,68,68,0.5)' : 'rgba(148,163,184,0.2)'}`, background: 'rgba(15,23,42,0.6)', color: '#e2e8f0', fontSize: '0.85rem' }}
              />
              <button onClick={handleCheck} style={{
                padding: '8px 16px', borderRadius: '6px', fontSize: '0.85rem', fontWeight: 600, cursor: 'pointer',
                background: 'rgba(99,102,241,0.15)', border: '1px solid rgba(99,102,241,0.3)', color: '#a5b4fc',
              }}>Check</button>
            </div>
            {checkResult && (
              <div aria-live="polite" style={{ fontSize: '0.75rem', marginTop: '4px', fontWeight: 600, color: checkResult.is_dnd ? '#ef4444' : checkResult.error ? '#f59e0b' : '#22c55e' }}>
                {checkResult.error ? checkResult.error : checkResult.is_dnd ? 'On DND list' : 'Not on DND list'}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Count + Table */}
      <div className="glass-panel" style={{ padding: '1.5rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h3 style={{ fontSize: '1rem', color: '#94a3b8', margin: 0 }}>
            DND Numbers <span style={{ fontSize: '0.8rem', color: '#64748b' }}>({totalCount} total)</span>
          </h3>
        </div>

        {loading ? (
          <div style={{ textAlign: 'center', padding: '2rem', color: '#94a3b8' }}>Loading...</div>
        ) : numbers.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '2rem', color: '#64748b' }}>No DND numbers found.</div>
        ) : (
          <>
            <div style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', fontSize: '0.8rem', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid rgba(148,163,184,0.1)' }}>
                    <th style={{ textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600 }}>Phone Number</th>
                    <th style={{ textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600 }}>Source</th>
                    <th style={{ textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600 }}>Added</th>
                    <th style={{ textAlign: 'right', padding: '8px 4px', color: '#64748b', fontWeight: 600 }}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {numbers.map(n => (
                    <tr key={n.phone} style={{ borderBottom: '1px solid rgba(148,163,184,0.06)' }}>
                      <td style={{ padding: '8px 4px', fontFamily: 'SFMono-Regular, Consolas, monospace', color: '#cbd5e1' }}>{n.phone}</td>
                      <td style={{ padding: '8px 4px' }}>{sourceBadge(n.source)}</td>
                      <td style={{ padding: '8px 4px', color: '#94a3b8' }}>{n.created_at ? new Date(n.created_at).toLocaleDateString() : '-'}</td>
                      <td style={{ padding: '8px 4px', textAlign: 'right' }}>
                        <button onClick={() => handleRemove(n.phone)} style={{
                          padding: '4px 10px', borderRadius: '4px', fontSize: '0.7rem', fontWeight: 600, cursor: 'pointer',
                          background: 'rgba(239,68,68,0.15)', border: '1px solid rgba(239,68,68,0.3)', color: '#fca5a5',
                        }}>Remove</button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', gap: '12px', marginTop: '1rem' }}>
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page <= 1}
                  style={{
                    padding: '6px 14px', borderRadius: '6px', fontSize: '0.8rem', fontWeight: 600, cursor: page <= 1 ? 'default' : 'pointer',
                    background: 'rgba(148,163,184,0.1)', border: '1px solid rgba(148,163,184,0.2)', color: page <= 1 ? '#475569' : '#94a3b8',
                  }}>Previous</button>
                <span style={{ fontSize: '0.8rem', color: '#94a3b8' }}>Page {page} of {totalPages}</span>
                <button
                  onClick={() => setPage(p => Math.min(totalPages, p + 1))} disabled={page >= totalPages}
                  style={{
                    padding: '6px 14px', borderRadius: '6px', fontSize: '0.8rem', fontWeight: 600, cursor: page >= totalPages ? 'default' : 'pointer',
                    background: 'rgba(148,163,184,0.1)', border: '1px solid rgba(148,163,184,0.2)', color: page >= totalPages ? '#475569' : '#94a3b8',
                  }}>Next</button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
