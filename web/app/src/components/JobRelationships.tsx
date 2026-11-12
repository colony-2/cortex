import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Space, Tag, Typography } from 'antd';
import { Link, useLocation } from 'react-router-dom';
import { listJobChildren, listJobs } from '@colony2/shared/api';
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

function useSiblings(projectId: string, jobId: string, goal?: string) {
  const [state, setState] = useState<{ rows: RecipeJobSummary[]; token?: string; loading: boolean; error: boolean }>({ rows: [], loading: !!goal, error: false });
  const generation = useRef(0);
  const busy = useRef(false);
  const load = (reset = false) => {
    if (!goal || (busy.current && !reset)) return;
    const request = reset ? ++generation.current : generation.current;
    const token = reset ? undefined : state.token;
    busy.current = true;
    setState(previous => ({ ...(reset ? { rows: [], token: undefined } : previous), loading: true, error: false }));
    void listJobs(projectId, { goal, status: [], includeHidden: true, pageSize: 20, pageToken: token }).then(page => {
      if (request !== generation.current) return;
      setState(previous => {
        const rows = [...previous.rows];
        const identity = (row: RecipeJobSummary) => JSON.stringify([row.tenant_id || projectId, row.job_id]);
        const seen = new Set([JSON.stringify([projectId, jobId]), ...rows.map(identity)]);
        for (const row of page.jobs) if (!seen.has(identity(row))) { rows.push(row); seen.add(identity(row)); }
        return { rows, token: page.next_page_token, loading: false, error: false };
      });
    }).catch(() => {
      if (request === generation.current) setState(previous => ({ ...previous, loading: false, error: true }));
    }).finally(() => { if (request === generation.current) busy.current = false; });
  };
  useEffect(() => { load(true); return () => { ++generation.current; busy.current = false; }; }, [projectId, jobId, goal]);
  return { ...state, load };
}

function Relationships({ projectId, jobId, job, currentGoal, showAttempts = false }: { projectId: string; jobId: string; job: RecipeJob; currentGoal?: string; showAttempts?: boolean }) {
  const { rows, token, loading, error, load } = useJobChildren(projectId, jobId);
  const siblings = useSiblings(projectId, jobId, showAttempts ? currentGoal : undefined);
  const location = useLocation();
  const parent = job.parent;
  const parentTenant = parent?.tenant_id || projectId;
  return <section aria-label="Related jobs and goals" className="job-relationships">
    <div>
      <h2>Related jobs and goals</h2>
      <Card size="small" variant="borderless">
        <Button disabled={loading || siblings.loading} onClick={() => { load(true); siblings.load(true); }}>Refresh relationships</Button>
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
        {showAttempts && <section className="relationship-group" aria-label="Other attempts at this goal">
          <h3>Other attempts at this goal</h3>
          <div>{!currentGoal ? <p>No goal recorded</p> : <>
            <Link className="compact-touch-link" to={goalPath(projectId, currentGoal)}>Goal · {currentGoal}</Link>
            <ul className="relationship-children">
              {siblings.rows.map(attempt => {
                const status = jobStatus(attempt);
                return <li key={JSON.stringify([attempt.tenant_id || projectId, attempt.job_id])}>
                  <div><Link className="compact-touch-link" to={jobPath(attempt.tenant_id || projectId, attempt.job_id)}
                    state={{ ...location.state, goalId: currentGoal }}>{attempt.title?.trim() || attempt.job_id}</Link>
                    <div className="relationship-identity">{attempt.job_id} · {cellLabel(attempt.cell_name)}</div></div>
                  <Tag color={status.color} style={{ alignSelf: 'center' }}>{status.label}</Tag>
                </li>;
              })}
            </ul>
            {siblings.loading && <p role="status">Loading other attempts…</p>}
            {siblings.error && <Alert type="warning" message="Other attempts unavailable" action={<Button onClick={() => siblings.load()}>Retry other attempts</Button>} />}
            {!siblings.rows.length && !siblings.loading && !siblings.error && !siblings.token && <p>No other attempts at this goal</p>}
            {siblings.token && !siblings.error && <Button disabled={siblings.loading} onClick={() => siblings.load()}>Load more attempts</Button>}
          </>}</div>
        </section>}
      </Card>
    </div>
  </section>;
}

// Remount on identity change so no obsolete rows appear even before effects run.
export default function JobRelationships(props: { projectId: string; jobId: string; job: RecipeJob; currentGoal?: string; showAttempts?: boolean }) {
  return <Relationships key={JSON.stringify([props.projectId, props.jobId, props.currentGoal])} {...props} />;
}
