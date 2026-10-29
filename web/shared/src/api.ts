import type {
  CortexCell,
  ListRecipeJobsResponse,
  Project,
  RecipeJob,
  RecipeJobStatus,
  RecipeJobFilterStatus,
  SubmitRecipeJobRequest,
  RestartContext,
} from './types';

import { API_BASE } from './apiBase';
import { parseJobJSON } from './jobJSON';

async function handleResponse(response: Response, operation: string) {
  if (!response.ok) {
    const errorText = await response.text().catch(() => 'Unknown error');
    throw new Error(`${operation} failed: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
  }
  return response;
}

export async function listProjects(tenantId?: string): Promise<Project[]> {
  const query = tenantId ? `?tenantId=${encodeURIComponent(tenantId)}` : '';
  const response = await fetch(`${API_BASE}/projects${query}`);
  await handleResponse(response, 'List projects');
  return response.json();
}

export async function listCells(projectId: string): Promise<CortexCell[]> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/cells`);
  await handleResponse(response, 'List cells');
  return response.json();
}

export async function listJobs(
  projectId: string,
  options: {
    status?: RecipeJobStatus[];
    displayStatus?: RecipeJobFilterStatus[];
    cell?: string;
    pageSize?: number;
    pageToken?: string;
    goal?: string;
  } = {},
): Promise<ListRecipeJobsResponse> {
  const params = new URLSearchParams();
  // An explicitly cleared filter means all statuses; omitted options retain API defaults.
  if (options.status?.length === 0 || options.displayStatus?.length === 0) params.set('status', 'all');
  for (const status of options.status || []) {
    params.append('status', status);
  }
  for (const status of options.displayStatus || []) params.append('displayStatus', status);
  if (options.cell) params.set('cell', options.cell);
  if (options.goal) params.set('goal', options.goal);
  if (options.pageSize) params.set('pageSize', String(options.pageSize));
  if (options.pageToken) params.set('pageToken', options.pageToken);
  const query = params.toString();
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs${query ? `?${query}` : ''}`,
  );
  await handleResponse(response, 'List jobs');
  return parseJobJSON(await response.text()) as ListRecipeJobsResponse;
}

export async function getJob(projectId: string, jobId: string): Promise<RecipeJob> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}`,
  );
  await handleResponse(response, 'Get job');
  return parseJobJSON(await response.text()) as RecipeJob;
}

export async function getRestartContext(projectId: string, jobId: string, boundary: number | string, signal?: AbortSignal): Promise<RestartContext> {
  const selector = typeof boundary === 'string' ? `scope_id=${encodeURIComponent(boundary)}` : `chapter_ordinal=${boundary}`;
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/restart-context?${selector}`,
    { signal },
  );
  await handleResponse(response, 'Inspect restart context');
  const context: RestartContext = await response.json();
  if (context.project_id !== projectId || context.job_id !== jobId || (typeof boundary === 'string' ? context.scope_id !== boundary : context.chapter_ordinal !== boundary)) {
    throw new Error('Restart context does not match the selected job and restart point.');
  }
  return context;
}

export async function cancelJob(projectId: string, jobId: string): Promise<void> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/cancel`,
    { method: 'POST' },
  );
  await handleResponse(response, 'Cancel job');
}

export async function deleteJob(projectId: string, jobId: string): Promise<void> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}`,
    { method: 'DELETE' },
  );
  await handleResponse(response, 'Delete job');
}

export async function submitJob(projectId: string, input: SubmitRecipeJobRequest): Promise<RecipeJob> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  await handleResponse(response, 'Submit job');
  return parseJobJSON(await response.text()) as RecipeJob;
}

/** Keep export bytes opaque: JSON parsing here can round persisted 64-bit integers. */
export async function getJobChaptersDownload(projectId: string, jobId: string, signal?: AbortSignal): Promise<Blob> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/chapters/download`,
    { signal },
  );
  await handleResponse(response, 'Download chapters');
  return response.blob();
}
