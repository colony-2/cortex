import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Space, Tag, Typography } from 'antd';
import { Link } from 'react-router-dom';
import { listJobChildren } from '@colony2/shared/api';
import type { RecipeJob, RecipeJobSummary } from '@colony2/shared/types';
import { goalPath, jobPath } from './JobWaitLinks';
import { cellLabel } from '../utils/cellPresentation';
import { jobStatus } from '../utils/jobStatus';

function useJobChildren(projectId: string, jobId: string) {
  const [state, setState] = useState<{ rows: RecipeJobSummary[]; token?: string; loading: boolean; error: boolean }>({ rows: [], loading: true, error: false });
  const generation = useRef(0);
  const busy = useRef(false);
  const load = (reset = false) => {
    if (busy.current && !reset) return;
    const request = reset ? ++generation.current : generation.current;
    const token = reset ? undefined : state.token;
    busy.current = true;
    setState(previous => ({ ...(reset ? { rows: [], token: undefined } : previous), loading: true, error: false }));
    void listJobChildren(projectId, jobId, { pageSize: 20, pageToken: token }).then(page => {
      if (request !== generation.current) return;
      setState(previous => {
        const rows = [...previous.rows];
        const seen = new Set(rows.map(row => JSON.stringify([row.tenant_id, row.job_id])));
        for (const row of page.jobs) {
          const key = JSON.stringify([row.tenant_id, row.job_id]);
          if (!seen.has(key)) { rows.push(row); seen.add(key); }
        }
        return { rows, token: page.next_page_token, loading: false, error: false };
      });
    }).catch(() => {
      if (request === generation.current) setState(previous => ({ ...previous, loading: false, error: true }));
    }).finally(() => { if (request === generation.current) busy.current = false; });
  };
  useEffect(() => {
    load(true);
    return () => { ++generation.current; busy.current = false; };
    // Identity is supplied by the keyed page-local component below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, jobId]);
  return { ...state, load };
}

function Relationships({ projectId, jobId, job }: { projectId: string; jobId: string; job: RecipeJob }) {
  const { rows, token, loading, error, load } = useJobChildren(projectId, jobId);
  const parent = job.parent;
  const parentTenant = parent?.tenant_id || projectId;
  return <section aria-label="Related jobs and goals" className="job-relationships">
    <details className="relationship-disclosure">
      <summary>Related jobs and goals</summary>
      <Card size="small" variant="borderless">
        <Button disabled={loading} onClick={() => load(true)}>Refresh relationships</Button>
        <div className="relationship-group">
          <Typography.Text type="secondary">Parent</Typography.Text>
          <Space wrap>
            {parent?.job_id && <Link className="compact-touch-link" to={jobPath(parentTenant, parent.job_id)}>Job · {parent.job_id}</Link>}
            {parent?.goal_id && <Link className="compact-touch-link" to={goalPath(parentTenant, parent.goal_id)}>Goal · {parent.goal_id}</Link>}
            {!parent?.job_id && !parent?.goal_id && <Typography.Text type="secondary">No parent recorded</Typography.Text>}
          </Space>
        </div>
        <div className="relationship-group">
          <Typography.Text type="secondary">Child jobs in this tenant</Typography.Text>
          <div>
            <ul className="relationship-children">
              {rows.map(child => {
                const status = jobStatus(child);
                return <li key={JSON.stringify([child.tenant_id, child.job_id])}>
                  <div><Link className="compact-touch-link" aria-label={`Job · ${child.title?.trim() || child.job_id}`} to={jobPath(projectId, child.job_id)}>{child.title?.trim() || child.job_id}</Link>
                    <div className="relationship-identity">{child.job_id} · {cellLabel(child.cell_name)}</div></div>
                  <Space wrap><Tag color={status.color}>{status.label}</Tag>
                    {child.goal_id ? <Link className="compact-touch-link" to={goalPath(projectId, child.goal_id)}>Goal · {child.goal_id}</Link> : <Typography.Text type="secondary">No goal recorded</Typography.Text>}
                  </Space>
                </li>;
              })}
            </ul>
            {!rows.length && !loading && !error && !token && <Typography.Text type="secondary">No child jobs recorded in this tenant</Typography.Text>}
            {loading && <div role="status">Loading child jobs...</div>}
            {error && <Alert type="warning" message="Child jobs unavailable" action={<Button onClick={() => load()}>Retry</Button>} />}
            {token && !error && <Button disabled={loading} onClick={() => load()}>Load more child jobs</Button>}
            {!!rows.length && <div className="relationship-identity">{rows.length} child jobs shown{token ? ' · More available' : ''}</div>}
          </div>
        </div>
      </Card>
    </details>
  </section>;
}

// Remount on identity change so no obsolete rows appear even before effects run.
export default function JobRelationships(props: { projectId: string; jobId: string; job: RecipeJob }) {
  return <Relationships key={JSON.stringify([props.projectId, props.jobId])} {...props} />;
}
