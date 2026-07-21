import React, { useState, useEffect } from 'react';
import { useToast } from '../contexts/UIContext';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const inputStyle = {
  padding: '10px 14px', borderRadius: 8, fontSize: 13,
  border: `1px solid ${T.border}`, background: '#fff', color: T.text,
  fontFamily: T.font, outline: 'none', width: '100%', boxSizing: 'border-box',
};

const btnPrimary = {
  background: T.accent, color: '#fff', border: 'none',
  borderRadius: 8, padding: '10px 20px', fontWeight: 600,
  cursor: 'pointer', fontFamily: T.font, fontSize: '0.9rem',
};

function todayStr() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

export default function AgentReportPage({ apiFetch, API_URL, campaigns = [] }) {
  const toast = useToast();
  const [from, setFrom] = useState(todayStr());
  const [to, setTo] = useState(todayStr());
  const [campaignId, setCampaignId] = useState('');
  const [downloading, setDownloading] = useState(false);

  const handleDownload = async () => {
    setDownloading(true);
    try {
      const url = `${API_URL}/analytics/agent-report?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&campaign_id=${encodeURIComponent(campaignId)}`;
      const res = await apiFetch(url);
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || `HTTP ${res.status}`);
      }
      const blob = await res.blob();
      const objectUrl = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = objectUrl;
      a.download = `agent_report_${from}_to_${to}.xlsx`;
      a.click();
      URL.revokeObjectURL(objectUrl);
      toast('Agent report downloaded');
    } catch (e) {
      toast(`Download failed: ${e.message}`);
    }
    setDownloading(false);
  };

  return (
    <div style={{ padding: '24px', maxWidth: 800, margin: '0 auto', fontFamily: T.font }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ margin: 0, fontSize: 24, fontWeight: 700, color: T.text }}>📊 Agent Performance Report</h1>
        <p style={{ margin: '6px 0 0', color: T.muted, fontSize: '0.9rem' }}>
          Download an Excel workbook with agent productivity, efficiency, and outcome sheets.
        </p>
      </div>

      <div style={{ ...card, padding: '1.5rem' }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 16, marginBottom: 20 }}>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>From</span>
            <input type="date" value={from} onChange={e => setFrom(e.target.value)} style={inputStyle} />
          </label>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>To</span>
            <input type="date" value={to} onChange={e => setTo(e.target.value)} style={inputStyle} />
          </label>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: T.sub }}>Campaign (optional)</span>
            <select value={campaignId} onChange={e => setCampaignId(e.target.value)} style={{ ...inputStyle, height: 38 }}>
              <option value="">All campaigns</option>
              {campaigns.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </label>
        </div>

        <div style={{ background: '#f8fafc', border: `1px solid ${T.border}`, borderRadius: 10, padding: '1rem', marginBottom: 20 }}>
          <div style={{ fontSize: 13, fontWeight: 600, color: T.text, marginBottom: 8 }}>Excel workbook includes:</div>
          <ul style={{ margin: 0, paddingLeft: 18, color: T.sub, fontSize: 13, lineHeight: 1.6 }}>
            <li><strong>Summary</strong> — calls, talk time, notes, appointments, conversions per agent</li>
            <li><strong>Call Activity</strong> — total, connected, completed, unanswered, busy, failed calls</li>
            <li><strong>Efficiency</strong> — talk time, idle time, average call duration, calls per hour</li>
            <li><strong>Outcomes</strong> — appointments, conversions, status updates, notes</li>
            <li><strong>Detail</strong> — raw activity log for the selected period</li>
          </ul>
        </div>

        <button onClick={handleDownload} disabled={downloading} style={{ ...btnPrimary, opacity: downloading ? 0.6 : 1 }}>
          {downloading ? 'Generating…' : 'Download Excel Report'}
        </button>
      </div>
    </div>
  );
}
