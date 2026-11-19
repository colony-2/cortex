import { Link, useLocation, useSearchParams } from 'react-router-dom';
import type { RecipeJobSummary } from '@colony2/shared/types';
import { requirementSummary } from '../utils/executionRequirements';
import { jobFilterSearch } from '../utils/jobFilters';
import './JobEnvironment.css';

export default function JobExecutionRequirements({ job, projectId, navigationState }: {
  job: RecipeJobSummary; projectId: string; navigationState?: Record<string, unknown>;
}) {
  const [params] = useSearchParams();
  const location = useLocation();
  const summary = requirementSummary(job);
  if (!summary) return null;
  const query = new URLSearchParams(jobFilterSearch(params));
  query.set('tab', 'env');
  return <Link className="job-requirements-summary" to={`/project/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(job.job_id)}/story?${query}`}
    state={{ ...navigationState, goalsReturnTo: navigationState?.goalsReturnTo || location.pathname + location.search }}>
    {summary}
  </Link>;
}
