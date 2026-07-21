import { useState, useEffect, useRef, useCallback } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { API_URL } from '../constants/api';

export default function NotificationBell({ apiFetch }) {
  const { fetchSseTicket } = useAuth();
  const [notifications, setNotifications] = useState([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [open, setOpen] = useState(false);
  const [connected, setConnected] = useState(false);
  const bellRef = useRef(null);

  const fetchNotifications = useCallback(async () => {
    try {
      const res = await apiFetch(`${API_URL}/notifications`);
      if (!res.ok) return;
      const data = await res.json();
      setNotifications(Array.isArray(data) ? data : []);
    } catch { /* ignore */ }
  }, [apiFetch]);

  const fetchUnreadCount = useCallback(async () => {
    try {
      const res = await apiFetch(`${API_URL}/notifications/unread-count`);
      if (!res.ok) return;
      const data = await res.json();
      setUnreadCount(data.count || 0);
    } catch { /* ignore */ }
  }, [apiFetch]);

  // Initial load.
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => {
    fetchNotifications();
    fetchUnreadCount();
  }, [fetchNotifications, fetchUnreadCount]);

  // SSE stream for real-time notifications.
  useEffect(() => {
    let es = null;
    let cancelled = false;

    const connect = async () => {
      try {
        const ticket = await fetchSseTicket();
        if (cancelled) return;
        es = new EventSource(`${API_URL}/sse/notifications?ticket=${encodeURIComponent(ticket)}`);
        es.addEventListener('notification', (e) => {
          try {
            const n = JSON.parse(e.data);
            setNotifications(prev => [n, ...prev]);
            setUnreadCount(c => c + 1);
          } catch { /* ignore */ }
        });
        es.onopen = () => setConnected(true);
        es.onerror = () => setConnected(false);
      } catch { setConnected(false); }
    };

    connect();
    return () => {
      cancelled = true;
      if (es) es.close();
    };
  }, [fetchSseTicket]);

  // Close dropdown on outside click.
  useEffect(() => {
    if (!open) return;
    const onDocClick = (e) => {
      if (bellRef.current && !bellRef.current.contains(e.target)) setOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, [open]);

  const markRead = async (id) => {
    try {
      const res = await apiFetch(`${API_URL}/notifications/${id}/read`, { method: 'PUT' });
      if (res.ok) {
        setNotifications(prev => prev.map(n => n.id === id ? { ...n, is_read: true } : n));
        setUnreadCount(c => Math.max(0, c - 1));
      }
    } catch { /* ignore */ }
  };

  const markAllRead = async () => {
    try {
      const res = await apiFetch(`${API_URL}/notifications/read-all`, { method: 'PUT' });
      if (res.ok) {
        setNotifications(prev => prev.map(n => ({ ...n, is_read: true })));
        setUnreadCount(0);
      }
    } catch { /* ignore */ }
  };

  const unread = notifications.filter(n => !n.is_read);

  return (
    <div ref={bellRef} style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen(o => !o)}
        style={{
          background: 'none', border: 'none', cursor: 'pointer', position: 'relative',
          padding: 8, borderRadius: 8,
        }}
        aria-label="Notifications"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#374151" strokeWidth="2">
          <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
          <path d="M13.73 21a2 2 0 0 1-3.46 0" />
        </svg>
        {unreadCount > 0 && (
          <span style={{
            position: 'absolute', top: 4, right: 4, background: '#ef4444', color: '#fff',
            borderRadius: 10, fontSize: 10, fontWeight: 700, padding: '1px 5px', minWidth: 16,
            textAlign: 'center',
          }}>
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div style={{
          position: 'absolute', top: 'calc(100% + 8px)', right: 0, width: 360, maxWidth: '90vw',
          background: '#fff', border: '1px solid #e5e7eb', borderRadius: 12,
          boxShadow: '0 10px 30px rgba(0,0,0,0.12)', zIndex: 150,
        }}>
          <div style={{
            display: 'flex', justifyContent: 'space-between', alignItems: 'center',
            padding: '12px 16px', borderBottom: '1px solid #e5e7eb',
          }}>
            <span style={{ fontSize: 14, fontWeight: 600, color: '#111827' }}>Notifications</span>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ fontSize: 10, color: connected ? '#10b981' : '#9ca3af' }}>
                {connected ? '● live' : '● offline'}
              </span>
              {unread.length > 0 && (
                <button
                  onClick={markAllRead}
                  style={{ fontSize: 11, color: '#6366f1', background: 'none', border: 'none', cursor: 'pointer' }}
                >
                  Mark all read
                </button>
              )}
            </div>
          </div>

          <div style={{ maxHeight: 360, overflowY: 'auto' }}>
            {notifications.length === 0 ? (
              <div style={{ padding: 24, textAlign: 'center', color: '#9ca3af', fontSize: 13 }}>
                No notifications yet.
              </div>
            ) : notifications.map(n => (
              <div
                key={n.id}
                style={{
                  padding: '12px 16px', borderBottom: '1px solid #f3f4f6',
                  background: n.is_read ? '#fff' : '#f9fafb',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
                  <div style={{ flex: 1 }}>
                    <div style={{ fontSize: 13, fontWeight: 600, color: '#111827' }}>{n.title}</div>
                    <div style={{ fontSize: 12, color: '#6b7280', marginTop: 2 }}>{n.body}</div>
                    <div style={{ fontSize: 10, color: '#9ca3af', marginTop: 4 }}>
                      {new Date(n.created_at).toLocaleString()}
                    </div>
                  </div>
                  {!n.is_read && (
                    <button
                      onClick={() => markRead(n.id)}
                      style={{
                        fontSize: 10, color: '#6366f1', background: 'none', border: 'none',
                        cursor: 'pointer', whiteSpace: 'nowrap',
                      }}
                    >
                      Mark read
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
