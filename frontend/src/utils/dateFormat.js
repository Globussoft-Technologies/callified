// MySQL TIMESTAMP columns store UTC (Docker container runs SYSTEM = UTC).
// Naive strings from the DB have no timezone suffix — append 'Z' so the
// JS Date constructor interprets them as UTC, then orgTimezone converts to
// the correct local display time.
function parseDbDate(dateStr) {
  if (!dateStr) return null;
  const normalized = dateStr.includes('T') ? dateStr : dateStr.replace(' ', 'T');
  // Already has a timezone designator — use as-is
  if (/Z$|[+-]\d{2}:\d{2}$/.test(normalized)) return new Date(normalized);
  // Naive string → treat as UTC
  return new Date(normalized + 'Z');
}

export function formatDateTime(dateStr, timezone, opts = {}) {
  if (!dateStr) return '—';
  const date = parseDbDate(dateStr);
  if (!date || isNaN(date.getTime())) return dateStr;
  const options = {
    year: 'numeric', month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit',
    hour12: true,
    ...opts,
    ...(timezone ? { timeZone: timezone } : {})
  };
  return date.toLocaleString(undefined, options);
}

export function formatDate(dateStr, timezone) {
  if (!dateStr) return '—';
  const date = parseDbDate(dateStr);
  if (!date || isNaN(date.getTime())) return dateStr;
  const options = {
    year: 'numeric', month: 'short', day: 'numeric',
    ...(timezone ? { timeZone: timezone } : {})
  };
  return date.toLocaleDateString(undefined, options);
}

export function formatTime(dateStr, timezone) {
  if (!dateStr) return '—';
  const date = parseDbDate(dateStr);
  if (!date || isNaN(date.getTime())) return dateStr;
  const options = {
    hour: '2-digit', minute: '2-digit', hour12: true,
    ...(timezone ? { timeZone: timezone } : {})
  };
  return date.toLocaleTimeString(undefined, options);
}
