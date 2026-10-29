import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Spin } from 'antd';
import { getJob } from '@colony2/shared/api';
import type { RecipeJob, RecipeJobSummary } from '@colony2/shared/types';
import JobListDetails from './JobListDetails';

// Mounted only for a visible selection, keyed by tenant and job by the parent.
// List responses never enter this state. Reopening remounts and revalidates.
export default function SelectedJobDetails({ projectId, jobId, summary, refresh, prompt, navigationState }: {
  projectId: string; jobId: string; summary: RecipeJobSummary; refresh: number; prompt?: string; navigationState: Record<string, unknown>;
}) {
  const [job, setJob] = useState<RecipeJob>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [retry, setRetry] = useState(0);
  const request = useRef<{ key: string; promise: ReturnType<typeof getJob> }>();
  useEffect(() => {
    let active = true;
    setLoading(true); setError(undefined);
    const key = JSON.stringify([projectId, jobId, refresh, retry]);
    // React StrictMode replays effects; reuse the same request for that replay.
    if (request.current?.key !== key) request.current = { key, promise: getJob(projectId, jobId) };
    request.current.promise.then(result => {
      if (active) setJob(result);
    }).catch(reason => {
      if (active) setError(reason instanceof Error ? reason.message : 'Could not load job details');
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [projectId, jobId, refresh, retry]);
  return <>
    {error && <Alert type="error" showIcon message="Could not load job details" description={error}
      action={<Button onClick={() => setRetry(value => value + 1)}>Retry details</Button>} />}
    {loading && <Spin size="small" aria-label="Loading dependency and membership details" />}
    <JobListDetails job={summary} inspection={job} projectId={projectId} showPrompt={!!summary.prompt && summary.prompt !== prompt} navigationState={navigationState} />
  </>;
}
