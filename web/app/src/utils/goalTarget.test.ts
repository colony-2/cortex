import { expect, it } from 'vitest';
import { goalTargets, cellGoalsPath } from './goalTarget';
import { registryCell } from '../test/cellFixtures';
it('uses registry IDs and excludes retired cells from submission choices', () => {
  const active = registryCell('platform');
  expect(goalTargets([active, { ...registryCell('old'), state: 'RETIRED' }])).toEqual([{ value: active.id, label: `${active.name} · ${active.repo}` }]);
  expect(goalTargets([{ ...active, name: 'renamed' }])[0].value).toBe(active.id);
});
it('links to tenant goals using stable ID with no repository/name inference', () => {
  const url = new URL(cellGoalsPath('a/b', 'cell-id'), 'https://cortex.test');
  expect(url.pathname).toBe('/project/a%2Fb/goals');
  expect(url.searchParams.get('cellId')).toBe('cell-id');
});
