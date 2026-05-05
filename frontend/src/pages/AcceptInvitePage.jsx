import React, { useState, useEffect } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { API_URL } from '../constants/api';
import { validatePasswordFull } from '../utils/passwordPolicy';

export default function AcceptInvitePage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const token = searchParams.get('token');

  const [inviteInfo, setInviteInfo] = useState(null);  // { email, full_name, role, org_name }
  const [infoError, setInfoError] = useState('');
  const [infoLoading, setInfoLoading] = useState(true);

  const [fullName, setFullName] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!token) { setInfoLoading(false); setInfoError('No token in URL.'); return; }
    fetch(`${API_URL}/team/invite-info?token=${encodeURIComponent(token)}`)
      .then(r => r.json().then(d => ({ ok: r.ok, data: d })))
      .then(({ ok, data }) => {
        if (!ok) { setInfoError(data.detail || 'Invalid or expired invite link.'); }
        else { setInviteInfo(data); setFullName(data.full_name || ''); }
      })
      .catch(() => setInfoError('Network error. Please try again.'))
      .finally(() => setInfoLoading(false));
  }, [token]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    if (!fullName.trim()) { setError('Full name is required.'); return; }
    const pwCheck = await validatePasswordFull(password);
    if (!pwCheck.valid) { setError(pwCheck.error); return; }
    if (password !== confirmPassword) { setError('Passwords do not match.'); return; }
    setLoading(true);
    try {
      const res = await fetch(`${API_URL}/team/accept-invite`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, password, full_name: fullName.trim() }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.detail || 'Failed to activate account');
      setSuccess(data.message);
      setTimeout(() => navigate('/'), 2500);
    } catch (err) {
      setError(err.message);
    }
    setLoading(false);
  };

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', flexDirection: 'column', alignItems: 'center',
      justifyContent: 'center', background: 'linear-gradient(135deg, #0f0c29, #302b63, #24243e)',
      padding: '20px',
    }}>
      {/* Logo */}
      <div style={{ textAlign: 'center', marginBottom: '1.5rem' }}>
        <h1 style={{
          fontSize: '2rem', fontWeight: 800, margin: 0,
          background: 'linear-gradient(135deg, #a78bfa, #6366f1)',
          WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent',
        }}>Callified AI</h1>
        <p style={{ color: '#94a3b8', margin: '6px 0 0', fontSize: '0.9rem' }}>Accept your team invite</p>
      </div>

      <div style={{
        width: '100%', maxWidth: '460px',
        background: 'rgba(15,23,42,0.85)', border: '1px solid rgba(148,163,184,0.12)',
        borderRadius: '16px', padding: '2rem', backdropFilter: 'blur(16px)',
      }}>
        {infoLoading ? (
          <div style={{ textAlign: 'center', color: '#94a3b8', padding: '2rem 0' }}>
            <div style={{ width: 32, height: 32, border: '3px solid rgba(255,255,255,0.1)', borderTop: '3px solid #a78bfa', borderRadius: '50%', animation: 'spin 0.8s linear infinite', margin: '0 auto 1rem' }} />
            <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
            Verifying invite link...
          </div>
        ) : infoError ? (
          <div style={{ textAlign: 'center', padding: '1rem 0' }}>
            <div style={{ fontSize: '2rem', marginBottom: '0.75rem' }}>🔗</div>
            <p style={{ color: '#fca5a5', fontWeight: 600, marginBottom: '0.5rem' }}>{infoError}</p>
            <p style={{ color: '#64748b', fontSize: '0.85rem' }}>Please ask your admin for a new invite link.</p>
          </div>
        ) : success ? (
          <div style={{ textAlign: 'center', padding: '1rem 0' }}>
            <div style={{ fontSize: '2.5rem', marginBottom: '0.75rem' }}>✅</div>
            <p style={{ color: '#86efac', fontWeight: 600, marginBottom: '0.5rem' }}>{success}</p>
            <p style={{ color: '#94a3b8', fontSize: '0.85rem' }}>Redirecting to login...</p>
          </div>
        ) : (
          <>
            {/* Invite context banner */}
            {inviteInfo && (
              <div style={{
                background: 'rgba(99,102,241,0.1)', border: '1px solid rgba(99,102,241,0.25)',
                borderRadius: '10px', padding: '14px 16px', marginBottom: '1.5rem',
                fontSize: '0.875rem', color: '#c7d2fe', lineHeight: 1.6,
              }}>
                You've been invited to <strong style={{ color: '#f1f5f9' }}>{inviteInfo.org_name || 'your organisation'}</strong> as a <strong style={{ color: '#f1f5f9' }}>{inviteInfo.role}</strong>.<br />
                Setting up the account for <strong style={{ color: '#f1f5f9' }}>{inviteInfo.email}</strong>.
              </div>
            )}

            <form onSubmit={handleSubmit}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
                {/* Full Name */}
                <div>
                  <label style={labelStyle}>Full Name</label>
                  <input
                    placeholder="Your full name" required
                    value={fullName} onChange={e => setFullName(e.target.value)}
                    style={inputStyle}
                  />
                </div>

                {/* New Password */}
                <div>
                  <label style={labelStyle}>New Password</label>
                  <input
                    type="password" placeholder="At least 8 characters" required
                    value={password} onChange={e => setPassword(e.target.value)}
                    style={inputStyle}
                  />
                </div>

                {/* Confirm Password */}
                <div>
                  <label style={labelStyle}>Confirm Password</label>
                  <input
                    type="password" placeholder="Type it again" required
                    value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)}
                    style={inputStyle}
                  />
                </div>

                {error && (
                  <div style={{ color: '#fca5a5', fontSize: '0.85rem', padding: '10px 14px', background: 'rgba(239,68,68,0.08)', borderRadius: '8px', border: '1px solid rgba(239,68,68,0.2)' }}>
                    {error}
                  </div>
                )}

                <button type="submit" disabled={loading} style={{
                  background: loading ? 'rgba(99,102,241,0.5)' : 'linear-gradient(135deg, #6366f1, #8b5cf6)',
                  border: 'none', borderRadius: '10px', color: '#fff',
                  padding: '13px', cursor: loading ? 'not-allowed' : 'pointer',
                  fontWeight: 700, fontSize: '1rem', marginTop: '4px', letterSpacing: '0.01em',
                }}>
                  {loading ? 'Activating...' : 'Activate Account'}
                </button>
              </div>
            </form>
          </>
        )}
      </div>
    </div>
  );
}

const labelStyle = {
  display: 'block', marginBottom: '6px', fontSize: '0.85rem',
  fontWeight: 600, color: '#cbd5e1',
};

const inputStyle = {
  background: 'rgba(30,41,59,0.8)', border: '1px solid rgba(148,163,184,0.2)',
  borderRadius: '8px', color: '#e2e8f0', padding: '12px 14px', fontSize: '0.9rem',
  outline: 'none', width: '100%', boxSizing: 'border-box',
};
