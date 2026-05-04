import React, { useState, useEffect, useRef } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';

const PRIMARY_TABS = [
  { id: 'crm',       label: '📊 CRM',            path: '/crm',       adminOnly: false },
  { id: 'products',  label: '📦 Products',        path: '/products',  adminOnly: true  },
  { id: 'campaigns', label: '📢 Campaigns',       path: '/campaigns', adminOnly: true  },
  { id: 'ops',       label: '📋 Ops & Tasks',     path: '/ops',       adminOnly: true  },
  { id: 'analytics', label: '📈 Analytics',       path: '/analytics', adminOnly: true  },
  { id: 'whatsapp',  label: '💬 WhatsApp Comms',  path: '/whatsapp',  adminOnly: true  },
];

const MORE_TABS = [
  { id: 'integrations', label: '🔌 Integrations',     path: '/integrations' },
  { id: 'monitor',      label: '🎙️ Monitor AI Calls', path: '/monitor'      },
  { id: 'knowledge',    label: '🧠 RAG Knowledge',    path: '/knowledge'    },
  { id: 'sandbox',      label: '🎯 AI Sandbox',       path: '/sandbox'      },
  { id: 'scheduled',    label: '📅 Scheduled',        path: '/scheduled'    },
  { id: 'billing',      label: '💳 Billing',          path: '/billing'      },
  { id: 'dnd',          label: '🚫 DND',              path: '/dnd'          },
  { id: 'settings',     label: '⚙️ Settings',         path: '/settings'     },
  { id: 'logs',         label: '📋 Live Logs',        path: '/logs'         },
  { id: 'team',         label: '👥 Team',             path: '/team'         },
];

export default function TopHeader({ userRole, currentUser, handleLogout }) {
  const navigate = useNavigate();
  const location = useLocation();
  const activeTab = location.pathname.replace('/', '') || 'crm';

  const [callingStatus, setCallingStatus] = useState(null);
  const [confirmLogout, setConfirmLogout] = useState(false);
  const [moreOpen, setMoreOpen] = useState(false);
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
    const handleOutside = (e) => {
      if (moreRef.current && !moreRef.current.contains(e.target)) setMoreOpen(false);
    };
    document.addEventListener('mousedown', handleOutside);
    return () => document.removeEventListener('mousedown', handleOutside);
  }, [moreOpen]);

  const activeInMore = MORE_TABS.some(t => t.id === activeTab);
  const activeMoreLabel = MORE_TABS.find(t => t.id === activeTab)?.label;

  return (
    <header className="header">
      <div className="logo" style={{display: 'flex', alignItems: 'center', gap: '10px', flexShrink: 0}}>
        <img src="/logo.png" alt="Globussoft Logo" style={{width: '32px', height: '32px', borderRadius: '8px', objectFit: 'contain'}} />
        Globussoft Generative AI Dialer <span className="badge" style={{background: 'rgba(34, 197, 94, 0.2)', color: '#4ade80'}}>LIVE</span>
      </div>

      <div className="tab-bar" style={{display: 'flex', gap: '6px', alignItems: 'center', flex: 1, flexWrap: 'nowrap', overflow: 'visible', minWidth: 0}}>
        {PRIMARY_TABS.map(tab => {
          if (tab.adminOnly && userRole !== 'Admin') return null;
          return (
            <button
              key={tab.id}
              data-testid={`tab-${tab.id}`}
              className={`tab-btn ${activeTab === tab.id ? 'active' : ''}`}
              onClick={() => navigate(tab.path)}
              style={{flexShrink: 0}}
            >
              {tab.label}
            </button>
          );
        })}

        {userRole === 'Admin' && (
          <div ref={moreRef} style={{position: 'relative', flexShrink: 0}}>
            <button
              data-testid="tab-more"
              className={`tab-btn ${activeInMore ? 'active' : ''}`}
              onClick={() => setMoreOpen(o => !o)}
            >
              {activeInMore ? activeMoreLabel : 'More'} ▾
            </button>

            {moreOpen && (
              <div style={{
                position: 'absolute',
                top: 'calc(100% + 6px)',
                left: 0,
                background: '#1e293b',
                border: '1px solid rgba(255,255,255,0.12)',
                borderRadius: '10px',
                padding: '6px',
                display: 'flex',
                flexDirection: 'column',
                gap: '3px',
                minWidth: '200px',
                zIndex: 1000,
                boxShadow: '0 8px 24px rgba(0,0,0,0.5)',
              }}>
                {MORE_TABS.map(tab => (
                  <button
                    key={tab.id}
                    data-testid={`tab-${tab.id}`}
                    className={`tab-btn ${activeTab === tab.id ? 'active' : ''}`}
                    onClick={() => { navigate(tab.path); setMoreOpen(false); }}
                    style={{textAlign: 'left', width: '100%', justifyContent: 'flex-start'}}
                  >
                    {tab.label}
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
            <div style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: '8px', padding: '0 10px', height: '38px' }}>
              <span style={{ fontSize: '0.78rem', color: '#fca5a5', whiteSpace: 'nowrap' }}>Log out?</span>
              <button onClick={handleLogout}
                style={{ height: '26px', padding: '0 10px', background: 'rgba(239,68,68,0.25)', border: '1px solid rgba(239,68,68,0.5)', borderRadius: '6px', color: '#fca5a5', cursor: 'pointer', fontWeight: 700, fontSize: '0.78rem' }}>
                Yes
              </button>
              <button onClick={() => setConfirmLogout(false)}
                style={{ height: '26px', padding: '0 10px', background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', color: '#94a3b8', cursor: 'pointer', fontSize: '0.78rem' }}>
                No
              </button>
            </div>
          ) : (
            <button data-testid="logout-btn" onClick={() => setConfirmLogout(true)}
              style={{
                height: '38px', display: 'inline-flex', alignItems: 'center', gap: '5px',
                padding: '0 14px', background: 'rgba(239,68,68,0.15)', border: '1px solid rgba(239,68,68,0.3)',
                borderRadius: '8px', color: '#fca5a5', cursor: 'pointer', fontWeight: 600,
                fontSize: '0.82rem', whiteSpace: 'nowrap',
              }}>
              🚪 Logout
            </button>
          )}
        </div>
      </div>
    </header>
  );
}
