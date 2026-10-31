import type { CortexCell, RecipeJobSummary } from '@colony2/shared/types';

export type GoalTarget = { value: string; label: string; name: string; cell?: string };
export const selfTarget: GoalTarget = { value: 'self', label: 'This cell (self)', name: 'self' };
const repositoryKey = (repo: string) => repo.replace(/^git\+/, '').replace(/\/$/, '').replace(/\.git$/, '');

export function goalTargets(catalogue: CortexCell[], jobs: Pick<RecipeJobSummary, 'cell_name' | 'repo'>[], saved: string[]): GoalTarget[] {
  const targets = new Map<string, GoalTarget>([['self', selfTarget]]);
  for (const cell of catalogue) {
    if (cell.kind === 'self') continue;
    const repo = cell.repository_source;
    if (!repo) continue;
    const value = `repo:${repositoryKey(repo)}`;
    targets.set(value, { value, name: cell.name, label: `${cell.name} · ${repo}`, cell: repo });
  }
  for (const job of [...jobs, ...saved.map(cell_name => ({ cell_name, repo: undefined }))]) {
    const name = job.cell_name;
    if (!name?.trim()) continue;
    const matching = catalogue.filter(cell => cell.name === name && (!job.repo || repositoryKey(cell.repository_source) === repositoryKey(job.repo)));
    if (matching.length) continue;
    const value = `name:${name}`;
    targets.set(value, { value, name, label: name, cell: name });
  }
  return [...targets.values()];
}

export function initialGoalTarget(targets: GoalTarget[], filter: string[] | null): GoalTarget {
  if (filter?.length !== 1 || !filter[0]?.trim()) return selfTarget;
  const matches = targets.filter(target => target.name === filter[0]);
  return matches.length === 1 ? matches[0] : selfTarget;
}

export function normalizeGoalRepository(input: string): string {
  const match = input.trim().match(/^(?:github\/|github\.com\/|https:\/\/github\.com\/)colony-2\/([A-Za-z0-9_.-]+)\/?$/);
  const name = match?.[1].replace(/\.git$/, '');
  if (!name || name === '.' || name === '..' || name.length > 100) {
    throw new Error('Enter github/colony-2/<name> or its GitHub HTTPS URL');
  }
  return `https://github.com/colony-2/${name}.git`;
}
