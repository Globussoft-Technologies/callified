import { useEffect, useState } from 'react';
import { Button, Checkbox, ConfigProvider, Dropdown, Input, Segmented, Select, Table, Tooltip, theme } from 'antd';
import { AppstoreOutlined, BarsOutlined, DeleteOutlined, EditOutlined, EllipsisOutlined, FileTextOutlined, PlusOutlined, SearchOutlined, TeamOutlined, UnorderedListOutlined } from '@ant-design/icons';
import './CampaignOverview.css';

export default function CampaignOverview({ campaigns, getStats, providerLabel, onCreate, onOpen, onAssign, onEdit, onDelete, canCreate, canAssign, canEdit, canDelete, canAddLeads, isDark, deleting, apiFetch, API_URL }) {
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('all');
  const [sort, setSort] = useState('attention');
  const [layout, setLayout] = useState('grid');
  const [columns, setColumns] = useState(2);
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [users, setUsers] = useState([]);
  const [assignments, setAssignments] = useState(null);
  const [assignmentError, setAssignmentError] = useState(false);
  useEffect(() => {
    if (!canAssign) return;
    let active = true;
    apiFetch(`${API_URL}/users`).then(r => r.ok ? r.json() : []).then(data => {
      if (active) setUsers(Array.isArray(data) ? data.filter(u => ['Agent', 'Executive', 'TeamLeader'].includes(u.role)) : []);
    }).catch(() => {});
    return () => { active = false; };
  }, [canAssign, apiFetch, API_URL]);
  useEffect(() => {
    if (!selectedUsers.length) return;
    let active = true;
    Promise.all(campaigns.map(async c => {
      const response = await apiFetch(`${API_URL}/campaigns/${c.id}/assigned-users`);
      if (!response.ok) throw new Error('Assignments unavailable');
      const data = await response.json();
      return [c.id, data.user_ids || []];
    })).then(data => { if (active) setAssignments(Object.fromEntries(data)); }).catch(() => { if (active) setAssignmentError(true); });
    return () => { active = false; };
  }, [selectedUsers, campaigns, apiFetch, API_URL]);
  const rows = campaigns.map(c => {
    const stats = getStats(c);
    const state = c.status === 'paused' ? 'paused' : c.status === 'completed' ? 'completed' : stats.total === 0 ? 'empty' : stats.remaining === 0 ? 'attempted' : 'ready';
    return { campaign: c, stats, state };
  });
  const labels = { ready: 'Ready', paused: 'Paused', completed: 'Completed', empty: 'No leads', attempted: 'All leads attempted' };
  const actionMenu = c => {
    const items = [...(canEdit ? [{ key: 'edit', label: 'Edit campaign', icon: <EditOutlined /> }] : []), ...(canDelete ? [{ key: 'delete', label: 'Delete campaign', icon: <DeleteOutlined />, danger: true, disabled: deleting }] : [])];
    return items.length ? <Dropdown trigger={['click']} menu={{ items, onClick: ({ key }) => key === 'edit' ? onEdit(c) : onDelete(c.id, c.name) }}><Button type="text" aria-label={`Actions for ${c.name}`} icon={<EllipsisOutlined />} /></Dropdown> : null;
  };
  const tableColumns = [
    { title: 'Campaign', key: 'campaign', width: 250, render: (_, r) => <div className="campaign-list-name"><button onClick={() => onOpen(r.campaign.id)}>{r.campaign.name}</button><span>{r.campaign.channel === 'whatsapp' ? 'WhatsApp' : 'Voice'} · {providerLabel(r.campaign)}</span></div> },
    { title: 'Status', key: 'state', width: 180, render: (_, r) => <span className={`campaign-overview-badge ${r.state}`}>{labels[r.state]}</span> },
    { title: 'Called / Total', key: 'called', width: 140, align: 'right', render: (_, r) => `${r.stats.called} / ${r.stats.total}` },
    ...[['Remaining', 'remaining'], ['Qualified', 'qualified'], ['Booked', 'booked']].map(([title, key]) => ({ title, key, width: 100, align: 'right', render: (_, r) => r.stats[key] })),
    { title: 'Actions', key: 'actions', width: 145, render: (_, r) => <div className="campaign-list-actions"><Tooltip title={r.stats.total === 0 && canAddLeads ? 'Add leads' : 'View leads'}><Button aria-label={`View leads for ${r.campaign.name}`} icon={r.stats.total === 0 && canAddLeads ? <PlusOutlined /> : <UnorderedListOutlined />} onClick={() => onOpen(r.campaign.id, r.stats.total === 0 && canAddLeads)} /></Tooltip>{canAssign && <Tooltip title="Assign users"><Button aria-label={`Assign users for ${r.campaign.name}`} icon={<TeamOutlined />} onClick={() => onAssign(r.campaign)} /></Tooltip>}{actionMenu(r.campaign)}</div> },
  ];
  const visible = rows.filter(r => r.campaign.name.toLowerCase().includes(search.trim().toLowerCase()) && (status === 'all' || r.state === status) && (!selectedUsers.length || assignments?.[r.campaign.id]?.some(id => selectedUsers.some(selected => String(id) === String(selected))))).sort((a, b) => sort === 'name' ? a.campaign.name.localeCompare(b.campaign.name) : sort === 'leads' ? b.stats.total - a.stats.total : b.stats.remaining - a.stats.remaining || Number(b.state === 'empty') - Number(a.state === 'empty') || a.campaign.name.localeCompare(b.campaign.name));
  return <ConfigProvider theme={{ algorithm: isDark ? theme.darkAlgorithm : theme.defaultAlgorithm, token: { colorPrimary: '#7948f5', borderRadius: 7 } }}>
    <header className="campaign-overview-heading"><div><h1 data-tour="campaign-list">Campaigns</h1><p>Manage outreach and track every lead</p></div>{canCreate && <Button data-tour="campaign-create" type="primary" icon={<PlusOutlined />} onClick={onCreate}>Create Campaign</Button>}</header>
    <div className="campaign-overview-toolbar">
      <Input aria-label="Search campaigns" prefix={<SearchOutlined />} placeholder="Search campaigns" value={search} onChange={e => setSearch(e.target.value)} allowClear />
      <Select aria-label="Campaign status" value={status} onChange={setStatus} options={[{ value: 'all', label: 'All statuses' }, ...Object.entries(labels).map(([value, label]) => ({ value, label }))]} />
      {canAssign && <Select className="campaign-user-filter" aria-label="Assigned users" placeholder="Assigned users" mode="multiple" value={selectedUsers} onChange={value => { setSelectedUsers(value); setAssignments(null); setAssignmentError(false); }} allowClear showSearch optionFilterProp="label" maxTagCount={1} maxTagPlaceholder={omitted => `+${omitted.length}`} menuItemSelectedIcon={null} optionRender={option => <span className="campaign-user-option"><Checkbox checked={selectedUsers.some(id => String(id) === String(option.value))} tabIndex={-1} aria-hidden="true" style={{ pointerEvents: 'none' }} /><span>{option.label}</span></span>} options={users.map(u => ({ value: u.id, label: u.full_name || u.email }))} />}
      <div className="campaign-overview-view-controls"><Select aria-label="Sort campaigns" value={sort} onChange={setSort} options={[{ value: 'attention', label: 'Sort: Needs attention' }, { value: 'name', label: 'Sort: Name' }, { value: 'leads', label: 'Sort: Most leads' }]} />
        {layout === 'grid' && <Segmented className="campaign-density-control" aria-label="Cards per row" value={columns} onChange={setColumns} options={[2, 3, 4, 5].map(value => ({ value, tooltip: `${value} cards per row`, label: <span aria-label={`${value} cards per row`} className="campaign-density-symbol" style={{ gridTemplateColumns: `repeat(${value}, 1fr)` }}>{Array.from({ length: value * 2 }, (_, i) => <i key={i} />)}</span> }))} />}
        <Segmented aria-label="Campaign view" value={layout} onChange={setLayout} options={[{ value: 'grid', icon: <AppstoreOutlined />, tooltip: 'Grid view', label: <span className="campaign-sr-only">Grid view</span> }, { value: 'list', icon: <BarsOutlined />, tooltip: 'List view', label: <span className="campaign-sr-only">List view</span> }]} />
      </div>
    </div>
    {selectedUsers.length > 0 && !assignments ? <div className="campaign-overview-empty" role="status">{assignmentError ? 'Unable to load assignments. Clear the user filter to view campaigns.' : 'Loading assignments...'}</div> : layout === 'list' ? <Table className="campaign-overview-table" size="small" rowKey={r => r.campaign.id} columns={tableColumns} dataSource={visible} pagination={false} scroll={{ x: 1000 }} /> : <div className={`campaign-overview-cards density-${columns}`} style={{ '--campaign-columns': columns }}>
      {visible.map(({ campaign: c, stats, state }) => {
        const percent = stats.total ? Math.min(100, Math.round(stats.called / stats.total * 100)) : 0;
        const menuItems = [...(canEdit ? [{ key: 'edit', label: 'Edit campaign', icon: <EditOutlined /> }] : []), ...(canDelete ? [{ key: 'delete', label: 'Delete campaign', icon: <DeleteOutlined />, danger: true, disabled: deleting }] : [])];
        return <article className="campaign-overview-card" key={c.id}>
          <div className="campaign-overview-card-heading"><div><h2>{c.name}</h2><p>{c.channel === 'whatsapp' ? 'WhatsApp' : 'Voice'} · {providerLabel(c)}</p></div><div className="campaign-overview-badges"><span className={`campaign-overview-badge ${state}`}>{labels[state]}</span>{menuItems.length > 0 && <Dropdown trigger={['click']} menu={{ items: menuItems, onClick: ({ key }) => key === 'edit' ? onEdit(c) : onDelete(c.id, c.name) }}><Button type="text" aria-label={`Actions for ${c.name}`} icon={<EllipsisOutlined />} /></Dropdown>}{stats.remaining > 0 && <span className="campaign-overview-badge remaining">{stats.remaining} leads remaining</span>}</div></div>
          {stats.total > 0 ? <><div className="campaign-overview-progress-label">{stats.called} of {stats.total} called <b>· {percent}%</b></div><div className="campaign-overview-progress" role="progressbar" aria-label={`${c.name} leads attempted`} aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100}><span style={{ width: `${percent}%` }} /></div><div className="campaign-overview-card-stats">{[['Remaining', stats.remaining], ['Qualified', stats.qualified], ['Booked', stats.booked]].map(([label, value]) => <div key={label}><span>{label}</span><strong>{value}</strong></div>)}</div></> : <div className="campaign-overview-no-leads"><FileTextOutlined /><strong>Add leads to get started</strong><span>No calling activity yet</span></div>}
          <div className="campaign-overview-actions"><Button type="primary" icon={stats.total || !canAddLeads ? <UnorderedListOutlined /> : <PlusOutlined />} onClick={() => onOpen(c.id, stats.total === 0 && canAddLeads)}>{stats.total ? 'View leads' : canAddLeads ? 'Add leads' : 'View campaign'}</Button>{canAssign ? <Button icon={<TeamOutlined />} onClick={() => onAssign(c)}>Assign users</Button> : <Button icon={<FileTextOutlined />} onClick={() => onOpen(c.id)}>View campaign</Button>}</div>
        </article>;
      })}
      {visible.length === 0 && <div className="campaign-overview-empty">{campaigns.length ? 'No campaigns match your filters.' : 'No campaigns yet.'}</div>}
    </div>}
  </ConfigProvider>;
}
