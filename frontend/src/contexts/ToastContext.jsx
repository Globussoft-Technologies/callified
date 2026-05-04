import React, { createContext, useContext, useState, useCallback } from 'react';

const ToastContext = createContext(null);

export function ToastProvider({ children }) {
  const [toasts, setToasts] = useState([]);

  const showToast = useCallback((message, type = 'success') => {
    const id = Date.now() + Math.random();
    setToasts(prev => [...prev.slice(-4), { id, message, type }]);
    setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 4000);
  }, []);

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      <div style={{
        position: 'fixed',
        bottom: '24px',
        right: '24px',
        zIndex: 9999,
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
        pointerEvents: 'none',
      }}>
        {toasts.map(t => (
          <div key={t.id} style={{
            padding: '12px 18px',
            borderRadius: '10px',
            background: t.type === 'error' ? 'rgba(239,68,68,0.18)' : 'rgba(34,197,94,0.18)',
            border: `1px solid ${t.type === 'error' ? 'rgba(239,68,68,0.45)' : 'rgba(34,197,94,0.45)'}`,
            color: t.type === 'error' ? '#fca5a5' : '#4ade80',
            fontWeight: 600,
            fontSize: '0.88rem',
            boxShadow: '0 4px 20px rgba(0,0,0,0.4)',
            backdropFilter: 'blur(8px)',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            minWidth: '200px',
            maxWidth: '360px',
            animation: 'fadeInUp 0.25s ease',
            pointerEvents: 'auto',
          }}>
            <span style={{fontSize: '1rem'}}>{t.type === 'error' ? '⚠️' : '✓'}</span>
            {t.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  return useContext(ToastContext);
}
