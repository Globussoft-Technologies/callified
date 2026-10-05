import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/UIContext';
import AppIcon from '../components/common/AppIcon';

const pendingStatuses = new Set(['pending', 'dialing']);

function getStats(campaign) {
  const raw = campaign.stats || {};
  const total = Number(raw.total) || 0;
  const attempted = Math.min(total, Number(raw.called) || 0);
  return { total, attempted, remaining: Math.max(0, total - attempted), qualified: Number(raw.qualified) || 0, booked: Number(raw.appointments) || 0 };
}

function formatDateTime(value) {
  if (!value) return 'Not scheduled';
  const date = new Date(value.includes('T') ? value : value.replace(' ', 'T'));
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
}

function cleanEventLabel(value) {
  return String(value || '').replace(/^\s*(?:📞|✅|🎯|❌|📵|⚠️?|💥|🚀|🏁|ℹ️?)\s*/u, '');
}

function lifecycleFor(campaign, stats, retryCount) {
  if (!stats.total) return { key: 'attention', label: 'Needs leads', tone: 'warning' };
  const status = String(campaign.status || 'active').toLowerCase();
  if (['paused', 'pause'].includes(status)) return { key: 'upcoming', label: 'Paused', tone: 'paused' };
  if (['scheduled', 'draft', 'pending'].includes(status)) return { key: 'upcoming', label: 'Scheduled', tone: 'scheduled' };
  if (['completed', 'complete', 'finished', 'stopped'].includes(status) || (!stats.remaining && !retryCount)) return { key: 'completed', label: 'Completed', tone: 'complete' };
  return { key: 'running', label: 'Running', tone: 'running' };
}

function CampaignRow({ item, onMonitor, onActivity }) {
  const { campaign, stats, outcomes, activeCalls, retries, nextScheduled, latestEvent, lifecycle } = item;
  const progress = stats.total ? Math.min(100, Math.round((stats.attempted / stats.total) * 100)) : 0;
  const answered = (Number(outcomes.connected) || 0) + (Number(outcomes.completed) || 0);
  const connected = activeCalls.length;
  const nextRetry = [...retries].sort((a, b) => new Date(a.retry_time) - new Date(b.retry_time))[0];
  return (
    <article className="campaign-ops-row">
      <div className="campaign-ops-identity">
        <div className="campaign-ops-name">{campaign.name}</div>
        <span className="campaign-ops-lifecycle" data-tone={lifecycle.tone}>{lifecycle.label}</span>
        <span className="campaign-ops-channel">{campaign.channel === 'whatsapp' ? 'WhatsApp' : 'Voice'}</span>
      </div>
      <div className="campaign-ops-progress">
        <div className="campaign-ops-value-line"><strong>{stats.attempted} of {stats.total}</strong><span>{progress}%</span></div>
        <span className="campaign-ops-caption">unique leads attempted</span>
        <div className="campaign-ops-progress-track"><span style={{ width: `${progress}%` }} /></div>
      </div>
      <div className="campaign-ops-live">
        <span className={connected ? 'is-live' : ''}><AppIcon name="phone" /> {connected} connected</span>
        <span title="Pre-connect dialing state is not exposed by the provider API"><AppIcon name="info" /> Dialing not tracked</span>
        <span><AppIcon name="team" /> {stats.remaining} waiting</span>
      </div>
      <div className="campaign-ops-attempts">
        <div><strong>{answered}</strong><span>Answered</span></div><div><strong>{Number(outcomes.unanswered) || 0}</strong><span>No answer</span></div>
        <div><strong>{Number(outcomes.busy) || 0}</strong><span>Busy</span></div><div><strong>{Number(outcomes.failed) || 0}</strong><span>Failed</span></div>
      </div>
      <div className="campaign-ops-conversions"><div><strong>{stats.qualified}</strong><span>Qualified</span></div><div><strong>{stats.booked}</strong><span>Booked</span></div></div>
      <div className="campaign-ops-schedule">
        <span><AppIcon name="retry" /> {retries.length} in retry queue</span>
        <small>{nextRetry ? `Next retry ${formatDateTime(nextRetry.retry_time)}` : `Next call ${formatDateTime(nextScheduled?.scheduled_time)}`}</small>
      </div>
      <div className="campaign-ops-event">
        {latestEvent ? <><span title={cleanEventLabel(latestEvent.label)}>{cleanEventLabel(latestEvent.label)}</span><small>{new Date(latestEvent.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</small></> : <span className="campaign-ops-empty">No recent activity</span>}
      </div>
      <div className="campaign-ops-actions">
        <button className="campaign-ops-monitor" disabled={!connected} onClick={() => onMonitor(campaign.id)} title={connected ? `Monitor ${connected} connected call${connected === 1 ? '' : 's'}` : 'No connected calls'}><AppIcon name="audio" /> Monitor{connected > 1 ? ` (${connected})` : ''}</button>
        <button className="campaign-ops-activity" onClick={() => onActivity(campaign.id)}><AppIcon name="history" /> Open activity</button>
      </div>
    </article>
  );
}

function CampaignGroup({ title, subtitle, tone, items, collapsed = false, onToggle, onMonitor, onActivity }) {
  if (!items.length) return null;
  return (
    <section className="campaign-ops-group" data-tone={tone}>
      <button className="campaign-ops-group-heading" onClick={onToggle} type="button" aria-expanded={!collapsed}>
        <span className="campaign-ops-group-icon"><AppIcon name={tone === 'attention' ? 'warning' : tone === 'completed' ? 'checkCircle' : tone === 'upcoming' ? 'calendar' : 'play'} /></span>
        <span><strong>{title} <small>({items.length})</small></strong><em>{subtitle}</em></span>
        {onToggle && <AppIcon name={collapsed ? 'right' : 'down'} />}
      </button>
      {!collapsed && <div className="campaign-ops-list">{items.map(item => <CampaignRow key={item.campaign.id} item={item} onMonitor={onMonitor} onActivity={onActivity} />)}</div>}
    </section>
  );
}

export default function CampaignProgressPage({ apiFetch, API_URL }) {
  const { fetchSseTicket } = useAuth();
  const toast = useToast();
  const navigate = useNavigate();
  const [campaigns, setCampaigns] = useState([]);
  const [outcomes, setOutcomes] = useState({});
  const [events, setEvents] = useState({});
  const [activeCalls, setActiveCalls] = useState([]);
  const [retries, setRetries] = useState({});
  const [scheduledCalls, setScheduledCalls] = useState([]);
  const [loading, setLoading] = useState(true);
  const [completedOpen, setCompletedOpen] = useState(false);
  const esMapRef = useRef(new Map());

  const fetchCampaigns = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const response = await apiFetch(`${API_URL}/campaigns`);
      if (!response.ok) throw new Error();
      const data = await response.json();
      setCampaigns(Array.isArray(data) ? data : []);
    } catch { if (!silent) toast('Failed to load campaigns'); }
    finally { if (!silent) setLoading(false); }
  }, [apiFetch, API_URL, toast]);

  const fetchLiveData = useCallback(async () => {
    try {
      const response = await apiFetch(`${API_URL}/active-calls`);
      if (response.ok) { const data = await response.json(); setActiveCalls(Array.isArray(data.active_calls) ? data.active_calls : []); }
    } catch { /* retain the last live snapshot */ }
  }, [apiFetch, API_URL]);

  useEffect(() => {
    fetchCampaigns(); fetchLiveData();
    const campaignTimer = window.setInterval(() => fetchCampaigns(true), 15000);
    const liveTimer = window.setInterval(fetchLiveData, 3000);
    return () => { window.clearInterval(campaignTimer); window.clearInterval(liveTimer); };
  }, [fetchCampaigns, fetchLiveData]);

  useEffect(() => {
    let cancelled = false;
    const refreshDetails = async () => {
      try {
        const scheduledResponse = await apiFetch(`${API_URL}/scheduled-calls?status=pending`);
        if (scheduledResponse.ok) {
          const data = await scheduledResponse.json();
          if (!cancelled) setScheduledCalls(Array.isArray(data) ? data : []);
        }
        const details = await Promise.all(campaigns.map(async campaign => {
          const [outcomeResponse, retryResponse] = await Promise.all([apiFetch(`${API_URL}/campaigns/${campaign.id}/call-outcome-stats`), apiFetch(`${API_URL}/campaigns/${campaign.id}/retries`)]);
          return {
            id: campaign.id,
            outcome: outcomeResponse.ok ? await outcomeResponse.json() : null,
            retries: retryResponse.ok ? await retryResponse.json() : null,
          };
        }));
        if (cancelled) return;
        setOutcomes(previous => details.reduce((next, detail) => detail.outcome ? { ...next, [detail.id]: detail.outcome } : next, previous));
        setRetries(previous => details.reduce((next, detail) => detail.retries ? { ...next, [detail.id]: detail.retries.filter(retry => pendingStatuses.has(String(retry.status).toLowerCase())) } : next, previous));
      } catch { /* auxiliary data must not block progress */ }
    };
    if (campaigns.length) refreshDetails();
    const detailsTimer = window.setInterval(refreshDetails, 15000);
    return () => { cancelled = true; window.clearInterval(detailsTimer); };
  }, [campaigns, apiFetch, API_URL]);

  useEffect(() => {
    const streams = esMapRef.current;
    const streamIds = campaigns.filter(c => String(c.status || 'active').toLowerCase() === 'active').map(c => c.id);
    const openStream = async campaignId => {
      if (streams.has(campaignId)) return;
      try {
        const ticket = await fetchSseTicket();
        const stream = new EventSource(`${API_URL}/campaign-events?ticket=${encodeURIComponent(ticket)}&campaign_id=${campaignId}`);
        streams.set(campaignId, stream);
        stream.onmessage = event => {
          let label = event.data; let ts = Date.now();
          try { const parsed = JSON.parse(event.data); label = parsed.label || label; const time = new Date(parsed.ts).getTime(); if (!Number.isNaN(time)) ts = time; } catch { /* legacy event */ }
          setEvents(previous => ({ ...previous, [campaignId]: [...(previous[campaignId] || []).slice(-19), { label, ts }] }));
          fetchCampaigns(true); fetchLiveData();
        };
      } catch { /* reconnect on the next render */ }
    };
    streamIds.forEach(openStream);
    streams.forEach((stream, id) => { if (!streamIds.includes(id)) { stream.close(); streams.delete(id); } });
    return () => { streams.forEach(stream => stream.close()); streams.clear(); };
  }, [campaigns, fetchSseTicket, API_URL, fetchCampaigns, fetchLiveData]);

  const grouped = useMemo(() => {
    const groups = { running: [], upcoming: [], attention: [], completed: [] };
    campaigns.forEach(campaign => {
      const stats = getStats(campaign); const campaignRetries = retries[campaign.id] || []; const lifecycle = lifecycleFor(campaign, stats, campaignRetries.length);
      groups[lifecycle.key].push({ campaign, stats, lifecycle, retries: campaignRetries, outcomes: outcomes[campaign.id] || {}, activeCalls: activeCalls.filter(call => Number(call.campaign_id) === Number(campaign.id)), nextScheduled: scheduledCalls.filter(call => Number(call.campaign_id) === Number(campaign.id)).sort((a, b) => new Date(a.scheduled_time) - new Date(b.scheduled_time))[0], latestEvent: (events[campaign.id] || []).at(-1) });
    });
    return groups;
  }, [campaigns, retries, outcomes, activeCalls, scheduledCalls, events]);

  const monitor = id => navigate(`/monitor?campaign_id=${id}`);
  const activity = id => navigate(`/logs?campaign_id=${id}`);
  return (
    <main className="campaign-ops-page">
      <header className="campaign-ops-header"><div><h1><AppIcon name="chart" /> Real-Time Campaign Progress</h1><p>Live calling operations across every campaign.</p></div><div className="campaign-ops-connection"><span /> Live <small>Updated automatically</small></div></header>
      <section className="campaign-ops-summary" aria-label="Campaign operations summary">
        <div><AppIcon name="play" /><span><small>Running</small><strong>{grouped.running.length}</strong></span></div>
        <div><AppIcon name="phone" /><span><small>Connected calls</small><strong>{activeCalls.length}</strong></span></div>
        <div><AppIcon name="team" /><span><small>Waiting leads</small><strong>{grouped.running.reduce((sum, item) => sum + item.stats.remaining, 0)}</strong></span></div>
        <div><AppIcon name="warning" /><span><small>Needs attention</small><strong>{grouped.attention.length}</strong></span></div>
      </section>
      {loading && !campaigns.length ? <div className="campaign-ops-state">Loading campaigns...</div> : !campaigns.length ? <div className="campaign-ops-state">No campaigns yet.</div> : <>
        <CampaignGroup title="Running campaigns" subtitle="Campaigns currently processing leads" tone="running" items={grouped.running} onMonitor={monitor} onActivity={activity} />
        <CampaignGroup title="Paused or scheduled" subtitle="Campaigns waiting to resume or start" tone="upcoming" items={grouped.upcoming} onMonitor={monitor} onActivity={activity} />
        <CampaignGroup title="Needs attention" subtitle="Campaigns that require input before calling" tone="attention" items={grouped.attention} onMonitor={monitor} onActivity={activity} />
        <CampaignGroup title="Completed campaigns" subtitle="Campaigns with all leads processed" tone="completed" items={grouped.completed} collapsed={!completedOpen} onToggle={() => setCompletedOpen(open => !open)} onMonitor={monitor} onActivity={activity} />
      </>}
    </main>
  );
}
