import React, { useState, useCallback } from 'react';
import { useToast } from '../contexts/UIContext';
import { formatDateTime } from '../utils/dateFormat';

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

const inputStyle = {
  padding: '10px 14px', borderRadius: 8, fontSize: 13,
  border: `1px solid ${T.border}`, background: '#fff', color: T.text,
  fontFamily: T.font, outline: 'none', width: '100%', boxSizing: 'border-box',
};

const btnPrimary = {
  background: T.accent, color: '#fff', border: 'none',
  borderRadius: 8, padding: '8px 18px', fontWeight: 600,
  cursor: 'pointer', fontFamily: T.font, fontSize: '0.9rem',
};

const typeMeta = {
  lead_created: { icon: '👤', color: '#6366f1', bg: 'rgba(99,102,241,0.08)', label: 'Lead' },
  note: { icon: '📝', color: '#f59e0b', bg: 'rgba(245,158,11,0.08)', label: 'Note' },
  call: { icon: '📞', color: '#10b981', bg: 'rgba(16,185,129,0.08)', label: 'Call' },
  scheduled_call: { icon: '📅', color: '#0891b2', bg: 'rgba(8,145,178,0.08)', label: 'Scheduled' },
  whatsapp: { icon: '💬', color: '#25D366', bg: 'rgba(37,211,102,0.08)', label: 'WhatsApp' },
  status_change: { icon: '🔖', color: '#64748b', bg: 'rgba(100,116,139,0.08)', label: 'Status' },
};

function maskPhone(phone) {
  if (!phone) return '-';
  const digits = String(phone).replace(/\D/g, '');
  if (digits.length <= 5) return digits;
  return digits.slice(0, 5) + 'X'.repeat(digits.length - 5);
}

export default function InteractionHistoryPage({ apiFetch, API_URL, orgTimezone }) {
  const toast = useToast();
  const [query, setQuery] = useState('');
  const [searching, setSearching] = useState(false);
  const [searchResults, setSearchResults] = useState([]);
  const [selectedLead, setSelectedLead] = useState(null);
  const [timeline, setTimeline] = useState(null);
  const [loading, setLoading] = useState(false);

  const searchLeads = useCallback(async (q = query) => {
    const trimmed = q.trim();
    if (!trimmed) {
      setSearchResults([]);
      return;
    }
    setSearching(true);
    try {
      const res = await apiFetch(`${API_URL}/leads/search?q=${encodeURIComponent(trimmed)}`);
      if (!res.ok) throw new Error('Search failed');
      const data = await res.json();
      setSearchResults(Array.isArray(data) ? data : []);
    } catch (e) {
      console.error('Lead search failed', e);
      setSearchResults([]);
    }
    setSearching(false);
  }, [apiFetch, API_URL, query]);

  const fetchTimeline = useCallback(async (leadId) => {
    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/leads/${leadId}/interactions`);
      if (!res.ok) throw new Error('Failed to load interactions');
      setTimeline(await res.json());
    } catch (e) {
      toast('Failed to load interaction history');
      console.error(e);
      setTimeline(null);
    }
    setLoading(false);
  }, [apiFetch, API_URL, toast]);

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') searchLeads();
  };

  const selectLead = (lead) => {
    setSelectedLead(lead);
    setSearchResults([]);
    setQuery(`${lead.first_name || ''} ${lead.last_name || ''} ${lead.phone}`.trim());
    fetchTimeline(lead.id);
  };

  return (
    <div style={{ padding: '24px', maxWidth: 1100, margin: '0 auto', fontFamily: T.font }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ margin: 0, fontSize: 24, fontWeight: 700, color: T.text }}>🕘 Interaction History</h1>
        <p style={{ margin: '6px 0 0', color: T.muted, fontSize: '0.9rem' }}>
          Search any customer to see their complete timeline: calls, notes, scheduled callbacks, and WhatsApp messages.
        </p>
      </div>

      <div style={{ ...card, padding: '1.25rem', marginBottom: 20 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
          <div style={{ position: 'relative', flex: 1, minWidth: 260 }}>
            <input
              type="text"
              value={query}
              onChange={e => { setQuery(e.target.value); if (!e.target.value.trim()) { setSearchResults([]); } }}
              onKeyDown={handleKeyDown}
              placeholder="Search by name, phone or company..."
              style={inputStyle}
            />
            {searchResults.length > 0 && (
              <div style={{
                position: 'absolute', top: 'calc(100% + 6px)', left: 0, right: 0,
                background: '#fff', border: `1px solid ${T.border}`, borderRadius: 10,
                boxShadow: '0 8px 24px rgba(0,0,0,0.08)', zIndex: 10, maxHeight: 300, overflow: 'auto',
              }}>
                {searchResults.map(lead => (
                  <button
                    key={lead.id}
                    onClick={() => selectLead(lead)}
                    style={{
                      width: '100%', textAlign: 'left', padding: '10px 14px', border: 'none',
                      borderBottom: `1px solid ${T.border}`, background: '#fff', cursor: 'pointer',
                      fontFamily: T.font, fontSize: 13,
                    }}
                  >
                    <div style={{ fontWeight: 600, color: T.text }}>{lead.first_name} {lead.last_name}</div>
                    <div style={{ fontFamily: T.mono, color: T.muted, fontSize: 12 }}>{maskPhone(lead.phone)}{lead.company ? ` · ${lead.company}` : ''}</div>
                  </button>
                ))}
              </div>
            )}
          </div>
          <button onClick={() => searchLeads()} disabled={searching} style={{ ...btnPrimary, opacity: searching ? 0.6 : 1 }}>
            {searching ? 'Searching…' : 'Search'}
          </button>
        </div>
      </div>

      {loading && (
        <div style={{ ...card, padding: '2rem', textAlign: 'center', color: T.muted }}>
          Loading interaction history…
        </div>
      )}

      {!loading && timeline && selectedLead && (
        <div style={{ ...card, padding: '1.5rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12, marginBottom: '1.25rem', paddingBottom: '1.25rem', borderBottom: `1px solid ${T.border}` }}>
            <div>
              <h2 style={{ margin: 0, fontSize: 20, fontWeight: 700, color: T.text }}>
                {timeline.lead.first_name} {timeline.lead.last_name}
              </h2>
              <div style={{ fontFamily: T.mono, color: T.muted, fontSize: 13, marginTop: 4 }}>
                {maskPhone(timeline.lead.phone)}{timeline.lead.company ? ` · ${timeline.lead.company}` : ''} · {timeline.lead.source || 'No source'} · {timeline.lead.status || 'New'}
              </div>
            </div>
            <div style={{ fontSize: 13, color: T.muted }}>
              {timeline.event_count} interaction{timeline.event_count === 1 ? '' : 's'}
            </div>
          </div>

          {timeline.events.length === 0 ? (
            <div style={{ color: T.muted, textAlign: 'center', padding: '2rem 0' }}>No interactions found for this customer.</div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              {timeline.events.map((event, idx) => {
                const meta = typeMeta[event.type] || typeMeta.status_change;
                const isLast = idx === timeline.events.length - 1;
                return (
                  <div key={`${event.type}-${event.id || idx}`} style={{ display: 'flex', gap: 14 }}>
                    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
                      <div style={{
                        width: 36, height: 36, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center',
                        background: meta.bg, color: meta.color, fontSize: 16,
                      }}>
                        {meta.icon}
                      </div>
                      {!isLast && <div style={{ width: 2, flex: 1, background: T.border, marginTop: 8 }} />}
                    </div>
                    <div style={{ flex: 1, paddingBottom: isLast ? 0 : 16 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
                        <span style={{ fontSize: 12, fontWeight: 700, color: meta.color, textTransform: 'uppercase', letterSpacing: 0.5 }}>{meta.label}</span>
                        <span style={{ fontSize: 12, color: T.muted }}>{formatDateTime(event.timestamp, orgTimezone)}</span>
                      </div>
                      <div style={{ fontSize: 15, fontWeight: 600, color: T.text, marginTop: 4 }}>{event.title}</div>
                      {event.body && <div style={{ fontSize: 13, color: T.sub, marginTop: 4, whiteSpace: 'pre-wrap' }}>{event.body}</div>}
                      {event.type === 'call' && event.metadata?.recording_url && (
                        <div style={{ marginTop: 8 }}>
                          <audio controls src={event.metadata.recording_url} style={{ height: 32, width: '100%', maxWidth: 360 }} />
                        </div>
                      )}
                      {event.type === 'call' && (
                        <div style={{ fontSize: 12, color: T.muted, marginTop: 4 }}>
                          Duration: {Number(event.metadata?.duration_s || 0).toFixed(1)}s · Outcome: {event.metadata?.outcome}
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {!loading && !timeline && !selectedLead && query.trim() && !searchResults.length && (
        <div style={{ ...card, padding: '1.25rem', textAlign: 'center', color: T.muted }}>
          Search for a customer above to view their interaction history.
        </div>
      )}
    </div>
  );
}
