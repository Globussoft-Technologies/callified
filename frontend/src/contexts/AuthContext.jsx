import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { API_URL } from '../constants/api';

const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [authToken, setAuthToken] = useState(localStorage.getItem('authToken') || null);
  const [currentUser, setCurrentUser] = useState(null);

  const apiFetch = useCallback(async (url, options = {}) => {
    return fetch(url, {
      ...options,
      headers: { ...options.headers, 'Authorization': `Bearer ${authToken}` }
    });
  }, [authToken]);

  // Check token on mount
  useEffect(() => {
    if (authToken) {
      fetch(`${API_URL}/auth/me`, { headers: { 'Authorization': `Bearer ${authToken}` } })
        .then(r => r.ok ? r.json() : Promise.reject())
        .then(u => setCurrentUser(u))
        .catch(() => { setAuthToken(null); localStorage.removeItem('authToken'); });
    }
  }, [authToken]);

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
    <AuthContext.Provider value={{ authToken, currentUser, setCurrentUser, apiFetch, login, signup, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
