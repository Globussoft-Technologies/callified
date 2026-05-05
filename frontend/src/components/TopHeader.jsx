import React, { useState, useEffect, useRef } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';

// Tabs that stay visible in the top nav. Everything else collapses into the
// More ▾ dropdown so the bar doesn't overflow into a horizontal scroll the
// way the all-flat layout did. Issue #36.
//
// AGENT_TABS = the subset Agents are allowed to see alongside CRM. The full
// PRIMARY_ADMIN_TABS list is for Admin only. Anything an Agent shouldn't be
// able to mutate (org-wide config, billing, integrations, team mgmt) stays
// admin-only via PRIMARY_ADMIN_TABS / MORE_ADMIN_TABS below — backend
// enforces the same split via adminAuth on the write endpoints.
const AGENT_TABS = [
  { id: 'campaigns', label: '📢 Campaigns', path: '/campaigns', testid: 'tab-campaigns' },
];

const PRIMARY_ADMIN_TABS = [
  { id: 'products',  label: '📦 Products',       path: '/products',  testid: 'tab-products' },
  { id: 'campaigns', label: '📢 Campaigns',      path: '/campaigns', testid: 'tab-campaigns' },
  { id: 'ops',       label: '📋 Ops & Tasks',    path: '/ops',       testid: 'tab-ops' },
  { id: 'analytics', label: '📈 Analytics',      path: '/analytics', testid: 'tab-analytics' },
  { id: 'whatsapp',  label: '💬 WhatsApp Comms', path: '/whatsapp',  testid: 'tab-whatsapp' },
];

// Lower-priority / less-frequently-used admin tabs that move under "More ▾".
const MORE_ADMIN_TABS = [
  { id: 'integrations', label: '🔌 Integrations',     path: '/integrations', testid: 'tab-integrations' },
  { id: 'monitor',      label: '🎙️ Monitor AI Calls', path: '/monitor',      testid: 'tab-monitor' },
  { id: 'knowledge',    label: '🧠 RAG Knowledge',    path: '/knowledge',    testid: 'tab-rag' },
  { id: 'sandbox',      label: '🎯 AI Sandbox',       path: '/sandbox',      testid: 'tab-sandbox' },
  { id: 'scheduled',    label: '📅 Scheduled',        path: '/scheduled',    testid: 'tab-scheduled' },
  { id: 'billing',      label: '💳 Billing',          path: '/billing',      testid: 'tab-billing' },
  { id: 'dnd',          label: '🚫 DND',              path: '/dnd',          testid: 'tab-dnd' },
  { id: 'settings',     label: '⚙️ Settings',         path: '/settings',     testid: 'tab-settings' },
  { id: 'logs',         label: '📋 Live Logs',        path: '/logs',         testid: 'tab-logs' },
  { id: 'team',         label: '👥 Team',             path: '/team',         testid: 'tab-team' },
];

export default function TopHeader({
  userRole,
  currentUser,
  handleLogout
}) {
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

  // Close the More dropdown when the user clicks anywhere outside it.
  useEffect(() => {
    if (!moreOpen) return;
    const onDocClick = (e) => {
      if (moreRef.current && !moreRef.current.contains(e.target)) setMoreOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, [moreOpen]);

  // Auto-close on route change so the menu doesn't linger after navigating.
  useEffect(() => { setMoreOpen(false); }, [location.pathname]);

  const moreActive = MORE_ADMIN_TABS.some(t => t.id === activeTab);
  const goTo = (path) => { setMoreOpen(false); navigate(path); };

  return (
    <header className="header">
      <div className="logo" style={{display: 'flex', alignItems: 'center', gap: '10px'}}>
        <img src="/logo.png" alt="Globussoft Logo" style={{width: '32px', height: '32px', borderRadius: '8px', objectFit: 'contain'}} />
        Globussoft Generative AI Dialer <span className="badge" style={{background: 'rgba(34, 197, 94, 0.2)', color: '#4ade80', ml: 2}}>LIVE</span>
      </div>

      <div className="tab-bar" style={{display: 'flex', gap: '8px', alignItems: 'center', flex: 1, flexWrap: 'nowrap'}}>
        <button data-testid="tab-crm" className={`tab-btn ${activeTab === 'crm' ? 'active' : ''}`} onClick={() => navigate('/crm')}>📊 CRM</button>
        {userRole === 'Agent' && AGENT_TABS.map(t => (
          <button key={t.id} data-testid={t.testid}
            className={`tab-btn ${activeTab === t.id ? 'active' : ''}`}
            onClick={() => navigate(t.path)}>
            {t.label}
          </button>
        ))}
        {userRole === 'Admin' && PRIMARY_ADMIN_TABS.map(t => (
          <button key={t.id} data-testid={t.testid}
            className={`tab-btn ${activeTab === t.id ? 'active' : ''}`}
            onClick={() => navigate(t.path)}>
            {t.label}
          </button>
        ))}
        {userRole === 'Admin' && (
          <div ref={moreRef} style={{position: 'relative'}}>
            <button data-testid="tab-more"
              className={`tab-btn ${moreActive ? 'active' : ''}`}
              onClick={() => setMoreOpen(o => !o)}
              aria-haspopup="true"
              aria-expanded={moreOpen}>
              More <span style={{fontSize: '0.7em', marginLeft: '4px'}}>▾</span>
            </button>
            {moreOpen && (
              <div role="menu" style={{
                position: 'absolute', top: 'calc(100% + 6px)', right: 0, minWidth: '220px',
                background: 'rgba(15,23,42,0.98)', border: '1px solid rgba(255,255,255,0.08)',
                borderRadius: '10px', padding: '6px',
                boxShadow: '0 12px 32px rgba(0,0,0,0.45)', zIndex: 1000,
                display: 'flex', flexDirection: 'column', gap: '2px',
              }}>
                {MORE_ADMIN_TABS.map(t => (
                  <button key={t.id} data-testid={t.testid} role="menuitem"
                    onClick={() => goTo(t.path)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: '8px',
                      padding: '8px 12px', textAlign: 'left', cursor: 'pointer',
                      background: activeTab === t.id ? 'rgba(99,102,241,0.18)' : 'transparent',
                      border: 'none', borderRadius: '6px',
                      color: activeTab === t.id ? '#a5b4fc' : '#cbd5e1',
                      fontSize: '0.85rem', fontWeight: activeTab === t.id ? 700 : 500,
                    }}>
                    {t.label}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}

        <div className="header-user-info" style={{marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: '8px', flexShrink: 0}}>
          {callingStatus && (
            <span style={{
              height: '38px',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '6px',
              padding: '0 12px',
              borderRadius: '8px',
              background: callingStatus.allowed ? 'rgba(34, 197, 94, 0.15)' : 'rgba(239, 68, 68, 0.15)',
              border: `1px solid ${callingStatus.allowed ? 'rgba(34, 197, 94, 0.3)' : 'rgba(239, 68, 68, 0.3)'}`,
              color: callingStatus.allowed ? '#4ade80' : '#fca5a5',
              fontWeight: 600,
              fontSize: '0.78rem',
              whiteSpace: 'nowrap',
            }}>
              <span style={{
                width: '7px', height: '7px', borderRadius: '50%',
                background: callingStatus.allowed ? '#22c55e' : '#ef4444',
                flexShrink: 0,
              }} />
              {callingStatus.allowed ? 'Calls Active' : 'Calls Paused'}
            </span>
          )}
          {currentUser && (
            <span style={{
              height: '38px',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '6px',
              padding: '0 12px',
              borderRadius: '8px',
              background: 'rgba(255,255,255,0.04)',
              border: '1px solid rgba(255,255,255,0.08)',
              fontSize: '0.78rem',
              color: '#94a3b8',
              whiteSpace: 'nowrap',
              fontWeight: 600,
            }}>
              👤 {currentUser.full_name || currentUser.email}{currentUser.org_name ? ` (${currentUser.org_name})` : ''}
            </span>
          )}
          {confirmLogout ? (
            <div style={{
              height: '38px', display: 'inline-flex', alignItems: 'center', gap: '6px',
              padding: '0 10px', background: 'rgba(239,68,68,0.08)',
              border: '1px solid rgba(239,68,68,0.3)', borderRadius: '8px',
            }}>
              <span style={{color: '#fbbf24', fontSize: '0.78rem', whiteSpace: 'nowrap'}}>Log out?</span>
              <button data-testid="logout-confirm-btn"
                onClick={() => { setConfirmLogout(false); handleLogout(); }}
                style={{
                  background: 'rgba(239,68,68,0.2)', border: '1px solid rgba(239,68,68,0.45)',
                  color: '#ef4444', borderRadius: '6px', padding: '4px 10px',
                  cursor: 'pointer', fontSize: '0.75rem', fontWeight: 600, whiteSpace: 'nowrap',
                }}>
                Confirm
              </button>
              <button onClick={() => setConfirmLogout(false)}
                style={{
                  background: 'transparent', border: '1px solid rgba(255,255,255,0.15)',
                  color: '#94a3b8', borderRadius: '6px', padding: '4px 10px',
                  cursor: 'pointer', fontSize: '0.75rem', whiteSpace: 'nowrap',
                }}>
                Cancel
              </button>
            </div>
          ) : (
            <button data-testid="logout-btn" onClick={() => setConfirmLogout(true)}
              style={{
                height: '38px',
                display: 'inline-flex',
                alignItems: 'center',
                gap: '5px',
                padding: '0 14px',
                background: 'rgba(239,68,68,0.15)',
                border: '1px solid rgba(239,68,68,0.3)',
                borderRadius: '8px',
                color: '#fca5a5',
                cursor: 'pointer',
                fontWeight: 600,
                fontSize: '0.82rem',
                whiteSpace: 'nowrap',
              }}>
              🚪 Logout
            </button>
          )}
        </div>
      </div>
    </header>
  );
}
