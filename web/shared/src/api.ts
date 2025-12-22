import type { Project, RelationshipGraph } from './types';

// Use relative URLs in production, localhost in development
const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

async function handleResponse(response: Response, operation: string) {
  if (!response.ok) {
    const errorText = await response.text().catch(() => 'Unknown error');
    throw new Error(`${operation} failed: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
  }
  return response;
}

export async function fetchGraph(projectId: string): Promise<RelationshipGraph> {
  try {
    const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/graph`);
    await handleResponse(response, 'Fetch graph');
    return response.json();
  } catch (error) {
    console.error('Graph fetch error:', error);
    throw new Error(`Failed to load graph data: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
}

export async function listProjects(): Promise<Project[]> {
  const response = await fetch(`${API_BASE}/projects`);
  await handleResponse(response, 'List projects');
  return response.json();
}

export async function createProject(input: { name: string; gitRepoPath: string }): Promise<Project> {
  const response = await fetch(`${API_BASE}/projects`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  await handleResponse(response, 'Create project');
  return response.json();
}

export async function syncCells(projectId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/cells/sync`, {
    method: 'POST',
  });
  await handleResponse(response, 'Sync cells');
}

export async function updateProject(
  projectId: string,
  input: {
    name?: string;
    gitRepoPath?: string;
    gitRepoBranch?: string;
    defaultTicketRecipe?: string;
  }
): Promise<Project> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  await handleResponse(response, 'Update project');
  return response.json();
}
