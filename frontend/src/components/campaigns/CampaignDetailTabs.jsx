import React from 'react';
import { BarChartOutlined, HistoryOutlined, RedoOutlined, TeamOutlined } from '@ant-design/icons';

const definitions = [
  { id: 'leads', icon: TeamOutlined, color: '#6366f1' },
  { id: 'calllog', icon: HistoryOutlined, color: '#10b981' },
  { id: 'insights', icon: BarChartOutlined, color: '#a855f7' },
  { id: 'retries', icon: RedoOutlined, color: '#f59e0b' },
];

export default function CampaignDetailTabs({ activeTab, onChange, leadCount = 0, callCount = 0, visible = definitions.map(tab => tab.id), tourExample = false }) {
  const labels = { leads: `Leads (${leadCount})`, calllog: `Call Log (${callCount})`, insights: 'Call Insights', retries: 'Retries' };
  return <div className="campaign-detail-tabs" style={{ display: 'flex', background: '#f4f5f9', border: '1px solid #e5e7eb', borderRadius: 8, padding: 3, gap: 2, width: 'fit-content', flexWrap: 'wrap' }}>
    {definitions.filter(tab => visible.includes(tab.id)).map(tab => <button key={tab.id} type="button" data-tour={!tourExample && tab.id === 'calllog' ? 'campaign-call-log' : undefined} onClick={() => onChange?.(tab.id)} style={{ padding: '6px 18px', borderRadius: 6, border: 'none', cursor: tourExample ? 'default' : 'pointer', fontSize: 13, fontWeight: 600, fontFamily: "'DM Sans', sans-serif", background: activeTab === tab.id ? tab.color : 'transparent', color: activeTab === tab.id ? '#fff' : '#9ca3af', transition: 'all 0.15s', display: 'inline-flex', alignItems: 'center', gap: 6 }}><tab.icon />{labels[tab.id]}</button>)}
  </div>;
}
