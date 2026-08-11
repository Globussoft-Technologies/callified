import React, { useState, useEffect, useRef } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import navLogo from '../assets/tg_image_3608761279.png';
import { useHideAiFeatures } from '../hooks/useHideAiFeatures';
import { useCall } from '../contexts/CallContext';
import { formatDateTime } from '../utils/dateFormat';
import { Popover, PopoverTrigger, PopoverContent, PopoverClose } from './ui/popover';
import { Menu, X } from 'lucide-react';

// Tabs that should be hidden when AI features are disabled for the user.
const AI_TAB_IDS = new Set(['analytics', 'monitor', 'knowledge', 'sandbox', 'whatsapp', 'receptionist', 'billing', 'logs', 'integrations', 'ops', 'dnd', 'scheduled', 'exotel-accounts', 'team']);

const AGENT_TABS = [
  { id: 'campaigns', label: 'Campaigns', path: '/campaigns', testid: 'tab-campaigns' },
];

const TEAM_LEADER_PRIMARY_TABS = [
  { id: 'campaigns', label: 'Campaigns', path: '/campaigns', testid: 'tab-campaigns' },
];

const PRIMARY_ADMIN_TABS = [
  { id: 'products',  label: 'Products',       path: '/products',  testid: 'tab-products' },
  { id: 'campaigns', label: 'Campaigns',      path: '/campaigns', testid: 'tab-campaigns' },
  { id: 'ops',       label: 'Ops & Tasks',    path: '/ops',       testid: 'tab-ops' },
  { id: 'analytics', label: 'Analytics',      path: '/analytics', testid: 'tab-analytics' },
  { id: 'whatsapp',  label: 'WhatsApp Comms', path: '/whatsapp',  testid: 'tab-whatsapp' },
];

// More-menu tabs accessible to Team Leaders (subset of MORE_ADMIN_TABS).
const TEAM_LEADER_MORE_TAB_IDS = new Set(['scheduled', 'interaction-history', 'settings']);

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
  { id: 'campaign-progress', label: 'Campaign Progress', path: '/campaign-progress', testid: 'tab-campaign-progress' },
  { id: 'billing',      label: 'Billing',         path: '/billing',      testid: 'tab-billing' },
  { id: 'dnd',          label: 'DND',             path: '/dnd',          testid: 'tab-dnd' },
  { id: 'executives',   label: 'Executives',      path: '/executives',   testid: 'tab-executives' },
  { id: 'settings',     label: 'Settings',        path: '/settings',     testid: 'tab-settings' },
  { id: 'logs',         label: 'Live Logs',       path: '/logs',         testid: 'tab-logs' },
  { id: 'team',         label: 'Team',            path: '/team',         testid: 'tab-team' },
  { id: 'receptionist', label: 'Receptionist',    path: '/receptionist', testid: 'tab-receptionist' },
];

const SUPER_ADMIN_TABS = [
  { id: 'subscriptions', label: 'Subscriptions', path: '/subscriptions', testid: 'tab-subscriptions' },
  { id: 'feature-flags', label: 'Feature Flags', path: '/feature-flags', testid: 'tab-feature-flags' },
];

const font = "Quicksand";

export default function TopHeader({ userRole, currentUser, handleLogout, apiFetch }) {
  const navigate = useNavigate();
  const location = useLocation();
  const activeTab = location.pathname.replace('/', '') || 'crm';
  const hideAiFeatures = useHideAiFeatures();

  const [callingStatus, setCallingStatus] = useState(null);
  const [notifOpen, setNotifOpen] = useState(false);
  const [confirmLogout, setConfirmLogout] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const notifRef = useRef(null);
  const headerRef = useRef(null);

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
    if (!notifOpen) return;
    const onDocClick = (e) => {
      if (notifRef.current && !notifRef.current.contains(e.target)) setNotifOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, [notifOpen]);

  const visibleMoreTabs = hideAiFeatures
    ? MORE_ADMIN_TABS.filter(t => !AI_TAB_IDS.has(t.id))
    : MORE_ADMIN_TABS;
  const goTo = (path) => navigate(path);

  // Super admins see the same navigation as admins, plus the super-admin-only tabs.
  const isAdminLike = userRole === 'Admin' || userRole === 'SuperAdmin' || currentUser?.is_super_admin;
  const isTeamLeader = userRole === 'TeamLeader';
  const primaryTabs = userRole === 'Agent'
    ? AGENT_TABS
    : isTeamLeader
      ? TEAM_LEADER_PRIMARY_TABS.filter(t => !hideAiFeatures || !AI_TAB_IDS.has(t.id))
      : isAdminLike
        ? PRIMARY_ADMIN_TABS.filter(t => !hideAiFeatures || !AI_TAB_IDS.has(t.id))
        : [];
  const roleFilteredMoreTabs = isTeamLeader
    ? visibleMoreTabs.filter(t => TEAM_LEADER_MORE_TAB_IDS.has(t.id))
    : visibleMoreTabs;
  const superAdminTabs = currentUser?.is_super_admin ? SUPER_ADMIN_TABS : [];
  const allMoreTabs = (isAdminLike || isTeamLeader) ? [...roleFilteredMoreTabs, ...superAdminTabs] : [];
  const mobileTabs = [
    { id: 'crm', label: 'CRM', path: '/crm', testid: 'tab-crm' },
    ...primaryTabs,
    ...allMoreTabs,
  ];
  const moreActive = allMoreTabs.some(t => t.id === activeTab);

  const userName = currentUser?.full_name || currentUser?.email || '';
  const userInitial = userName.charAt(0).toUpperCase();
  const orgName = currentUser?.org_name || '';

  useEffect(() => {
    setMobileMenuOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!mobileMenuOpen) return;
    const closeMobileMenu = (event) => {
      if (event.key === 'Escape' || (event.type === 'mousedown' && headerRef.current && !headerRef.current.contains(event.target))) {
        setMobileMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', closeMobileMenu);
    document.addEventListener('keydown', closeMobileMenu);
    return () => {
      document.removeEventListener('mousedown', closeMobileMenu);
      document.removeEventListener('keydown', closeMobileMenu);
    };
  }, [mobileMenuOpen]);

  const tabBtn = (id, label, path, testid) => {
    const isActive = activeTab === id;
    return (
      <button
        key={id}
        data-testid={testid}
        onClick={() => navigate(path)}
        style={{
          background: isActive ? '#eef2ff' : 'transparent',
          border: isActive ? '1px solid #c7d2fe' : '1px solid transparent',
          cursor: 'pointer',
          padding: '5px 13px', borderRadius: 999,
          fontSize: 13, fontWeight: isActive ? 700 : 600,
          color: isActive ? '#4338ca' : '#475569',
          fontFamily: font, whiteSpace: 'nowrap',
          transition: 'background 0.15s, color 0.15s',
        }}
        onMouseEnter={e => { if (!isActive) { e.currentTarget.style.color = '#1f2937'; } }}
        onMouseLeave={e => { if (!isActive) { e.currentTarget.style.color = '#475569'; } }}
      >
        {label}
      </button>
    );
  };

  return (
    <header ref={headerRef} className="top-header" style={{
      display: 'flex', flexWrap: 'nowrap', alignItems: 'center', justifyContent: 'space-between', gap: '8px',
      padding: '0 24px', height: 72,
      background: '#ffffff', borderBottom: '1px solid #e5e7eb',
      boxShadow: '0 12px 40px rgba(15, 23, 42, 0.06)',
      position: 'sticky', top: 0, zIndex: 100,
      width: '100%', boxSizing: 'border-box',
    }}>

      {/* Logo */}
      <div
        className="top-header-logo"
        onClick={() => navigate('/crm')}
        style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', flexShrink: 0, marginRight: 12 }}>
        <img src={navLogo} alt="Callified" style={{ height: 50, width: 50, objectFit: 'contain', borderRadius: 10 }} />
        <span className="top-header-brand" style={{
          fontSize: 25,
          fontWeight: 900,
          fontFamily: font,
          background: 'linear-gradient(135deg, #7c3aed 0%, #0ea5e9 100%)',
          WebkitBackgroundClip: 'text',
          WebkitTextFillColor: 'transparent',
          backgroundClip: 'text',
          color: 'transparent',
          letterSpacing: '0.02em',
        }}>
          Callified
        </span>
      </div>

      {/* Tabs */}
      <nav className="top-header-desktop-nav" style={{ display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'nowrap', overflowX: 'auto', overflowY: 'visible', padding: '4px 0' }}>
        {tabBtn('crm', 'CRM', '/crm', 'tab-crm')}

        {primaryTabs.map(t => tabBtn(t.id, t.label, t.path, t.testid))}

        {(isAdminLike || isTeamLeader) && (
          (() => {
            if (allMoreTabs.length === 1) {
              const t = allMoreTabs[0];
              return tabBtn(t.id, t.label, t.path, t.testid);
            }
            if (allMoreTabs.length === 0) return null;
            return (
              <Popover>
                <PopoverTrigger asChild>
                  <button
                    type="button"
                    data-testid="tab-more"
                    aria-haspopup="true"
                    className={`inline-flex items-center gap-2 rounded-full border px-4 py-2 text-sm font-semibold cursor-pointer transition-colors duration-150 ${moreActive ? 'border-slate-300 bg-slate-100 text-slate-950' : 'border-transparent bg-transparent text-slate-700 hover:bg-slate-50'}`}
                  >
                    More <span className="text-xs leading-none">▾</span>
                  </button>
                </PopoverTrigger>
                <PopoverContent side="bottom" align="end" alignOffset={8} className="w-[min(280px,100vw)] p-0 bg-white border border-slate-200 relative max-h-[calc(100svh-90px)] top-5 2xl:top-5 overflow-y-auto shadow-[0_16px_48px_rgba(15,23,42,0.12)]">
                  <div role="menu" className="flex flex-col gap-1.5 p-2">
                    {roleFilteredMoreTabs.map(t => (
                      <PopoverClose asChild key={t.id}>
                        <button data-testid={t.testid} role="menuitem"
                          onClick={() => goTo(t.path)}
                          className={`w-full rounded-xl px-3 cursor-pointer py-2 text-left text-sm font-medium transition-colors duration-150 ${activeTab === t.id ? 'bg-indigo-50 text-indigo-600' : 'text-slate-700 hover:bg-slate-100'}`}>
                          {t.label}
                        </button>
                      </PopoverClose>
                    ))}
                    {superAdminTabs.map(t => (
                      <PopoverClose asChild key={t.id}>
                        <button data-testid={t.testid} role="menuitem"
                          onClick={() => goTo(t.path)}
                          className={`w-full rounded-xl px-3 py-2 text-left text-sm font-medium transition-colors duration-150 ${activeTab === t.id ? 'bg-indigo-50 text-indigo-600' : 'text-slate-700 hover:bg-slate-50'}`}>
                          {t.label}
                        </button>
                      </PopoverClose>
                    ))}
                  </div>
                </PopoverContent>
              </Popover>
            );
          })()
        )}
      </nav>

      {/* Right side */}
      <div className="top-header-actions" style={{ display: 'flex', alignItems: 'center', gap: 16, flexShrink: 0, marginLeft: 12 }}>

        {/* Agent presence toggle */}
        <button
          className="top-header-desktop-account"
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

        {/* Bell */}
        <div ref={notifRef} style={{ position: 'relative' }}>
          <div
            data-testid="header-bell"
            onClick={() => setNotifOpen(o => !o)}
            style={{ position: 'relative', cursor: 'pointer', width: 22, height: 22 }}
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#9ca3af" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
              <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
            </svg>
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
                          📅 {call.scheduled_time ? formatDateTime(call.scheduled_time) : ''}
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

        {/* User avatar + name */}
        {currentUser && (
          <div className="top-header-desktop-account" style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
            <div style={{
              width: 30, height: 30, borderRadius: '50%', flexShrink: 0,
              background: 'linear-gradient(135deg, #6366f1, #a855f7)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 12, fontWeight: 700, color: '#fff', fontFamily: font,
            }}>
              {userInitial}
            </div>
            <span style={{ fontSize: 13, fontWeight: 600, color: '#111827', fontFamily: font, whiteSpace: 'nowrap' }}>
              {userName}{orgName ? ` (${orgName})` : ''}
            </span>
          </div>
        )}

        {/* Logout */}
        {confirmLogout ? (
          <div className="top-header-desktop-account" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
            <span style={{ color: '#f59e0b', fontSize: 13, whiteSpace: 'nowrap', fontFamily: font }}>Log out?</span>
            <button data-testid="logout-confirm-btn"
              onClick={() => { setConfirmLogout(false); handleLogout(); }}
              style={{
                background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.3)',
                color: '#ef4444', borderRadius: 6, padding: '4px 10px',
                cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: font, whiteSpace: 'nowrap',
              }}>Confirm</button>
            <button onClick={() => setConfirmLogout(false)}
              style={{
                background: 'transparent', border: '1px solid #e5e7eb',
                color: '#9ca3af', borderRadius: 6, padding: '4px 10px',
                cursor: 'pointer', fontSize: 12, fontFamily: font, whiteSpace: 'nowrap',
              }}>Cancel</button>
          </div>
        ) : (
          <button className="top-header-desktop-account" data-testid="logout-btn" onClick={() => setConfirmLogout(true)}
            style={{
              background: 'transparent', border: '1px solid #e5e7eb',
              borderRadius: 8, padding: '6px 14px',
              color: '#374151', cursor: 'pointer',
              fontWeight: 600, fontSize: 13, fontFamily: font, whiteSpace: 'nowrap',
            }}>
            Logout
          </button>
        )}

        <button
          type="button"
          className="top-header-mobile-menu-button"
          aria-label={mobileMenuOpen ? 'Close navigation menu' : 'Open navigation menu'}
          aria-expanded={mobileMenuOpen}
          onClick={() => setMobileMenuOpen(open => !open)}
        >
          {mobileMenuOpen ? <X size={22} /> : <Menu size={22} />}
        </button>
      </div>

      {mobileMenuOpen && (
        <div className="top-header-mobile-menu">
          {currentUser && (
            <div className="top-header-mobile-user">
              <div className="top-header-mobile-avatar">{userInitial}</div>
              <div style={{ minWidth: 0 }}>
                <div className="top-header-mobile-user-name">{userName}</div>
                {orgName && <div className="top-header-mobile-org">{orgName}</div>}
              </div>
            </div>
          )}

          <nav className="top-header-mobile-nav" aria-label="Mobile navigation">
            {mobileTabs.map(tab => (
              <button
                key={tab.id}
                type="button"
                data-testid={`mobile-${tab.testid}`}
                className={activeTab === tab.id ? 'active' : ''}
                onClick={() => goTo(tab.path)}
              >
                {tab.label}
              </button>
            ))}
          </nav>

          <div className="top-header-mobile-footer">
            <button type="button" className="top-header-mobile-status" onClick={toggleBreak}>
              <span style={{ background: statusColor }} />
              Status: {statusLabel}
            </button>
            {confirmLogout ? (
              <div className="top-header-mobile-logout-confirm">
                <button type="button" onClick={() => { setConfirmLogout(false); handleLogout(); }}>Confirm logout</button>
                <button type="button" onClick={() => setConfirmLogout(false)}>Cancel</button>
              </div>
            ) : (
              <button type="button" className="top-header-mobile-logout" onClick={() => setConfirmLogout(true)}>
                Logout
              </button>
            )}
          </div>
        </div>
      )}
    </header>
  );
}
