import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { API_URL } from '../constants/api';

const AuthContext = createContext(null);

// Storage-aware helpers. When a tab is flagged as an impersonation session
// (sessionStorage.authMode === 'impersonation'), all auth state lives in
// sessionStorage so it dies with the tab and does NOT clobber the developer's
// own JWT in localStorage. Every other tab uses localStorage exactly as before.
//
// The check runs at call time (not module load) so we pick up the flag set by
// SsoExchange.jsx synchronously before AuthProvider's initial state read.
function getStore() {
  try {
    if (sessionStorage.getItem('authMode') === 'impersonation') {
      return sessionStorage;
    }
  } catch {
    /* private mode / Safari quirks — fall through to localStorage */
  }
  return localStorage;
}

// Safely parse a cached user blob from the active store.
function loadCachedUser() {
  try {
    const raw = getStore().getItem('currentUser');
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

export function AuthProvider({ children }) {
  const [authToken, setAuthToken] = useState(() => getStore().getItem('authToken') || null);
  // Seed currentUser from storage so the dashboard renders instantly on
  // refresh — no login-page flash, no loading splash. /auth/me revalidates in
  // the background and clears the session if the token is no longer good.
  const [currentUser, setCurrentUser] = useState(loadCachedUser);
  // authReady becomes true once /auth/me has run (or when there's no token).
  // apiFetch only calls clearSession on 401 after authReady=true to avoid a
  // race where a component's first fetch clears a stale-but-not-yet-validated
  // token before /auth/me gets a chance to do it cleanly.
  const [authReady, setAuthReady] = useState(!localStorage.getItem('authToken'));

  const clearSession = useCallback(() => {
    setAuthToken(null);
    setCurrentUser(null);
    setAuthReady(true);
    localStorage.removeItem('authToken');
    localStorage.removeItem('currentUser');
  }, []);

  const apiFetch = useCallback(async (url, options = {}) => {
    const res = await fetch(url, {
      ...options,
      headers: { ...options.headers, 'Authorization': `Bearer ${authToken}` }
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
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (!authToken) { setAuthReady(true); return; }
    setAuthReady(false);
    fetch(`${API_URL}/auth/me`, { headers: { 'Authorization': `Bearer ${authToken}` } })
      .then(r => r.ok ? r.json() : Promise.reject())
      .then(u => {
        setCurrentUser(u);
        localStorage.setItem('currentUser', JSON.stringify(u));
        setAuthReady(true);
      })
      .catch(() => clearSession());
  }, [authToken, clearSession]);

  const login = async (email, password) => {
    const res = await fetch(`${API_URL}/auth/login`, {
      method: 'POST', headers: {'Content-Type': 'application/json'},
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
    const store = getStore();
    store.setItem('authToken', data.access_token);
    store.setItem('currentUser', JSON.stringify(data.user));
    return data;
  };

  // Helper: true when the current user should not see AI-related UI sections.
  const hideAiFeatures = Boolean(currentUser?.hide_ai_features);

  const signup = async (orgName, fullName, email, password) => {
    const res = await fetch(`${API_URL}/auth/signup`, {
      method: 'POST', headers: {'Content-Type': 'application/json'},
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
    const store = getStore();
    store.setItem('authToken', data.access_token);
    store.setItem('currentUser', JSON.stringify(data.user));
    return data;
  };

  const logout = clearSession;

  // loginWithToken finishes an SSO handshake (or developer impersonation).
  // The backend already minted our own JWT; this helper commits the token and
  // pulls the canonical user profile from /auth/me so the SPA boots into the
  // same shape as a regular password login.
  //
  // Storage destination depends on sessionStorage.authMode — impersonation
  // tabs write to sessionStorage, every other path writes to localStorage.
  const loginWithToken = async (token) => {
    setAuthToken(token);
    const store = getStore();
    store.setItem('authToken', token);
    const res = await fetch(`${API_URL}/auth/me`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) {
      clearSession();
      throw new Error('SSO token rejected by /auth/me');
    }
    const user = await res.json();
    setCurrentUser(user);
    store.setItem('currentUser', JSON.stringify(user));
    if (user && user.dev_actor) {
      try { sessionStorage.setItem('devActor', user.dev_actor); } catch {}
    }
    return user;
  };

  return (
    <AuthContext.Provider value={{ authToken, currentUser, setCurrentUser, authReady, apiFetch, fetchSseTicket, login, signup, logout, loginWithToken, hideAiFeatures }}>
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
