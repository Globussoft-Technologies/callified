import React, { useState, useEffect } from 'react';

// Predefined top-up amounts (rupees). Shown as quick-pick buttons in the
// modal; the user can also type a custom value. Kept aligned with the ₹5/min
// rate so each amount maps to a clean minute count: ₹100 → 20 min, ₹500 →
// 100 min, etc.
const TOPUP_PRESETS = [100, 500, 1000, 5000];

export default function BillingPage({ apiFetch, API_URL }) {
  const [plans, setPlans] = useState([]);
  const [subscription, setSubscription] = useState(null);
  const [usage, setUsage] = useState(null);
  const [payments, setPayments] = useState([]);
  const [invoices, setInvoices] = useState([]);
  const [credits, setCredits] = useState(null);          // { balance_paise, rate_per_min_paise, minutes_available }
  const [creditTxns, setCreditTxns] = useState([]);
  const [topupOpen, setTopupOpen] = useState(false);
  const [topupAmount, setTopupAmount] = useState(500);
  const [topupBusy, setTopupBusy] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => { fetchAll(); }, []);

  const fetchAll = async () => {
    setLoading(true);
    try {
      const [plansRes, subRes, usageRes, payRes, invRes, creditsRes, creditTxRes] = await Promise.all([
        apiFetch(`${API_URL}/billing/plans`),
        apiFetch(`${API_URL}/billing/subscription`),
        apiFetch(`${API_URL}/billing/usage`),
        apiFetch(`${API_URL}/billing/payments`),
        apiFetch(`${API_URL}/billing/invoices`),
        apiFetch(`${API_URL}/billing/credits`),
        apiFetch(`${API_URL}/billing/credits/transactions`),
      ]);
      const plansData = await plansRes.json();
      if (Array.isArray(plansData)) setPlans(plansData);
      const subData = await subRes.json();
      if (subData && !subData.error) setSubscription(subData);
      const usageData = await usageRes.json();
      if (usageData && !usageData.error) setUsage(usageData);
      const payData = await payRes.json();
      if (Array.isArray(payData)) setPayments(payData);
      try {
        const invData = await invRes.json();
        if (Array.isArray(invData)) setInvoices(invData);
      } catch(e) { setInvoices([]); }
      try {
        const c = await creditsRes.json();
        if (c && !c.error) setCredits(c);
      } catch(e) { setCredits(null); }
      try {
        const t = await creditTxRes.json();
        if (Array.isArray(t)) setCreditTxns(t);
      } catch(e) { setCreditTxns([]); }
    } catch(e) { console.error('Billing fetch error:', e); }
    setLoading(false);
  };

  const ratePerMin = credits?.rate_per_min_paise || 500; // default ₹5/min
  const minutesFor = (rupees) => Math.floor((rupees * 100) / ratePerMin);

  const handleTopup = async () => {
    if (!topupAmount || topupAmount <= 0) return;
    setTopupBusy(true);
    try {
      const res = await apiFetch(`${API_URL}/billing/credits/topup`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ amount_inr: Number(topupAmount) }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        alert('Top-up failed: ' + (body.error || `HTTP ${res.status}`));
        setTopupBusy(false);
        return;
      }
      const order = await res.json();
      if (order.order_id && order.key_id) {
        openCreditsRazorpay(order);
      } else {
        // Razorpay not configured (dev) — surface a clear message instead of
        // silently succeeding. Production has key_id set in .env.
        alert('Razorpay is not configured on the server. Top-ups are disabled until RAZORPAY_KEY_ID is set.');
      }
    } catch(e) {
      alert('Top-up failed: ' + e.message);
    } finally {
      setTopupBusy(false);
    }
  };

  const openCreditsRazorpay = (order) => {
    const options = {
      key: order.key_id,
      amount: order.amount,
      currency: order.currency || 'INR',
      name: 'Callified AI',
      description: order.description || `${order.amount_inr} call credits`,
      order_id: order.order_id,
      handler: async (response) => {
        const verifyRes = await apiFetch(`${API_URL}/billing/credits/verify`, {
          method: 'POST', headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            razorpay_order_id: response.razorpay_order_id,
            razorpay_payment_id: response.razorpay_payment_id,
            razorpay_signature: response.razorpay_signature,
          }),
        });
        if (verifyRes.ok) {
          setTopupOpen(false);
          fetchAll();
        } else {
          const body = await verifyRes.json().catch(() => ({}));
          alert('Payment verification failed: ' + (body.error || 'unknown'));
        }
      },
      modal: { ondismiss: () => setTopupBusy(false) },
      theme: { color: '#6366f1' },
    };
    const rzp = new window.Razorpay(options);
    rzp.open();
  };

  const formatINR = (paise) => {
    const rupees = paise / 100;
    return new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 0 }).format(rupees);
  };

  const handleSubscribe = async (planId) => {
    try {
      const res = await apiFetch(`${API_URL}/billing/create-order`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ plan_id: planId }),
      });
      const order = await res.json();
      if (order.order_id) {
        openRazorpay(order, planId);
      } else {
        // No Razorpay keys configured — create subscription directly for testing
        const subRes = await apiFetch(`${API_URL}/billing/subscribe`, {
          method: 'POST', headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ plan_id: planId }),
        });
        if (subRes.ok) { fetchAll(); }
      }
    } catch(e) {
      // Razorpay not configured — fall back to direct subscription
      try {
        const subRes = await apiFetch(`${API_URL}/billing/subscribe`, {
          method: 'POST', headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ plan_id: planId }),
        });
        if (subRes.ok) { fetchAll(); }
      } catch(e2) { alert('Failed to subscribe: ' + e2.message); }
    }
  };

  const openRazorpay = (order, planId) => {
    const options = {
      key: order.key_id,
      amount: order.amount,
      currency: order.currency,
      name: 'Callified AI',
      description: order.plan_name,
      order_id: order.order_id,
      handler: async (response) => {
        const verifyRes = await apiFetch(`${API_URL}/billing/verify-payment`, {
          method: 'POST', headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            razorpay_order_id: response.razorpay_order_id,
            razorpay_payment_id: response.razorpay_payment_id,
            razorpay_signature: response.razorpay_signature,
            plan_id: planId,
          }),
        });
        if (verifyRes.ok) { fetchAll(); }
      },
      theme: { color: '#6366f1' },
    };
    const rzp = new window.Razorpay(options);
    rzp.open();
  };

  const handleCancel = async () => {
    if (!confirm('Are you sure you want to cancel your subscription?')) return;
    try {
      await apiFetch(`${API_URL}/billing/cancel`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ reason: 'User cancelled' }),
      });
      fetchAll();
    } catch(e) { alert('Failed to cancel'); }
  };

  if (loading) return <div className="page-container"><div className="glass-panel" style={{padding: '2rem', textAlign: 'center'}}>Loading billing...</div></div>;

  const hasActiveSub = subscription && subscription.status && subscription.status !== 'none';
  const usagePercent = usage?.has_subscription ? Math.min(100, (usage.minutes_used / usage.minutes_included) * 100) : 0;

  return (
    <div className="page-container">
      <h2 style={{marginBottom: '1.5rem'}}>Billing</h2>

      {/* Call Credits — prepaid pay-per-minute (default ₹5/min) */}
      <div className="glass-panel" style={{marginBottom: '1.5rem', padding: '1.5rem',
        background: 'linear-gradient(135deg, rgba(99,102,241,0.08), rgba(34,211,238,0.05))',
        border: '1px solid rgba(99,102,241,0.2)'}}>
        <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '1rem'}}>
          <div>
            <div style={{fontSize: '0.75rem', color: '#a5b4fc', textTransform: 'uppercase', letterSpacing: '1px', fontWeight: 600}}>📞 Call Credits</div>
            <div style={{display: 'flex', alignItems: 'baseline', gap: '14px', marginTop: '6px'}}>
              <div style={{fontSize: '2rem', fontWeight: 900, color: '#e2e8f0'}}>
                {formatINR(credits?.balance_paise || 0)}
              </div>
              <div style={{fontSize: '1rem', color: '#22d3ee', fontWeight: 700}}>
                ≈ {credits?.minutes_available ?? 0} min
              </div>
            </div>
            <div style={{fontSize: '0.75rem', color: '#64748b', marginTop: '4px'}}>
              Rate: {formatINR(ratePerMin)}/min &middot; Real telephony calls only (web-sim is free)
            </div>
          </div>
          <button onClick={() => { setTopupAmount(500); setTopupOpen(true); }} className="btn-primary"
            style={{padding: '10px 18px', fontSize: '0.85rem', fontWeight: 700,
              background: 'linear-gradient(135deg, #6366f1, #22d3ee)', border: 'none'}}>
            + Add Credits
          </button>
        </div>

        {/* Compact ledger */}
        {creditTxns.length > 0 && (
          <details style={{marginTop: '1rem'}}>
            <summary style={{cursor: 'pointer', color: '#94a3b8', fontSize: '0.8rem', fontWeight: 600, userSelect: 'none'}}>
              Ledger ({creditTxns.length} entries)
            </summary>
            <table style={{width: '100%', fontSize: '0.75rem', marginTop: '10px', borderCollapse: 'collapse'}}>
              <thead>
                <tr style={{borderBottom: '1px solid rgba(148,163,184,0.1)', color: '#64748b'}}>
                  <th style={{textAlign: 'left', padding: '6px 4px', fontWeight: 600}}>When</th>
                  <th style={{textAlign: 'left', padding: '6px 4px', fontWeight: 600}}>Type</th>
                  <th style={{textAlign: 'right', padding: '6px 4px', fontWeight: 600}}>Amount</th>
                  <th style={{textAlign: 'right', padding: '6px 4px', fontWeight: 600}}>Balance</th>
                  <th style={{textAlign: 'left', padding: '6px 4px', fontWeight: 600}}>Reference</th>
                </tr>
              </thead>
              <tbody>
                {creditTxns.map(t => (
                  <tr key={t.id} style={{borderBottom: '1px solid rgba(148,163,184,0.05)'}}>
                    <td style={{padding: '6px 4px', color: '#cbd5e1'}}>{new Date(t.created_at?.replace(' ', 'T') + 'Z').toLocaleString()}</td>
                    <td style={{padding: '6px 4px'}}>
                      <span style={{
                        padding: '1px 6px', borderRadius: '4px', fontSize: '0.7rem', fontWeight: 600,
                        background: t.type === 'purchase' ? 'rgba(34,197,94,0.15)' : t.type === 'call_deduction' ? 'rgba(239,68,68,0.1)' : 'rgba(148,163,184,0.15)',
                        color: t.type === 'purchase' ? '#22c55e' : t.type === 'call_deduction' ? '#f87171' : '#94a3b8',
                      }}>{t.type}</span>
                    </td>
                    <td style={{padding: '6px 4px', textAlign: 'right', fontWeight: 700,
                      color: t.delta_paise >= 0 ? '#22c55e' : '#f87171'}}>
                      {t.delta_paise >= 0 ? '+' : ''}{formatINR(t.delta_paise)}
                    </td>
                    <td style={{padding: '6px 4px', textAlign: 'right', color: '#cbd5e1'}}>{formatINR(t.balance_after_paise)}</td>
                    <td style={{padding: '6px 4px', color: '#64748b', fontFamily: 'monospace', fontSize: '0.7rem'}}>
                      {t.type === 'call_deduction' && t.call_duration_s > 0
                        ? `${t.call_duration_s.toFixed(1)}s · ${(t.reference || '').slice(0, 8)}`
                        : (t.reference || '—')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </details>
        )}
      </div>

      {/* Top-up modal */}
      {topupOpen && (
        <div onClick={() => !topupBusy && setTopupOpen(false)} style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex',
          alignItems: 'center', justifyContent: 'center', zIndex: 1000, padding: '1rem'
        }}>
          <div onClick={e => e.stopPropagation()} className="glass-panel" style={{
            maxWidth: '480px', width: '100%', padding: '1.75rem',
            background: '#0f172a', border: '1px solid rgba(99,102,241,0.3)'
          }}>
            <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem'}}>
              <h3 style={{margin: 0, fontSize: '1.1rem'}}>Add Call Credits</h3>
              <button onClick={() => !topupBusy && setTopupOpen(false)} disabled={topupBusy}
                style={{background: 'transparent', border: 'none', color: '#94a3b8', fontSize: '1.4rem', cursor: 'pointer'}}>×</button>
            </div>

            <div style={{fontSize: '0.85rem', color: '#94a3b8', marginBottom: '1rem'}}>
              Pay-per-minute at {formatINR(ratePerMin)} per minute. Credits never expire.
            </div>

            <div style={{display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '8px', marginBottom: '1rem'}}>
              {TOPUP_PRESETS.map(amt => {
                const selected = Number(topupAmount) === amt;
                return (
                  <button key={amt} onClick={() => setTopupAmount(amt)} disabled={topupBusy}
                    style={{
                      padding: '12px', borderRadius: '8px', cursor: 'pointer', textAlign: 'center',
                      background: selected ? 'rgba(99,102,241,0.18)' : 'rgba(255,255,255,0.03)',
                      border: selected ? '2px solid #6366f1' : '1px solid rgba(148,163,184,0.15)',
                      color: '#e2e8f0',
                    }}>
                    <div style={{fontSize: '1.1rem', fontWeight: 800}}>₹{amt.toLocaleString('en-IN')}</div>
                    <div style={{fontSize: '0.75rem', color: '#22d3ee', marginTop: '2px'}}>{minutesFor(amt)} min</div>
                  </button>
                );
              })}
            </div>

            <div style={{marginBottom: '1rem'}}>
              <label style={{display: 'block', fontSize: '0.8rem', color: '#94a3b8', marginBottom: '6px'}}>Custom amount (₹)</label>
              <input type="number" min="1" max="100000" step="1" value={topupAmount} disabled={topupBusy}
                onChange={e => setTopupAmount(e.target.value)}
                className="form-input" style={{width: '100%'}} />
              <div style={{fontSize: '0.75rem', color: '#64748b', marginTop: '4px'}}>
                = {minutesFor(Number(topupAmount) || 0)} minutes at {formatINR(ratePerMin)}/min
              </div>
            </div>

            <div style={{display: 'flex', gap: '8px', justifyContent: 'flex-end'}}>
              <button onClick={() => setTopupOpen(false)} disabled={topupBusy}
                style={{padding: '8px 14px', borderRadius: '6px', cursor: 'pointer', background: 'transparent',
                  border: '1px solid rgba(148,163,184,0.2)', color: '#94a3b8'}}>Cancel</button>
              <button onClick={handleTopup} disabled={topupBusy || !topupAmount || Number(topupAmount) <= 0}
                className="btn-primary"
                style={{padding: '8px 16px', fontSize: '0.85rem', fontWeight: 700,
                  opacity: topupBusy ? 0.6 : 1, cursor: topupBusy ? 'wait' : 'pointer'}}>
                {topupBusy ? '⏳ Processing…' : `Pay ₹${Number(topupAmount).toLocaleString('en-IN')}`}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Current Subscription + Usage */}
      {hasActiveSub && usage?.has_subscription && (
        <div className="glass-panel" style={{marginBottom: '1.5rem', padding: '1.5rem'}}>
          <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'start', flexWrap: 'wrap', gap: '1rem'}}>
            <div>
              <div style={{fontSize: '0.75rem', color: '#64748b', textTransform: 'uppercase', letterSpacing: '1px'}}>Current Plan</div>
              <div style={{fontSize: '1.4rem', fontWeight: 800, marginTop: '4px'}}>{usage.plan_name}</div>
              <div style={{fontSize: '0.8rem', color: '#94a3b8', marginTop: '4px'}}>
                Status: <span style={{color: subscription.status === 'active' ? '#22c55e' : subscription.status === 'trialing' ? '#f59e0b' : '#ef4444', fontWeight: 600}}>
                  {subscription.status.toUpperCase()}
                </span>
              </div>
              {usage.period_end && (
                <div style={{fontSize: '0.75rem', color: '#64748b', marginTop: '4px'}}>
                  Renews: {new Date(usage.period_end).toLocaleDateString()}
                </div>
              )}
            </div>
            <button className="btn-danger" onClick={handleCancel} style={{fontSize: '0.75rem', padding: '6px 14px'}}>Cancel Plan</button>
          </div>

          {/* Usage bar */}
          <div style={{marginTop: '1.5rem'}}>
            <div style={{display: 'flex', justifyContent: 'space-between', fontSize: '0.8rem', marginBottom: '6px'}}>
              <span style={{color: '#94a3b8'}}>Minutes Used</span>
              <span style={{fontWeight: 700}}>{usage.minutes_used} / {usage.minutes_included} min</span>
            </div>
            <div style={{background: 'rgba(100,116,139,0.2)', borderRadius: '8px', height: '12px', overflow: 'hidden'}}>
              <div style={{
                width: `${usagePercent}%`,
                height: '100%',
                borderRadius: '8px',
                background: usagePercent > 90 ? 'linear-gradient(90deg, #ef4444, #dc2626)' :
                             usagePercent > 70 ? 'linear-gradient(90deg, #f59e0b, #eab308)' :
                             'linear-gradient(90deg, #6366f1, #22d3ee)',
                transition: 'width 0.5s ease',
              }} />
            </div>
            <div style={{display: 'flex', justifyContent: 'space-between', fontSize: '0.7rem', color: '#64748b', marginTop: '4px'}}>
              <span>{Math.round(usagePercent)}% used</span>
              <span>{usage.minutes_remaining} min remaining</span>
            </div>
            {usage.overage_minutes > 0 && (
              <div style={{fontSize: '0.75rem', color: '#ef4444', marginTop: '8px', fontWeight: 600}}>
                Overage: {usage.overage_minutes} min ({formatINR(usage.overage_cost_paise)})
              </div>
            )}
          </div>
        </div>
      )}

      {/* Plans */}
      <div style={{marginBottom: '1.5rem'}}>
        <h3 style={{fontSize: '1rem', marginBottom: '1rem', color: '#94a3b8'}}>
          {hasActiveSub ? 'Change Plan' : 'Choose a Plan'}
        </h3>
        <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: '1rem'}}>
          {plans.map(plan => {
            const isCurrentPlan = hasActiveSub && subscription.plan_id === plan.id;
            return (
              <div key={plan.id} className="glass-panel" style={{
                padding: '1.5rem',
                border: isCurrentPlan ? '2px solid #6366f1' : undefined,
                position: 'relative',
              }}>
                {isCurrentPlan && (
                  <div style={{position: 'absolute', top: '12px', right: '12px', background: '#6366f1', color: '#fff', fontSize: '0.65rem', padding: '2px 8px', borderRadius: '4px', fontWeight: 700}}>CURRENT</div>
                )}
                <div style={{fontSize: '1rem', fontWeight: 700}}>{plan.name}</div>
                <div style={{fontSize: '1.8rem', fontWeight: 900, marginTop: '8px'}}>{formatINR(plan.price_paise)}<span style={{fontSize: '0.8rem', color: '#64748b', fontWeight: 400}}>/{plan.billing_interval}</span></div>
                <div style={{fontSize: '0.85rem', color: '#22d3ee', fontWeight: 600, marginTop: '4px'}}>{plan.minutes_included.toLocaleString()} minutes included</div>
                <div style={{fontSize: '0.75rem', color: '#64748b', marginTop: '2px'}}>Extra: {formatINR(plan.extra_minute_paise)}/min</div>
                {plan.trial_days > 0 && (
                  <div style={{fontSize: '0.75rem', color: '#f59e0b', marginTop: '4px', fontWeight: 600}}>{plan.trial_days}-day free trial</div>
                )}
                <ul style={{marginTop: '16px', listStyle: 'none', padding: 0}}>
                  {(plan.features || []).map((f, i) => (
                    <li key={i} style={{fontSize: '0.8rem', color: '#cbd5e1', padding: '4px 0', display: 'flex', alignItems: 'center', gap: '8px'}}>
                      <span style={{color: '#22d3ee', fontSize: '0.75rem'}}>&#10003;</span> {f}
                    </li>
                  ))}
                </ul>
                {!isCurrentPlan && (
                  <button className="btn-primary" onClick={() => handleSubscribe(plan.id)}
                    style={{width: '100%', marginTop: '16px', padding: '10px', fontSize: '0.85rem'}}>
                    {plan.trial_days > 0 ? 'Start Free Trial' : 'Subscribe'}
                  </button>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* Payment History */}
      {payments.length > 0 && (
        <div className="glass-panel" style={{padding: '1.5rem', marginBottom: '1.5rem'}}>
          <h3 style={{fontSize: '1rem', marginBottom: '1rem', color: '#94a3b8'}}>Payment History</h3>
          <table style={{width: '100%', fontSize: '0.8rem', borderCollapse: 'collapse'}}>
            <thead>
              <tr style={{borderBottom: '1px solid rgba(148,163,184,0.1)'}}>
                <th style={{textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Date</th>
                <th style={{textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Amount</th>
                <th style={{textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Status</th>
                <th style={{textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Payment ID</th>
              </tr>
            </thead>
            <tbody>
              {payments.map(p => (
                <tr key={p.id} style={{borderBottom: '1px solid rgba(148,163,184,0.06)'}}>
                  <td style={{padding: '8px 4px'}}>{new Date(p.created_at).toLocaleDateString()}</td>
                  <td style={{padding: '8px 4px', fontWeight: 600}}>{formatINR(p.amount_paise)}</td>
                  <td style={{padding: '8px 4px'}}>
                    <span style={{
                      padding: '2px 8px', borderRadius: '4px', fontSize: '0.7rem', fontWeight: 600,
                      background: p.status === 'captured' ? 'rgba(34,197,94,0.15)' : p.status === 'failed' ? 'rgba(239,68,68,0.15)' : 'rgba(148,163,184,0.15)',
                      color: p.status === 'captured' ? '#22c55e' : p.status === 'failed' ? '#ef4444' : '#94a3b8',
                    }}>{p.status}</span>
                  </td>
                  <td style={{padding: '8px 4px', color: '#64748b', fontSize: '0.7rem', fontFamily: 'monospace'}}>{p.razorpay_payment_id || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Invoices */}
      <div className="glass-panel" style={{padding: '1.5rem'}}>
        <h3 style={{fontSize: '1rem', marginBottom: '1rem', color: '#94a3b8'}}>Invoices</h3>
        {invoices.length === 0 ? (
          <div style={{textAlign: 'center', padding: '1.5rem', color: '#64748b', fontSize: '0.85rem'}}>No invoices yet</div>
        ) : (
          <table style={{width: '100%', fontSize: '0.8rem', borderCollapse: 'collapse'}}>
            <thead>
              <tr style={{borderBottom: '1px solid rgba(148,163,184,0.1)'}}>
                <th style={{textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Invoice #</th>
                <th style={{textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Date</th>
                <th style={{textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Amount</th>
                <th style={{textAlign: 'left', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Status</th>
                <th style={{textAlign: 'right', padding: '8px 4px', color: '#64748b', fontWeight: 600}}>Download</th>
              </tr>
            </thead>
            <tbody>
              {invoices.map(inv => (
                <tr key={inv.id} style={{borderBottom: '1px solid rgba(148,163,184,0.06)'}}>
                  <td style={{padding: '8px 4px', fontFamily: 'monospace', color: '#cbd5e1'}}>{inv.invoice_number || inv.id}</td>
                  <td style={{padding: '8px 4px'}}>{inv.created_at ? new Date(inv.created_at).toLocaleDateString() : inv.date ? new Date(inv.date).toLocaleDateString() : '-'}</td>
                  <td style={{padding: '8px 4px', fontWeight: 600}}>{formatINR(inv.amount_paise)}</td>
                  <td style={{padding: '8px 4px'}}>
                    <span style={{
                      padding: '2px 8px', borderRadius: '4px', fontSize: '0.7rem', fontWeight: 600,
                      background: inv.status === 'paid' ? 'rgba(34,197,94,0.15)' : inv.status === 'pending' ? 'rgba(245,158,11,0.15)' : 'rgba(148,163,184,0.15)',
                      color: inv.status === 'paid' ? '#22c55e' : inv.status === 'pending' ? '#fbbf24' : '#94a3b8',
                    }}>{inv.status}</span>
                  </td>
                  <td style={{padding: '8px 4px', textAlign: 'right'}}>
                    <button onClick={async () => {
                      // Fetch the PDF as a blob via the Authorization header
                      // and open the resulting object URL — keeps the JWT out
                      // of the URL (issue #80). Backend path uses the invoice
                      // number, not the integer id.
                      try {
                        const res = await apiFetch(`${API_URL}/billing/invoices/${encodeURIComponent(inv.invoice_number || inv.id)}/download`);
                        if (!res.ok) { alert(`Invoice download failed (HTTP ${res.status})`); return; }
                        const blob = await res.blob();
                        const objURL = URL.createObjectURL(blob);
                        window.open(objURL, '_blank', 'noopener,noreferrer');
                        setTimeout(() => URL.revokeObjectURL(objURL), 60_000);
                      } catch (e) { alert('Invoice download failed: ' + (e?.message || 'network error')); }
                    }} style={{
                      padding: '4px 10px', borderRadius: '4px', fontSize: '0.7rem', fontWeight: 600, cursor: 'pointer',
                      background: 'rgba(99,102,241,0.15)', border: '1px solid rgba(99,102,241,0.3)', color: '#a5b4fc',
                    }}>View / Print</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
