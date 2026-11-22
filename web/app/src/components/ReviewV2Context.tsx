import { useEffect } from 'react';
import { Button } from 'antd';
import { Link } from 'react-router-dom';
import type { RecipeJob } from '@colony2/shared';
import { useReviewAncestors } from '../hooks/useReviewAncestors';
import { useReviewJobMetadata } from '../hooks/useReviewJobMetadata';
import { headline } from '../executionTree/presentation';
import { jobPath } from './JobWaitLinks';

export default function ReviewV2Context({tenant, job, metadata, onMetadata}: {tenant: string; job: string; metadata?: RecipeJob; onMetadata?: (value: RecipeJob) => void}) {
  const branch = useReviewAncestors(tenant, job);
  const fallback = useReviewJobMetadata(tenant, metadata ? [] : [job]);
  const current = metadata ?? fallback.metadata.get(job)?.job;
  useEffect(() => { if (current) onMetadata?.(current); }, [current, onMetadata]);
  return <section className="review-v2-context" aria-label="Review context">
    <div>Cell: <strong>{current?.cell_name || 'Cell unavailable'}</strong>
      {!current && <Button size="small" onClick={fallback.refresh}>Retry cell context</Button>}</div>
    <nav aria-label="Job ancestors"><ol>{branch.nodes.map(node => <li key={node.job_id}>
      <Link to={jobPath(tenant, node.job_id)} aria-current={node.job_id === job ? 'page' : undefined}>{headline(node)}</Link>
    </li>)}</ol></nav>
    {branch.loading && <span role="status">Loading ancestors…</span>}
    {branch.error && <div role="status">Ancestor branch incomplete. {branch.error} <Button size="small" onClick={branch.retry}>Retry ancestors</Button></div>}
  </section>;
}
