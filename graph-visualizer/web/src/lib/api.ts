import type { DependencyGraph } from '../types';

const API_BASE_URL = 'http://localhost:8080';

export async function fetchGraph(): Promise<DependencyGraph> {
  const response = await fetch(`${API_BASE_URL}/api/graph`);
  if (!response.ok) {
    throw new Error('Failed to fetch graph data');
  }
  return response.json();
}