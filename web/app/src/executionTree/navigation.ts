export const jobBasePath = (tenant: string, job: string) => `/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}`;
export const dagPath = (tenant: string, job: string, type?: string) => `${jobBasePath(tenant, job)}/${type === 'recipe' ? 'story?tab=dag' : 'tree'}`;

/** Only recorded DAG destinations in this tenant may be used as return links. */
export function dagReturnPath(value: unknown, tenant: string): string | undefined {
  if (typeof value !== 'string') return;
  const match = value.match(/^\/project\/([^/]+)\/jobs\/([^/]+)\/(?:tree|story\?tab=dag)$/);
  if (!match || match[1] !== encodeURIComponent(tenant)) return;
  try {
    const job = decodeURIComponent(match[2]);
    if (!job || job === '.' || job === '..' || /[/\\?#]/.test(job)) return;
  } catch { return; }
  return value;
}
