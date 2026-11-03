import React, { useEffect, useState } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { API_URL } from '../constants/api';

export default function IncomingCallRouting({ orgId }) {
  const { apiFetch } = useAuth();
  const [numbers, setNumbers] = useState([]);
  const [accounts, setAccounts] = useState([]);
  const [routes, setRoutes] = useState({});
  const [draft, setDraft] = useState({ name: '', phone: '' });
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const request = async (path, options) => {
    const response = await apiFetch(`${API_URL}${path}`, options);
    const data = response.status === 204 ? null : await response.json();
    if (!response.ok) throw new Error(data?.error || 'Could not update call forwarding.');
    return data;
  };
  const load = async () => {
    const [savedNumbers, savedAccounts] = await Promise.all([request('/forwarding-numbers'), request('/exotel-accounts')]);
    const plivoAccounts = savedAccounts.filter(account => account.provider === 'plivo' && account.direction === 'inbound');
    const savedRoutes = await Promise.all(plivoAccounts.map(async account => [account.id, await request(`/exotel-accounts/${account.id}/incoming-route`)]));
    setNumbers(savedNumbers); setAccounts(plivoAccounts); setRoutes(Object.fromEntries(savedRoutes));
  };
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    load().catch(error => { if (!cancelled) setMessage(error.message); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orgId]);
  const mutate = async (action, success) => {
    setBusy(true); setMessage('');
    try { await action(); await load(); setMessage(success); }
    catch (error) { setMessage(error.message); }
    finally { setBusy(false); }
  };
  const saveNumber = () => mutate(async () => {
    await request(`/forwarding-numbers${draft.id ? `/${draft.id}` : ''}`, {
      method: draft.id ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: draft.name, phone: draft.phone }),
    });
    setDraft({ name: '', phone: '' });
  }, 'Forwarding number saved.');
  const input = { padding: 9, border: '1px solid #d1d5db', borderRadius: 6, font: 'inherit' };
  const button = { ...input, cursor: 'pointer', background: '#fff' };
  return (
    <section style={{ marginBottom: 16, padding: 18, background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8 }}>
      <h2 style={{ margin: 0, fontSize: 16 }}>Call Forwarding</h2>
      <p style={{ fontSize: 13, color: '#6b7280' }}>Choose who answers incoming Plivo calls: AI Receptionist or a saved phone number. Your Plivo webhook URLs stay the same.</p>
      {loading ? <p>Loading...</p> : <>
        <h3 style={{ fontSize: 14 }}>Saved forwarding numbers</h3>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <input aria-label="Contact name" placeholder="Name, e.g. Sales" value={draft.name} maxLength={100} disabled={busy} onChange={event => setDraft({ ...draft, name: event.target.value })} style={input} />
          <input aria-label="Forwarding phone number" type="tel" placeholder="Phone with country code" value={draft.phone} disabled={busy} onChange={event => setDraft({ ...draft, phone: event.target.value })} style={input} />
          <button style={button} disabled={busy || !draft.name.trim() || !draft.phone.trim()} onClick={saveNumber}>{draft.id ? 'Save Number' : 'Add Number'}</button>
          {draft.id && <button style={button} disabled={busy} onClick={() => setDraft({ name: '', phone: '' })}>Cancel</button>}
        </div>
        {numbers.map(number => <div key={number.id} style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap', padding: '10px 0' }}>
          <span>{number.name} · {number.phone}</span>
          <button style={button} disabled={busy} onClick={() => setDraft(number)}>Edit</button>
          <button style={button} disabled={busy} onClick={() => mutate(() => request(`/forwarding-numbers/${number.id}`, { method: 'DELETE' }), 'Number deleted.')}>Delete</button>
        </div>)}
        <h3 style={{ fontSize: 14 }}>Incoming destinations</h3>
        {accounts.length === 0 && <p>Add an incoming Plivo provider account to configure forwarding.</p>}
        {accounts.map(account => {
          const route = routes[account.id] || { mode: 'ai', destination_id: 0 };
          return <div key={account.id} style={{ padding: '12px 0', borderTop: '1px solid #e5e7eb', display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
            <span>{account.name} · {account.caller_id}</span>
            <select aria-label={`Incoming destination for ${account.name}`} disabled={busy} style={input} value={route.mode === 'ai' ? 'ai' : String(route.destination_id)} onChange={event => setRoutes(previous => ({ ...previous, [account.id]: event.target.value === 'ai' ? { mode: 'ai', destination_id: 0 } : { mode: 'phone', destination_id: Number(event.target.value) } }))}>
              <option value="ai">AI Receptionist</option>
              {numbers.map(number => <option key={number.id} value={number.id}>{number.name} · {number.phone}</option>)}
            </select>
            <button style={button} disabled={busy} onClick={() => mutate(() => request(`/exotel-accounts/${account.id}/incoming-route`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(route) }), 'Destination saved for new incoming calls.')}>Save Destination</button>
          </div>;
        })}
        <p style={{ fontSize: 12, color: '#6b7280' }}>Forwarded calls ring for up to 30 seconds and end if unanswered. Plivo call charges apply. Browser tests continue to use AI.</p>
      </>}
      {message && <p role="status" style={{ fontSize: 13 }}>{message}</p>}
    </section>
  );
}
