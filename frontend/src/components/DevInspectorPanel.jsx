import React, { useState, useEffect } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { useOrg } from '../contexts/OrgContext';
import { useVoice } from '../contexts/VoiceContext';
import { useCall } from '../contexts/CallContext';
import { INDIAN_LANGUAGES } from '../constants/voices';

// DevInspectorPanel — floating debug overlay for developer-tagged sessions.
// Mounted by App.jsx only when:
//   sessionStorage.authMode === 'impersonation'   (set by SsoExchange)
//   OR localStorage.devInspector === 'on'         (manual toggle in TopHeader)
//
// Surfaces: identity (actor + target), voice stack, selected org, call state,
// last dial intents (real Dial + Sim, on CRM + Campaigns pages), and recent
// SSE campaign events. Regular users never load this component because the
// conditional mount in App.jsx evaluates false for them.

const langName = (code) =>
  INDIAN_LANGUAGES.find(l => l.code === code)?.name || code || '—';

export default function DevInspectorPanel() {
  const { currentUser, fetchSseTicket } = useAuth();
  const { selectedOrg, orgTimezone, orgs } = useOrg();
  const { activeVoiceProvider, activeVoiceId, activeLanguage, savedVoiceName } = useVoice();
  const { lastDialIntents, dialingId, webCallActive } = useCall();

  const [expanded, setExpanded] = useState(true);
  const [recentEvents, setRecentEvents] = useState([]);

  // Detect mode at render time so a navigation that flips storage shows up
  // without an extra remount.
  const isImpersonation = (() => {
    try { return sessionStorage.getItem('authMode') === 'impersonation'; }
    catch { return false; }
  })();
  const devActor = (() => {
    try { return sessionStorage.getItem('devActor') || ''; }
    catch { return ''; }
  })() || currentUser?.dev_actor || '';

  // Auto-collapse after a brief intro window so it doesn't fight the UI.
  useEffect(() => {
    const t = setTimeout(() => setExpanded(false), 5000);
    return () => clearTimeout(t);
  }, []);

  // SSE feed — last 10 campaign events. Same wiring as LogsTab: short-lived
  // ticket → ?ticket=… so the auth JWT never appears in the URL.
  useEffect(() => {
    let es = null;
    let cancelled = false;
    (async () => {
      try {
        const ticket = await fetchSseTicket();
        if (cancelled) return;
        const url = `/api/campaign-events?campaign_id=all&ticket=${encodeURIComponent(ticket)}`;
        es = new EventSource(url);
        es.onmessage = (msg) => {
          try {
            const ev = JSON.parse(msg.data);
            setRecentEvents(prev => [{ ts: Date.now(), ...ev }, ...prev].slice(0, 10));
          } catch { /* malformed — skip */ }
        };
        es.onerror = () => { /* the connection will retry on its own */ };
      } catch { /* no ticket → just skip the live events panel */ }
    })();
    return () => { cancelled = true; if (es) es.close(); };
  }, [fetchSseTicket]);

  const orgName = orgs?.find?.(o => String(o.id) === String(selectedOrg))?.name;

  const last = lastDialIntents?.[0];

  return (
    <div style={S.wrap}>
      <div style={S.header} onClick={() => setExpanded(x => !x)}>
        <span style={S.modeDot(isImpersonation ? '#ef4444' : '#f59e0b')} />
        <strong style={{ fontSize: 12 }}>
          {isImpersonation ? 'IMPERSONATION' : 'DEV INSPECTOR'}
        </strong>
        <span style={{ marginLeft: 'auto', fontSize: 11, color: '#94a3b8' }}>
          {expanded ? '▾' : '▸'}
        </span>
      </div>

      {expanded && (
        <div style={S.body}>
          <Section title="Session">
            <Row k="acting as" v={currentUser?.email || '—'} />
            {devActor && <Row k="on behalf of" v={devActor} highlight />}
            <Row k="role" v={currentUser?.role || '—'} />
            <Row k="org" v={`${currentUser?.org_name || orgName || '—'} (#${currentUser?.org_id || '?'})`} />
            {orgTimezone && <Row k="tz" v={orgTimezone} />}
          </Section>

          <Section title="Voice stack (active)">
            <Row k="provider" v={activeVoiceProvider || '—'} />
            <Row k="voice" v={savedVoiceName ? `${savedVoiceName} (${activeVoiceId || '—'})` : (activeVoiceId || '—')} />
            <Row k="language" v={`${langName(activeLanguage)} · ${activeLanguage || '—'}`} />
          </Section>

          <Section title="Last dial intent">
            {!last && <div style={S.empty}>No dial yet.</div>}
            {last && (
              <>
                <Row k="kind" v={<span style={S.kindBadge(last.kind)}>{last.kind}</span>} />
                <Row k="when" v={new Date(last.ts).toLocaleTimeString()} />
                <Row k="voice" v={`${last.voiceProvider || '—'} / ${last.voiceName || last.voiceId || '—'}`} />
                <Row k="language" v={`${langName(last.language)} · ${last.language || '—'}`} />
                <Row k="lead" v={`${last.leadName || '—'} #${last.leadId || '?'}${last.leadPhone ? ' · ' + last.leadPhone : ''}`} />
                {last.campaignId && <Row k="campaign" v={`#${last.campaignId}`} />}
                {last.note && <div style={S.note}>{last.note}</div>}
              </>
            )}
            {lastDialIntents?.length > 1 && (
              <details style={{ marginTop: 6 }}>
                <summary style={S.summary}>history ({lastDialIntents.length - 1} more)</summary>
                <div style={{ marginTop: 6, fontSize: 11, color: '#cbd5e1' }}>
                  {lastDialIntents.slice(1).map((it, i) => (
                    <div key={i} style={S.histRow}>
                      <span style={S.kindBadge(it.kind)}>{it.kind}</span>
                      <span>{new Date(it.ts).toLocaleTimeString()}</span>
                      <span>{it.voiceProvider}/{it.voiceName || it.voiceId}</span>
                      <span>{it.language}</span>
                      <span>#{it.leadId}</span>
                    </div>
                  ))}
                </div>
              </details>
            )}
          </Section>

          <Section title="Live call state">
            <Row k="dialing lead" v={dialingId ?? '—'} />
            <Row k="web-call active" v={webCallActive ? `#${webCallActive}` : '—'} />
          </Section>

          <Section title={`Recent events (${recentEvents.length})`}>
            {recentEvents.length === 0 && <div style={S.empty}>No events yet.</div>}
            {recentEvents.slice(0, 6).map((e, i) => (
              <div key={i} style={S.eventRow}>
                <span style={{ color: '#a5b4fc' }}>{(e.event_type || e.type || 'event')}</span>
                <span style={{ color: '#94a3b8' }}>{new Date(e.ts).toLocaleTimeString()}</span>
                <span style={{ color: '#cbd5e1', fontSize: 10 }}>{(e.detail || e.message || '').slice(0, 60)}</span>
              </div>
            ))}
          </Section>

          <button
            style={S.copyBtn}
            onClick={() => {
              const dump = {
                actor: devActor,
                target: currentUser,
                voice: { provider: activeVoiceProvider, voiceId: activeVoiceId, language: activeLanguage, savedVoiceName },
                org: { selectedOrg, orgTimezone, orgName },
                lastDialIntents,
                recentEvents,
              };
              navigator.clipboard.writeText(JSON.stringify(dump, null, 2));
            }}
            title="Copy panel state to clipboard"
          >
            Copy state as JSON
          </button>
        </div>
      )}
    </div>
  );
}

function Section({ title, children }) {
  return (
    <div style={S.section}>
      <div style={S.sectionTitle}>{title}</div>
      <div>{children}</div>
    </div>
  );
}

function Row({ k, v, highlight }) {
  return (
    <div style={{ ...S.row, ...(highlight ? { background: 'rgba(239,68,68,0.08)' } : null) }}>
      <span style={S.rowK}>{k}</span>
      <span style={S.rowV}>{v}</span>
    </div>
  );
}

const S = {
  wrap: {
    position: 'fixed', right: 16, bottom: 16, width: 360, maxWidth: 'calc(100vw - 32px)',
    maxHeight: 'calc(100vh - 32px)', overflow: 'hidden',
    zIndex: 9999, background: '#0f172a', border: '1px solid #334155',
    borderRadius: 10, boxShadow: '0 24px 48px rgba(0,0,0,0.6)',
    color: '#e2e8f0', fontFamily: 'system-ui, -apple-system, sans-serif',
    display: 'flex', flexDirection: 'column',
  },
  header: {
    padding: '10px 12px', background: '#1e293b', borderBottom: '1px solid #334155',
    display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', userSelect: 'none',
  },
  modeDot: (color) => ({
    width: 8, height: 8, borderRadius: '50%', background: color,
    boxShadow: `0 0 8px ${color}`,
  }),
  body: { padding: '10px 12px', overflowY: 'auto', flex: 1 },
  section: { marginBottom: 12 },
  sectionTitle: {
    fontSize: 10, color: '#94a3b8', textTransform: 'uppercase', letterSpacing: 0.6,
    marginBottom: 4, fontWeight: 700,
  },
  row: { display: 'flex', justifyContent: 'space-between', gap: 8, padding: '2px 4px', fontSize: 12, borderRadius: 3 },
  rowK: { color: '#94a3b8', flexShrink: 0 },
  rowV: { color: '#e2e8f0', textAlign: 'right', wordBreak: 'break-all' },
  empty: { color: '#64748b', fontSize: 11, fontStyle: 'italic' },
  note: { fontSize: 10, color: '#fbbf24', marginTop: 4, fontStyle: 'italic' },
  summary: { fontSize: 11, color: '#94a3b8', cursor: 'pointer' },
  histRow: {
    display: 'grid', gridTemplateColumns: 'auto auto 1fr auto auto',
    gap: 6, padding: '2px 0', borderBottom: '1px dashed #1f2937',
  },
  kindBadge: (kind) => {
    const palette = {
      dial: '#fca5a5',
      'sim-web-call': '#86efac',
      'campaign-dial': '#fcd34d',
      'campaign-sim': '#a5b4fc',
    };
    return {
      padding: '1px 5px', borderRadius: 3, fontSize: 10, fontWeight: 700,
      color: palette[kind] || '#cbd5e1',
      background: 'rgba(255,255,255,0.04)', border: '1px solid #334155',
      whiteSpace: 'nowrap',
    };
  },
  eventRow: {
    display: 'grid', gridTemplateColumns: 'auto auto 1fr', gap: 6,
    fontSize: 11, padding: '2px 0', borderBottom: '1px dashed #1f2937',
  },
  copyBtn: {
    padding: '6px 10px', borderRadius: 4, cursor: 'pointer',
    background: 'rgba(99,102,241,0.15)', border: '1px solid rgba(99,102,241,0.4)',
    color: '#a5b4fc', fontSize: 11, fontWeight: 600, width: '100%',
  },
};
