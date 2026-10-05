import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { formatDateTime } from '../utils/dateFormat';

const emptyFilters = { from: '', to: '', campaignId: '', agentId: '', disposition: '', search: '' };

function durationLabel(value) {
  const total = Math.max(0, Math.round(Number(value) || 0));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  return hours > 0
    ? `${hours}h ${String(minutes).padStart(2, '0')}m`
    : `${minutes}m ${String(seconds).padStart(2, '0')}s`;
}

function makeQuery(filters, page, limit) {
  const query = new URLSearchParams({ page: String(page), limit: String(limit) });
  if (filters.from) query.set('from', filters.from);
  if (filters.to) query.set('to', filters.to);
  if (filters.campaignId) query.set('campaign_ids', filters.campaignId);
  if (filters.agentId) query.set('agent_ids', filters.agentId);
  if (filters.disposition) query.set('dispositions', filters.disposition);
  if (filters.search.trim()) query.set('search', filters.search.trim());
  return query;
}

function DispositionBadge({ label, source }) {
  return (
    <span className="lcsr-disposition">
      {label || 'Not available'}
      {source === 'human' && <span className="lcsr-reviewed">Reviewed</span>}
    </span>
  );
}

export default function LeadCallStatusReportPage({ apiFetch, API_URL, campaigns = [], orgTimezone }) {
  const [filters, setFilters] = useState(emptyFilters);
  const [appliedFilters, setAppliedFilters] = useState(emptyFilters);
  const [options, setOptions] = useState({ agents: [], dispositions: [] });
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [detail, setDetail] = useState(null);
  const [history, setHistory] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const limit = 50;

  useEffect(() => {
    apiFetch(`${API_URL}/reports/lead-call-status/options`)
      .then(async response => {
        if (!response.ok) {
          const body = await response.json();
          throw new Error(body.error || body.detail || 'Could not load filters');
        }
        return response.json();
      })
      .then(data => setOptions({ agents: data.agents || [], dispositions: data.dispositions || [] }))
      .catch(() => setOptions({ agents: [], dispositions: [] }));
  }, [apiFetch, API_URL]);

  const fetchReport = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const query = makeQuery(appliedFilters, page, limit);
      const response = await apiFetch(`${API_URL}/reports/lead-call-status?${query}`);
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || data.detail || 'Could not load report');
      setItems(data.items || []);
      setTotal(data.total || 0);
    } catch (err) {
      setError(err.message || 'Could not load report');
      setItems([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [apiFetch, API_URL, appliedFilters, page]);

  useEffect(() => { fetchReport(); }, [fetchReport]);

  const summary = useMemo(() => items.reduce((result, item) => ({
    calls: result.calls + Number(item.call_count || 0),
    duration: result.duration + Number(item.total_duration_s || 0),
    reviewed: result.reviewed + (item.disposition_source === 'human' ? 1 : 0),
  }), { calls: 0, duration: 0, reviewed: 0 }), [items]);

  const applyFilters = event => {
    event.preventDefault();
    setPage(1);
    setAppliedFilters({ ...filters });
  };

  const clearFilters = () => {
    setFilters(emptyFilters);
    setAppliedFilters(emptyFilters);
    setPage(1);
  };

  const openDetail = async item => {
    setDetail(item);
    setHistory([]);
    setHistoryLoading(true);
    try {
      const query = makeQuery(appliedFilters, 1, 500);
      const response = await apiFetch(`${API_URL}/leads/${item.lead_id}/call-status-history?${query}`);
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || data.detail || 'Could not load call history');
      setHistory(data.items || []);
    } catch (err) {
      setHistory([{ error: err.message || 'Could not load call history' }]);
    } finally {
      setHistoryLoading(false);
    }
  };

  const download = async format => {
    const query = makeQuery(appliedFilters, 1, 1);
    query.delete('page');
    query.delete('limit');
    query.set('format', format);
    const response = await apiFetch(`${API_URL}/reports/lead-call-status/export?${query}`);
    if (!response.ok) {
      setError('Could not download the report');
      return;
    }
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `lead-call-status.${format}`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  };

  const pageCount = Math.max(1, Math.ceil(total / limit));

  return (
    <section className="lcsr-page">
      <div className="lcsr-heading">
        <div>
          <h1>Lead Call Status</h1>
          <p>Latest outcome and call activity for each lead</p>
        </div>
        <div className="lcsr-actions">
          <button type="button" className="lcsr-button secondary" onClick={() => download('csv')}>Export CSV</button>
          <button type="button" className="lcsr-button primary" onClick={() => download('xlsx')}>Export Excel</button>
        </div>
      </div>

      <form className="lcsr-filters" onSubmit={applyFilters}>
        <label><span>From</span><input type="date" value={filters.from} onChange={event => setFilters(current => ({ ...current, from: event.target.value }))} /></label>
        <label><span>To</span><input type="date" value={filters.to} onChange={event => setFilters(current => ({ ...current, to: event.target.value }))} /></label>
        <label><span>Campaign</span><select value={filters.campaignId} onChange={event => setFilters(current => ({ ...current, campaignId: event.target.value }))}><option value="">All campaigns</option>{campaigns.map(campaign => <option key={campaign.id} value={campaign.id}>{campaign.name}</option>)}</select></label>
        <label><span>Agent</span><select value={filters.agentId} onChange={event => setFilters(current => ({ ...current, agentId: event.target.value }))}><option value="">All agents</option>{options.agents.map(agent => <option key={agent.id} value={agent.id}>{agent.name}</option>)}</select></label>
        <label><span>Disposition</span><select value={filters.disposition} onChange={event => setFilters(current => ({ ...current, disposition: event.target.value }))}><option value="">All dispositions</option>{options.dispositions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
        <label className="lcsr-search"><span>Lead</span><input type="search" placeholder="Name, phone or company" value={filters.search} onChange={event => setFilters(current => ({ ...current, search: event.target.value }))} /></label>
        <div className="lcsr-filter-actions"><button type="button" className="lcsr-button secondary" onClick={clearFilters}>Clear</button><button className="lcsr-button primary" type="submit">Apply</button></div>
      </form>

      <div className="lcsr-metrics">
        <div><span>Leads in view</span><strong>{total.toLocaleString()}</strong></div>
        <div><span>Calls on this page</span><strong>{summary.calls.toLocaleString()}</strong></div>
        <div><span>Total talk time</span><strong>{durationLabel(summary.duration)}</strong></div>
        <div><span>Human reviewed</span><strong>{summary.reviewed.toLocaleString()}</strong></div>
      </div>

      <div className="lcsr-table-shell">
        {error && <div className="lcsr-error">{error}</div>}
        <div className="lcsr-table-scroll">
          <table className="lcsr-table">
            <thead><tr><th>Lead</th><th>Campaign</th><th>Last call</th><th>Disposition</th><th>AI summary</th><th>Agent</th><th>Calls</th><th>Duration</th></tr></thead>
            <tbody>
              {!loading && items.map(item => (
                <tr key={item.lead_id} onClick={() => openDetail(item)} tabIndex="0" onKeyDown={event => { if (event.key === 'Enter') openDetail(item); }}>
                  <td><strong>{item.lead_name}</strong><small>{item.phone}</small></td>
                  <td>{item.campaign_name}</td>
                  <td>{formatDateTime(item.last_call_date, orgTimezone)}</td>
                  <td><DispositionBadge label={item.disposition_label} source={item.disposition_source} /></td>
                  <td><span className="lcsr-summary" title={item.ai_summary}>{item.ai_summary || 'Not available'}</span></td>
                  <td>{item.agent_name}</td>
                  <td className="numeric">{item.call_count}</td>
                  <td className="numeric">{durationLabel(item.total_duration_s)}</td>
                </tr>
              ))}
              {loading && <tr><td colSpan="8" className="lcsr-empty">Loading report...</td></tr>}
              {!loading && !items.length && <tr><td colSpan="8" className="lcsr-empty">No call dispositions match these filters.</td></tr>}
            </tbody>
          </table>
        </div>
        <div className="lcsr-pagination">
          <span>{total ? `${(page - 1) * limit + 1}-${Math.min(page * limit, total)} of ${total}` : '0 results'}</span>
          <div><button type="button" disabled={page === 1} onClick={() => setPage(value => value - 1)}>Previous</button><span>Page {page} of {pageCount}</span><button type="button" disabled={page >= pageCount} onClick={() => setPage(value => value + 1)}>Next</button></div>
        </div>
      </div>

      {detail && (
        <div className="lcsr-modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setDetail(null); }}>
          <aside className="lcsr-detail" role="dialog" aria-modal="true" aria-label={`${detail.lead_name} call history`}>
            <header><div><h2>{detail.lead_name}</h2><p>{detail.phone} | {detail.call_count} calls</p></div><button type="button" onClick={() => setDetail(null)} aria-label="Close">x</button></header>
            <div className="lcsr-detail-body">
              {historyLoading && <div className="lcsr-empty">Loading call history...</div>}
              {!historyLoading && history.map((call, index) => call.error ? <div className="lcsr-error" key="error">{call.error}</div> : (
                <article className="lcsr-call" key={call.disposition_id || index}>
                  <div className="lcsr-call-top"><DispositionBadge label={call.disposition_label} source={call.disposition_source} /><time>{formatDateTime(call.call_date, orgTimezone)}</time></div>
                  <p>{call.summary || 'No AI summary is available for this call.'}</p>
                  <dl><div><dt>Campaign</dt><dd>{call.campaign_name}</dd></div><div><dt>Agent</dt><dd>{call.agent_name}</dd></div><div><dt>Duration</dt><dd>{durationLabel(call.duration_s)}</dd></div><div><dt>Sentiment</dt><dd>{call.sentiment || 'Not available'}</dd></div></dl>
                  {call.objections && <div className="lcsr-call-note"><strong>Objections</strong><span>{call.objections}</span></div>}
                  {call.next_action && <div className="lcsr-call-note"><strong>Next action</strong><span>{call.next_action}</span></div>}
                </article>
              ))}
            </div>
          </aside>
        </div>
      )}
    </section>
  );
}
