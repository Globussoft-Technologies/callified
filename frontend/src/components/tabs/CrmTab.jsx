import React, { useState, useEffect } from 'react';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  pink: '#ec4899', cyan: '#06b6d4', wa: '#25D366',
  text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

function Sparkline({ data, color, width = 64, height = 28 }) {
  const max = Math.max(...data, 1);
  const min = Math.min(...data, 0);
  const range = max - min || 1;
  const pts = data.map((v, i) => [
    (i / Math.max(data.length - 1, 1)) * width,
    height - ((v - min) / range) * (height - 6) - 3,
  ]);
  const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');
  const area = `${line} L${pts[pts.length - 1][0].toFixed(1)},${height} L${pts[0][0].toFixed(1)},${height} Z`;
  const gid = `sg${color.replace(/[^a-z0-9]/gi, '')}`;
  return (
    <svg width={width} height={height} style={{ overflow: 'visible', display: 'block', flexShrink: 0 }}>
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.18" />
          <stop offset="100%" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${gid})`} />
      <path d={line} fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function DonutRing({ pct, color, size = 36, stroke = 4 }) {
  const r = (size - stroke) / 2;
  const circ = 2 * Math.PI * r;
  const dash = Math.max(0, Math.min(pct, 100)) / 100 * circ;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={T.border} strokeWidth={stroke} />
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={color} strokeWidth={stroke}
        strokeDasharray={`${dash.toFixed(1)} ${circ.toFixed(1)}`} strokeLinecap="round"
        transform={`rotate(-90 ${size / 2} ${size / 2})`} />
    </svg>
  );
}

function makeSpark(val, len = 10) {
  const arr = [];
  for (let i = 0; i < len; i++) {
    const base = val * (i + 1) / len;
    const wobble = val * 0.12 * Math.sin(i * 1.4 + val * 0.01);
    arr.push(Math.max(0, Math.round(base + wobble)));
  }
  return arr;
}

export default function CrmTab({
  userRole, API_URL,
  apiFetch,
  campaigns, dashSummary, onCampaignClick
}) {
  const activeCampaigns   = campaigns.filter(c => c.status === 'active');
  const campaignsCount    = dashSummary?.campaigns     ?? activeCampaigns.length;
  const totalLeads        = dashSummary?.total_leads   ?? campaigns.reduce((s, c) => s + (c.stats?.total        || 0), 0);
  const totalCalled       = dashSummary?.called        ?? campaigns.reduce((s, c) => s + (c.stats?.called       || 0), 0);
  const totalQualified    = dashSummary?.qualified     ?? campaigns.reduce((s, c) => s + (c.stats?.qualified    || 0), 0);
  const totalAppointments = dashSummary?.appointments  ?? campaigns.reduce((s, c) => s + (c.stats?.appointments || 0), 0);

  const canSeeCampaigns = userRole === 'Admin' || userRole === 'Agent';
  const isAdmin = userRole === 'Admin';

  const [activeModal, setActiveModal] = useState(null);
  const [modalSearch, setModalSearch] = useState('');
  const [modalLeads, setModalLeads] = useState([]);
  const [modalLeadsLoading, setModalLeadsLoading] = useState(false);

  const closeModal = () => { setActiveModal(null); setModalSearch(''); setModalLeads([]); };

  useEffect(() => {
     
    if (!activeModal || activeModal === 'campaigns') { setModalLeads([]); return; }
    const fetch = async () => {
      setModalLeadsLoading(true);
      try {
        const results = await Promise.all(
          campaigns.map(async c => {
            const res = await apiFetch(`${API_URL}/campaigns/${c.id}/leads`);
            const data = await res.json();
            const leads = Array.isArray(data) ? data : (data?.leads || []);
            return leads.map(l => ({ ...l, campaignName: c.name, campaignId: c.id }));
          })
        );
        setModalLeads(results.flat());
      } catch { setModalLeads([]);  }
      setModalLeadsLoading(false);
    };
    fetch();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeModal]);

  const callPct = totalLeads > 0 ? Math.round((totalCalled / totalLeads) * 100) : 0;
  const qualPct = totalCalled > 0 ? Math.round((totalQualified / totalCalled) * 100) : 0;
  const apptPct = totalQualified > 0 ? Math.round((totalAppointments / totalQualified) * 100) : 0;

  const statCards = [
    { label: 'CAMPAIGNS',    value: campaignsCount,    color: T.cyan,   badge: `${activeCampaigns.length} active`, sub: `${activeCampaigns.length} active`,  modal: 'campaigns'    },
    { label: 'TOTAL LEADS',  value: totalLeads,        color: T.accent, badge: `all campaigns`,                   sub: 'across all campaigns',               modal: 'leads'        },
    { label: 'CALLED',       value: totalCalled,       color: T.green,  badge: `${callPct}%`,                     sub: `${callPct}% call rate`,              modal: 'called'       },
    { label: 'QUALIFIED',    value: totalQualified,    color: T.pink,   badge: `${qualPct}%`,                     sub: `${qualPct}% qual rate`,              modal: 'qualified'    },
    { label: 'APPOINTMENTS', value: totalAppointments, color: T.amber,  badge: `${apptPct}%`,                     sub: 'booked total',                       modal: 'appointments' },
  ];

  const funnel = [
    { label: 'Campaigns',    val: campaignsCount,    color: T.cyan    },
    { label: 'Leads',        val: totalLeads,        color: T.accent  },
    { label: 'Called',       val: totalCalled,       color: '#818cf8' },
    { label: 'Qualified',    val: totalQualified,    color: T.green   },
    { label: 'Appointments', val: totalAppointments, color: T.amber   },
  ];
  const funnelMax = Math.max(campaignsCount, totalLeads) || 1;

  const barData = campaigns.slice(0, 12).map(c => ({ name: c.name, val: c.stats?.called || 0 }));
  const barMax  = Math.max(...barData.map(b => b.val), 1);

  const modalConfigs = {
    campaigns:    { title: 'All Campaigns',       subtitle: `${campaigns.length} total · ${activeCampaigns.length} active` },
    leads:        { title: 'Total Leads',          subtitle: `${totalLeads.toLocaleString()} leads across ${campaigns.length} campaigns` },
    called:       { title: 'Calls Made',           subtitle: `${totalCalled.toLocaleString()} called · ${callPct}% call rate` },
    qualified:    { title: 'Qualified Leads',      subtitle: `${totalQualified.toLocaleString()} qualified · ${qualPct}% qual rate` },
    appointments: { title: 'Appointments Booked',  subtitle: `${totalAppointments.toLocaleString()} appointments total` },
  };

  return (
    <div style={{ padding: '24px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Heading + toggle */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 20 }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>Dashboard</h2>
          {activeCampaigns.length > 0 && (
            <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
              AI is running {activeCampaigns.length} active campaign{activeCampaigns.length !== 1 ? 's' : ''}
            </p>
          )}
        </div>
      </div>

      {/* 5 stat cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 12, marginBottom: 16 }}>
        {statCards.map(s => (
          <div key={s.label}
            onClick={() => setActiveModal(s.modal)}
            style={{ ...card, padding: '16px 18px', cursor: 'pointer', transition: 'box-shadow 0.15s, transform 0.15s' }}
            onMouseEnter={e => { e.currentTarget.style.boxShadow = `0 4px 16px ${s.color}26`; e.currentTarget.style.transform = 'translateY(-1px)'; }}
            onMouseLeave={e => { e.currentTarget.style.boxShadow = ''; e.currentTarget.style.transform = ''; }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 8 }}>
              <div style={{ fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: '0.06em', display: 'flex', alignItems: 'center', gap: 4 }}>
                {s.label}
                <span style={{ fontSize: 9, color: s.color }}>↗</span>
              </div>
              <span style={{ fontSize: 10, fontWeight: 700, color: T.green, background: 'rgba(16,185,129,0.1)', padding: '1px 7px', borderRadius: 20 }}>
                {s.badge}
              </span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end' }}>
              <div>
                <div style={{ fontSize: 28, fontWeight: 700, fontFamily: T.mono, color: s.color, lineHeight: 1 }}>
                  {s.value.toLocaleString()}
                </div>
                <div style={{ fontSize: 11, color: T.muted, marginTop: 4 }}>{s.sub}</div>
              </div>
              <Sparkline data={makeSpark(s.value)} color={s.color} width={64} height={28} />
            </div>
          </div>
        ))}
      </div>

      {/* Stat card detail modal */}
      {activeModal && (() => {
        const cfg = modalConfigs[activeModal];
        const activeColor = statCards.find(s => s.modal === activeModal)?.color || T.accent;
        const isCampaignModal = activeModal === 'campaigns';

        const statusFilter = {
          leads:        () => true,
          called:       l => l.status && l.status !== 'New',
          qualified:    l => ['Qualified', 'Appointment Set', 'Converted'].includes(l.status),
          appointments: l => ['Appointment Set', 'Converted'].includes(l.status),
        };

        const q = modalSearch.toLowerCase();
        const filteredCampaigns = campaigns.filter(c => c.name?.toLowerCase().includes(q));
        const filteredLeads = modalLeads
          .filter(statusFilter[activeModal] || (() => true))
          .filter(l => `${l.first_name} ${l.last_name}`.toLowerCase().includes(q) || (l.phone || '').includes(q) || (l.campaignName || '').toLowerCase().includes(q));

        const statusColors = {
          'New':             { bg: 'rgba(148,163,184,0.12)', color: T.muted },
          'Contacted':       { bg: 'rgba(99,102,241,0.1)',   color: T.accent },
          'Connected':       { bg: 'rgba(99,102,241,0.1)',   color: T.accent },
          'Interested':      { bg: 'rgba(16,185,129,0.1)',   color: T.green },
          'Not Interested':  { bg: 'rgba(239,68,68,0.1)',    color: T.red },
          'Qualified':       { bg: 'rgba(16,185,129,0.1)',   color: T.green },
          'Appointment Set': { bg: 'rgba(245,158,11,0.1)',   color: T.amber },
          'Converted':       { bg: 'rgba(16,185,129,0.15)',  color: T.green },
          'Lost':            { bg: 'rgba(239,68,68,0.1)',    color: T.red },
          'Junk':            { bg: 'rgba(156,163,175,0.15)', color: T.muted },
        };

        return (
          <div onClick={closeModal} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.35)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
            <div onClick={e => e.stopPropagation()} style={{
              background: T.card, borderRadius: 16, width: 560, maxWidth: '90vw',
              maxHeight: '80vh', display: 'flex', flexDirection: 'column',
              boxShadow: '0 20px 60px rgba(0,0,0,0.18)', border: `1px solid ${T.border}`,
            }}>
              <div style={{ padding: '20px 24px 16px', borderBottom: `1px solid ${T.border}`, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div>
                  <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: T.text }}>{cfg.title}</h3>
                  <p style={{ margin: '2px 0 0', fontSize: 12, color: T.muted }}>{cfg.subtitle}</p>
                </div>
                <button onClick={closeModal} style={{ background: 'none', border: 'none', fontSize: 20, color: T.muted, cursor: 'pointer', padding: '2px 6px', lineHeight: 1 }}>×</button>
              </div>
              <div style={{ padding: '12px 24px', borderBottom: `1px solid ${T.border}` }}>
                <input autoFocus type="text"
                  placeholder={isCampaignModal ? 'Search campaigns...' : 'Search leads...'}
                  value={modalSearch} onChange={e => setModalSearch(e.target.value)}
                  style={{ width: '100%', boxSizing: 'border-box', padding: '9px 14px', borderRadius: 8, border: `1px solid ${T.border}`, background: T.bg, color: T.text, fontSize: 13, fontFamily: T.font, outline: 'none' }}
                />
              </div>
              <div style={{ overflowY: 'auto', flex: 1, padding: '8px 0' }}>
                {isCampaignModal ? (
                  filteredCampaigns.length === 0 ? (
                    <div style={{ textAlign: 'center', color: T.muted, padding: '2rem', fontSize: 13 }}>No campaigns found</div>
                  ) : filteredCampaigns.map((c, i, arr) => {
                    const total = c.stats?.total || 0;
                    const called = c.stats?.called || 0;
                    const pct = total > 0 ? Math.round((called / total) * 100) : 0;
                    const isActive = c.status === 'active';
                    return (
                      <div key={c.id} onClick={() => { closeModal(); onCampaignClick(c); }}
                        style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '12px 24px', cursor: 'pointer', borderBottom: i < arr.length - 1 ? `1px solid ${T.border}` : 'none', transition: 'background 0.1s' }}
                        onMouseEnter={e => e.currentTarget.style.background = T.bg}
                        onMouseLeave={e => e.currentTarget.style.background = 'transparent'}
                      >
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 3 }}>
                            <span style={{ fontSize: 13, fontWeight: 600, color: T.text, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.name}</span>
                            <span style={{ fontSize: 10, fontWeight: 700, padding: '1px 7px', borderRadius: 20, flexShrink: 0, background: isActive ? 'rgba(16,185,129,0.1)' : 'rgba(148,163,184,0.12)', color: isActive ? T.green : T.muted }}>{c.status}</span>
                          </div>
                          <div style={{ fontSize: 11, color: T.muted }}>{called}/{total} called · {pct}% progress</div>
                        </div>
                        <div style={{ position: 'relative', width: 36, height: 36, flexShrink: 0 }}>
                          <DonutRing pct={pct} color={isActive ? T.accent : T.muted} size={36} stroke={4} />
                          <span style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 8, fontWeight: 700, color: isActive ? T.accent : T.muted }}>{pct}%</span>
                        </div>
                      </div>
                    );
                  })
                ) : modalLeadsLoading ? (
                  <div style={{ textAlign: 'center', color: T.muted, padding: '2rem', fontSize: 13 }}>Loading leads...</div>
                ) : filteredLeads.length === 0 ? (
                  <div style={{ textAlign: 'center', color: T.muted, padding: '2rem', fontSize: 13 }}>No leads found</div>
                ) : filteredLeads.map((lead, i, arr) => {
                  const sc = statusColors[lead.status] || statusColors['New'];
                  const campaign = campaigns.find(c => c.id === lead.campaignId);
                  return (
                    <div key={`${lead.id}-${i}`}
                      onClick={() => { if (campaign) { closeModal(); onCampaignClick(campaign); } }}
                      style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '12px 24px', borderBottom: i < arr.length - 1 ? `1px solid ${T.border}` : 'none', cursor: campaign ? 'pointer' : 'default', transition: 'background 0.1s' }}
                      onMouseEnter={e => { if (campaign) e.currentTarget.style.background = T.bg; }}
                      onMouseLeave={e => e.currentTarget.style.background = 'transparent'}
                    >
                      <div style={{ width: 34, height: 34, borderRadius: '50%', background: `${activeColor}18`, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        <span style={{ fontSize: 13, fontWeight: 700, color: activeColor }}>
                          {(lead.first_name?.[0] || '?').toUpperCase()}
                        </span>
                      </div>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontSize: 13, fontWeight: 600, color: T.text, marginBottom: 2 }}>
                          {lead.first_name} {lead.last_name}
                        </div>
                        <div style={{ fontSize: 11, color: T.muted }}>
                          {lead.phone} · {lead.campaignName}
                        </div>
                      </div>
                      <span style={{ fontSize: 10, fontWeight: 700, padding: '2px 9px', borderRadius: 20, flexShrink: 0, background: sc.bg, color: sc.color }}>
                        {lead.status || 'New'}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        );
      })()}

      {/* Charts row */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 260px 240px', gap: 12 }}>

        {/* Bar chart */}
        <div style={{ ...card, padding: '18px 20px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
            <div style={{ fontSize: 13, fontWeight: 700, color: T.sub }}>Call Volume — per Campaign</div>
            <div style={{ fontSize: 11, color: T.muted }}>All campaigns</div>
          </div>
          {barData.length > 0 ? (
            <div style={{ display: 'flex', alignItems: 'flex-end', gap: 6, height: 100 }}>
              {barData.map((b, i) => (
                <div key={i} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 3 }}>
                  <div title={`${b.name}: ${b.val}`} style={{
                    width: '100%', borderRadius: '3px 3px 0 0',
                    height: `${Math.max(4, Math.round((b.val / barMax) * 90))}px`,
                    background: i === 0
                      ? `linear-gradient(180deg, ${T.accent}, ${T.pink})`
                      : 'rgba(99,102,241,0.15)',
                    transition: 'height 0.4s',
                  }} />
                  <div style={{ fontSize: 8, color: T.muted, textAlign: 'center', width: '100%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {b.name.split(' ')[0]}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div style={{ height: 100, display: 'flex', alignItems: 'center', justifyContent: 'center', color: T.muted, fontSize: 13 }}>
              No campaign data yet
            </div>
          )}
        </div>

        {/* Conversion Funnel */}
        <div style={{ ...card, padding: '18px 20px' }}>
          <div style={{ fontSize: 13, fontWeight: 700, color: T.sub, marginBottom: 14 }}>Conversion Funnel</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {funnel.map((f, i) => (
              <div key={i}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 3 }}>
                  <span style={{ fontSize: 11, color: T.sub }}>{f.label}</span>
                  <span style={{ fontSize: 11, fontFamily: T.mono, fontWeight: 700, color: f.color }}>
                    {f.val.toLocaleString()}
                  </span>
                </div>
                <div style={{ height: 6, background: T.border, borderRadius: 3, overflow: 'hidden' }}>
                  <div style={{
                    height: '100%',
                    width: `${Math.round((f.val / funnelMax) * 100)}%`,
                    background: f.color, borderRadius: 3, transition: 'width 0.5s',
                  }} />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Active Campaigns compact */}
        <div style={{ ...card, padding: '18px 20px' }}>
          <div style={{ fontSize: 13, fontWeight: 700, color: T.sub, marginBottom: 14 }}>Active Campaigns</div>
          {canSeeCampaigns && activeCampaigns.length > 0 ? (
            activeCampaigns.slice(0, 5).map((c, i) => {
              const total  = c.stats?.total  || 0;
              const called = c.stats?.called || 0;
              const pct    = total > 0 ? Math.round((called / total) * 100) : 0;
              return (
                <div key={c.id} onClick={() => onCampaignClick(c)}
                  style={{
                    padding: '10px 0',
                    borderBottom: i < activeCampaigns.length - 1 ? `1px solid ${T.border}` : 'none',
                    display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                    cursor: 'pointer',
                  }}>
                  <div style={{ minWidth: 0, marginRight: 8 }}>
                    <div style={{ fontSize: 12, fontWeight: 600, color: T.text, marginBottom: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {c.name}
                    </div>
                    <div style={{ fontSize: 10, color: T.muted }}>{called}/{total} called</div>
                  </div>
                  <div style={{ position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center', width: 36, height: 36, flexShrink: 0 }}>
                    <DonutRing pct={pct} color={T.accent} size={36} stroke={4} />
                    <span style={{ position: 'absolute', fontSize: 8, fontWeight: 700, color: T.accent }}>{pct}%</span>
                  </div>
                </div>
              );
            })
          ) : (
            <div style={{ fontSize: 12, color: T.muted, textAlign: 'center', padding: '20px 0' }}>
              {isAdmin ? 'No active campaigns' : 'No campaigns yet'}
            </div>
          )}
        </div>

      </div>
    </div>
  );
}
