import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/UIContext';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const cardStyle = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
  padding: '20px',
};

function formatDuration(sec) {
  if (!sec || sec <= 0) return '0s';
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  return `${hr}h ${min % 60}m`;
}

function getStats(campaign) {
  const s = campaign.stats || {};
  const total = s.total || 0;
  const called = s.called || 0;
  return {
    total,
    called,
    remaining: Math.max(0, total - called),
    qualified: s.qualified || 0,
    booked: s.appointments || 0,
  };
}

export default function CampaignProgressPage({ apiFetch, API_URL }) {
  const { fetchSseTicket } = useAuth();
  const toast = useToast();
  const [campaigns, setCampaigns] = useState([]);
  const [outcomes, setOutcomes] = useState({});
  const [events, setEvents] = useState({});
  const [loading, setLoading] = useState(true);
  const esMapRef = useRef(new Map());

  const fetchCampaigns = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/campaigns`);
      if (!res.ok) throw new Error('Failed to load campaigns');
      const data = await res.json();
      const list = Array.isArray(data) ? data : [];
      setCampaigns(prev => {
        // Preserve live event buffers when refreshing stats.
        const merged = list.map(c => {
          const existing = prev.find(p => p.id === c.id);
          return existing ? { ...c, _fetchedAt: Date.now() } : c;
        });
        return merged;
      });
    } catch (e) {
      if (!silent) toast('Failed to load campaigns');
    }
    if (!silent) setLoading(false);
  }, [apiFetch, API_URL, toast]);

  const fetchOutcomes = useCallback(async (campaignId) => {
    try {
      const res = await apiFetch(`${API_URL}/campaigns/${campaignId}/call-outcome-stats`);
      if (!res.ok) return;
      const data = await res.json();
      setOutcomes(prev => ({ ...prev, [campaignId]: data }));
    } catch { /* ignore */ }
  }, [apiFetch, API_URL]);

  useEffect(() => {
    fetchCampaigns();
    const id = setInterval(() => fetchCampaigns(true), 15000);
    return () => clearInterval(id);
  }, [fetchCampaigns]);

  // Fetch outcome stats once per campaign when the list loads.
  useEffect(() => {
    campaigns.forEach(c => {
      if (!outcomes[c.id]) fetchOutcomes(c.id);
    });
  }, [campaigns, fetchOutcomes, outcomes]);

  // Open/close SSE streams for active campaigns.
  useEffect(() => {
    const activeIds = campaigns
      .filter(c => (c.status || 'active') === 'active')
      .map(c => c.id);

    const openStream = async (campaignId) => {
      if (esMapRef.current.has(campaignId)) return;
      try {
        const ticket = await fetchSseTicket();
        const es = new EventSource(`${API_URL}/campaign-events?ticket=${encodeURIComponent(ticket)}&campaign_id=${campaignId}`);
        esMapRef.current.set(campaignId, es);
        es.onmessage = (e) => {
          let label = e.data;
          let ts = Date.now();
          try {
            const j = JSON.parse(e.data);
            if (j && typeof j.label === 'string') label = j.label;
            if (j && j.ts) {
              const parsed = new Date(j.ts).getTime();
              if (!Number.isNaN(parsed)) ts = parsed;
            }
          } catch { /* plain-text legacy event */ }
          setEvents(prev => {
            const list = prev[campaignId] || [];
            return { ...prev, [campaignId]: [...list.slice(-19), { ts, label }] };
          });
          // Refresh campaign stats after a live event so progress bars move.
          fetchCampaigns(true);
        };
        es.onerror = () => { /* EventSource auto-reconnects */ };
      } catch { /* ignore */ }
    };

    activeIds.forEach(openStream);

    // Close streams for campaigns that are no longer active or no longer in the list.
    esMapRef.current.forEach((es, id) => {
      if (!activeIds.includes(id)) {
        es.close();
        esMapRef.current.delete(id);
      }
    });

    return () => {
      esMapRef.current.forEach(es => es.close());
      esMapRef.current.clear();
    };
  }, [campaigns, fetchSseTicket, API_URL, fetchCampaigns]);

  const sortedCampaigns = [...campaigns].sort((a, b) => {
    // Active first, then by created_at desc.
    const aActive = (a.status || 'active') === 'active' ? 1 : 0;
    const bActive = (b.status || 'active') === 'active' ? 1 : 0;
    if (aActive !== bActive) return bActive - aActive;
    return new Date(b.created_at || 0) - new Date(a.created_at || 0);
  });

  return (
    <div style={{ padding: '24px', maxWidth: 1400, margin: '0 auto', fontFamily: T.font, background: T.bg, minHeight: 'calc(100vh - 56px)' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 24, fontWeight: 700, color: T.text }}>📊 Real-Time Campaign Progress</h1>
          <p style={{ margin: '6px 0 0', color: T.muted, fontSize: 13 }}>
            Live progress, outcomes, and events for every campaign.
          </p>
        </div>
        <div style={{ fontSize: 12, color: T.muted }}>
          {campaigns.filter(c => (c.status || 'active') === 'active').length} active
        </div>
      </div>

      {loading && campaigns.length === 0 ? (
        <div style={{ ...cardStyle, padding: '3rem', textAlign: 'center', color: T.muted }}>Loading campaigns…</div>
      ) : campaigns.length === 0 ? (
        <div style={{ ...cardStyle, padding: '3rem', textAlign: 'center', color: T.muted }}>No campaigns yet.</div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(360px, 1fr))', gap: 16 }}>
          {sortedCampaigns.map(campaign => {
            const stats = getStats(campaign);
            const calledPct = stats.total > 0 ? Math.round((stats.called / stats.total) * 100) : 0;
            const isActive = (campaign.status || 'active') === 'active';
            const out = outcomes[campaign.id] || {};
            const campEvents = events[campaign.id] || [];
            const typeColor = campaign.channel === 'whatsapp' ? '#25D366' : T.accent;
            return (
              <div key={campaign.id} style={cardStyle}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 12 }}>
                  <div>
                    <div style={{ fontSize: 16, fontWeight: 700, color: T.text }}>{campaign.name}</div>
                    <div style={{ display: 'flex', gap: 8, marginTop: 6 }}>
                      <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: typeColor, background: `${typeColor}18` }}>
                        {campaign.channel === 'whatsapp' ? 'WhatsApp' : 'Voice'}
                      </span>
                      <span style={{ fontSize: 11, fontWeight: 600, padding: '2px 10px', borderRadius: 20, color: isActive ? T.green : T.muted, background: isActive ? 'rgba(16,185,129,0.1)' : 'rgba(156,163,175,0.12)' }}>
                        {isActive ? 'Active' : (campaign.status || 'paused')}
                      </span>
                    </div>
                  </div>
                  <div style={{ fontSize: 28, fontWeight: 800, color: T.accent, fontFamily: T.mono }}>{calledPct}%</div>
                </div>

                <div style={{ height: 8, background: '#e5e7eb', borderRadius: 4, overflow: 'hidden', marginBottom: 16 }}>
                  <div style={{ height: '100%', width: `${calledPct}%`, background: 'linear-gradient(90deg, #6366f1, #ec4899)', borderRadius: 4, transition: 'width 0.5s' }} />
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 8, marginBottom: 16 }}>
                  {[
                    { label: 'Total', val: stats.total },
                    { label: 'Called', val: stats.called },
                    { label: 'Remaining', val: stats.remaining },
                    { label: 'Qualified', val: stats.qualified },
                    { label: 'Booked', val: stats.booked },
                  ].map(({ label, val }) => (
                    <div key={label} style={{ textAlign: 'center' }}>
                      <div style={{ fontSize: 16, fontWeight: 700, color: val === 0 ? T.muted : T.text, fontFamily: T.mono }}>{val}</div>
                      <div style={{ fontSize: 10, color: T.muted, textTransform: 'uppercase', letterSpacing: 0.5 }}>{label}</div>
                    </div>
                  ))}
                </div>

                <div style={{ background: '#f8fafc', border: `1px solid ${T.border}`, borderRadius: 10, padding: '10px 12px', marginBottom: 14 }}>
                  <div style={{ fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: 8 }}>Call Outcomes</div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: T.sub }}>
                    <span>Connected: <strong>{out.connected || 0}</strong></span>
                    <span>Completed: <strong>{out.completed || 0}</strong></span>
                    <span>Unanswered: <strong>{out.unanswered || 0}</strong></span>
                    <span>Busy: <strong>{out.busy || 0}</strong></span>
                    <span>Failed: <strong>{out.failed || 0}</strong></span>
                  </div>
                </div>

                <div style={{ background: '#f8fafc', border: `1px solid ${T.border}`, borderRadius: 10, padding: '10px 12px', minHeight: 90 }}>
                  <div style={{ fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: 8 }}>Live Events</div>
                  {campEvents.length === 0 ? (
                    <div style={{ fontSize: 12, color: T.muted }}>Waiting for events…</div>
                  ) : (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 6, maxHeight: 120, overflowY: 'auto' }}>
                      {campEvents.slice().reverse().map((ev, i) => (
                        <div key={i} style={{ fontSize: 12, color: T.sub, display: 'flex', gap: 8 }}>
                          <span style={{ color: T.muted, fontFamily: T.mono, fontSize: 11 }}>{new Date(ev.ts).toLocaleTimeString()}</span>
                          <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{ev.label}</span>
                        </div>
                      ))}
                    </div>
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
