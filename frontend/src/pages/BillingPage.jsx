import { useToast, useConfirm } from '../contexts/UIContext';
import React, { useState, useEffect, useRef } from 'react';

const TOPUP_PRESETS = [100, 500, 1000, 5000];

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const thStyle = {
  fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase',
  letterSpacing: '0.07em', padding: '0 0 10px', textAlign: 'left',
  borderBottom: `1px solid ${T.border}`,
};
const tdStyle = { fontSize: 13, color: T.sub, padding: '13px 0', borderBottom: `1px solid ${T.border}`, verticalAlign: 'middle' };

export default function BillingPage({ apiFetch, API_URL }) {
  const toast = useToast();
  const confirmDialog = useConfirm();
  const [plans, setPlans]               = useState([]);
  const [subscription, setSubscription] = useState(null);
  const [usage, setUsage]               = useState(null);
  const [payments, setPayments]         = useState([]);
  const [invoices, setInvoices]         = useState([]);
  const [credits, setCredits]           = useState(null);
  const [creditTxns, setCreditTxns]     = useState([]);
  const [topupOpen, setTopupOpen]       = useState(false);
  const [topupAmount, setTopupAmount]   = useState(500);
  const [topupBusy, setTopupBusy]       = useState(false);
  const [viewingInvoice, setViewingInvoice] = useState(null);
  const [invoiceLoading, setInvoiceLoading] = useState(false);
  const invoiceFrameRef = useRef(null);
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
      const plansData = await plansRes.json(); if (Array.isArray(plansData)) setPlans(plansData);
      const subData   = await subRes.json();   if (subData && !subData.error) setSubscription(subData);
      const usageData = await usageRes.json(); if (usageData && !usageData.error) setUsage(usageData);
      const payData   = await payRes.json();   if (Array.isArray(payData)) setPayments(payData);
      try { const invData = await invRes.json(); if (Array.isArray(invData)) setInvoices(invData); } catch(e) { setInvoices([]); }
      try { const c = await creditsRes.json(); if (c && !c.error) setCredits(c); } catch(e) { setCredits(null); }
      try { const t = await creditTxRes.json(); if (Array.isArray(t)) setCreditTxns(t); } catch(e) { setCreditTxns([]); }
    } catch(e) { console.error('Billing fetch error:', e); }
    setLoading(false);
  };

  const ratePerMin  = credits?.rate_per_min_paise || 500;
  const minutesFor  = (rupees) => Math.floor((rupees * 100) / ratePerMin);
  const formatINR   = (paise) => new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 0 }).format(paise / 100);

  const handleTopup = async () => {
    if (!topupAmount || topupAmount <= 0) return;
    setTopupBusy(true);
    try {
      const res = await apiFetch(`${API_URL}/billing/credits/topup`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ amount_inr: Number(topupAmount) }),
      });
      if (!res.ok) { const body = await res.json().catch(() => ({})); toast('Top-up failed: ' + (body.error || `HTTP ${res.status}`), 'error'); setTopupBusy(false); return; }
      const order = await res.json();
      if (order.order_id && order.key_id) { openCreditsRazorpay(order); }
      else { toast('Razorpay is not configured on the server. Top-ups are disabled until RAZORPAY_KEY_ID is set.', 'error'); }
    } catch(e) { toast('Top-up failed: ' + e.message, 'error'); }
    finally { setTopupBusy(false); }
  };

  const openCreditsRazorpay = (order) => {
    const options = {
      key: order.key_id, amount: order.amount, currency: order.currency || 'INR',
      name: 'Callified AI', description: order.description || `${order.amount_inr} call credits`,
      order_id: order.order_id,
      handler: async (response) => {
        const verifyRes = await apiFetch(`${API_URL}/billing/credits/verify`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ razorpay_order_id: response.razorpay_order_id, razorpay_payment_id: response.razorpay_payment_id, razorpay_signature: response.razorpay_signature }),
        });
        if (verifyRes.ok) { setTopupOpen(false); fetchAll(); }
        else { const body = await verifyRes.json().catch(() => ({})); toast('Payment verification failed: ' + (body.error || 'unknown'), 'error'); }
      },
      modal: { ondismiss: () => setTopupBusy(false) },
      theme: { color: '#6366f1' },
    };
    new window.Razorpay(options).open();
  };

  const handleSubscribe = async (planId) => {
    try {
      const res = await apiFetch(`${API_URL}/billing/create-order`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ plan_id: planId }) });
      const order = await res.json();
      if (order.order_id) { openRazorpay(order, planId); }
      else {
        const subRes = await apiFetch(`${API_URL}/billing/subscribe`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ plan_id: planId }) });
        if (subRes.ok) { fetchAll(); }
      }
    } catch(e) {
      try {
        const subRes = await apiFetch(`${API_URL}/billing/subscribe`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ plan_id: planId }) });
        if (subRes.ok) { fetchAll(); }
      } catch(e2) { toast('Failed to subscribe: ' + e2.message, 'error'); }
    }
  };

  const openRazorpay = (order, planId) => {
    const options = {
      key: order.key_id, amount: order.amount, currency: order.currency,
      name: 'Callified AI', description: order.plan_name, order_id: order.order_id,
      handler: async (response) => {
        const verifyRes = await apiFetch(`${API_URL}/billing/verify-payment`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ razorpay_order_id: response.razorpay_order_id, razorpay_payment_id: response.razorpay_payment_id, razorpay_signature: response.razorpay_signature, plan_id: planId }),
        });
        if (verifyRes.ok) { fetchAll(); }
      },
      theme: { color: '#6366f1' },
    };
    new window.Razorpay(options).open();
  };

  const handleCancel = async () => {
    const ok = await confirmDialog({
      title: 'Cancel subscription',
      message: 'Are you sure you want to cancel your subscription? You\'ll lose access at the end of the current billing period.',
      okText: 'Cancel subscription',
      cancelText: 'Keep it',
      danger: true,
    });
    if (!ok) return;
    try {
      await apiFetch(`${API_URL}/billing/cancel`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ reason: 'User cancelled' }) });
      fetchAll();
    } catch(e) { toast('Failed to cancel', 'error'); }
  };

  if (loading) return <div style={{ padding: '3rem', textAlign: 'center', color: T.muted, fontFamily: T.font }}>Loading billing…</div>;

  const hasActiveSub  = subscription && subscription.status && subscription.status !== 'none';
  const usagePercent  = usage?.has_subscription ? Math.min(100, (usage.minutes_used / usage.minutes_included) * 100) : 0;

  // Mark the middle plan as POPULAR
  const popularIdx = Math.floor(plans.length / 2);

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Title */}
      <h2 style={{ margin: '0 0 24px', fontSize: 22, fontWeight: 700, color: T.text }}>Billing</h2>

      {/* Call Credits card */}
      <div style={{ ...card, marginBottom: 20, padding: '22px 28px', borderLeft: `4px solid ${T.accent}` }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <div>
            <div style={{ fontSize: 10, fontWeight: 700, color: T.accent, textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 8 }}>
              📞 Call Credits
            </div>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 12, marginBottom: 4 }}>
              <div style={{ fontSize: 32, fontWeight: 900, fontFamily: T.mono, color: T.text }}>
                {formatINR(credits?.balance_paise || 0)}
              </div>
              <div style={{ fontSize: 14, color: T.accent, fontWeight: 700 }}>
                ≈ {credits?.minutes_available ?? 0} min
              </div>
            </div>
            <div style={{ fontSize: 12, color: T.muted, marginBottom: 6 }}>
              Rate: {formatINR(ratePerMin)}/min · Real telephony calls only (web-sim is free)
            </div>
            {creditTxns.length > 0 && (
              <details>
                <summary style={{ cursor: 'pointer', color: T.accent, fontSize: 12, fontWeight: 600, userSelect: 'none' }}>
                  ▸ Ledger ({creditTxns.length} entries)
                </summary>
                <table style={{ width: '100%', fontSize: 12, marginTop: 10, borderCollapse: 'collapse' }}>
                  <thead>
                    <tr>
                      {['When', 'Type', 'Amount', 'Balance', 'Reference'].map(h => (
                        <th key={h} style={{ ...thStyle, fontSize: 9, padding: '0 0 6px' }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {creditTxns.map(t => (
                      <tr key={t.id}>
                        <td style={{ ...tdStyle, fontSize: 11, padding: '8px 0' }}>{new Date(t.created_at?.replace(' ', 'T') + 'Z').toLocaleString()}</td>
                        <td style={{ ...tdStyle, fontSize: 11, padding: '8px 0' }}>
                          <span style={{
                            padding: '1px 7px', borderRadius: 10, fontSize: 10, fontWeight: 600,
                            background: t.type === 'purchase' ? 'rgba(16,185,129,0.1)' : t.type === 'call_deduction' ? 'rgba(239,68,68,0.1)' : `rgba(156,163,175,0.15)`,
                            color: t.type === 'purchase' ? T.green : t.type === 'call_deduction' ? T.red : T.muted,
                          }}>{t.type}</span>
                        </td>
                        <td style={{ ...tdStyle, fontSize: 11, padding: '8px 0', fontWeight: 700, fontFamily: T.mono, color: t.delta_paise >= 0 ? T.green : T.red }}>
                          {t.delta_paise >= 0 ? '+' : ''}{formatINR(t.delta_paise)}
                        </td>
                        <td style={{ ...tdStyle, fontSize: 11, padding: '8px 0', fontFamily: T.mono }}>{formatINR(t.balance_after_paise)}</td>
                        <td style={{ ...tdStyle, fontSize: 10, padding: '8px 0', color: T.muted, fontFamily: 'monospace' }}>
                          {t.type === 'call_deduction' && t.call_duration_s > 0 ? `${t.call_duration_s.toFixed(1)}s · ${(t.reference || '').slice(0, 8)}` : (t.reference || '—')}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </details>
            )}
          </div>
          <button onClick={() => { setTopupAmount(500); setTopupOpen(true); }} style={{
            padding: '10px 20px', borderRadius: 8, border: 'none', cursor: 'pointer',
            background: T.accent, color: '#fff', fontSize: 13, fontWeight: 700, fontFamily: T.font, flexShrink: 0,
          }}>
            + Add Credits
          </button>
        </div>
      </div>

      {/* Active subscription + usage */}
      {hasActiveSub && usage?.has_subscription && (
        <div style={{ ...card, marginBottom: 20, padding: '22px 28px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 16 }}>
            <div>
              <div style={{ fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 4 }}>Current Plan</div>
              <div style={{ fontSize: 20, fontWeight: 800, color: T.text }}>{usage.plan_name}</div>
              <div style={{ fontSize: 12, color: T.muted, marginTop: 4 }}>
                Status: <span style={{ color: subscription.status === 'active' ? T.green : subscription.status === 'trialing' ? T.amber : T.red, fontWeight: 700 }}>
                  {subscription.status.toUpperCase()}
                </span>
              </div>
              {usage.period_end && <div style={{ fontSize: 12, color: T.muted, marginTop: 2 }}>Renews: {new Date(usage.period_end).toLocaleDateString()}</div>}
            </div>
            <button onClick={handleCancel} style={{
              padding: '7px 16px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer', fontFamily: T.font,
              background: 'rgba(239,68,68,0.08)', border: `1px solid rgba(239,68,68,0.25)`, color: T.red,
            }}>Cancel Plan</button>
          </div>
          <div style={{ fontSize: 12, color: T.muted, display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
            <span>Minutes Used</span>
            <span style={{ fontWeight: 700, color: T.sub }}>{usage.minutes_used} / {usage.minutes_included} min</span>
          </div>
          <div style={{ background: T.border, borderRadius: 6, height: 8, overflow: 'hidden' }}>
            <div style={{
              width: `${usagePercent}%`, height: '100%', borderRadius: 6,
              background: usagePercent > 90 ? T.red : usagePercent > 70 ? T.amber : T.accent,
              transition: 'width 0.5s',
            }} />
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: T.muted, marginTop: 4 }}>
            <span>{Math.round(usagePercent)}% used</span>
            <span>{usage.minutes_remaining} min remaining</span>
          </div>
          {usage.overage_minutes > 0 && (
            <div style={{ fontSize: 12, color: T.red, marginTop: 8, fontWeight: 600 }}>
              Overage: {usage.overage_minutes} min ({formatINR(usage.overage_cost_paise)})
            </div>
          )}
        </div>
      )}

      {/* Plans */}
      {plans.length > 0 && (
        <div style={{ marginBottom: 20 }}>
          <h3 style={{ margin: '0 0 16px', fontSize: 16, fontWeight: 700, color: T.text }}>
            {hasActiveSub ? 'Change Plan' : 'Choose a Plan'}
          </h3>
          <div style={{ display: 'grid', gridTemplateColumns: `repeat(${plans.length}, 1fr)`, gap: 12 }}>
            {plans.map((plan, idx) => {
              const isCurrentPlan = hasActiveSub && subscription.plan_id === plan.id;
              const isPopular     = idx === popularIdx;
              return (
                <div key={plan.id} style={{
                  ...card,
                  padding: '24px',
                  position: 'relative',
                  border: isPopular || isCurrentPlan ? `2px solid ${T.accent}` : `1px solid ${T.border}`,
                }}>
                  {isPopular && (
                    <div style={{
                      position: 'absolute', top: -12, left: '50%', transform: 'translateX(-50%)',
                      background: T.accent, color: '#fff', fontSize: 10, fontWeight: 700,
                      padding: '3px 12px', borderRadius: 20, letterSpacing: '0.07em',
                    }}>POPULAR</div>
                  )}
                  {isCurrentPlan && !isPopular && (
                    <div style={{
                      position: 'absolute', top: 12, right: 12, background: T.accent, color: '#fff',
                      fontSize: 10, fontWeight: 700, padding: '2px 8px', borderRadius: 4,
                    }}>CURRENT</div>
                  )}
                  <div style={{ fontSize: 16, fontWeight: 700, color: T.text, marginBottom: 8 }}>{plan.name}</div>
                  <div style={{ marginBottom: 6 }}>
                    <span style={{ fontSize: 24, fontWeight: 900, fontFamily: T.mono, color: T.accent }}>{formatINR(plan.price_paise)}</span>
                    <span style={{ fontSize: 13, color: T.muted }}>/{plan.billing_interval}</span>
                  </div>
                  <div style={{ fontSize: 13, color: T.muted, marginBottom: 4 }}>{plan.minutes_included.toLocaleString()} min included</div>
                  <div style={{ fontSize: 12, color: T.muted, marginBottom: plan.trial_days > 0 ? 4 : 16 }}>
                    Extra: {formatINR(plan.extra_minute_paise)}/min
                  </div>
                  {plan.trial_days > 0 && (
                    <div style={{ fontSize: 12, color: T.amber, fontWeight: 600, marginBottom: 16 }}>{plan.trial_days}-day free trial</div>
                  )}
                  {(plan.features || []).length > 0 && (
                    <ul style={{ margin: '0 0 16px', padding: 0, listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 4 }}>
                      {plan.features.map((f, i) => (
                        <li key={i} style={{ fontSize: 12, color: T.sub, display: 'flex', alignItems: 'center', gap: 6 }}>
                          <span style={{ color: T.green, fontWeight: 700 }}>✓</span> {f}
                        </li>
                      ))}
                    </ul>
                  )}
                  {!isCurrentPlan && (
                    <button onClick={() => handleSubscribe(plan.id)} style={{
                      width: '100%', padding: '10px', borderRadius: 8, fontSize: 13, fontWeight: 700,
                      cursor: 'pointer', fontFamily: T.font,
                      background: isPopular ? T.accent : T.card,
                      color: isPopular ? '#fff' : T.sub,
                      border: isPopular ? 'none' : `1px solid ${T.border}`,
                    }}>
                      {plan.trial_days > 0 ? 'Start Free Trial' : 'Select Plan'}
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Payment History */}
      {payments.length > 0 && (
        <div style={{ ...card, padding: '24px 28px', marginBottom: 16 }}>
          <h3 style={{ margin: '0 0 16px', fontSize: 15, fontWeight: 700, color: T.text }}>Payment History</h3>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Date', 'Amount', 'Status', 'Payment ID'].map(h => <th key={h} style={thStyle}>{h}</th>)}
              </tr>
            </thead>
            <tbody>
              {payments.map((p, i) => {
                const isLast = i === payments.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                return (
                  <tr key={p.id}>
                    <td style={rowTd}>{new Date(p.created_at).toLocaleDateString()}</td>
                    <td style={{ ...rowTd, fontWeight: 700, fontFamily: T.mono }}>{formatINR(p.amount_paise)}</td>
                    <td style={rowTd}>
                      <span style={{
                        padding: '2px 10px', borderRadius: 20, fontSize: 11, fontWeight: 600,
                        background: p.status === 'captured' ? 'rgba(16,185,129,0.1)' : p.status === 'failed' ? 'rgba(239,68,68,0.1)' : 'rgba(156,163,175,0.12)',
                        color: p.status === 'captured' ? T.green : p.status === 'failed' ? T.red : T.muted,
                      }}>{p.status}</span>
                    </td>
                    <td style={{ ...rowTd, fontFamily: 'monospace', fontSize: 11, color: T.muted }}>{p.razorpay_payment_id || '—'}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Invoices */}
      <div style={{ ...card, padding: '24px 28px' }}>
        <h3 style={{ margin: '0 0 16px', fontSize: 15, fontWeight: 700, color: T.text }}>Invoices</h3>
        {invoices.length === 0 ? (
          <p style={{ color: T.muted, textAlign: 'center', padding: '1.5rem 0', fontSize: 13 }}>No invoices yet</p>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Invoice #', 'Date', 'Amount', 'Status', 'Download'].map((h, i) => (
                  <th key={h} style={{ ...thStyle, textAlign: i === 4 ? 'right' : 'left' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {invoices.map((inv, i) => {
                const isLast = i === invoices.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                return (
                  <tr key={inv.id}>
                    <td style={{ ...rowTd, fontFamily: 'monospace', fontSize: 12, color: T.sub }}>{inv.invoice_number || inv.id}</td>
                    <td style={rowTd}>{inv.created_at ? new Date(inv.created_at).toLocaleDateString() : inv.date ? new Date(inv.date).toLocaleDateString() : '-'}</td>
                    <td style={{ ...rowTd, fontWeight: 700, fontFamily: T.mono }}>{formatINR(inv.amount_paise)}</td>
                    <td style={rowTd}>
                      <span style={{
                        padding: '2px 10px', borderRadius: 20, fontSize: 11, fontWeight: 600,
                        background: inv.status === 'paid' ? 'rgba(16,185,129,0.1)' : inv.status === 'pending' ? 'rgba(245,158,11,0.1)' : 'rgba(156,163,175,0.12)',
                        color: inv.status === 'paid' ? T.green : inv.status === 'pending' ? T.amber : T.muted,
                      }}>{inv.status}</span>
                    </td>
                    <td style={{ ...rowTd, textAlign: 'right' }}>
                      <button onClick={async () => {
                        setInvoiceLoading(true);
                        try {
                          const res = await apiFetch(`${API_URL}/billing/invoices/${encodeURIComponent(inv.invoice_number || inv.id)}/download`);
                          if (!res.ok) { toast(`Invoice fetch failed (HTTP ${res.status})`, 'error'); return; }
                          const blob = await res.blob();
                          setViewingInvoice({ number: inv.invoice_number || inv.id, blobUrl: URL.createObjectURL(blob) });
                        } catch(e) { toast('Invoice fetch failed: ' + (e?.message || 'network error'), 'error'); }
                        finally { setInvoiceLoading(false); }
                      }} disabled={invoiceLoading} style={{
                        padding: '5px 12px', borderRadius: 6, fontSize: 12, fontWeight: 600, fontFamily: T.font,
                        cursor: invoiceLoading ? 'wait' : 'pointer', opacity: invoiceLoading ? 0.6 : 1,
                        background: T.card, border: `1px solid ${T.border}`, color: T.sub,
                      }}>
                        {invoiceLoading ? 'Loading…' : 'View / Print'}
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* Top-up modal */}
      {topupOpen && (
        <div onClick={() => !topupBusy && setTopupOpen(false)} style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.25)',
          display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000, padding: '1rem',
        }}>
          <div onClick={e => e.stopPropagation()} style={{
            ...card, maxWidth: 480, width: '100%', padding: '28px',
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: T.text }}>Add Call Credits</h3>
              <button onClick={() => !topupBusy && setTopupOpen(false)} disabled={topupBusy}
                style={{ background: 'transparent', border: 'none', color: T.muted, fontSize: 22, cursor: 'pointer', lineHeight: 1 }}>×</button>
            </div>
            <p style={{ margin: '0 0 16px', fontSize: 13, color: T.muted }}>
              Pay-per-minute at {formatINR(ratePerMin)} per minute. Credits never expire.
            </p>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 8, marginBottom: 16 }}>
              {TOPUP_PRESETS.map(amt => {
                const sel = Number(topupAmount) === amt;
                return (
                  <button key={amt} onClick={() => setTopupAmount(amt)} disabled={topupBusy} style={{
                    padding: 12, borderRadius: 8, cursor: 'pointer', textAlign: 'center',
                    background: sel ? 'rgba(99,102,241,0.08)' : T.bg,
                    border: sel ? `2px solid ${T.accent}` : `1px solid ${T.border}`,
                    color: T.text,
                  }}>
                    <div style={{ fontSize: 16, fontWeight: 800, fontFamily: T.mono }}>₹{amt.toLocaleString('en-IN')}</div>
                    <div style={{ fontSize: 11, color: T.accent, marginTop: 2 }}>{minutesFor(amt)} min</div>
                  </button>
                );
              })}
            </div>
            <label style={{ display: 'block', fontSize: 12, color: T.muted, marginBottom: 6 }}>Custom amount (₹)</label>
            <input type="number" min="1" max="100000" step="1" value={topupAmount} disabled={topupBusy}
              onChange={e => setTopupAmount(e.target.value)}
              style={{
                width: '100%', padding: '9px 12px', borderRadius: 8, border: `1px solid ${T.border}`,
                background: T.card, color: T.text, fontSize: 13, fontFamily: T.font, outline: 'none', boxSizing: 'border-box',
              }} />
            <div style={{ fontSize: 11, color: T.muted, marginTop: 4, marginBottom: 20 }}>
              = {minutesFor(Number(topupAmount) || 0)} minutes at {formatINR(ratePerMin)}/min
            </div>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button onClick={() => setTopupOpen(false)} disabled={topupBusy} style={{
                padding: '8px 16px', borderRadius: 8, cursor: 'pointer', background: T.bg,
                border: `1px solid ${T.border}`, color: T.sub, fontSize: 13, fontFamily: T.font,
              }}>Cancel</button>
              <button onClick={handleTopup} disabled={topupBusy || !topupAmount || Number(topupAmount) <= 0} style={{
                padding: '8px 18px', borderRadius: 8, fontSize: 13, fontWeight: 700, fontFamily: T.font,
                background: T.accent, color: '#fff', border: 'none', cursor: topupBusy ? 'wait' : 'pointer',
                opacity: (topupBusy || !topupAmount || Number(topupAmount) <= 0) ? 0.6 : 1,
              }}>
                {topupBusy ? '⏳ Processing…' : `Pay ₹${Number(topupAmount).toLocaleString('en-IN')}`}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Invoice viewer modal */}
      {viewingInvoice && (
        <div onClick={() => { if (viewingInvoice?.blobUrl) URL.revokeObjectURL(viewingInvoice.blobUrl); setViewingInvoice(null); }}
          style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.35)', backdropFilter: 'blur(4px)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 10000, padding: '1rem' }}>
          <div onClick={e => e.stopPropagation()} style={{
            width: '100%', maxWidth: 880, height: '90vh', maxHeight: 1100,
            background: T.card, border: `1px solid ${T.border}`, borderRadius: 12,
            boxShadow: '0 24px 48px rgba(0,0,0,0.15)', display: 'flex', flexDirection: 'column', overflow: 'hidden',
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '14px 20px', borderBottom: `1px solid ${T.border}` }}>
              <div>
                <div style={{ fontSize: 10, color: T.muted, textTransform: 'uppercase', letterSpacing: '0.07em' }}>Invoice</div>
                <div style={{ fontFamily: 'monospace', fontSize: 14, fontWeight: 700, color: T.text }}>{viewingInvoice.number}</div>
              </div>
              <div style={{ display: 'flex', gap: 8 }}>
                <button onClick={() => { try { invoiceFrameRef.current?.contentWindow?.print(); } catch(e) { toast('Print failed: ' + (e?.message || 'unknown'), 'error'); } }}
                  style={{ padding: '6px 14px', borderRadius: 6, cursor: 'pointer', background: T.accent, border: 'none', color: '#fff', fontSize: 12, fontWeight: 700, fontFamily: T.font }}>
                  🖨 Print
                </button>
                <a href={viewingInvoice.blobUrl} download={`${viewingInvoice.number}.pdf`}
                  style={{ padding: '6px 14px', borderRadius: 6, background: T.bg, border: `1px solid ${T.border}`, color: T.sub, fontSize: 12, fontWeight: 600, textDecoration: 'none', display: 'flex', alignItems: 'center' }}>
                  ↓ Download
                </a>
                <button onClick={() => { if (viewingInvoice?.blobUrl) URL.revokeObjectURL(viewingInvoice.blobUrl); setViewingInvoice(null); }}
                  style={{ padding: '6px 12px', borderRadius: 6, cursor: 'pointer', background: T.bg, border: `1px solid ${T.border}`, color: T.sub, fontSize: 12, fontFamily: T.font }}>
                  Close
                </button>
              </div>
            </div>
            <iframe ref={invoiceFrameRef} src={viewingInvoice.blobUrl} title={`Invoice ${viewingInvoice.number}`} style={{ flex: 1, width: '100%', border: 'none', background: '#fff' }} />
          </div>
        </div>
      )}
    </div>
  );
}
