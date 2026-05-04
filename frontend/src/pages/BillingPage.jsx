import React, { useState, useEffect } from 'react';

export default function BillingPage({ apiFetch, API_URL }) {
  const [plans, setPlans] = useState([]);
  const [subscription, setSubscription] = useState(null);
  const [usage, setUsage] = useState(null);
  const [payments, setPayments] = useState([]);
  const [invoices, setInvoices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showCancelConfirm, setShowCancelConfirm] = useState(false);
  const [cancelTyped, setCancelTyped] = useState('');
  const [cancelling, setCancelling] = useState(false);
  const [cancelError, setCancelError] = useState('');

  useEffect(() => { fetchAll(); }, []);

  const fetchAll = async () => {
    setLoading(true);
    try {
      const [plansRes, subRes, usageRes, payRes, invRes] = await Promise.all([
        apiFetch(`${API_URL}/billing/plans`),
        apiFetch(`${API_URL}/billing/subscription`),
        apiFetch(`${API_URL}/billing/usage`),
        apiFetch(`${API_URL}/billing/payments`),
        apiFetch(`${API_URL}/billing/invoices`),
      ]);
      setPlans(await plansRes.json());
      setSubscription(await subRes.json());
      setUsage(await usageRes.json());
      setPayments(await payRes.json());
      try { setInvoices(await invRes.json()); } catch(e) { setInvoices([]); }
    } catch(e) { console.error('Billing fetch error:', e); }
    setLoading(false);
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
    setCancelling(true);
    setCancelError('');
    try {
      const res = await apiFetch(`${API_URL}/billing/cancel`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ reason: 'User cancelled' }),
      });
      if (!res.ok) {
        const d = await res.json();
        setCancelError(d.detail || 'Failed to cancel subscription. Please try again.');
        setCancelling(false);
        return;
      }
      setShowCancelConfirm(false);
      setCancelTyped('');
      fetchAll();
    } catch(e) {
      setCancelError('Failed to cancel subscription. Please try again.');
    }
    setCancelling(false);
  };

  if (loading) return <div className="page-container"><div className="glass-panel" style={{padding: '2rem', textAlign: 'center'}}>Loading billing...</div></div>;

  const hasActiveSub = subscription && subscription.status && subscription.status !== 'none';
  const usagePercent = usage?.has_subscription ? Math.min(100, (usage.minutes_used / usage.minutes_included) * 100) : 0;

  return (
    <div className="page-container">
      <h2 style={{marginBottom: '1.5rem'}}>Billing</h2>

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
              {subscription.max_campaigns != null && (
                <div style={{fontSize: '0.75rem', color: '#64748b', marginTop: '4px'}}>
                  Campaign limit: <span style={{color: '#e2e8f0', fontWeight: 600}}>{subscription.max_campaigns} active</span>
                </div>
              )}
            </div>
            {!showCancelConfirm && (
              <button
                onClick={() => { setShowCancelConfirm(true); setCancelTyped(''); setCancelError(''); }}
                style={{
                  padding: '6px 14px', fontSize: '0.75rem', fontWeight: 600, cursor: 'pointer', borderRadius: '6px',
                  background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', color: '#fca5a5',
                }}>
                Cancel Plan
              </button>
            )}
          </div>

          {/* Inline cancellation confirmation */}
          {showCancelConfirm && (
            <div style={{
              marginTop: '1.25rem', padding: '1.25rem', borderRadius: '10px',
              background: 'rgba(239,68,68,0.07)', border: '1px solid rgba(239,68,68,0.25)',
            }}>
              <div style={{fontWeight: 700, color: '#fca5a5', marginBottom: '8px', fontSize: '0.95rem'}}>
                Cancel your {usage.plan_name} plan?
              </div>
              <ul style={{margin: '0 0 12px', paddingLeft: '18px', color: '#94a3b8', fontSize: '0.82rem', lineHeight: 1.7}}>
                <li>Your plan stays active until the end of the current billing period (<strong style={{color: '#e2e8f0'}}>{usage.period_end ? new Date(usage.period_end).toLocaleDateString() : 'current period end'}</strong>).</li>
                <li>After that, your account reverts to the free tier and AI calling will be paused.</li>
                <li>Your data, leads, and campaigns are preserved — you can resubscribe at any time.</li>
              </ul>
              <label style={{display: 'flex', alignItems: 'center', gap: '10px', cursor: 'pointer', marginBottom: '14px', userSelect: 'none'}}>
                <input
                  type="checkbox"
                  checked={cancelTyped === 'confirmed'}
                  onChange={e => { setCancelTyped(e.target.checked ? 'confirmed' : ''); setCancelError(''); }}
                  style={{width: '16px', height: '16px', accentColor: '#ef4444', cursor: 'pointer'}}
                />
                <span style={{fontSize: '0.82rem', color: '#e2e8f0'}}>
                  I understand my plan will be cancelled at the end of the billing period.
                </span>
              </label>
              <div style={{display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap'}}>
                <button
                  onClick={handleCancel}
                  disabled={cancelTyped !== 'confirmed' || cancelling}
                  style={{
                    padding: '7px 18px', borderRadius: '6px', fontSize: '0.82rem', fontWeight: 700,
                    cursor: cancelTyped !== 'confirmed' ? 'not-allowed' : 'pointer',
                    background: cancelTyped === 'confirmed' ? 'rgba(239,68,68,0.8)' : 'rgba(239,68,68,0.2)',
                    border: '1px solid rgba(239,68,68,0.4)', color: '#fca5a5',
                    opacity: cancelling ? 0.6 : 1,
                  }}>
                  {cancelling ? 'Cancelling…' : 'Confirm Cancellation'}
                </button>
                <button
                  onClick={() => { setShowCancelConfirm(false); setCancelTyped(''); setCancelError(''); }}
                  disabled={cancelling}
                  style={{
                    padding: '7px 16px', borderRadius: '6px', fontSize: '0.82rem', cursor: 'pointer',
                    background: 'rgba(148,163,184,0.1)', border: '1px solid rgba(148,163,184,0.2)', color: '#94a3b8',
                  }}>
                  Keep Plan
                </button>
              </div>
              {cancelError && (
                <div style={{marginTop: '8px', fontSize: '0.78rem', color: '#fca5a5'}}>{cancelError}</div>
              )}
            </div>
          )}

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
                <div style={{fontSize: '0.75rem', color: '#64748b', marginTop: '2px'}}>
                  Campaigns: <span style={{color: '#e2e8f0', fontWeight: 600}}>{plan.max_campaigns != null ? `Up to ${plan.max_campaigns}` : 'Unlimited'}</span>
                </div>
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
          <div style={{textAlign: 'center', padding: '1.5rem', fontSize: '0.85rem'}}>
            {subscription && subscription.status === 'active' ? (
              <span style={{color: '#f59e0b'}}>
                ⚠ Your subscription is active but no invoice was found.{' '}
                <button onClick={fetchAll}
                  style={{background: 'none', border: 'none', color: '#60a5fa', cursor: 'pointer', textDecoration: 'underline', fontSize: 'inherit', padding: 0}}>
                  Refresh
                </button>
                {' '}— if the issue persists, contact support.
              </span>
            ) : (
              <span style={{color: '#64748b'}}>No invoices yet</span>
            )}
          </div>
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
                    <button onClick={() => {
                      const token = localStorage.getItem('authToken');
                      window.open(`${API_URL}/billing/invoices/${inv.id}/download?token=${encodeURIComponent(token)}`, '_blank');
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
