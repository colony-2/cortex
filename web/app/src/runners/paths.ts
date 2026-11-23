export const runnersPath = (tenant: string) => `/project/${encodeURIComponent(tenant)}/runners`;
export const providerPath = (tenant: string, provider: string) =>
  `${runnersPath(tenant)}/providers/${encodeURIComponent(provider)}`;

function query(values: Record<string, string | undefined>) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) if (value) params.set(key, value);
  return params.size ? `?${params}` : '';
}
export const instancesPath = (tenant: string, filters: Record<string, string> = {}) =>
  `${runnersPath(tenant)}/instances${query(filters)}`;
export const launchPath = (
  tenant: string,
  launch: string,
  context: { provider?: string; jobId?: string } = {},
  logsOnly = false,
) =>
  `${runnersPath(tenant)}/launches/${encodeURIComponent(launch)}${logsOnly ? '/logs' : ''}${query(context)}`;

export const jobStoryPath = (tenant: string, job: string) =>
  `/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/story`;
