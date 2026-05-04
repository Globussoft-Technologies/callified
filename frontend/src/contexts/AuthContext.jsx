import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { API_URL } from '../constants/api';

const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [authToken, setAuthToken] = useState(localStorage.getItem('authToken') || null);
  const [currentUser, setCurrentUser] = useState(null);
  const [loading, setLoading] = useState(!!localStorage.getItem('authToken'));

  const apiFetch = useCallback(async (url, options = {}) => {
    return fetch(url, {
      ...options,
      headers: { ...options.headers, 'Authorization': `Bearer ${authToken}` }
    });
  }, [authToken]);

  // Validate token once on mount only. Only clear on 401 — not on network errors
  // (server restart, brief 502) so a temporary outage doesn't force logout.
  useEffect(() => {
    if (!authToken) { setLoading(false); return; }
    fetch(`${API_URL}/auth/me`, { headers: { 'Authorization': `Bearer ${authToken}` } })
      .then(r => {
        if (r.status === 401 || r.status === 403) {
          setAuthToken(null);
          localStorage.removeItem('authToken');
        } else if (r.ok) {
          r.json().then(u => setCurrentUser(u));
        }
      })
      .catch(() => { /* network error — keep token, user stays logged in */ })
      .finally(() => setLoading(false));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const login = async (email, password) => {
    const res = await fetch(`${API_URL}/auth/login`, {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ email, password })
    });
    if (!res.ok) throw new Error((await res.json()).detail || 'Login failed');
    const data = await res.json();
    setAuthToken(data.access_token);
    setCurrentUser(data.user);
    localStorage.setItem('authToken', data.access_token);
    return data;
  };

  const signup = async (orgName, fullName, email, password) => {
    const res = await fetch(`${API_URL}/auth/signup`, {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ org_name: orgName, full_name: fullName, email, password })
    });
    if (!res.ok) throw new Error((await res.json()).detail || 'Signup failed');
    const data = await res.json();
    setAuthToken(data.access_token);
    setCurrentUser(data.user);
    localStorage.setItem('authToken', data.access_token);
    return data;
  };

  const logout = () => {
    setAuthToken(null);
    setCurrentUser(null);
    localStorage.removeItem('authToken');
  };

  return (
    <AuthContext.Provider value={{ authToken, currentUser, setCurrentUser, apiFetch, login, signup, logout, loading }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
