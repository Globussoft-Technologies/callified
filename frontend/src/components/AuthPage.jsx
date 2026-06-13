import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { API_URL } from '../constants/api';
import loginIcon from '../assets/tg_image_3608761279.png';

export default function AuthPage({ redirectTo = '/crm' }) {
  const { login, signup } = useAuth();
  const navigate = useNavigate();

  const [authPage, setAuthPage] = useState('login');
  const [authError, setAuthError] = useState('');
  const [authLoading, setAuthLoading] = useState(false);
  const [authForm, setAuthForm] = useState({ org_name: '', full_name: '', email: '', password: '' });
  const [forgotEmail, setForgotEmail] = useState('');
  const [forgotSuccess, setForgotSuccess] = useState('');
  const [subscriptionError, setSubscriptionError] = useState(null);
  const [showPassword, setShowPassword] = useState(false);

  const handleLogin = async (e) => {
    e.preventDefault();
    setAuthError('');
    setSubscriptionError(null);
    setAuthLoading(true);
    try {
      await login(authForm.email, authForm.password);
    } catch (err) {
      if (err.status === 403 && ['SUBSCRIPTION_EXPIRED', 'SUBSCRIPTION_NOT_FOUND', 'SUBSCRIPTION_INACTIVE'].includes(err.code)) {
        setSubscriptionError({
          code: err.code,
          message: err.message,
          expiresAt: err.expiresAt,
          plan: err.plan,
          supportEmail: err.supportEmail || 'support@callified.ai',
        });
      } else {
        setAuthError(err.message);
      }
    }
    setAuthLoading(false);
  };

  const handleSignup = async (e) => {
    e.preventDefault();
    setAuthError(''); setSubscriptionError(null); setAuthLoading(true);
    try {
      await signup(authForm.org_name, authForm.full_name, authForm.email, authForm.password);
    } catch (err) {
      if (err.status === 403 && ['SUBSCRIPTION_EXPIRED', 'SUBSCRIPTION_NOT_FOUND', 'SUBSCRIPTION_INACTIVE'].includes(err.code)) {
        setSubscriptionError({
          code: err.code,
          message: err.message,
          expiresAt: err.expiresAt,
          plan: err.plan,
          supportEmail: err.supportEmail || 'support@callified.ai',
        });
      } else {
        setAuthError(err.message);
      }
    }
    setAuthLoading(false);
  };

  const handleForgotPassword = async (e) => {
    e.preventDefault();
    setAuthError(''); setForgotSuccess(''); setAuthLoading(true);
    try {
      const res = await fetch(`${API_URL}/auth/forgot-password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: forgotEmail }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.detail || 'Request failed');
      setForgotSuccess(data.message);
    } catch (err) { setAuthError(err.message); }
    setAuthLoading(false);
  };

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'linear-gradient(135deg, #0f0c29, #302b63, #24243e)', padding: '20px'
    }}>
      <div style={{ width: '100%', maxWidth: '440px' }}>
        <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <h1 style={{ fontSize: '2rem', fontWeight: 800, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, margin: 0 }}>
            <img src={loginIcon} alt="" style={{ height: '1.2em', width: '1.2em', objectFit: 'contain' }} />
            <span style={{ background: 'linear-gradient(135deg, #a78bfa, #22d3ee)', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent' }}>
              Callified AI
            </span>
          </h1>
          <p style={{ color: '#94a3b8', fontSize: '0.95rem', marginTop: '0.5rem' }}>AI-Powered Lead Qualification Platform</p>
          <span style={{ display: 'none' }} data-version="2.0.1" />
        </div>

        <div className="glass-panel" style={{ padding: '2rem' }}>
          {authPage !== 'forgot' && (
            <div style={{ display: 'flex', marginBottom: '1.5rem', borderRadius: '8px', overflow: 'hidden', border: '1px solid rgba(255,255,255,0.1)' }}>
              <button data-testid="auth-login-tab" onClick={() => { setAuthPage('login'); setAuthError(''); setForgotSuccess(''); }}
                style={{
                  flex: 1, padding: '10px', border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: '0.9rem',
                  background: authPage === 'login' ? 'rgba(167,139,250,0.2)' : 'transparent',
                  color: authPage === 'login' ? '#a78bfa' : '#64748b'
                }}>
                Login
              </button>
              <button data-testid="auth-signup-tab" onClick={() => { setAuthPage('signup'); setAuthError(''); setForgotSuccess(''); }}
                style={{
                  flex: 1, padding: '10px', border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: '0.9rem',
                  background: authPage === 'signup' ? 'rgba(34,211,238,0.2)' : 'transparent',
                  color: authPage === 'signup' ? '#22d3ee' : '#64748b'
                }}>
                Sign Up
              </button>
            </div>
          )}

          {authPage === 'forgot' && (
            <div style={{ marginBottom: '1.5rem' }}>
              <button onClick={() => { setAuthPage('login'); setAuthError(''); setForgotSuccess(''); }}
                style={{ background: 'none', border: 'none', color: '#a78bfa', cursor: 'pointer', fontSize: '0.9rem', padding: 0 }}>
                &larr; Back to Login
              </button>
              <h2 style={{ fontSize: '1.3rem', fontWeight: 700, marginTop: '0.75rem', marginBottom: 0, background: 'linear-gradient(135deg, #a78bfa, #7c3aed)', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent' }}>Forgot Password</h2>
            </div>
          )}

          {authError && (
            <div style={{ background: 'rgba(239,68,68,0.15)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#fca5a5', fontSize: '0.85rem' }}>
              {authError}
            </div>
          )}

          {forgotSuccess && (
            <div style={{ background: 'rgba(34,197,94,0.15)', border: '1px solid rgba(34,197,94,0.3)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#86efac', fontSize: '0.85rem' }}>
              {forgotSuccess}
            </div>
          )}

          {subscriptionError && (
            <div style={{ background: 'rgba(245,158,11,0.15)', border: '1px solid rgba(245,158,11,0.4)', borderRadius: '12px', padding: '1.25rem', marginBottom: '1rem', color: '#fcd34d' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.75rem' }}>
                <span style={{ fontSize: '1.5rem' }}>⚠️</span>
                <h3 style={{ margin: 0, fontSize: '1.1rem', fontWeight: 700, color: '#fbbf24' }}>
                  {subscriptionError.code === 'SUBSCRIPTION_EXPIRED' ? 'Subscription Expired' : 'Subscription Required'}
                </h3>
              </div>
              <p style={{ margin: '0 0 1rem 0', fontSize: '0.9rem', lineHeight: 1.5 }}>
                {subscriptionError.message}
              </p>
              {subscriptionError.expiresAt && (
                <p style={{ margin: '0 0 1rem 0', fontSize: '0.8rem', color: '#fde68a' }}>
                  Expired on: {new Date(subscriptionError.expiresAt).toLocaleDateString()}
                </p>
              )}
              <button
                onClick={() => window.location.href = `mailto:${subscriptionError.supportEmail}?subject=Subscription Renewal Request`}
                style={{
                  background: 'linear-gradient(135deg, #f59e0b, #d97706)',
                  border: 'none',
                  borderRadius: '8px',
                  color: '#fff',
                  cursor: 'pointer',
                  fontSize: '0.9rem',
                  fontWeight: 600,
                  padding: '10px 18px',
                }}
              >
                📧 Contact Support to Renew
              </button>
            </div>
          )}

          {authPage === 'forgot' ? (
            <form onSubmit={handleForgotPassword}>
              <div className="form-group">
                <label>Email Address</label>
                <input className="form-input" type="email" placeholder="you@company.com" required
                  value={forgotEmail} onChange={e => setForgotEmail(e.target.value)} />
              </div>
              <button type="submit" className="btn-primary" disabled={authLoading}
                style={{
                  width: '100%', padding: '12px', marginTop: '0.5rem', fontSize: '1rem', fontWeight: 700,
                  background: 'linear-gradient(135deg, #a78bfa, #7c3aed)'
                }}>
                {authLoading ? 'Sending...' : 'Send Reset Link'}
              </button>
            </form>
          ) : (
            <>
              <form onSubmit={authPage === 'login' ? handleLogin : handleSignup}>
                {authPage === 'signup' && (
                  <>
                    <div className="form-group">
                      <label>Organization Name</label>
                      <input data-testid="auth-org-name" className="form-input" placeholder="e.g. Globussoft" required
                        value={authForm.org_name} onChange={e => setAuthForm({ ...authForm, org_name: e.target.value })} />
                    </div>
                    <div className="form-group">
                      <label>Your Full Name</label>
                      <input data-testid="auth-full-name" className="form-input" placeholder="e.g. Sumit Kumar" required
                        value={authForm.full_name} onChange={e => setAuthForm({ ...authForm, full_name: e.target.value })} />
                    </div>
                  </>
                )}
                <div className="form-group">
                  <label>Email</label>
                  <input data-testid="auth-email" className="form-input" type="email" placeholder="you@company.com" required
                    value={authForm.email} onChange={e => setAuthForm({ ...authForm, email: e.target.value })} />
                </div>
                <div className="form-group">
                  <label>Password</label>
                  <div style={{ position: 'relative' }}>
                    <input data-testid="auth-password" className="form-input" type={showPassword ? 'text' : 'password'} placeholder="••••••••" required minLength={8} maxLength={128}
                      value={authForm.password} onChange={e => setAuthForm({ ...authForm, password: e.target.value })}
                      style={{ paddingRight: '2.75rem' }} />
                    <button
                      type="button"
                      data-testid="auth-password-toggle"
                      onClick={() => setShowPassword(v => !v)}
                      aria-label={showPassword ? 'Hide password' : 'Show password'}
                      style={{
                        position: 'absolute',
                        right: '0.6rem',
                        top: '50%',
                        transform: 'translateY(-50%)',
                        background: 'transparent',
                        border: 'none',
                        color: '#94a3b8',
                        cursor: 'pointer',
                        fontSize: '1rem',
                        padding: '0.25rem',
                        lineHeight: 1,
                      }}
                    >
                      {showPassword ? (
                        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                          <path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/>
                          <path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/>
                          <path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61"/>
                          <line x1="2" x2="22" y1="2" y2="22"/>
                        </svg>
                      ) : (
                        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                          <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/>
                          <circle cx="12" cy="12" r="3"/>
                        </svg>
                      )}
                    </button>
                  </div>
                </div>
                <button data-testid="auth-submit" type="submit" className="btn-primary" disabled={authLoading}
                  style={{
                    width: '100%', padding: '12px', marginTop: '0.5rem', fontSize: '1rem', fontWeight: 700,
                    background: authPage === 'login' ? 'linear-gradient(135deg, #a78bfa, #7c3aed)' : 'linear-gradient(135deg, #22d3ee, #06b6d4)'
                  }}>
                  {authLoading ? 'Please wait...' : (authPage === 'login' ? 'Login' : 'Create Account')}
                </button>
              </form>
              {authPage === 'login' && (
                <div style={{ textAlign: 'center', marginTop: '1rem' }}>
                  <button onClick={() => { setAuthPage('forgot'); setAuthError(''); setForgotSuccess(''); setForgotEmail(''); }}
                    style={{ background: 'none', border: 'none', color: '#94a3b8', cursor: 'pointer', fontSize: '0.85rem', textDecoration: 'underline' }}>
                    Forgot password?
                  </button>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
