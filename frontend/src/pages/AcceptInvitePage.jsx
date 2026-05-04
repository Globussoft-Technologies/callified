import React, { useState, useEffect } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { API_URL } from '../constants/api';

export default function AcceptInvitePage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const token = searchParams.get('token');

  const [invite, setInvite] = useState(null);
  const [fetchError, setFetchError] = useState('');
  const [loadingInvite, setLoadingInvite] = useState(true);

  const [fullName, setFullName] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!token) { setLoadingInvite(false); return; }
    (async () => {
      try {
        const res = await fetch(`${API_URL}/invite/${encodeURIComponent(token)}`);
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
          setFetchError(data.error || data.detail || 'This invite link is invalid or has expired.');
        } else {
          setInvite(data);
          setFullName(data.full_name || '');
        }
      } catch (e) {
        setFetchError('Network error. Please check your connection and try again.');
      }
      setLoadingInvite(false);
    })();
  }, [token]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(''); setSuccess('');
    if (password.length < 8) { setError('Password must be at least 8 characters.'); return; }
    if (password !== confirmPassword) { setError('Passwords do not match.'); return; }

    setSubmitting(true);
    try {
      const res = await fetch(`${API_URL}/invite/${encodeURIComponent(token)}/accept`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password, full_name: fullName }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || data.detail || 'Failed to accept invite');
      setSuccess(data.message || 'Your account is ready. Please sign in.');
    } catch (err) {
      setError(err.message);
    }
    setSubmitting(false);
  };

  return (
    <div style={{minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'linear-gradient(135deg, #0f0c29, #302b63, #24243e)', padding: '20px'}}>
      <div style={{width: '100%', maxWidth: '460px'}}>
        <div style={{textAlign: 'center', marginBottom: '2rem'}}>
          <h1 style={{fontSize: '2rem', fontWeight: 800, background: 'linear-gradient(135deg, #a78bfa, #22d3ee)', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent'}}>
            Callified AI
          </h1>
          <p style={{color: '#94a3b8', fontSize: '0.95rem'}}>Accept your team invite</p>
        </div>

        <div className="glass-panel" style={{padding: '2rem'}}>
          {!token ? (
            <div style={{textAlign: 'center'}}>
              <p style={{color: '#fca5a5', marginBottom: '1rem'}}>Invalid invite link. No token provided.</p>
              <button onClick={() => navigate('/')}
                style={{background: 'none', border: 'none', color: '#a78bfa', cursor: 'pointer', textDecoration: 'underline'}}>
                Back to Login
              </button>
            </div>
          ) : loadingInvite ? (
            <div style={{textAlign: 'center', color: '#94a3b8'}}>Validating invite...</div>
          ) : fetchError ? (
            <div style={{textAlign: 'center'}}>
              <p style={{color: '#fca5a5', marginBottom: '1rem'}}>{fetchError}</p>
              <button onClick={() => navigate('/')}
                style={{background: 'none', border: 'none', color: '#a78bfa', cursor: 'pointer', textDecoration: 'underline'}}>
                Back to Login
              </button>
            </div>
          ) : success ? (
            <div style={{textAlign: 'center'}}>
              <div style={{background: 'rgba(34,197,94,0.15)', border: '1px solid rgba(34,197,94,0.3)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#86efac', fontSize: '0.9rem'}}>
                {success}
              </div>
              <button onClick={() => navigate('/')}
                className="btn-primary"
                style={{padding: '12px 28px', fontSize: '1rem', fontWeight: 700, background: 'linear-gradient(135deg, #a78bfa, #7c3aed)'}}>
                Go to Login
              </button>
            </div>
          ) : (
            <>
              <div style={{marginBottom: '1.5rem', padding: '12px 14px', background: 'rgba(99,102,241,0.1)', border: '1px solid rgba(99,102,241,0.3)', borderRadius: '8px', color: '#c7d2fe', fontSize: '0.9rem'}}>
                You've been invited to <b>{invite?.org_name || 'Callified'}</b> as a <b>{invite?.role}</b>.<br />
                Setting up the account for <b>{invite?.email}</b>.
              </div>
              {error && (
                <div style={{background: 'rgba(239,68,68,0.15)', border: '1px solid rgba(239,68,68,0.3)', borderRadius: '8px', padding: '10px 14px', marginBottom: '1rem', color: '#fca5a5', fontSize: '0.85rem'}}>
                  {error}
                </div>
              )}
              <form onSubmit={handleSubmit}>
                <div className="form-group">
                  <label>Full Name</label>
                  <input className="form-input" type="text" placeholder="Your name" required
                    value={fullName} onChange={e => setFullName(e.target.value)} />
                </div>
                <div className="form-group">
                  <label>New Password</label>
                  <input className="form-input" type="password" placeholder="At least 8 characters" required minLength={8} maxLength={128}
                    value={password} onChange={e => setPassword(e.target.value)} />
                </div>
                <div className="form-group">
                  <label>Confirm Password</label>
                  <input className="form-input" type="password" placeholder="Type it again" required minLength={8} maxLength={128}
                    value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} />
                </div>
                <button type="submit" className="btn-primary" disabled={submitting}
                  style={{width: '100%', padding: '12px', marginTop: '0.5rem', fontSize: '1rem', fontWeight: 700,
                    background: 'linear-gradient(135deg, #a78bfa, #7c3aed)'}}>
                  {submitting ? 'Activating...' : 'Activate Account'}
                </button>
              </form>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
