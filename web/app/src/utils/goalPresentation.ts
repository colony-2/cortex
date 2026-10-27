import type { RecipeJob } from '@colony2/shared/types';

export function abbreviatePrompt(prompt: string, limit = 75) {
  const characters = Array.from(prompt.trim().replace(/\s+/g, ' '));
  return characters.length > limit ? `${characters.slice(0, limit).join('').trimEnd()}...` : characters.join('');
}

export function jobTimestamp(job: RecipeJob) {
  const timestamp = Date.parse(job.submitted_at || job.created_at);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

export function newestFirst(jobs: RecipeJob[]) {
  return [...jobs].sort((a, b) => jobTimestamp(b) - jobTimestamp(a));
}

export function latestActiveJob(jobs: RecipeJob[]) {
  const ordered = newestFirst(jobs);
  return ordered.find(job => job.store === 'ACTIVE' && !['COMPLETED', 'CANCELLED'].includes(job.status)) || ordered[0];
}
