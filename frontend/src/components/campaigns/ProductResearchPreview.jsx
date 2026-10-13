import AppIcon from '../common/AppIcon';

const stages = { queued: 'Queued', fetching: 'Reading website', summarizing: 'Preparing summary', ready: 'Ready for review', applied: 'Applied', failed: 'Research failed', 'poll-error': 'Connection interrupted' };

export default function ProductResearchPreview({ data, productName, canApply = true, onApply, onRetry, onRefresh }) {
  if (!data?.job) return null;
  const { job } = data;
  const running = ['queued', 'fetching', 'summarizing'].includes(job.status);
  return <section className="product-research-preview" aria-label={`Website research for ${productName}`}>
    <header><strong>Website research</strong><span role="status">{running && <AppIcon name="loading" spin />} {stages[job.status] || job.status}</span></header>
    {data.sources?.length > 0 && <details><summary>{data.sources.length} source pages</summary><ul>{data.sources.map(url => <li key={url}><a href={url} target="_blank" rel="noopener noreferrer">{url}</a></li>)}</ul></details>}
    {data.warnings?.length > 0 && <details><summary>Some pages were unavailable</summary><ul>{data.warnings.map((warning, index) => <li key={index}>{warning}</li>)}</ul></details>}
    {job.status === 'ready' && <>{!canApply && <p role="status">Website URL changed. Refresh website before applying.</p>}<label htmlFor={`research-${job.id}`}>Research preview</label><textarea id={`research-${job.id}`} readOnly value={job.summary} /><div className="product-research-actions"><button onClick={onRefresh} disabled={data.applying}>Refresh website</button><button className="product-save-button" onClick={onApply} disabled={data.applying || !canApply}><AppIcon name={data.applying ? 'loading' : 'checkCircle'} spin={data.applying} /> {data.applying ? 'Applying...' : 'Apply approved research'}</button></div></>}
    {['failed', 'poll-error'].includes(job.status) && <button onClick={onRetry}>Retry research</button>}
  </section>;
}
