import React, { useState, useEffect, useRef, useMemo } from 'react';
import { useAuth } from '../../contexts/AuthContext';

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

const PER_PAGE = 50;
const ACTIVITY_BUFFER = 1000;

function withDate(label, tsMs) {
  const d = new Date(tsMs);
  const dd = String(d.getDate()).padStart(2, '0');
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const yyyy = d.getFullYear();
  const dateStr = `${dd}/${mm}/${yyyy}`;
  if (/\[\d{2}:\d{2}:\d{2}\]/.test(label)) {
    return label.replace(/\[(\d{2}:\d{2}:\d{2})\]/, `[${dateStr} $1]`);
  }
  return `[${dateStr}] ${label}`;
}

function parseActivity(entry) {
  const line = entry.line;
  let parsed = null;
  try {
    const j = JSON.parse(line);
    if (j && typeof j === 'object' && j.label) {
      parsed = {
        tsMs: j.ts ? new Date(j.ts).getTime() : entry.arrivedAt,
        campaignId: j.campaign_id ?? null,
        status: (j.status || '').toUpperCase(),
        leadName: j.lead_name || '',
        phone: j.phone || '',
        label: j.label,
      };
    }
  } catch { /* legacy plain-text */  }
  if (!parsed) {
    const status = (line.match(/—\s*([A-Z][A-Z_-]+)/) || [])[1] || '';
    parsed = {
      tsMs: entry.arrivedAt,
      campaignId: null,
      status,
      leadName: '',
      phone: '',
      label: line,
    };
  }
  parsed.raw = parsed.label.replace(/\s*\(\s*\)/g, '');
  return parsed;
}

export default function LogsTab({ API_URL, apiFetch }) {
  const { fetchSseTicket } = useAuth();
  const [mode, setMode] = useState('activity');
  const [filter, setFilter] = useState('');
  const [paused, setPaused] = useState(false);
  const [activityLogs, setActivityLogs] = useState([]);
  const [statusFilter, setStatusFilter] = useState('');
  const [campaignFilter, setCampaignFilter] = useState('');
  const [dateFrom, setDateFrom] = useState('');
  const [dateTo, setDateTo] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [campaigns, setCampaigns] = useState([]);
  const [confirmClear, setConfirmClear] = useState(false);
  const [streamStatus, setStreamStatus] = useState('connecting');
  const verboseRef = useRef(null);
  const activityEsRef = useRef(null);
  const verboseEsRef = useRef(null);

  useEffect(() => {
    if (!apiFetch) return;
    let cancelled = false;
    apiFetch(`${API_URL}/campaigns`)
      .then(r => r.ok ? r.json() : [])
      .then(list => { if (!cancelled && Array.isArray(list)) setCampaigns(list); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [apiFetch, API_URL]);

  useEffect(() => {
    if (activityEsRef.current) activityEsRef.current.close();
    const cid = campaignFilter || 'all';
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setActivityLogs([]);
    setStreamStatus('connecting');
    let cancelled = false;
    let es = null;
    fetchSseTicket().then(ticket => {
      if (cancelled) return;
      es = new EventSource(`${API_URL}/campaign-events?ticket=${encodeURIComponent(ticket)}&campaign_id=${cid}`);
      es.onopen = () => { if (!cancelled) setStreamStatus('connected'); };
      es.onerror = () => { if (!cancelled) setStreamStatus('error'); };
      es.onmessage = (ev) => {
        if (!paused) {
          setActivityLogs(prev => [...prev.slice(-ACTIVITY_BUFFER), { arrivedAt: Date.now(), line: ev.data }]);
        }
      };
      activityEsRef.current = es;
    }).catch(() => { if (!cancelled) setStreamStatus('error'); });
    return () => { cancelled = true; if (es) es.close(); };
  }, [paused, campaignFilter, API_URL, fetchSseTicket]);

  useEffect(() => {
    if (mode !== 'verbose' || !verboseRef.current) return;
    if (verboseEsRef.current) verboseEsRef.current.close();
    const el = verboseRef.current;
    el.innerHTML = '';
    let cancelled = false;
    let es = null;
    fetchSseTicket().then(ticket => {
      if (cancelled || !verboseRef.current) return;
      es = new EventSource(`${API_URL}/live-logs?ticket=${encodeURIComponent(ticket)}`);
      verboseEsRef.current = es;
      es.onmessage = (ev) => {
        if (paused) return;
        if (filter && !ev.data.toLowerCase().includes(filter.toLowerCase())) return;
        const line = document.createElement('div');
        line.textContent = ev.data;
        line.style.padding = '3px 12px';
        line.style.fontFamily = '"JetBrains Mono", "Fira Code", monospace';
        line.style.fontSize = '0.75rem';
        line.style.borderBottom = '1px solid rgba(255,255,255,0.03)';
        line.style.lineHeight = '1.4';
        if (ev.data.includes('ERROR')) { line.style.color = '#f87171'; line.style.background = 'rgba(239,68,68,0.06)'; }
        else if (ev.data.includes('WARNING')) { line.style.color = '#fbbf24'; }
        else if (ev.data.includes('[STT]')) { line.style.color = '#4ade80'; }
        else if (ev.data.includes('[LLM]')) { line.style.color = '#67e8f9'; }
        else if (ev.data.includes('TTS')) { line.style.color = '#a78bfa'; }
        else if (ev.data.includes('GREETING') || ev.data.includes('RECORDING')) { line.style.color = '#f59e0b'; }
        else if (ev.data.includes('DIAL') || ev.data.includes('EXOTEL')) { line.style.color = '#60a5fa'; }
        else if (ev.data.includes('HANGUP') || ev.data.includes('CLOSED')) { line.style.color = '#fb923c'; }
        else if (ev.data.includes('DEBUG-REC')) { line.style.color = '#22d3ee'; }
        else { line.style.color = '#94a3b8'; }
        el.appendChild(line);
        if (el.children.length > 500) el.removeChild(el.firstChild);
        el.scrollTop = el.scrollHeight;
      };
    }).catch(() => {});
    return () => { cancelled = true; if (es) es.close(); };
  }, [mode, paused, filter, API_URL, fetchSseTicket]);

  const activityIcon = (text) => {
    if (text.includes('📞')) return { bg: 'rgba(99,102,241,0.06)', border: 'rgba(99,102,241,0.2)' };
    if (text.includes('✅') || text.includes('🎯')) return { bg: 'rgba(16,185,129,0.06)', border: 'rgba(16,185,129,0.2)' };
    if (text.includes('❌')) return { bg: 'rgba(245,158,11,0.06)', border: 'rgba(245,158,11,0.2)' };
    if (text.includes('📵') || text.includes('⚠️') || text.includes('💥')) return { bg: 'rgba(239,68,68,0.06)', border: 'rgba(239,68,68,0.2)' };
    if (text.includes('🚀') || text.includes('🏁')) return { bg: 'rgba(139,92,246,0.06)', border: 'rgba(139,92,246,0.2)' };
    return { bg: T.bg, border: T.border };
  };

  const parsedLogs = useMemo(() => activityLogs.map(parseActivity), [activityLogs]);

  const statusOptions = useMemo(() => {
    const s = new Set();
    parsedLogs.forEach(p => { if (p.status) s.add(p.status); });
    return Array.from(s).sort();
  }, [parsedLogs]);

  const campaignOptions = useMemo(() => {
    const seen = new Map();
    campaigns.forEach(c => { if (c && c.id != null) seen.set(String(c.id), c.name || `Campaign #${c.id}`); });
    parsedLogs.forEach(p => {
      if (p.campaignId != null) {
        const k = String(p.campaignId);
        if (!seen.has(k)) seen.set(k, `Campaign #${k}`);
      }
    });
    return Array.from(seen.entries())
      .sort((a, b) => Number(a[0]) - Number(b[0]))
      .map(([id, name]) => ({ id, name }));
  }, [campaigns, parsedLogs]);

  const filteredLogs = useMemo(() => {
    const q = search.trim().toLowerCase();
    const fromMs = dateFrom ? new Date(dateFrom + 'T00:00:00').getTime() : null;
    const toMs = dateTo ? new Date(dateTo + 'T23:59:59.999').getTime() : null;
    return parsedLogs.filter(p => {
      if (statusFilter && p.status !== statusFilter) return false;
      if (fromMs !== null && p.tsMs < fromMs) return false;
      if (toMs !== null && p.tsMs > toMs) return false;
      if (q && !(p.raw + ' ' + p.leadName + ' ' + p.phone).toLowerCase().includes(q)) return false;
      return true;
    });
  }, [parsedLogs, statusFilter, dateFrom, dateTo, search]);

  const reversedLogs = useMemo(() => [...filteredLogs].reverse(), [filteredLogs]);

  const totalPages = Math.max(1, Math.ceil(reversedLogs.length / PER_PAGE));
  const safePage = Math.min(page, totalPages);
  const pageLogs = reversedLogs.slice((safePage - 1) * PER_PAGE, safePage * PER_PAGE);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setPage(1); }, [statusFilter, campaignFilter, dateFrom, dateTo, search]);

  const handleClear = () => {
    setActivityLogs([]);
    if (verboseRef.current) verboseRef.current.innerHTML = '';
  };

  const inputStyle = {
    height: 32, padding: '4px 10px', borderRadius: 8, fontSize: 12,
    border: `1px solid ${T.border}`, background: T.card, color: T.text,
    fontFamily: T.font, outline: 'none', cursor: 'pointer',
  };

  const streamMap = {
    connecting: { color: T.amber, bg: 'rgba(245,158,11,0.1)', dot: T.amber, label: 'Connecting' },
    connected:  { color: T.green, bg: 'rgba(16,185,129,0.1)', dot: T.green, label: 'Live' },
    error:      { color: T.red,   bg: 'rgba(239,68,68,0.1)',  dot: T.red,   label: 'Disconnected' },
  };

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title + controls */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20, flexWrap: 'wrap', gap: 10 }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
            <span style={{ color: T.accent }}>Live</span> Logs
          </h2>
          <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
            Real-time campaign activity feed and verbose system log stream.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>

          {/* Mode toggle */}
          <div style={{ display: 'flex', background: T.bg, borderRadius: 8, padding: 3, gap: 2, border: `1px solid ${T.border}` }}>
            <button onClick={() => setMode('activity')}
              style={{
                padding: '5px 14px', borderRadius: 6, border: 'none', cursor: 'pointer',
                fontSize: 12, fontWeight: 600, fontFamily: T.font,
                background: mode === 'activity' ? T.accent : 'transparent',
                color: mode === 'activity' ? '#fff' : T.muted,
              }}>
              📋 Activity
            </button>
            <button onClick={() => setMode('verbose')}
              style={{
                padding: '5px 14px', borderRadius: 6, border: 'none', cursor: 'pointer',
                fontSize: 12, fontWeight: 600, fontFamily: T.font,
                background: mode === 'verbose' ? T.accent : 'transparent',
                color: mode === 'verbose' ? '#fff' : T.muted,
              }}>
              🔧 Verbose
            </button>
          </div>

          {mode === 'verbose' && (
            <input placeholder="Filter logs..." value={filter}
              onChange={e => setFilter(e.target.value)}
              style={{ ...inputStyle, width: 160 }} />
          )}

          {/* Pause/Live toggle */}
          <button onClick={() => setPaused(!paused)}
            style={{
              padding: '6px 14px', borderRadius: 8, cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
              border: `1px solid ${paused ? 'rgba(239,68,68,0.3)' : 'rgba(16,185,129,0.3)'}`,
              background: paused ? 'rgba(239,68,68,0.08)' : 'rgba(16,185,129,0.08)',
              color: paused ? T.red : T.green,
            }}>
            {paused ? '⏸ Paused' : '▶ Live'}
          </button>

          {/* Clear */}
          {confirmClear ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '4px 10px', borderRadius: 8, background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.2)' }}>
              <span style={{ color: T.amber, fontSize: 12 }}>Clear logs?</span>
              <button onClick={() => { setConfirmClear(false); handleClear(); }}
                style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.3)', color: T.red, borderRadius: 6, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font }}>
                Confirm
              </button>
              <button onClick={() => setConfirmClear(false)}
                style={{ background: 'transparent', border: `1px solid ${T.border}`, color: T.muted, borderRadius: 6, padding: '3px 10px', cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
                Cancel
              </button>
            </div>
          ) : (
            <button onClick={() => setConfirmClear(true)}
              style={{ padding: '6px 12px', borderRadius: 8, border: `1px solid rgba(239,68,68,0.2)`, background: 'rgba(239,68,68,0.06)', color: T.red, cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
              🗑️ Clear
            </button>
          )}
        </div>
      </div>

      {/* Activity filters */}
      {mode === 'activity' && (
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 12, flexWrap: 'wrap' }}>
          <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)} style={{ ...inputStyle, width: 150 }}>
            <option value="">All statuses</option>
            {statusOptions.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
          <select value={campaignFilter} onChange={e => setCampaignFilter(e.target.value)} style={{ ...inputStyle, width: 180 }}>
            <option value="">All campaigns</option>
            {campaignOptions.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
          <input type="date" value={dateFrom} onChange={e => setDateFrom(e.target.value)} title="From date" style={{ ...inputStyle, width: 140 }} />
          <input type="date" value={dateTo} onChange={e => setDateTo(e.target.value)} title="To date" style={{ ...inputStyle, width: 140 }} />
          <input placeholder="Search name or phone..." value={search} onChange={e => setSearch(e.target.value)} style={{ ...inputStyle, width: 200 }} />
          {(statusFilter || campaignFilter || dateFrom || dateTo || search) && (
            <button onClick={() => { setStatusFilter(''); setCampaignFilter(''); setDateFrom(''); setDateTo(''); setSearch(''); }}
              style={{ padding: '4px 10px', borderRadius: 6, border: `1px solid ${T.border}`, background: T.card, color: T.sub, cursor: 'pointer', fontSize: 12, fontFamily: T.font }}>
              Reset
            </button>
          )}
          <span style={{ fontSize: 12, color: T.muted }}>{filteredLogs.length} of {activityLogs.length} events</span>
          {(() => {
            const s = streamMap[streamStatus] || streamMap.connecting;
            return (
              <span style={{
                display: 'inline-flex', alignItems: 'center', gap: 5,
                padding: '3px 10px', borderRadius: 12, fontSize: 11,
                color: s.color, background: s.bg, border: `1px solid ${s.color}40`, fontWeight: 600,
              }}>
                <span style={{ width: 6, height: 6, borderRadius: '50%', background: s.dot }} />
                {s.label}
              </span>
            );
          })()}
        </div>
      )}

      {/* Verbose legend */}
      {mode === 'verbose' && (
        <div style={{ display: 'flex', gap: 12, marginBottom: 10, fontSize: 11, flexWrap: 'wrap', color: T.muted }}>
          <span style={{ color: '#4ade80' }}>● STT</span>
          <span style={{ color: '#67e8f9' }}>● LLM</span>
          <span style={{ color: '#a78bfa' }}>● TTS</span>
          <span style={{ color: '#60a5fa' }}>● DIAL</span>
          <span style={{ color: T.amber }}>● GREETING/REC</span>
          <span style={{ color: '#fb923c' }}>● HANGUP</span>
          <span style={{ color: '#22d3ee' }}>● DEBUG</span>
          <span style={{ color: T.red }}>● ERROR</span>
        </div>
      )}

      {/* Activity log area */}
      {mode === 'activity' && (
        <>
          <div style={{ ...card, height: '62vh', overflowY: 'auto', padding: 8 }}>
            {pageLogs.length === 0 ? (
              <div style={{ textAlign: 'center', color: T.muted, padding: '3rem' }}>
                <div style={{ fontSize: 32, marginBottom: 12 }}>📡</div>
                {activityLogs.length === 0 ? (
                  streamStatus === 'error' ? (
                    <>
                      <div style={{ color: T.red }}>Stream disconnected.</div>
                      <div style={{ fontSize: 12, marginTop: 8 }}>The browser will retry automatically; check your network if this persists.</div>
                    </>
                  ) : streamStatus === 'connected' ? (
                    <>
                      <div>Connected — no recent campaign activity.</div>
                      <div style={{ fontSize: 12, marginTop: 8 }}>The last 7 days of events are replayed on connect; new events appear here in real time.</div>
                    </>
                  ) : (
                    <>
                      <div>Connecting to event stream…</div>
                      <div style={{ fontSize: 12, marginTop: 8 }}>Once connected, recent events replay and new dials stream in live.</div>
                    </>
                  )
                ) : (
                  <div>No events match the current filters.</div>
                )}
              </div>
            ) : (
              pageLogs.map((p, i) => {
                const style = activityIcon(p.raw);
                return (
                  <div key={`${safePage}-${i}`} style={{
                    padding: '8px 12px', marginBottom: 4, borderRadius: 6,
                    background: style.bg, borderLeft: `3px solid ${style.border}`,
                    fontSize: 13, color: T.sub, fontFamily: T.font,
                  }}>
                    {withDate(p.raw, p.tsMs)}
                  </div>
                );
              })
            )}
          </div>

          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', gap: 12, marginTop: 12, fontSize: 13 }}>
            <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={safePage <= 1}
              style={{
                padding: '5px 14px', borderRadius: 8, fontFamily: T.font,
                border: `1px solid ${T.border}`, background: T.card,
                color: safePage <= 1 ? T.muted : T.sub,
                cursor: safePage <= 1 ? 'not-allowed' : 'pointer',
              }}>
              ← Prev
            </button>
            <span style={{ color: T.muted }}>Page {safePage} of {totalPages}</span>
            <button onClick={() => setPage(p => Math.min(totalPages, p + 1))} disabled={safePage >= totalPages}
              style={{
                padding: '5px 14px', borderRadius: 8, fontFamily: T.font,
                border: `1px solid ${T.border}`, background: T.card,
                color: safePage >= totalPages ? T.muted : T.sub,
                cursor: safePage >= totalPages ? 'not-allowed' : 'pointer',
              }}>
              Next →
            </button>
          </div>
        </>
      )}

      {/* Verbose terminal — kept dark intentionally */}
      {mode === 'verbose' && (
        <div ref={verboseRef} style={{
          background: 'rgba(2,6,23,0.95)', border: `1px solid ${T.border}`,
          borderRadius: 12, height: '68vh', overflowY: 'auto', overflowX: 'hidden',
        }} />
      )}
    </div>
  );
}
