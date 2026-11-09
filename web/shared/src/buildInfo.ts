import { API_BASE } from './apiBase';
export interface BuildIdentity { version: string | null; commit: string | null }
export interface LibraryBuild extends BuildIdentity { module: string }
export interface BuildInformation { libraries: { c2j: LibraryBuild; jobdb: LibraryBuild } }
export async function getBuildInformation(signal?: AbortSignal): Promise<BuildInformation> {
  const response = await fetch(`${API_BASE}/build-info`, { signal });
  if (!response.ok) throw new Error(`Build information request failed (${response.status})`);
  const data = await response.json();
  for (const key of ['c2j', 'jobdb']) {
    const value = data?.libraries?.[key];
    if (!value || typeof value.module !== 'string' || !['version', 'commit'].every(field => value[field] === null || typeof value[field] === 'string')) {
      throw new Error('Unsupported build information response');
    }
  }
  return data;
}
