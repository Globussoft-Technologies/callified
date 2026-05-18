import React, { useState, useEffect } from 'react';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', pink: '#ec4899',
  text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const thStyle = {
  padding: '0 0 12px', textAlign: 'left', fontSize: 13, fontWeight: 800,
  color: T.text, textTransform: 'uppercase', letterSpacing: '0.06em',
  borderBottom: `2px solid ${T.border}`,
};
const tdStyle = {
  padding: '13px 0', fontSize: 13, color: T.sub,
  borderBottom: `1px solid ${T.border}`, verticalAlign: 'middle',
};

const LANG_NAMES = {
  hi: 'Hindi', bn: 'Bengali', mr: 'Marathi', en: 'English',
  ta: 'Tamil', te: 'Telugu', kn: 'Kannada', ml: 'Malayalam',
  gu: 'Gujarati', pa: 'Punjabi', or: 'Odia', as: 'Assamese',
};

function ScoreBadge({ score }) {
  const color = score >= 4 ? T.green : score >= 3 ? T.amber : score > 0 ? T.red : T.muted;
  return <span style={{ color, fontWeight: 700, fontFamily: T.mono }}>{score > 0 ? score.toFixed(1) : '—'}</span>;
}

export default function AnalyticsPage({ apiFetch, API_URL }) {
  const [data, setData]       = useState(null);
  const [langData, setLangData] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    (async () => {
      try {
        const [dashRes, langRes] = await Promise.all([
          apiFetch(`${API_URL}/analytics/dashboard`),
          apiFetch(`${API_URL}/analytics/languages`),
        ]);
        const dash = await dashRes.json();
        const lang = await langRes.json();
        setData(dash && typeof dash === 'object' && !Array.isArray(dash) ? dash : null);
        setLangData(Array.isArray(lang) ? lang : []);
      } catch (e) { console.error('Failed to load analytics', e); }
      finally { setLoading(false); }
    })();
  }, []);

  if (loading) return <div style={{ padding: '3rem', textAlign: 'center', color: T.muted, fontFamily: T.font }}>Loading analytics…</div>;
  if (!data)   return <div style={{ padding: '3rem', textAlign: 'center', color: T.muted, fontFamily: T.font }}>Failed to load analytics data.</div>;

  const dailyCalls         = data.daily_calls          || [];
  const sentimentBreakdown = data.sentiment_breakdown  || { positive: 0, neutral: 0, negative: 0 };
  const campaignPerformance = data.campaign_performance || [];
  const topFailureReasons  = data.top_failure_reasons  || [];

  const maxDaily      = Math.max(...dailyCalls.map(d => d.count), 1);
  const sentimentTotal = (sentimentBreakdown.positive + sentimentBreakdown.neutral + sentimentBreakdown.negative) || 1;

  const pickupRate = Math.round((data.pickup_rate || 0) * 100);
  const apptRate   = Math.round((data.appointment_rate || 0) * 100);

  const statCards = [
    { label: 'TOTAL CALLS',      value: data.total_calls || 0,                       color: T.text,  suffix: '' },
    { label: 'CALLS TODAY',      value: data.calls_today || 0,                       color: T.text,  suffix: '' },
    { label: 'PICKUP RATE',      value: pickupRate,                                  color: pickupRate >= 50 ? T.green : T.red, suffix: '%' },
    { label: 'APPOINTMENT RATE', value: apptRate,                                    color: apptRate >= 20 ? T.green : T.amber, suffix: '%' },
    { label: 'AVG DURATION',     value: Math.round(data.avg_call_duration_sec || 0), color: T.accent, suffix: 's' },
    { label: 'THIS WEEK',        value: data.calls_this_week || 0,                   color: T.text,  suffix: '' },
  ];

  const handleExportCSV = async () => {
    try {
      const res = await apiFetch(`${API_URL}/analytics/export/csv`);
      const blob = await res.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = res.headers.get('content-disposition')?.split('filename=')[1] || 'callified_report.csv';
      document.body.appendChild(a); a.click(); a.remove();
      window.URL.revokeObjectURL(url);
    } catch (e) { console.error('CSV export failed', e); }
  };

  const handleExportReport = async () => {
    try {
      const res = await apiFetch(`${API_URL}/analytics/export/report`);
      const html = await res.text();
      const win = window.open('', '_blank');
      win.document.write(html); win.document.close();
    } catch (e) { console.error('Report export failed', e); }
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 24 }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
            <span style={{ color: T.amber }}>Analytics</span> Dashboard
          </h2>
          <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>Real-time metrics from your AI dialer campaigns.</p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={handleExportCSV} style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '8px 16px', borderRadius: 8, fontSize: 13, fontWeight: 600, cursor: 'pointer', fontFamily: T.font,
            background: T.card, border: `1px solid ${T.border}`, color: T.sub,
          }}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
            Export CSV
          </button>
          <button onClick={handleExportReport} style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '8px 16px', borderRadius: 8, fontSize: 13, fontWeight: 600, cursor: 'pointer', fontFamily: T.font,
            background: T.accent, border: 'none', color: '#fff',
          }}>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
            Export Report
          </button>
        </div>
      </div>

      {/* Stat cards — 4 top row, 2 second row */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 12 }}>
        {statCards.slice(0, 4).map(s => (
          <div key={s.label} style={{ ...card, padding: '18px 22px' }}>
            <div style={{ fontSize: 12, fontWeight: 800, color: T.text, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 10 }}>{s.label}</div>
            <div style={{ fontSize: 30, fontWeight: 700, fontFamily: T.mono, color: s.color, lineHeight: 1 }}>{s.value}{s.suffix}</div>
          </div>
        ))}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 20 }}>
        {statCards.slice(4).map(s => (
          <div key={s.label} style={{ ...card, padding: '18px 22px' }}>
            <div style={{ fontSize: 12, fontWeight: 800, color: T.text, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 10 }}>{s.label}</div>
            <div style={{ fontSize: 30, fontWeight: 700, fontFamily: T.mono, color: s.color, lineHeight: 1 }}>{s.value}{s.suffix}</div>
          </div>
        ))}
      </div>

      {/* Charts row */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 300px', gap: 12, marginBottom: 16 }}>

        {/* Daily Calls Bar Chart */}
        <div style={{ ...card, padding: '20px 24px' }}>
          <div style={{ fontSize: 12, fontWeight: 800, color: T.text, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 20 }}>
            Daily Calls (Last 7 Days)
          </div>
          <div style={{ display: 'flex', alignItems: 'flex-end', gap: 8, height: 120 }}>
            {dailyCalls.map((d, i) => {
              const pct = Math.max(4, (d.count / maxDaily) * 100);
              const dt  = new Date(d.date + 'T12:00:00');
              const monthDay = dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
              const weekday  = dt.toLocaleDateString('en-US', { weekday: 'short' });
              const isMax = d.count === maxDaily && d.count > 0;
              return (
                <div key={i} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', height: '100%', justifyContent: 'flex-end' }}>
                  {d.count > 0 && <span style={{ fontSize: 10, color: T.sub, marginBottom: 4, fontFamily: T.mono, fontWeight: 600 }}>{d.count}</span>}
                  <div style={{
                    width: '100%', borderRadius: '4px 4px 0 0',
                    height: `${pct}%`,
                    background: isMax
                      ? `linear-gradient(180deg, ${T.accent}, ${T.pink})`
                      : 'rgba(99,102,241,0.15)',
                    transition: 'height 0.4s',
                  }} />
                  <span style={{ fontSize: 9, color: T.muted, marginTop: 5, fontWeight: 600 }}>{monthDay}</span>
                  <span style={{ fontSize: 9, color: T.muted }}>{weekday}</span>
                </div>
              );
            })}
          </div>
        </div>

        {/* Customer Sentiment */}
        <div style={{ ...card, padding: '20px 24px' }}>
          <div style={{ fontSize: 12, fontWeight: 800, color: T.text, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 20 }}>
            Customer Sentiment
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {[
              { label: 'Positive', count: sentimentBreakdown.positive, color: T.green },
              { label: 'Neutral',  count: sentimentBreakdown.neutral,  color: T.amber },
              { label: 'Negative', count: sentimentBreakdown.negative, color: T.red   },
            ].map(s => {
              const pct = Math.round((s.count / sentimentTotal) * 100);
              return (
                <div key={s.label}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 5, fontSize: 13 }}>
                    <span style={{ color: T.sub, fontWeight: 500 }}>{s.label}</span>
                    <span style={{ color: T.muted, fontFamily: T.mono, fontSize: 12 }}>{s.count} ({pct}%)</span>
                  </div>
                  <div style={{ height: 6, background: T.border, borderRadius: 3, overflow: 'hidden' }}>
                    <div style={{ height: '100%', width: `${pct}%`, background: s.color, borderRadius: 3, transition: 'width 0.4s' }} />
                  </div>
                </div>
              );
            })}
            {sentimentTotal <= 1 && sentimentBreakdown.positive === 0 && (
              <p style={{ color: T.muted, fontSize: 12, marginTop: 8 }}>No sentiment data yet.</p>
            )}
          </div>
        </div>
      </div>

      {/* Campaign Performance */}
      <div style={{ ...card, padding: '20px 28px', marginBottom: 16 }}>
        <div style={{ fontSize: 12, fontWeight: 800, color: T.text, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 16 }}>
          Campaign Performance
        </div>
        {campaignPerformance.length === 0 ? (
          <p style={{ color: T.muted, fontSize: 13 }}>No campaigns found.</p>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={thStyle}>Campaign</th>
                <th style={{ ...thStyle, textAlign: 'center' }}>Calls</th>
                <th style={{ ...thStyle, textAlign: 'center' }}>Appointments</th>
                <th style={{ ...thStyle, textAlign: 'center' }}>Avg Score</th>
              </tr>
            </thead>
            <tbody>
              {campaignPerformance.map((c, i) => {
                const isLast = i === campaignPerformance.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                return (
                  <tr key={c.campaign_id}>
                    <td style={{ ...rowTd, fontWeight: 600, color: T.text }}>{c.name}</td>
                    <td style={{ ...rowTd, textAlign: 'center', fontFamily: T.mono }}>{c.calls}</td>
                    <td style={{ ...rowTd, textAlign: 'center', fontFamily: T.mono }}>
                      <span style={{ color: c.appointments > 0 ? T.green : T.muted }}>{c.appointments}</span>
                    </td>
                    <td style={{ ...rowTd, textAlign: 'center' }}>
                      <ScoreBadge score={c.avg_score} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* Top Failure Reasons */}
      {topFailureReasons.length > 0 && (
        <div style={{ ...card, padding: '20px 28px', marginBottom: 16 }}>
          <div style={{ fontSize: 12, fontWeight: 800, color: T.text, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 16 }}>
            Top Failure Reasons
          </div>
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {topFailureReasons.map((r, i) => (
              <div key={i} style={{
                display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                padding: '11px 0',
                borderBottom: i < topFailureReasons.length - 1 ? `1px solid ${T.border}` : 'none',
              }}>
                <span style={{ fontSize: 13, color: T.sub }}>{r.reason}</span>
                <span style={{ fontSize: 13, color: T.red, fontWeight: 700, fontFamily: T.mono }}>{r.count}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Language Performance */}
      <div style={{ ...card, padding: '20px 28px', marginBottom: 16 }}>
        <div style={{ fontSize: 12, fontWeight: 800, color: T.text, textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 16 }}>
          Language Performance
        </div>
        {langData.length === 0 ? (
          <p style={{ color: T.muted, fontSize: 13 }}>No language data yet.</p>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={thStyle}>Language</th>
                <th style={{ ...thStyle, textAlign: 'center' }}>Total Calls</th>
                <th style={{ ...thStyle, textAlign: 'center' }}>Appointments</th>
                <th style={{ ...thStyle, textAlign: 'center' }}>Conversion Rate</th>
                <th style={{ ...thStyle, textAlign: 'center' }}>Avg Quality</th>
                <th style={{ ...thStyle, textAlign: 'center' }}>Avg Duration</th>
              </tr>
            </thead>
            <tbody>
              {langData.map((row, i) => {
                const isLast = i === langData.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                return (
                  <tr key={row.language}>
                    <td style={{ ...rowTd, fontWeight: 600, color: T.text }}>{LANG_NAMES[row.language] || row.language}</td>
                    <td style={{ ...rowTd, textAlign: 'center', fontFamily: T.mono }}>{row.total_calls}</td>
                    <td style={{ ...rowTd, textAlign: 'center', fontFamily: T.mono }}>
                      <span style={{ color: row.appointments > 0 ? T.green : T.muted }}>{row.appointments}</span>
                    </td>
                    <td style={{ ...rowTd, textAlign: 'center', fontFamily: T.mono }}>{row.conversion_rate}%</td>
                    <td style={{ ...rowTd, textAlign: 'center' }}><ScoreBadge score={row.avg_score} /></td>
                    <td style={{ ...rowTd, textAlign: 'center', fontFamily: T.mono }}>{Math.round(row.avg_duration)}s</td>
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
