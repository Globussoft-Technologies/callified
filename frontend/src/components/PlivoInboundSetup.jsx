import React, { useState } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { API_URL } from '../constants/api';

export default function PlivoInboundSetup({ accountId }) {
  const { apiFetch } = useAuth();
  const [open, setOpen] = useState(false);
  const [setup, setSetup] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const toggle = async () => {
    setOpen(!open);
    if (open || setup || loading) return;
    setLoading(true);
    setError('');
    try {
      const response = await apiFetch(`${API_URL}/exotel-accounts/${accountId}/plivo-inbound-setup`);
      const body = await response.json();
      if (!response.ok) throw new Error(body.error || 'Could not load incoming call setup');
      setSetup(body);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ marginTop: 10, fontSize: 12 }}>
      <button type="button" onClick={toggle} style={{ cursor: 'pointer', color: '#6366f1', border: 0, background: 'transparent', padding: 0 }}>
        {open ? 'Hide incoming call setup' : 'Incoming call setup'}
      </button>
      {open && (
        <div style={{ marginTop: 8, maxWidth: 650 }}>
          {loading && <p>Loading…</p>}
          {error && <p role="alert" style={{ color: '#ef4444' }}>{error}</p>}
          {setup && <>
            <p>Create a Plivo XML application using these URLs with POST, then assign it to {setup.number}. Choose AI Receptionist or a saved forwarding number under AI Receptionist → Call Forwarding.</p>
            {[['Answer URL', setup.answer_url], ['Hangup URL', setup.hangup_url]].map(([label, value]) => (
              <label key={label} style={{ display: 'block', marginTop: 6 }}>
                {label} (POST)
                <input readOnly value={value} onFocus={event => event.target.select()} style={{ display: 'block', width: '100%', marginTop: 4, padding: 7 }} />
              </label>
            ))}
            <p>Keep this account as Outbound if you also use it for campaigns. Saving these settings in Callified does not change routing in Plivo.</p>
          </>}
        </div>
      )}
    </div>
  );
}
