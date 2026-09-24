import type {
  CortexCell,
  ListRecipeJobsResponse,
  Project,
  RecipeJob,
  RecipeJobStatus,
  SubmitRecipeJobRequest,
} from './types';

import { API_BASE } from './apiBase';

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
    cell?: string;
    pageSize?: number;
    pageToken?: string;
  } = {},
): Promise<ListRecipeJobsResponse> {
  const params = new URLSearchParams();
  for (const status of options.status || []) {
    params.append('status', status);
  }
  if (options.cell) params.set('cell', options.cell);
  if (options.pageSize) params.set('pageSize', String(options.pageSize));
  if (options.pageToken) params.set('pageToken', options.pageToken);
  const query = params.toString();
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs${query ? `?${query}` : ''}`,
  );
  await handleResponse(response, 'List jobs');
  return response.json();
}

export async function getJob(projectId: string, jobId: string): Promise<RecipeJob> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}`,
  );
  await handleResponse(response, 'Get job');
  return response.json();
}

export async function submitJob(projectId: string, input: SubmitRecipeJobRequest): Promise<RecipeJob> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  await handleResponse(response, 'Submit job');
  return response.json();
}
