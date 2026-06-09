import React from 'react';

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

const DEPT_COLORS = {
  Sales:     { bg: 'rgba(99,102,241,0.1)',  color: T.accent },
  Analytics: { bg: 'rgba(6,182,212,0.1)',   color: '#0891b2' },
  Ops:       { bg: 'rgba(16,185,129,0.1)',  color: T.green  },
  Finance:   { bg: 'rgba(245,158,11,0.1)',  color: T.amber  },
  HR:        { bg: 'rgba(236,72,153,0.1)',  color: '#db2777' },
};

function DeptBadge({ dept }) {
  const c = DEPT_COLORS[dept] || { bg: 'rgba(99,102,241,0.1)', color: T.accent };
  return (
    <span style={{ fontSize: 11, fontWeight: 600, padding: '3px 10px', borderRadius: 20, background: c.bg, color: c.color }}>
      {dept}
    </span>
  );
}

function StatusBadge({ status, onDone, taskId }) {
  const done = status === 'Complete';
  if (done) {
    return (
      <span style={{ fontSize: 11, fontWeight: 600, padding: '3px 10px', borderRadius: 20, background: 'rgba(16,185,129,0.1)', color: T.green }}>
        done
      </span>
    );
  }
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <span style={{ fontSize: 11, fontWeight: 600, padding: '3px 10px', borderRadius: 20, background: 'rgba(245,158,11,0.1)', color: T.amber }}>
        pending
      </span>
      <button onClick={() => onDone(taskId)} style={{
        fontSize: 11, fontWeight: 600, padding: '3px 10px', borderRadius: 20,
        background: T.accent, color: '#fff', border: 'none', cursor: 'pointer', fontFamily: T.font,
      }}>
        Mark Done
      </button>
    </div>
  );
}

export default function OpsTab({ reports, tasks, handleCompleteTask }) {
  const metrics = [
    { label: 'CLOSED DEALS',     value: reports?.closed_deals           ?? 0, color: T.green },
    { label: 'VERIFIED PUNCHES', value: reports?.valid_site_punches     ?? 0, color: T.text  },
    { label: 'PENDING TASKS',    value: reports?.pending_internal_tasks ?? 0, color: T.amber },
  ];

  const thStyle = {
    fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase',
    letterSpacing: '0.07em', padding: '0 0 12px', textAlign: 'left', borderBottom: `1px solid ${T.border}`,
  };
  const tdStyle = {
    fontSize: 13, color: T.sub, padding: '14px 0', borderBottom: `1px solid ${T.border}`, verticalAlign: 'middle',
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title */}
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>Ops &amp; Tasks</h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Internal cross-department workflows triggered by CRM events.
        </p>
      </div>

      {/* Metric cards */}
      {reports && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 12, marginBottom: 24 }}>
          {metrics.map(m => (
            <div key={m.label} style={{ ...card, padding: '22px 28px' }}>
              <div style={{ fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 12 }}>
                {m.label}
              </div>
              <div style={{ fontSize: 36, fontWeight: 700, fontFamily: T.font, color: m.color, lineHeight: 1 }}>
                {m.value}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tasks table */}
      <div style={{ ...card, padding: '24px 28px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
          <h3 style={{ margin: 0, fontSize: 15, fontWeight: 700, color: T.text }}>
            Internal Cross-Department Tasks
          </h3>
        </div>

        {tasks.length === 0 ? (
          <p style={{ color: T.muted, textAlign: 'center', padding: '2.5rem 0', margin: 0, fontSize: 14 }}>
            No internal workflows active. Try closing a lead in CRM!
          </p>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                <th style={{ ...thStyle, width: '38%' }}>Task</th>
                <th style={thStyle}>Department</th>
                <th style={thStyle}>Client</th>
                <th style={{ ...thStyle, textAlign: 'right' }}>Status</th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((t, i) => {
                const isLast = i === tasks.length - 1;
                const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                return (
                  <tr key={t.id}>
                    <td style={{ ...rowTd, fontWeight: 600, color: T.text, paddingRight: 16 }}>
                      {t.description}
                    </td>
                    <td style={{ ...rowTd, paddingRight: 16 }}>
                      <DeptBadge dept={t.department} />
                    </td>
                    <td style={{ ...rowTd, color: T.muted, paddingRight: 16 }}>
                      {t.first_name} {t.last_name}
                    </td>
                    <td style={{ ...rowTd, textAlign: 'right' }}>
                      <StatusBadge status={t.status} onDone={handleCompleteTask} taskId={t.id} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
