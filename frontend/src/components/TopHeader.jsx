import React, { useState, useEffect, useRef } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import navLogo from '../assets/tg_image_3608761279.png';

const AGENT_TABS = [
  { id: 'campaigns', label: 'Campaigns', path: '/campaigns', testid: 'tab-campaigns' },
];

const PRIMARY_ADMIN_TABS = [
  { id: 'products',  label: 'Products',       path: '/products',  testid: 'tab-products' },
  { id: 'campaigns', label: 'Campaigns',      path: '/campaigns', testid: 'tab-campaigns' },
  { id: 'ops',       label: 'Ops & Tasks',    path: '/ops',       testid: 'tab-ops' },
  { id: 'analytics', label: 'Analytics',      path: '/analytics', testid: 'tab-analytics' },
  { id: 'whatsapp',  label: 'WhatsApp Comms', path: '/whatsapp',  testid: 'tab-whatsapp' },
];

const MORE_ADMIN_TABS = [
  { id: 'integrations',     label: 'Integrations',      path: '/integrations',      testid: 'tab-integrations' },
  { id: 'exotel-accounts', label: 'Provider Accounts',  path: '/exotel-accounts',   testid: 'tab-exotel-accounts' },
  { id: 'monitor',      label: 'Monitor AI Calls',path: '/monitor',      testid: 'tab-monitor' },
  { id: 'knowledge',    label: 'RAG Knowledge',   path: '/knowledge',    testid: 'tab-rag' },
  { id: 'sandbox',      label: 'AI Sandbox',      path: '/sandbox',      testid: 'tab-sandbox' },
  { id: 'scheduled',    label: 'Scheduled',       path: '/scheduled',    testid: 'tab-scheduled' },
  { id: 'billing',      label: 'Billing',         path: '/billing',      testid: 'tab-billing' },
  { id: 'dnd',          label: 'DND',             path: '/dnd',          testid: 'tab-dnd' },
  { id: 'settings',     label: 'Settings',        path: '/settings',     testid: 'tab-settings' },
  { id: 'logs',         label: 'Live Logs',       path: '/logs',         testid: 'tab-logs' },
  { id: 'team',         label: 'Team',            path: '/team',         testid: 'tab-team' },
  { id: 'receptionist', label: 'Receptionist',    path: '/receptionist', testid: 'tab-receptionist' },
];

const font = "'DM Sans', sans-serif";

export default function TopHeader({ userRole, currentUser, handleLogout }) {
  const navigate = useNavigate();
  const location = useLocation();
  const activeTab = location.pathname.replace('/', '') || 'crm';

  const [callingStatus, setCallingStatus] = useState(null);
  const [moreOpen, setMoreOpen] = useState(false);
  const [confirmLogout, setConfirmLogout] = useState(false);
  const moreRef = useRef(null);

  useEffect(() => {
    const fetchStatus = () => {
      const token = localStorage.getItem('token');
      if (!token) return;
      fetch('/api/calling-status', { headers: { Authorization: `Bearer ${token}` } })
        .then(r => r.json())
        .then(data => setCallingStatus(data))
        .catch(() => {});
    };
    fetchStatus();
    const interval = setInterval(fetchStatus, 60000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (!moreOpen) return;
    const onDocClick = (e) => {
      if (moreRef.current && !moreRef.current.contains(e.target)) setMoreOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, [moreOpen]);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setMoreOpen(false); }, [location.pathname]);

  const moreActive = MORE_ADMIN_TABS.some(t => t.id === activeTab);
  const goTo = (path) => { setMoreOpen(false); navigate(path); };

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
        style={{
          background: 'none', border: 'none', cursor: 'pointer',
          padding: '6px 10px', borderRadius: 6,
          fontSize: 13, fontWeight: isActive ? 600 : 500,
          color: isActive ? '#6366f1' : '#374151',
          fontFamily: font, whiteSpace: 'nowrap',
          transition: 'color 0.15s',
        }}
        onMouseEnter={e => { if (!isActive) e.currentTarget.style.color = '#111827'; }}
        onMouseLeave={e => { if (!isActive) e.currentTarget.style.color = '#374151'; }}
      >
        {label}
      </button>
    );
  };

  return (
    <header style={{
      display: 'flex', flexWrap: 'nowrap', alignItems: 'center', gap: '6px',
      padding: '0 24px', height: 56,
      background: '#ffffff', borderBottom: '1px solid #e5e7eb',
      boxShadow: '0 1px 4px rgba(0,0,0,0.05)',
      position: 'sticky', top: 0, zIndex: 100,
      width: '100%', boxSizing: 'border-box',
    }}>

      {/* Logo */}
      <div
        onClick={() => navigate('/crm')}
        style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', flexShrink: 0, marginRight: 12 }}>
        <img src={navLogo} alt="Callified" style={{ height: 36, width: 36, objectFit: 'contain', borderRadius: 10 }} />
        <span style={{ fontSize: 15, fontWeight: 700, color: '#111827', fontFamily: font }}>
          Callified
        </span>
      </div>

      {/* Tabs */}
      <nav style={{ display: 'flex', alignItems: 'center', gap: 2, flex: 1, flexWrap: 'nowrap', overflow: 'visible' }}>
        {tabBtn('crm', 'CRM', '/crm', 'tab-crm')}

        {userRole === 'Agent' && AGENT_TABS.map(t => tabBtn(t.id, t.label, t.path, t.testid))}
        {userRole === 'Admin' && PRIMARY_ADMIN_TABS.map(t => tabBtn(t.id, t.label, t.path, t.testid))}

        {userRole === 'Admin' && (
          <div ref={moreRef} style={{ position: 'relative' }}>
            <button
              data-testid="tab-more"
              onClick={() => setMoreOpen(o => !o)}
              aria-haspopup="true"
              aria-expanded={moreOpen}
              style={{
                background: moreActive ? 'rgba(99,102,241,0.1)' : 'rgba(99,102,241,0.08)',
                border: '1px solid rgba(99,102,241,0.2)',
                borderRadius: 20, cursor: 'pointer',
                padding: '5px 14px', fontSize: 13,
                fontWeight: 600, color: '#6366f1',
                fontFamily: font, whiteSpace: 'nowrap',
                display: 'flex', alignItems: 'center', gap: 4,
              }}>
              More <span style={{ fontSize: '0.7em' }}>▾</span>
            </button>
            {moreOpen && (
              <div role="menu" style={{
                position: 'absolute', top: 'calc(100% + 6px)', right: 0, minWidth: '220px',
                background: '#ffffff', border: '1px solid #e5e7eb',
                borderRadius: 10, padding: 6,
                boxShadow: '0 8px 24px rgba(0,0,0,0.10)', zIndex: 1000,
                display: 'flex', flexDirection: 'column', gap: 2,
              }}>
                {MORE_ADMIN_TABS.map(t => (
                  <button key={t.id} data-testid={t.testid} role="menuitem"
                    onClick={() => goTo(t.path)}
                    style={{
                      display: 'flex', alignItems: 'center',
                      padding: '8px 12px', textAlign: 'left', cursor: 'pointer',
                      background: activeTab === t.id ? 'rgba(99,102,241,0.08)' : 'transparent',
                      border: 'none', borderRadius: 6,
                      color: activeTab === t.id ? '#6366f1' : '#374151',
                      fontSize: 13, fontWeight: activeTab === t.id ? 700 : 500,
                      fontFamily: font,
                    }}>
                    {t.label}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </nav>

      {/* Right side */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexShrink: 0, marginLeft: 8 }}>

        {/* AI Active status */}
        {callingStatus && (
          <span style={{
            display: 'inline-flex', alignItems: 'center', gap: 5,
            fontSize: 13, fontWeight: 600, fontFamily: font,
            color: callingStatus.allowed ? '#10b981' : '#ef4444',
          }}>
            <span style={{
              width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
              background: callingStatus.allowed ? '#10b981' : '#ef4444',
            }} />
            {callingStatus.allowed ? 'AI Active' : 'AI Paused'}
          </span>
        )}

        {/* Bell */}
        <div style={{ position: 'relative', cursor: 'pointer', width: 22, height: 22 }}>
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#9ca3af" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
            <path d="M13.73 21a2 2 0 0 1-3.46 0"/>
          </svg>
          <span style={{
            position: 'absolute', top: -2, right: -2,
            width: 7, height: 7, borderRadius: '50%',
            background: '#ef4444', border: '1.5px solid #fff',
          }} />
        </div>

        {/* User avatar + name */}
        {currentUser && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
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
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
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
          <button data-testid="logout-btn" onClick={() => setConfirmLogout(true)}
            style={{
              background: 'transparent', border: '1px solid #e5e7eb',
              borderRadius: 8, padding: '6px 14px',
              color: '#374151', cursor: 'pointer',
              fontWeight: 600, fontSize: 13, fontFamily: font, whiteSpace: 'nowrap',
            }}>
            Logout
          </button>
        )}
      </div>
    </header>
  );
}
