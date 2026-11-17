export function reportFilename({ name, role, from, to, campaign, format }) {
  const clean = value => String(value || '').normalize('NFC')
    .replace(/[^\p{L}\p{N}\p{M}@._-]+/gu, '_')
    .replace(/^[._-]+|[._-]+$/g, '').slice(0, 80);
  const dates = from && to ? (from === to ? from : `${from}_to_${to}`) : 'All_Dates';
  return [name || 'Performance', role, 'Report', campaign, dates]
    .filter(Boolean).map(clean).filter(Boolean).join('_') + (format === 'pdf' ? '.pdf' : '.xlsx');
}
