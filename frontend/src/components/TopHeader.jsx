import React, { useState, useEffect, useRef } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import navLogo from '../assets/tg_image_3608761279.png';
import { useHideAiFeatures } from '../hooks/useHideAiFeatures';
import { useCall } from '../contexts/CallContext';
import { useAuth } from '../contexts/AuthContext';
import { formatDateTime } from '../utils/dateFormat';
import AppIcon from './common/AppIcon';
import { useTheme } from '../contexts/ThemeContext';
import { isCustomerProductionDomain } from '../utils/domainFeatures';

// Tabs that should be hidden when AI features are disabled for the user.
// Note: exotel-accounts is reachable from Settings for manual accounts, so it is hidden here to avoid duplication.
const AI_TAB_IDS = new Set(['monitor', 'knowledge', 'sandbox', 'whatsapp', 'ai-receptionist', 'receptionist', 'billing', 'logs', 'integrations', 'ops', 'dnd', 'scheduled', 'campaign-progress', 'exotel-accounts']);
// CRM integrations remain hidden until token handling, provider contracts,
// deduplication, and bidirectional sync are production-ready.
const HIDDEN_TAB_IDS = new Set(['integrations', 'whatsapp', 'knowledge']);

const AGENT_TABS = [
  { id: 'campaigns', label: 'Campaigns', path: '/campaigns', testid: 'tab-campaigns' },
];

const TEAM_LEADER_PRIMARY_TABS = [
  { id: 'campaigns', label: 'Campaigns', path: '/campaigns', testid: 'tab-campaigns' },
];

const PRIMARY_ADMIN_TABS = [
  { id: 'campaigns', label: 'Campaigns',      path: '/campaigns', testid: 'tab-campaigns' },
  { id: 'manual-dial', label: 'Calls',        path: '/manual-dial', testid: 'tab-calls' },
  { id: 'products',  label: 'Products',       path: '/products',  testid: 'tab-products' },
  { id: 'whatsapp',  label: 'Communications', path: '/whatsapp',  testid: 'tab-whatsapp' },
  { id: 'analytics', label: 'Analytics',      path: '/analytics', testid: 'tab-analytics' },
];

// More-menu tabs accessible to Team Leaders (subset of MORE_ADMIN_TABS).
const TEAM_LEADER_MORE_TAB_IDS = new Set(['scheduled', 'interaction-history', 'lead-call-status', 'settings']);

const MORE_ADMIN_TABS = [
  { id: 'integrations',     label: 'Integrations',      path: '/integrations',      testid: 'tab-integrations' },
  { id: 'exotel-accounts', label: 'Provider Accounts',  path: '/exotel-accounts',   testid: 'tab-exotel-accounts' },
  { id: 'monitor',      label: 'Monitor AI Calls',path: '/monitor',      testid: 'tab-monitor' },
  { id: 'knowledge',    label: 'RAG Knowledge',   path: '/knowledge',    testid: 'tab-rag' },
  { id: 'sandbox',      label: 'AI Sandbox',      path: '/sandbox',      testid: 'tab-sandbox' },
  { id: 'scheduled',    label: 'Scheduled',       path: '/scheduled',    testid: 'tab-scheduled' },
  { id: 'interaction-history', label: 'Interaction History', path: '/interaction-history', testid: 'tab-interaction-history' },
  { id: 'agent-presence', label: 'Agent Presence', path: '/agent-presence', testid: 'tab-agent-presence' },
  { id: 'agent-report', label: 'Agent Report', path: '/agent-report', testid: 'tab-agent-report' },
  { id: 'lead-call-status', label: 'Lead Call Status', path: '/lead-call-status', testid: 'tab-lead-call-status' },
  { id: 'campaign-progress', label: 'Campaign Progress', path: '/campaign-progress', testid: 'tab-campaign-progress' },
  { id: 'billing',      label: 'Billing',         path: '/billing',      testid: 'tab-billing' },
  { id: 'dnd',          label: 'DND',             path: '/dnd',          testid: 'tab-dnd' },
  { id: 'delete-leads', label: 'Delete Leads',    path: '/delete-leads', testid: 'tab-delete-leads' },
  { id: 'settings',     label: 'Settings',        path: '/settings',     testid: 'tab-settings' },
  { id: 'logs',         label: 'Live Logs',       path: '/logs',         testid: 'tab-logs' },
  { id: 'team',         label: 'Team',            path: '/team',         testid: 'tab-team' },
  { id: 'ai-receptionist', label: 'AI Receptionist', path: '/ai-receptionist', testid: 'tab-ai-receptionist' },
];

const MORE_GROUPS = [
  { id: 'operations', label: 'Operations', tabIds: ['ops', 'scheduled', 'monitor', 'agent-presence'] },
  { id: 'reports', label: 'Reports', tabIds: ['campaign-progress', 'agent-report', 'lead-call-status', 'interaction-history'] },
  { id: 'ai-tools', label: 'AI tools', tabIds: ['knowledge', 'sandbox', 'ai-receptionist'] },
  { id: 'admin', label: 'Admin', tabIds: ['team', 'settings', 'billing', 'integrations', 'exotel-accounts'] },
  { id: 'system', label: 'System', tabIds: ['logs', 'dnd', 'delete-leads'] },
];

const NAV_ICONS = {
  crm: 'team', campaigns: 'audio', 'manual-dial': 'phone', products: 'product',
  whatsapp: 'message', analytics: 'chart', ops: 'tool', scheduled: 'calendar',
  monitor: 'sound', 'agent-presence': 'user', 'campaign-progress': 'chart',
  'agent-report': 'team', 'lead-call-status': 'file', 'interaction-history': 'history',
  knowledge: 'book', sandbox: 'experiment', 'ai-receptionist': 'robot', team: 'team',
  settings: 'settings', billing: 'bank', integrations: 'api', 'exotel-accounts': 'phone',
  logs: 'file', dnd: 'stop', 'delete-leads': 'delete', subscriptions: 'bank',
  'feature-flags': 'power',
};

const TAB_PERMISSION = {
  products: 'products.view',
  campaigns: 'campaigns.view',
  analytics: 'reports.view',
  'exotel-accounts': 'provider_accounts.global',
  monitor: 'monitor.view',
  knowledge: 'knowledge.manage',
  scheduled: 'calls.schedule',
  'agent-report': 'reports.view',
  'lead-call-status': 'reports.view',
  'campaign-progress': 'reports.view',
  billing: 'billing.manage',
  dnd: 'dnd.manage',
  'delete-leads': 'crm.delete',
  executives: 'executives.manage',
  settings: 'settings.manage',
  logs: 'logs.view',
  team: 'team.view',
  receptionist: 'settings.manage',
  integrations: 'integrations.manage',
  whatsapp: 'whatsapp.manage',
};

const SUPER_ADMIN_TABS = [
  { id: 'subscriptions', label: 'Subscriptions', path: '/subscriptions', testid: 'tab-subscriptions' },
  { id: 'feature-flags', label: 'Feature Flags', path: '/feature-flags', testid: 'tab-feature-flags' },
];

const font = "'DM Sans', sans-serif";

export default function TopHeader({ userRole, currentUser, handleLogout, apiFetch }) {
  const navigate = useNavigate();
  const location = useLocation();
  const activeTab = location.pathname.split('/').filter(Boolean)[0] || 'crm';
  const hideAiFeatures = useHideAiFeatures();
  const hidePrelaunchAiTools = isCustomerProductionDomain();
  const { hasPermission } = useAuth();
  const { isDark, toggleTheme } = useTheme();

  const [, setCallingStatus] = useState(null);
  const [moreOpen, setMoreOpen] = useState(false);
  const [notifOpen, setNotifOpen] = useState(false);
  const [userOpen, setUserOpen] = useState(false);
  const [confirmLogout, setConfirmLogout] = useState(false);
  const moreRef = useRef(null);
  const notifRef = useRef(null);
  const userRef = useRef(null);

  const { dueScheduledCalls, dismissScheduledCall, triggerBrowserCall, browserCallDialing, refreshScheduledCalls, manualPresenceStatus, setManualPresenceStatus } = useCall();
  const notifCount = dueScheduledCalls.length;

  const statusLabel = manualPresenceStatus === 'break' ? 'On Break' : 'Idle';
  const statusColor = manualPresenceStatus === 'break' ? '#f59e0b' : '#10b981';
  const toggleBreak = () => {
    setManualPresenceStatus(manualPresenceStatus === 'break' ? 'idle' : 'break');
  };

  useEffect(() => {
    const fetchStatus = () => {
      apiFetch('/api/calling-status')
        .then(r => r.json())
        .then(data => setCallingStatus(data))
        .catch(() => {});
    };
    fetchStatus();
    const interval = setInterval(fetchStatus, 60000);
    return () => clearInterval(interval);
  }, [apiFetch]);

  useEffect(() => {
    if (!moreOpen) return;
    const onDocClick = (e) => {
      if (moreRef.current && !moreRef.current.contains(e.target)) setMoreOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, [moreOpen]);

  useEffect(() => {
    if (!notifOpen) return;
    const onDocClick = (e) => {
      if (notifRef.current && !notifRef.current.contains(e.target)) setNotifOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, [notifOpen]);

  useEffect(() => {
    if (!userOpen) return;
    const onDocClick = (e) => {
      if (userRef.current && !userRef.current.contains(e.target)) setUserOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, [userOpen]);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setMoreOpen(false); setNotifOpen(false); setUserOpen(false); }, [location.pathname]);

  const visibleMoreTabs = MORE_ADMIN_TABS
    .filter(t => !HIDDEN_TAB_IDS.has(t.id))
    .filter(t => !hidePrelaunchAiTools || !['sandbox', 'ai-receptionist'].includes(t.id))
    .filter(t => !hideAiFeatures || !AI_TAB_IDS.has(t.id));
  const moreActive = visibleMoreTabs.some(t => t.id === activeTab);
  const goTo = (path) => { setMoreOpen(false); navigate(path); };

  // Super admins see the same navigation as admins, plus the super-admin-only tabs.
  const isAdminLike = userRole === 'Admin' || userRole === 'SuperAdmin' || currentUser?.is_super_admin;
  const isTeamLeader = userRole === 'TeamLeader';
  const canViewCampaigns = hasPermission('campaigns.view');
  const allowedTab = (tab) => {
    const key = TAB_PERMISSION[tab.id];
    return !key || hasPermission(key);
  };

  const userName = currentUser?.full_name || currentUser?.email || '';
  const userInitial = userName.charAt(0).toUpperCase();
  const orgName = currentUser?.org_name || '';

  const tabBtn = (id, label, path, testid) => {
    const isActive = activeTab === id;
    return (
      <button
        key={id}
        data-testid={testid}
        onClick={() => navigate(path)}
        className={`top-nav-link${isActive ? ' is-active' : ''}`}
      >
        <AppIcon name={NAV_ICONS[id]} />
        {label}
      </button>
    );
  };

  return (
    <header className="focused-top-header" style={{
      display: 'flex', flexWrap: 'nowrap', alignItems: 'center', gap: '6px',
      padding: '0 24px', height: 56,
      background: 'var(--surface)', borderBottom: '1px solid var(--border)',
      boxShadow: '0 1px 4px rgba(0,0,0,0.05)',
      position: 'sticky', top: 0, zIndex: 100,
      width: '100%', boxSizing: 'border-box',
    }}>

      {/* Logo */}
      <div
        onClick={() => navigate('/crm')}
        style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', flexShrink: 0, marginRight: 12 }}>
        <img src={navLogo} alt="Callified" style={{ height: 36, width: 36, objectFit: 'contain', borderRadius: 10 }} />
        <span style={{ fontSize: 15, fontWeight: 700, color: 'var(--text-primary)', fontFamily: font }}>
          Callified
        </span>
      </div>

      {/* Tabs */}
      <nav className="focused-top-nav">
        {tabBtn('crm', 'CRM', '/crm', 'tab-crm')}

        {(userRole === 'Agent' || userRole === 'Executive') && canViewCampaigns && AGENT_TABS.map(t => tabBtn(t.id, t.label, t.path, t.testid))}
        {isTeamLeader && TEAM_LEADER_PRIMARY_TABS
          .filter(t => t.id !== 'campaigns' || canViewCampaigns)
          .filter(t => !hideAiFeatures || !AI_TAB_IDS.has(t.id))
          .map(t => tabBtn(t.id, t.label, t.path, t.testid))}
        {isAdminLike && PRIMARY_ADMIN_TABS
          .filter(t => t.id !== 'campaigns' || canViewCampaigns)
          .filter(t => !HIDDEN_TAB_IDS.has(t.id))
          .filter(allowedTab)
          .filter(t => !hideAiFeatures || !AI_TAB_IDS.has(t.id))
          .map(t => tabBtn(t.id, t.label, t.path, t.testid))}

        {(isAdminLike || isTeamLeader) && (
          (() => {
            const superAdminTabs = currentUser?.is_super_admin ? SUPER_ADMIN_TABS : [];
            const roleFilteredMoreTabs = isTeamLeader
              ? visibleMoreTabs.filter(t => TEAM_LEADER_MORE_TAB_IDS.has(t.id)).filter(allowedTab)
              : visibleMoreTabs.filter(allowedTab);
            const allMoreTabs = [...roleFilteredMoreTabs, ...superAdminTabs];
            if (allMoreTabs.length === 1) {
              const t = allMoreTabs[0];
              return tabBtn(t.id, t.label, t.path, t.testid);
            }
            if (allMoreTabs.length === 0) return null;
            return (
              <div ref={moreRef} className="top-nav-more">
                <button
                  data-testid="tab-more"
                  onClick={() => setMoreOpen(o => !o)}
                  aria-haspopup="true"
                  aria-expanded={moreOpen}
                  className={`top-nav-more-trigger${moreActive ? ' is-active' : ''}`}>
                  More <AppIcon name="down" />
                </button>
                {moreOpen && (
                  <div role="menu" className="top-nav-mega-menu">
                    {MORE_GROUPS.map(group => {
                      const tabs = roleFilteredMoreTabs.filter(t => group.tabIds.includes(t.id));
                      if (!tabs.length) return null;
                      return <section key={group.id} className="top-nav-menu-group">
                        <h3>{group.label}</h3>
                        {tabs.map(t => (
                          <button key={t.id} data-testid={t.testid} role="menuitem"
                            className={activeTab === t.id ? 'is-active' : ''}
                            onClick={() => goTo(t.path)}>
                            <AppIcon name={NAV_ICONS[t.id]} />
                            <span>{t.label}</span>
                          </button>
                        ))}
                      </section>;
                    })}
                    {superAdminTabs.length > 0 && <section className="top-nav-menu-group">
                      <h3>Platform</h3>
                      {superAdminTabs.map(t => (
                        <button key={t.id} data-testid={t.testid} role="menuitem"
                          className={activeTab === t.id ? 'is-active' : ''}
                          onClick={() => goTo(t.path)}>
                          <AppIcon name={NAV_ICONS[t.id]} />
                          <span>{t.label}</span>
                        </button>
                      ))}
                    </section>}
                  </div>
                )}
              </div>
            );
          })()
        )}
      </nav>

      {/* Right side */}
      <div className="focused-top-tools">

        {/* Agent presence toggle */}
        <button
          onClick={toggleBreak}
          title="Toggle break status"
          style={{
            display: 'inline-flex', alignItems: 'center', gap: 5,
            padding: '4px 10px', borderRadius: 20, border: `1px solid ${statusColor}`,
            background: `${statusColor}15`, color: statusColor,
            fontSize: 12, fontWeight: 600, cursor: 'pointer', fontFamily: font,
          }}
        >
          <span style={{ width: 7, height: 7, borderRadius: '50%', background: statusColor }} />
          {statusLabel}
        </button>

        {/* Theme */}
        <button
          type="button"
          className="theme-toggle"
          onClick={toggleTheme}
          aria-label={isDark ? 'Switch to light theme' : 'Switch to dark theme'}
          title={isDark ? 'Switch to light theme' : 'Switch to dark theme'}
        >
          <AppIcon name={isDark ? 'sun' : 'moon'} />
        </button>

        {/* Bell */}
        <div ref={notifRef} style={{ position: 'relative' }}>
          <div
            data-testid="header-bell"
            onClick={() => setNotifOpen(o => !o)}
            style={{ position: 'relative', cursor: 'pointer', width: 22, height: 22 }}
          >
            <AppIcon name="bell" style={{ fontSize: 20, color: 'var(--text-muted)' }} />
            {notifCount > 0 && (
              <span style={{
                position: 'absolute', top: -5, right: -6,
                minWidth: 16, height: 16, borderRadius: '50%',
                background: '#ef4444', border: '1.5px solid #fff',
                color: '#fff', fontSize: 10, fontWeight: 700,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontFamily: font, padding: '0 4px', boxSizing: 'border-box'
              }}>
                {notifCount > 9 ? '9+' : notifCount}
              </span>
            )}
          </div>
          {notifOpen && (
            <div style={{
              position: 'absolute', top: 'calc(100% + 8px)', right: -10, minWidth: '280px', maxWidth: '320px',
              background: '#ffffff', border: '1px solid #e5e7eb', borderRadius: 12,
              boxShadow: '0 8px 24px rgba(0,0,0,0.10)', zIndex: 1000,
              padding: '12px 0',
            }}>
              <div style={{
                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                padding: '0 16px 10px', borderBottom: '1px solid #f3f4f6',
              }}>
                <span style={{ fontSize: 14, fontWeight: 700, color: '#111827', fontFamily: font }}>Scheduled callbacks</span>
              </div>
              {notifCount === 0 ? (
                <div style={{ padding: '20px 16px', textAlign: 'center', color: '#6b7280', fontSize: 13, fontFamily: font }}>
                  No new notifications
                </div>
              ) : (
                <div style={{ maxHeight: '60vh', overflowY: 'auto', padding: '8px 0' }}>
                  {dueScheduledCalls.map(call => (
                    <div key={call.id} style={{
                      display: 'flex', alignItems: 'center', gap: 8,
                      padding: '10px 16px', borderBottom: '1px solid #f3f4f6'
                    }}>
                      <div style={{ flex: 1, minWidth: 0, textAlign: 'left' }}>
                        <div style={{ fontSize: 13, fontWeight: 600, color: '#111827', fontFamily: font }}>
                          {call.first_name || 'Unnamed'}
                        </div>
                        <div style={{ fontSize: 11, color: '#6b7280', fontFamily: font, marginTop: 2 }}>
                          {call.phone || 'No phone'} • {call.executive_name || 'Unassigned'}
                        </div>
                        <div style={{ fontSize: 11, color: '#4b5563', fontFamily: font, marginTop: 2 }}>
                          <AppIcon name="calendar" style={{ marginRight: 4 }} />
                          {call.scheduled_time ? formatDateTime(call.scheduled_time) : ''}
                        </div>
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
                        <button
                          onClick={() => {
                            triggerBrowserCall(
                              { id: call.lead_id, first_name: call.first_name || '', last_name: '', phone: call.phone || '' },
                              call.campaign_id
                            );
                            dismissScheduledCall(call.id);
                            refreshScheduledCalls?.();
                            setNotifOpen(false);
                          }}
                          disabled={browserCallDialing}
                          style={{
                            padding: '5px 10px', borderRadius: 6, border: 'none', cursor: 'pointer',
                            background: 'linear-gradient(135deg, #16a34a, #22c55e)', color: '#fff',
                            fontSize: 11, fontWeight: 600, fontFamily: font,
                            opacity: browserCallDialing ? 0.6 : 1
                          }}>
                          Call Now
                        </button>
                        <button
                          onClick={() => {
                            dismissScheduledCall(call.id);
                            refreshScheduledCalls?.();
                          }}
                          style={{
                            padding: '5px 10px', borderRadius: 6, cursor: 'pointer',
                            background: 'rgba(148,163,184,0.12)', border: '1px solid rgba(148,163,184,0.3)',
                            color: '#475569', fontSize: 11, fontWeight: 600, fontFamily: font
                          }}>
                          Dismiss
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        {/* User menu */}
        {currentUser && (
          <div ref={userRef} className="top-user-menu">
            <button type="button" className="top-user-trigger" onClick={() => setUserOpen(open => !open)}
              aria-haspopup="menu" aria-expanded={userOpen}>
              <span className="top-user-avatar">{userInitial}</span>
              <span className="top-user-copy">
                <strong>{userName}</strong>
                {orgName && <small>{orgName}</small>}
              </span>
              <AppIcon name="down" />
            </button>
            {userOpen && <div className="top-user-popover" role="menu">
              <div className="top-user-summary">
                <strong>{userName}</strong>
                <span>{currentUser.email}</span>
                {orgName && <small>{orgName}</small>}
              </div>
              <button type="button" role="menuitem" onClick={() => goTo('/settings')}>
                <AppIcon name="settings" /> Account settings
              </button>
              {!confirmLogout ? (
                <button data-testid="logout-btn" type="button" role="menuitem" className="is-danger"
                  onClick={() => setConfirmLogout(true)}>
                  <AppIcon name="disconnect" /> Log out
                </button>
              ) : (
                <div className="top-user-logout-confirm">
                  <span>Log out of Callified?</span>
                  <div>
                    <button data-testid="logout-confirm-btn" onClick={handleLogout}>Log out</button>
                    <button onClick={() => setConfirmLogout(false)}>Cancel</button>
                  </div>
                </div>
              )}
            </div>}
          </div>
        )}
      </div>
    </header>
  );
}
