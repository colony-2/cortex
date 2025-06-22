import type { DependencyGraph, NodePosition } from './types';

const API_BASE = 'http://localhost:8080/api';

export async function fetchGraph(): Promise<DependencyGraph> {
  const response = await fetch(`${API_BASE}/graph`);
  if (!response.ok) {
    throw new Error('Failed to fetch graph');
  }
  return response.json();
}

export async function fetchPositions(): Promise<NodePosition[]> {
  const response = await fetch(`${API_BASE}/positions`);
  if (!response.ok) {
    throw new Error('Failed to fetch positions');
  }
  const data = await response.json();
  // Ensure we always return an array
  return Array.isArray(data) ? data : [];
}

export async function savePositions(positions: NodePosition[]): Promise<void> {
  const response = await fetch(`${API_BASE}/positions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(positions),
  });
  if (!response.ok) {
    throw new Error('Failed to save positions');
  }
}

export async function fetchFiles(nodeId: string, path: string = ''): Promise<{ files: any[], path: string }> {
  const url = `${API_BASE}/files/${nodeId}?path=${encodeURIComponent(path)}`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Failed to load files: ${response.status} ${response.statusText}`);
  }
  return response.json();
}