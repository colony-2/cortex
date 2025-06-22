import type { DependencyGraph } from '../types';

const API_BASE_URL = 'http://localhost:8080';

export async function fetchGraph(): Promise<DependencyGraph> {
  const response = await fetch(`${API_BASE_URL}/api/graph`);
  if (!response.ok) {
    throw new Error('Failed to fetch graph data');
  }
  return response.json();
}

export async function fetchPositions(): Promise<Record<string, { nodeId: string; x: number; y: number }>> {
  const response = await fetch(`${API_BASE_URL}/api/positions`);
  if (!response.ok) {
    throw new Error('Failed to fetch positions');
  }
  return response.json();
}

export async function savePositions(positions: Array<{ nodeId: string; x: number; y: number }>): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/positions`, {
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