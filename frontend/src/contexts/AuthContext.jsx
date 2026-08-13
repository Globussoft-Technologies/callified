import React, { createContext, useContext, useState, useEffect, useCallback, useMemo } from 'react';
import { API_URL } from '../constants/api';

const AuthContext = createContext(null);

// Clear everything the app may have stored in the browser so a logout leaves
// no cached credentials, preferences, or stale data behind.
function clearBrowserData() {
  try { localStorage.clear(); } catch { /* ignore */ }
  try { sessionStorage.clear(); } catch { /* ignore */ }
  try {
    if (typeof document !== 'undefined' && document.cookie) {
      const hostname = window.location.hostname;
      document.cookie.split(';').forEach(c => {
        const name = c.split('=')[0].trim();
        // Best-effort deletion for current path and root, with and without domain.
        document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;`;
        document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/; domain=${hostname};`;
      });
    }
  } catch { /* ignore */ }
  if (typeof caches !== 'undefined') {
    caches.keys().then(keys => Promise.all(keys.map(k => caches.delete(k)))).catch(() => {});
  }
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.getRegistrations().then(regs => regs.forEach(r => r.unregister())).catch(() => {});
  }
}

export function AuthProvider({ children }) {
  const [authToken, setAuthToken] = useState(null);
  const [currentUser, setCurrentUser] = useState(null);
  const [permissions, setPermissions] = useState(null);
  // authReady becomes true once /auth/me has run (or when there's no token).
  // apiFetch only calls clearSession on 401 after authReady=true to avoid a
  // race where a component's first fetch clears a stale-but-not-yet-validated
  // token before /auth/me gets a chance to do it cleanly.
  const [authReady, setAuthReady] = useState(false);

  const clearSession = useCallback(() => {
    setAuthToken(null);
    setCurrentUser(null);
    setPermissions(null);
    setAuthReady(true);
    clearBrowserData();
  }, []);

  const apiFetch = useCallback(async (url, options = {}) => {
    const res = await fetch(url, {
      ...options,
      credentials: 'include',
      headers: {
        ...options.headers,
        ...(authToken ? { 'Authorization': `Bearer ${authToken}` } : {}),
      },
    });
    if (res.status === 401) {
      if (authReady) clearSession();
      throw new Error('Session expired');
    }
    return res;
  }, [authToken, clearSession, authReady]);

  // Mints a 60-second SSE ticket via Authorization header, returning the
  // ticket string. Callers append it as ?ticket=… to EventSource URLs so the
  // long-lived auth JWT never appears in URLs (issue #80).
  const fetchSseTicket = useCallback(async () => {
    const res = await apiFetch(`${API_URL}/sse/ticket`);
    if (!res.ok) throw new Error(`sse ticket: ${res.status}`);
    const data = await res.json();
    return data.ticket;
  }, [apiFetch]);
  

  // Background revalidation: if we have a token, verify it's still valid.
  // Runs without blocking the UI — dashboard is already on-screen.
  // Sets authReady=true when done so apiFetch knows it's safe to clear the
  // session on 401 (rather than racing with this check on first render).
  useEffect(() => {
    setAuthReady(false);
    fetch(`${API_URL}/auth/me`, { credentials: 'include', headers: authToken ? { 'Authorization': `Bearer ${authToken}` } : {} })
      .then(r => r.ok ? r.json() : Promise.reject())
      .then(u => {
        setCurrentUser(u);
        setAuthReady(true);
        fetch(`${API_URL}/users/me/permissions`, {
          credentials: 'include',
          headers: authToken ? { 'Authorization': `Bearer ${authToken}` } : {},
        })
          .then(r => r.ok ? r.json() : Promise.reject())
          .then(data => {
            setPermissions(Array.isArray(data.permissions) ? data.permissions : []);
          })
          .catch(() => setPermissions(null));
      })
      .catch(() => {
        setAuthToken(null);
        setCurrentUser(null);
        setPermissions(null);
        setAuthReady(true);
      });
  }, [authToken, clearSession]);

  const login = async (email, password) => {
    const res = await fetch(`${API_URL}/auth/login`, {
      method: 'POST', credentials: 'include', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ email, password })
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      const err = new Error(data.error || data.detail || 'Login failed');
      err.code = data.code || null;
      err.expiresAt = data.expires_at || null;
      err.plan = data.plan || null;
      err.supportEmail = data.support_email || null;
      err.status = res.status;
      throw err;
    }
    const data = await res.json();
    setAuthToken(data.access_token);
    setCurrentUser(data.user);
    return data;
  };

  // Helper: true when the current user should not see AI-related UI sections.
  const hideAiFeatures = Boolean(currentUser?.hide_ai_features);
  const permissionSet = useMemo(() => new Set(permissions || []), [permissions]);
  const hasPermission = useCallback((key) => {
    if (!key) return true;
    if (permissions === null) return true;
    return permissionSet.has(key);
  }, [permissionSet, permissions]);

  const signup = async (orgName, fullName, email, password) => {
    const res = await fetch(`${API_URL}/auth/signup`, {
      method: 'POST', credentials: 'include', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ org_name: orgName, full_name: fullName, email, password })
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      const err = new Error(data.error || data.detail || 'Signup failed');
      err.code = data.code || null;
      err.expiresAt = data.expires_at || null;
      err.plan = data.plan || null;
      err.supportEmail = data.support_email || null;
      err.status = res.status;
      throw err;
    }
    const data = await res.json();
    setAuthToken(data.access_token);
    setCurrentUser(data.user);
    return data;
  };

  const logout = useCallback(() => {
    fetch(`${API_URL}/auth/logout`, { method: 'POST', credentials: 'include' })
      .catch(() => {})
      .finally(() => {
        clearSession();
        if (typeof window !== 'undefined') {
          window.location.href = '/';
        }
      });
  }, [clearSession]);

  // loginWithToken finishes an SSO handshake. The backend already minted our
  // own JWT and bounced the browser to /sso/return?token=…; this helper
  // commits the token and pulls the canonical user profile from /auth/me so
  // the SPA boots into the same shape as a regular password login.
  const loginWithToken = async (token) => {
    const res = await fetch(`${API_URL}/auth/session`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    });
    if (!res.ok) {
      clearSession();
      throw new Error('SSO token rejected');
    }
    const data = await res.json();
    setAuthToken(token);
    setCurrentUser(data.user);
    return data.user;
  };

  return (
    <AuthContext.Provider value={{ authToken, currentUser, setCurrentUser, authReady, loading: !authReady, apiFetch, fetchSseTicket, login, signup, logout, loginWithToken, hideAiFeatures, permissions, hasPermission }}>
      {children}
    </AuthContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
