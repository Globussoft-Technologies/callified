import React, { useState, useEffect } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { API_URL } from '../constants/api';

export default function FeatureFlagsPage({ apiFetch }) {
  const { currentUser } = useAuth();
  const [email, setEmail] = useState('');
  const [hideAiFeatures, setHideAiFeatures] = useState(false);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState(null);
  const [error, setError] = useState(null);
  const [lookupEmail, setLookupEmail] = useState('');
  const [flag, setFlag] = useState(null);
  const [lookupLoading, setLookupLoading] = useState(false);

  const showMessage = (text, type = 'success') => {
    setMessage({ text, type });
    setTimeout(() => setMessage(null), 5000);
  };

  const handleSave = async (e) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/admin/feature-flags`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, hide_ai_features: hideAiFeatures }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Failed to save feature flag');
      showMessage(`Feature flag saved for ${data.email}`);
      setEmail('');
      setHideAiFeatures(false);
    } catch (err) {
      setError(err.message);
    }
    setLoading(false);
  };

  const handleLookup = async (e) => {
    e.preventDefault();
    setError(null);
    setFlag(null);
    setLookupLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/admin/feature-flags/${encodeURIComponent(lookupEmail)}`);
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Flag not found');
      setFlag(data);
      setEmail(data.email);
      setHideAiFeatures(data.hide_ai_features);
    } catch (err) {
      setError(err.message);
    }
    setLookupLoading(false);
  };

  const handleDelete = async () => {
    if (!flag?.email) return;
    setError(null);
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/admin/feature-flags/${encodeURIComponent(flag.email)}`, {
        method: 'DELETE',
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to delete feature flag');
      }
      showMessage(`Feature flag removed for ${flag.email}`);
      setFlag(null);
      setEmail('');
      setHideAiFeatures(false);
      setLookupEmail('');
    } catch (err) {
      setError(err.message);
    }
    setLoading(false);
  };

  const inputStyle = {
    width: '100%',
    padding: '10px 12px',
    borderRadius: 8,
    border: '1px solid #d1d5db',
    fontSize: 14,
    outline: 'none',
    boxSizing: 'border-box',
  };

  const labelStyle = {
    display: 'block',
    marginBottom: 6,
    fontSize: 13,
    fontWeight: 600,
    color: '#374151',
  };

  const cardStyle = {
    background: '#fff',
    borderRadius: 12,
    padding: '1.5rem',
    boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
    border: '1px solid #e5e7eb',
    marginBottom: '1.5rem',
  };

  if (!currentUser?.is_super_admin) {
    return (
      <div style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>
        <h2>Access Denied</h2>
        <p>You do not have permission to manage feature flags.</p>
      </div>
    );
  }

  return (
    <div style={{ padding: '1.5rem 2rem', maxWidth: 900, margin: '0 auto' }}>
      <h1 style={{ fontSize: '1.6rem', fontWeight: 700, marginBottom: '1.5rem', color: '#111827' }}>
        Feature Flags
      </h1>

      {message && (
        <div style={{
          padding: '12px 16px', borderRadius: 8, marginBottom: '1rem',
          background: message.type === 'success' ? '#dcfce7' : '#fee2e2',
          color: message.type === 'success' ? '#166534' : '#991b1b',
          border: `1px solid ${message.type === 'success' ? '#86efac' : '#fca5a5'}`,
        }}>
          {message.text}
        </div>
      )}

      {error && (
        <div style={{
          padding: '12px 16px', borderRadius: 8, marginBottom: '1rem',
          background: '#fee2e2', color: '#991b1b', border: '1px solid #fca5a5',
        }}>
          {error}
        </div>
      )}

      <div style={cardStyle}>
        <h2 style={{ fontSize: '1.1rem', fontWeight: 600, margin: '0 0 1rem 0', color: '#111827' }}>
          Lookup Feature Flag
        </h2>
        <form onSubmit={handleLookup} style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'flex-end' }}>
          <div style={{ flex: 1, minWidth: 220 }}>
            <label style={labelStyle}>Email</label>
            <input
              type="email"
              style={inputStyle}
              placeholder="user@example.com"
              value={lookupEmail}
              onChange={e => setLookupEmail(e.target.value)}
              required
            />
          </div>
          <button
            type="submit"
            disabled={lookupLoading}
            className="btn-primary"
            style={{ padding: '10px 20px', borderRadius: 8, fontWeight: 600 }}
          >
            {lookupLoading ? 'Looking up...' : 'Lookup'}
          </button>
        </form>

        {flag && (
          <div style={{
            marginTop: '1rem', padding: '12px 16px', borderRadius: 8,
            background: '#f3f4f6', border: '1px solid #e5e7eb',
          }}>
            <p style={{ margin: '0 0 0.5rem 0', fontSize: 14 }}>
              <strong>{flag.email}</strong>
            </p>
            <p style={{ margin: 0, fontSize: 14, color: flag.hide_ai_features ? '#991b1b' : '#166534' }}>
              {flag.hide_ai_features ? 'AI features hidden' : 'AI features visible'}
            </p>
          </div>
        )}
      </div>

      <div style={cardStyle}>
        <h2 style={{ fontSize: '1.1rem', fontWeight: 600, margin: '0 0 1rem 0', color: '#111827' }}>
          {flag ? 'Update Feature Flag' : 'Set Feature Flag'}
        </h2>
        <form onSubmit={handleSave}>
          <div style={{ marginBottom: '1rem' }}>
            <label style={labelStyle}>Email</label>
            <input
              type="email"
              style={inputStyle}
              placeholder="user@example.com"
              value={email}
              onChange={e => setEmail(e.target.value)}
              required
            />
          </div>

          <div style={{
            display: 'flex', alignItems: 'center', gap: 10,
            padding: '12px 14px', borderRadius: 8, border: '1px solid #d1d5db',
            marginBottom: '1rem', cursor: 'pointer',
          }} onClick={() => setHideAiFeatures(v => !v)}>
            <input
              type="checkbox"
              checked={hideAiFeatures}
              onChange={e => setHideAiFeatures(e.target.checked)}
              style={{ width: 18, height: 18, cursor: 'pointer' }}
            />
            <span style={{ fontSize: 14, fontWeight: 500, color: '#374151' }}>
              Hide AI-related UI sections for this email
            </span>
          </div>

          <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
            <button
              type="submit"
              disabled={loading}
              className="btn-primary"
              style={{ padding: '10px 20px', borderRadius: 8, fontWeight: 600 }}
            >
              {loading ? 'Saving...' : (flag ? 'Update' : 'Save')}
            </button>
            {flag && (
              <button
                type="button"
                onClick={handleDelete}
                disabled={loading}
                style={{
                  padding: '10px 20px', borderRadius: 8, fontWeight: 600,
                  background: '#fee2e2', color: '#991b1b', border: '1px solid #fca5a5',
                  cursor: 'pointer',
                }}
              >
                Remove Flag
              </button>
            )}
          </div>
        </form>
      </div>
    </div>
  );
}
