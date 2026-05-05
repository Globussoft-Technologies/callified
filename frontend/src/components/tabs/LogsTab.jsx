import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react';

const PAGE_SIZE = 50;

const STATUS_OPTIONS = [
  { value: '', label: 'All Statuses' },
  { value: 'DIALING', label: 'Dialing' },
  { value: 'RINGING', label: 'Ringing' },
  { value: 'CONNECTED', label: 'Connected' },
  { value: 'COMPLETED', label: 'Completed' },
  { value: 'NO-ANSWER', label: 'No Answer' },
  { value: 'BUSY', label: 'Busy' },
  { value: 'VOICEMAIL', label: 'Voicemail' },
  { value: 'RETRY', label: 'Retry' },
  { value: 'FAILED', label: 'Failed' },
  { value: 'HANGUP', label: 'Hangup' },
];

const inp = {
  background: 'rgba(255,255,255,0.05)',
  border: '1px solid rgba(255,255,255,0.1)',
  borderRadius: '6px',
  color: '#e2e8f0',
  fontSize: '0.8rem',
  padding: '5px 10px',
  height: '32px',
  outline: 'none',
};

export default function LogsTab({ API_URL, authToken }) {
  const [mode, setMode]       = useState('activity');
  const [paused, setPaused]   = useState(false);

  // Activity filters
  const [actSearch,   setActSearch]   = useState('');
  const [actStatus,   setActStatus]   = useState('');
  const [actCampaign, setActCampaign] = useState('');
  const [dateFrom,    setDateFrom]    = useState('');
  const [dateTo,      setDateTo]      = useState('');

  const [verboseFilter, setVerboseFilter] = useState('');
  const [page,          setPage]          = useState(0);
  const [activityLogs,  setActivityLogs]  = useState([]); // [{text, ts}]
  const [campaigns,     setCampaigns]     = useState([]);
  const [showClearConfirm, setShowClearConfirm] = useState(false);
  const [clearKey,      setClearKey]      = useState(0); // bumped on clear to force SSE reconnect

  // ── Refs ────────────────────────────────────────────────────────────────────
  const verboseRef        = useRef(null);
  const activityEsRef     = useRef(null);
  const verboseEsRef      = useRef(null);
  // Mirrors of state used inside SSE handlers — avoids stale closures without
  // having to recreate the EventSource every time the value changes.
  const pausedRef         = useRef(paused);
  const verboseFilterRef  = useRef(verboseFilter);

  useEffect(() => { pausedRef.current = paused; }, [paused]);
  useEffect(() => { verboseFilterRef.current = verboseFilter; }, [verboseFilter]);

  // Reset page when filters change
  useEffect(() => { setPage(0); }, [actSearch, actStatus, actCampaign, dateFrom, dateTo]);

  // ── Campaign list for the campaign dropdown ──────────────────────────────
  useEffect(() => {
    fetch(`${API_URL}/campaigns`, {
      headers: { Authorization: `Bearer ${authToken}` },
    })
      .then(r => r.ok ? r.json() : [])
      .then(data => setCampaigns(Array.isArray(data) ? data : []))
      .catch(() => {});
  }, []);

  // ── Activity feed — reconnect whenever the campaign filter changes ───────────
  useEffect(() => {
    setActivityLogs([]); // clear stale events from previous campaign
    const campaignId = actCampaign || 0;
    // `cancelled` prevents the onerror retry from firing after this effect has
    // already been superseded by a new render (campaign changed, clearKey bumped).
    let cancelled = false;
    const connect = () => {
      if (cancelled) return;
      if (activityEsRef.current) activityEsRef.current.abort();
      const ctrl = new AbortController();
      activityEsRef.current = ctrl;
      (async () => {
        try {
          const res = await fetch(`${API_URL}/campaign-events?campaign_id=${campaignId}`, {
            headers: { Authorization: `Bearer ${authToken}` },
            signal: ctrl.signal,
          });
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          const reader = res.body.getReader();
          const dec = new TextDecoder();
          let buf = '';
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            buf += dec.decode(value, { stream: true });
            const lines = buf.split('\n');
            buf = lines.pop();
            for (const line of lines) {
              if (!line.startsWith('data: ')) continue;
              if (pausedRef.current) continue;
              setActivityLogs(prev => [...prev.slice(-500), { text: line.slice(6), ts: new Date() }]);
            }
          }
        } catch (e) {
          if (e.name === 'AbortError') return;
        }
        if (!cancelled) setTimeout(connect, 3000);
      })();
    };
    connect();
    return () => {
      cancelled = true;
      if (activityEsRef.current) { activityEsRef.current.abort(); activityEsRef.current = null; }
    };
  }, [actCampaign, clearKey]); // eslint-disable-line react-hooks/exhaustive-deps

  // ── Verbose feed — reconnect only when switching TO verbose mode ──────────
  useEffect(() => {
    if (mode !== 'verbose') {
      if (verboseEsRef.current) { verboseEsRef.current.abort(); verboseEsRef.current = null; }
      return;
    }
    // Wait one tick for the ref div to be in the DOM
    const tid = setTimeout(() => {
      const el = verboseRef.current;
      if (!el) return;
      if (verboseEsRef.current) verboseEsRef.current.abort();
      el.innerHTML = '';

      const ctrl = new AbortController();
      verboseEsRef.current = ctrl;
      (async () => {
        try {
          const res = await fetch(`${API_URL}/live-logs`, {
            headers: { Authorization: `Bearer ${authToken}` },
            signal: ctrl.signal,
          });
          if (!res.ok) return;
          const reader = res.body.getReader();
          const dec = new TextDecoder();
          let buf = '';
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            buf += dec.decode(value, { stream: true });
            const lines = buf.split('\n');
            buf = lines.pop();
            for (const line of lines) {
              if (!line.startsWith('data: ')) continue;
              const d = line.slice(6);
              if (pausedRef.current) continue;
              if (verboseFilterRef.current && !d.toLowerCase().includes(verboseFilterRef.current.toLowerCase())) continue;
              const row = document.createElement('div');
              row.textContent = d;
              row.style.cssText = 'padding:3px 12px;font-family:"JetBrains Mono","Fira Code",monospace;font-size:0.75rem;border-bottom:1px solid rgba(255,255,255,0.03);line-height:1.4;';
              if      (d.includes('ERROR'))                         { row.style.color = '#f87171'; row.style.background = 'rgba(239,68,68,0.06)'; }
              else if (d.includes('WARNING'))                       { row.style.color = '#fbbf24'; }
              else if (d.includes('[STT]'))                         { row.style.color = '#4ade80'; }
              else if (d.includes('[LLM]'))                         { row.style.color = '#67e8f9'; }
              else if (d.includes('TTS'))                           { row.style.color = '#a78bfa'; }
              else if (d.includes('GREETING')||d.includes('RECORDING')) { row.style.color = '#f59e0b'; }
              else if (d.includes('DIAL')||d.includes('EXOTEL'))   { row.style.color = '#60a5fa'; }
              else if (d.includes('HANGUP')||d.includes('CLOSED')) { row.style.color = '#fb923c'; }
              else if (d.includes('DEBUG-REC'))                    { row.style.color = '#22d3ee'; }
              else                                                  { row.style.color = '#64748b'; }
              el.appendChild(row);
              if (el.children.length > 500) el.removeChild(el.firstChild);
              el.scrollTop = el.scrollHeight;
            }
          }
        } catch (e) { /* AbortError = intentional close */ }
      })();
    }, 0);

    return () => {
      clearTimeout(tid);
      if (verboseEsRef.current) { verboseEsRef.current.abort(); verboseEsRef.current = null; }
    };
  }, [mode]); // only reconnect when mode changes

  // ── Helpers ──────────────────────────────────────────────────────────────
  const activityIcon = useCallback((text) => {
    if (text.includes('📞')) return { bg: 'rgba(96,165,250,0.1)', border: 'rgba(96,165,250,0.2)' };
    if (text.includes('✅') || text.includes('🎯')) return { bg: 'rgba(34,197,94,0.1)', border: 'rgba(34,197,94,0.2)' };
    if (text.includes('❌')) return { bg: 'rgba(245,158,11,0.1)', border: 'rgba(245,158,11,0.2)' };
    if (text.includes('📵') || text.includes('⚠️') || text.includes('💥')) return { bg: 'rgba(239,68,68,0.1)', border: 'rgba(239,68,68,0.2)' };
    if (text.includes('🚀') || text.includes('🏁')) return { bg: 'rgba(139,92,246,0.1)', border: 'rgba(139,92,246,0.2)' };
    return { bg: 'rgba(255,255,255,0.03)', border: 'rgba(255,255,255,0.05)' };
  }, []);

  // ── Filter + reverse (newest first) ──────────────────────────────────────
  const filteredLogs = useMemo(() => {
    const lower    = actSearch.toLowerCase();
    const statusUp = actStatus.toUpperCase();
    return [...activityLogs].reverse().filter(({ text, ts }) => {
      if (lower    && !text.toLowerCase().includes(lower))    return false;
      if (statusUp && !text.toUpperCase().includes(statusUp)) return false;
      if (dateFrom || dateTo) {
        const d = ts.toISOString().slice(0, 10);
        if (dateFrom && d < dateFrom) return false;
        if (dateTo   && d > dateTo)   return false;
      }
      return true;
    });
  }, [activityLogs, actSearch, actStatus, dateFrom, dateTo]);

  const totalPages = Math.max(1, Math.ceil(filteredLogs.length / PAGE_SIZE));
  const safePage   = Math.min(page, totalPages - 1);
  const pagedLogs  = filteredLogs.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE);
  const hasFilters = actSearch || actStatus || actCampaign || dateFrom || dateTo;

  const resetFilters = () => { setActSearch(''); setActStatus(''); setActCampaign(''); setDateFrom(''); setDateTo(''); };
  const confirmClear = async () => {
    // Clear server-side buffers so SSE reconnects start fresh
    const campaignId = actCampaign || 0;
    try {
      await fetch(`${API_URL}/campaign-events/clear?campaign_id=${campaignId}`, {
        method: 'POST', headers: { Authorization: `Bearer ${authToken}` },
      });
    } catch (_) {}
    try {
      await fetch(`${API_URL}/live-logs/clear`, {
        method: 'POST', headers: { Authorization: `Bearer ${authToken}` },
      });
    } catch (_) {}
    // Clear local state
    setActivityLogs([]);
    if (verboseRef.current) verboseRef.current.innerHTML = '';
    setPage(0);
    setShowClearConfirm(false);
    // Bump clearKey → triggers activity-feed useEffect to reconnect against the empty buffer
    setClearKey(k => k + 1);
  };

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div style={{ padding: '1rem' }}>

      {/* Clear Confirmation */}
      {showClearConfirm && (
        <div onClick={() => setShowClearConfirm(false)}
          style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.75)', zIndex: 9999,
            display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <div onClick={e => e.stopPropagation()}
            style={{ background: '#1e293b', border: '1px solid rgba(255,255,255,0.12)', borderRadius: '12px',
              padding: '2rem', maxWidth: '380px', width: '90%', textAlign: 'center',
              boxShadow: '0 25px 60px rgba(0,0,0,0.6)' }}>
            <div style={{ fontSize: '2rem', marginBottom: '12px' }}>🗑️</div>
            <h3 style={{ margin: '0 0 8px', color: '#e2e8f0', fontSize: '1.1rem' }}>Clear Logs?</h3>
            <p style={{ fontSize: '0.85rem', color: '#94a3b8', margin: '0 0 1.5rem', lineHeight: 1.5 }}>
              Clears the live log feed — logs won't reappear on refresh.<br />Call records and transcripts are not deleted.
            </p>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'center' }}>
              <button onClick={() => setShowClearConfirm(false)}
                style={{ padding: '8px 22px', borderRadius: '8px', border: '1px solid rgba(255,255,255,0.1)',
                  background: 'rgba(255,255,255,0.05)', color: '#94a3b8', cursor: 'pointer', fontSize: '0.85rem' }}>
                Cancel
              </button>
              <button onClick={confirmClear}
                style={{ padding: '8px 22px', borderRadius: '8px', border: 'none',
                  background: '#dc2626', color: '#fff', cursor: 'pointer', fontWeight: 700, fontSize: '0.85rem' }}>
                Yes, Clear
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        marginBottom: '1rem', flexWrap: 'wrap', gap: '10px' }}>
        <h2 style={{ margin: 0 }}>📡 System Logs</h2>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap' }}>
          <div style={{ display: 'flex', borderRadius: '8px', overflow: 'hidden', border: '1px solid rgba(255,255,255,0.1)' }}>
            {[['activity','📋 Activity','rgba(34,197,94,0.2)','#22c55e'],
              ['verbose', '🔧 Verbose', 'rgba(96,165,250,0.2)','#60a5fa']].map(([val, label, bg, col]) => (
              <button key={val} onClick={() => setMode(val)}
                style={{ padding: '6px 16px', border: 'none', cursor: 'pointer', fontSize: '0.8rem', fontWeight: 600,
                  background: mode === val ? bg : 'transparent', color: mode === val ? col : '#64748b' }}>
                {label}
              </button>
            ))}
          </div>

          {mode === 'verbose' && (
            <input value={verboseFilter} onChange={e => setVerboseFilter(e.target.value)}
              placeholder="Filter verbose logs..." style={{ ...inp, width: '180px' }} />
          )}

          <button onClick={() => setPaused(p => !p)}
            style={{ padding: '6px 14px', borderRadius: '6px', border: '1px solid rgba(255,255,255,0.1)',
              background: paused ? 'rgba(239,68,68,0.15)' : 'rgba(34,197,94,0.15)',
              color: paused ? '#ef4444' : '#22c55e', cursor: 'pointer', fontSize: '0.8rem', fontWeight: 600 }}>
            {paused ? '⏸ Paused' : '▶ Live'}
          </button>

          <button onClick={() => setShowClearConfirm(true)}
            style={{ padding: '6px 12px', borderRadius: '6px', border: '1px solid rgba(239,68,68,0.25)',
              background: 'rgba(239,68,68,0.1)', color: '#fca5a5', cursor: 'pointer', fontSize: '0.8rem' }}>
            🗑️ Clear
          </button>
        </div>
      </div>

      {/* Activity Filter Bar */}
      {mode === 'activity' && (
        <div style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.07)',
          borderRadius: '8px', padding: '10px 14px', marginBottom: '10px' }}>
          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', alignItems: 'center' }}>
            <input value={actSearch} onChange={e => setActSearch(e.target.value)}
              placeholder="Search name, phone, lead..."
              style={{ ...inp, flex: '1 1 180px', minWidth: '140px' }} />

            <select value={actCampaign} onChange={e => setActCampaign(e.target.value)}
              style={{ ...inp, flex: '0 1 180px', minWidth: '130px', cursor: 'pointer' }}>
              <option value="">All Campaigns</option>
              {campaigns.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>

            <select value={actStatus} onChange={e => setActStatus(e.target.value)}
              style={{ ...inp, flex: '0 1 150px', minWidth: '120px', cursor: 'pointer' }}>
              {STATUS_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
            </select>

            <div style={{ display: 'flex', alignItems: 'center', gap: '4px', flex: '0 0 auto' }}>
              <span style={{ color: '#64748b', fontSize: '0.75rem', whiteSpace: 'nowrap' }}>Date:</span>
              <input type="date" value={dateFrom} onChange={e => setDateFrom(e.target.value)}
                style={{ ...inp, width: '130px', colorScheme: 'dark' }} />
              <span style={{ color: '#64748b' }}>–</span>
              <input type="date" value={dateTo} onChange={e => setDateTo(e.target.value)}
                style={{ ...inp, width: '130px', colorScheme: 'dark' }} />
            </div>

            {hasFilters && (
              <button onClick={resetFilters}
                style={{ ...inp, cursor: 'pointer', padding: '5px 12px',
                  color: '#94a3b8', background: 'rgba(255,255,255,0.05)' }}>
                ✕ Reset
              </button>
            )}

            <span style={{ fontSize: '0.75rem', color: '#475569', marginLeft: 'auto', whiteSpace: 'nowrap' }}>
              {filteredLogs.length}{hasFilters ? ` / ${activityLogs.length}` : ''} event{filteredLogs.length !== 1 ? 's' : ''}
            </span>
          </div>
        </div>
      )}

      {/* Verbose Legend */}
      {mode === 'verbose' && (
        <div style={{ display: 'flex', gap: '12px', marginBottom: '10px', fontSize: '0.7rem', flexWrap: 'wrap' }}>
          {[['#4ade80','STT'],['#67e8f9','LLM'],['#a78bfa','TTS'],['#60a5fa','DIAL'],
            ['#f59e0b','GREETING/REC'],['#fb923c','HANGUP'],['#22d3ee','DEBUG'],['#f87171','ERROR']
          ].map(([c, l]) => <span key={l} style={{ color: c }}>● {l}</span>)}
        </div>
      )}

      {/* Activity Panel */}
      {mode === 'activity' && (
        <>
          <div style={{ background: 'rgba(2,6,23,0.8)', border: '1px solid rgba(255,255,255,0.06)',
            borderRadius: '8px', height: '60vh', overflowY: 'auto', padding: '8px' }}>
            {pagedLogs.length === 0 ? (
              <div style={{ textAlign: 'center', color: '#64748b', padding: '3rem' }}>
                <div style={{ fontSize: '2rem', marginBottom: '12px' }}>📡</div>
                <div>{activityLogs.length === 0 ? 'Waiting for campaign activity...' : 'No events match the current filters.'}</div>
                {activityLogs.length === 0 && (
                  <div style={{ fontSize: '0.8rem', marginTop: '8px' }}>Start a campaign dial to see live events here.</div>
                )}
              </div>
            ) : pagedLogs.map(({ text, ts }, i) => {
              const s = activityIcon(text);
              const dateLabel = ts ? ts.toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' }) : '';
              return (
                <div key={i} style={{ padding: '8px 12px', marginBottom: '4px', borderRadius: '6px',
                  background: s.bg, borderLeft: `3px solid ${s.border}`,
                  fontSize: '0.85rem', color: '#e2e8f0', fontFamily: 'system-ui', display: 'flex', alignItems: 'baseline', gap: '8px' }}>
                  {dateLabel && (
                    <span style={{ flexShrink: 0, fontSize: '0.72rem', color: '#64748b', fontWeight: 500 }}>{dateLabel}</span>
                  )}
                  <span>{text}</span>
                </div>
              );
            })}
          </div>

          {/* Pagination */}
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            marginTop: '10px', fontSize: '0.8rem', flexWrap: 'wrap', gap: '8px' }}>
            <span style={{ color: '#475569' }}>
              {filteredLogs.length === 0 ? 'No events'
                : `Showing ${safePage * PAGE_SIZE + 1}–${Math.min((safePage + 1) * PAGE_SIZE, filteredLogs.length)} of ${filteredLogs.length}`}
            </span>
            {filteredLogs.length > PAGE_SIZE && (
              <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
                {[
                  { label: '⟨⟨', dis: safePage === 0,              act: () => setPage(0) },
                  { label: '← Newer', dis: safePage === 0,         act: () => setPage(p => Math.max(0, p - 1)) },
                  { label: `${safePage + 1} / ${totalPages}`, dis: true, act: null },
                  { label: 'Older →', dis: safePage >= totalPages - 1, act: () => setPage(p => Math.min(totalPages - 1, p + 1)) },
                  { label: '⟩⟩', dis: safePage >= totalPages - 1,  act: () => setPage(totalPages - 1) },
                ].map(({ label, dis, act }) => (
                  <button key={label} onClick={act || undefined} disabled={dis}
                    style={{ padding: '3px 10px', borderRadius: '5px',
                      border: '1px solid rgba(255,255,255,0.1)',
                      background: 'rgba(255,255,255,0.05)',
                      color: dis ? '#334155' : '#94a3b8',
                      cursor: dis ? 'default' : 'pointer', fontSize: '0.78rem' }}>
                    {label}
                  </button>
                ))}
              </div>
            )}
          </div>
        </>
      )}

      {/* Verbose Panel */}
      {mode === 'verbose' && (
        <div ref={verboseRef} style={{ background: 'rgba(2,6,23,0.8)', border: '1px solid rgba(255,255,255,0.06)',
          borderRadius: '8px', height: '70vh', overflowY: 'auto', overflowX: 'hidden' }} />
      )}
    </div>
  );
}
