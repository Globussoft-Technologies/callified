import React, { useState, useEffect, useCallback, useRef } from 'react';
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

const statusMeta = {
  offline: { label: 'Offline', color: '#9ca3af', bg: 'rgba(156,163,175,0.12)' },
  idle: { label: 'Idle', color: '#10b981', bg: 'rgba(16,185,129,0.12)' },
  break: { label: 'On Break', color: '#f59e0b', bg: 'rgba(245,158,11,0.12)' },
  on_call: { label: 'On Call', color: '#ef4444', bg: 'rgba(239,68,68,0.12)' },
};

function timeAgo(iso) {
  if (!iso) return '—';
  const then = new Date(iso);
  const now = new Date();
  const sec = Math.floor((now - then) / 1000);
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  return `${Math.floor(hr / 24)}d ago`;
}

function durationSince(iso) {
  if (!iso) return '';
  const sec = Math.floor((new Date() - new Date(iso)) / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m`;
  return `${Math.floor(min / 60)}h ${min % 60}m`;
}

function formatDuration(sec) {
  if (!sec || sec <= 0) return '0s';
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  return `${hr}h ${min % 60}m`;
}

export default function AgentPresencePage({ apiFetch, API_URL }) {
  const { currentUser, fetchSseTicket } = useAuth();
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState('all');
  const esRef = useRef(null);

  const fetchPresence = useCallback(async () => {
    try {
      const res = await apiFetch(`${API_URL}/presence`);
      if (!res.ok) throw new Error('Failed to load presence');
      setAgents(await res.json());
    } catch (e) {
      console.error('Presence load failed', e);
    }
    setLoading(false);
  }, [apiFetch, API_URL]);

  useEffect(() => {
    fetchPresence();

    let stopped = false;
    (async () => {
      try {
        const ticket = await fetchSseTicket();
        if (stopped) return;
        const es = new EventSource(`${API_URL}/sse/presence?ticket=${encodeURIComponent(ticket)}`);
        esRef.current = es;
        es.onmessage = (e) => {
          try {
            const data = JSON.parse(e.data);
            if (Array.isArray(data)) setAgents(data);
          } catch { /* ignore */ }
        };
        es.onerror = (e) => { console.error('Presence SSE error', e); };
      } catch (e) { console.error('Presence SSE setup failed', e); }
    })();

    return () => {
      stopped = true;
      if (esRef.current) { esRef.current.close(); esRef.current = null; }
    };
  }, [fetchPresence, fetchSseTicket, API_URL]);

  const filtered = filter === 'all' ? agents : agents.filter(a => a.status === filter);
  const counts = {
    online: agents.filter(a => a.status !== 'offline').length,
    idle: agents.filter(a => a.status === 'idle').length,
    on_call: agents.filter(a => a.status === 'on_call').length,
    break: agents.filter(a => a.status === 'break').length,
    offline: agents.filter(a => a.status === 'offline').length,
  };

  return (
    <div style={{ padding: '24px', maxWidth: 1200, margin: '0 auto', fontFamily: T.font }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ margin: 0, fontSize: 24, fontWeight: 700, color: T.text }}>🟢 Live Agent Status</h1>
        <p style={{ margin: '6px 0 0', color: T.muted, fontSize: '0.9rem' }}>
          Real-time presence for every agent in your organization.
        </p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: 12, marginBottom: 24 }}>
        {[
          { key: 'online', label: 'Online', color: T.green },
          { key: 'idle', label: 'Idle', color: T.green },
          { key: 'on_call', label: 'On Call', color: T.red },
          { key: 'break', label: 'On Break', color: T.amber },
          { key: 'offline', label: 'Offline', color: T.muted },
        ].map(item => (
          <div key={item.key} style={{ ...card, padding: '1rem', textAlign: 'center' }}>
            <div style={{ fontSize: 24, fontWeight: 700, color: item.color }}>{counts[item.key]}</div>
            <div style={{ fontSize: 12, color: T.muted, marginTop: 4 }}>{item.label}</div>
          </div>
        ))}
      </div>

      <div style={{ ...card, padding: '1rem', marginBottom: 20, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {[
          { key: 'all', label: 'All' },
          { key: 'idle', label: 'Idle' },
          { key: 'on_call', label: 'On Call' },
          { key: 'break', label: 'On Break' },
          { key: 'offline', label: 'Offline' },
        ].map(f => (
          <button
            key={f.key}
            onClick={() => setFilter(f.key)}
            style={{
              padding: '6px 14px', borderRadius: 20, border: `1px solid ${T.border}`,
              background: filter === f.key ? T.accent : '#fff', color: filter === f.key ? '#fff' : T.sub,
              cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
            }}
          >
            {f.label}
          </button>
        ))}
      </div>

      {loading ? (
        <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>Loading agents…</div>
      ) : filtered.length === 0 ? (
        <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>No agents match the selected filter.</div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 16 }}>
          {filtered.map(agent => {
            const meta = statusMeta[agent.status] || statusMeta.offline;
            return (
              <div key={agent.user_id} style={{ ...card, padding: '1rem' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 10 }}>
                  <div style={{
                    width: 10, height: 10, borderRadius: '50%', background: meta.color, flexShrink: 0,
                    boxShadow: `0 0 0 4px ${meta.bg}`,
                  }} />
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: 15, fontWeight: 700, color: T.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {agent.full_name || agent.email}
                    </div>
                    <div style={{ fontSize: 12, color: T.muted }}>{agent.email}</div>
                  </div>
                  <span style={{
                    fontSize: 11, fontWeight: 700, padding: '3px 10px', borderRadius: 20,
                    color: meta.color, background: meta.bg,
                  }}>
                    {meta.label}
                  </span>
                </div>
                <div style={{ fontSize: 12, color: T.sub, display: 'flex', flexDirection: 'column', gap: 4 }}>
                  <div>Role: <strong>{agent.role}</strong></div>
                  <div>Total talk time: <strong>{formatDuration(agent.total_talk_time_s)}</strong></div>
                  <div>Total idle time: <strong>{formatDuration(agent.total_idle_time_s)}</strong></div>
                  {agent.status === 'idle' && agent.last_seen_at && (
                    <div>Idle for: <strong>{durationSince(agent.last_seen_at)}</strong></div>
                  )}
                  {agent.status === 'on_call' && agent.on_call_since && (
                    <div>On call for: <strong>{durationSince(agent.on_call_since)}</strong></div>
                  )}
                  {agent.status === 'break' && agent.break_since && (
                    <div>On break for: <strong>{durationSince(agent.break_since)}</strong></div>
                  )}
                  {agent.status === 'offline' && (
                    <div>Last seen: {timeAgo(agent.last_seen_at)}</div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
