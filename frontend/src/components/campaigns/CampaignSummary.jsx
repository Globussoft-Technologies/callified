import './CampaignSummary.css';

export default function CampaignSummary({ stats, outcomes }) {
  const percent = stats.total > 0 ? Math.min(100, Math.round(stats.called / stats.total * 100)) : 0;
  const leads = [
    ['Total leads', stats.total, 'accent'], ['Called', stats.called, ''],
    ['Remaining', stats.remaining, 'amber'], ['Qualified', stats.qualified, 'green'],
    ['Appointments', stats.booked, 'cyan'],
  ];
  const attempts = [
    ['Total', outcomes.total, 'accent'], ['Connected', outcomes.connected, 'green'],
    ['Completed', outcomes.completed, 'cyan'], ['Unanswered', outcomes.unanswered, 'amber'],
    ['Busy / Failed', (outcomes.busy || 0) + (outcomes.failed || 0), 'red'],
  ];
  return <section className="campaign-detail-summary" aria-label="Campaign summary">
    {[['Lead progress', leads], ['Call attempts', attempts]].map(([heading, metrics], index) => <div className="campaign-summary-row" key={heading}>
      <h3>{heading}</h3>
      <dl>{metrics.map(([label, value, tone]) => <div key={label}><dt>{label}</dt><dd className={tone}>{value ?? 0}</dd></div>)}</dl>
      {index === 0 && <div className="campaign-summary-completion"><div className="campaign-summary-track" role="progressbar" aria-label="Unique leads attempted" aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100}><span style={{ width: `${percent}%` }} /></div><strong>{stats.total > 0 ? `${percent}%` : 'No leads'}</strong></div>}
    </div>)}
  </section>;
}
