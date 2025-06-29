import type { RelationshipGraph, NodePosition } from './types';

// Use relative URLs in production, localhost in development
const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

async function handleResponse(response: Response, operation: string) {
  if (!response.ok) {
    const errorText = await response.text().catch(() => 'Unknown error');
    throw new Error(`${operation} failed: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`);
  }
  return response;
}

export async function fetchGraph(): Promise<RelationshipGraph> {
  try {
    const response = await fetch(`${API_BASE}/graph`);
    await handleResponse(response, 'Fetch graph');
    return response.json();
  } catch (error) {
    console.error('Graph fetch error:', error);
    throw new Error(`Failed to load graph data: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
}

export async function fetchPositions(): Promise<NodePosition[]> {
  try {
    const response = await fetch(`${API_BASE}/positions`);
    await handleResponse(response, 'Fetch positions');
    const data = await response.json();
    // Ensure we always return an array
    return Array.isArray(data) ? data : [];
  } catch (error) {
    console.error('Positions fetch error:', error);
    throw new Error(`Failed to load positions: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
}

export async function savePositions(positions: NodePosition[]): Promise<void> {
  try {
    const response = await fetch(`${API_BASE}/positions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(positions),
    });
    await handleResponse(response, 'Save positions');
  } catch (error) {
    console.error('Positions save error:', error);
    throw new Error(`Failed to save positions: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
}

export async function fetchFiles(nodeId: string, path: string = ''): Promise<{ files: any[], path: string }> {
  try {
    const url = `${API_BASE}/nodes/${nodeId}/files?path=${encodeURIComponent(path)}`;
    const response = await fetch(url);
    await handleResponse(response, 'Fetch files');
    return response.json();
  } catch (error) {
    console.error('Files fetch error:', error);
    throw new Error(`Failed to load files: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
}