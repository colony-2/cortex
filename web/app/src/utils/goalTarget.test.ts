import { expect, it } from 'vitest';
import { goalTargets, cellInboxPath } from './goalTarget';
import { registryCell } from '../test/cellFixtures';
it('uses registry IDs and excludes retired cells from submission choices', () => {
  const active = registryCell('platform');
  expect(goalTargets([active, { ...registryCell('old'), state: 'RETIRED' }])).toEqual([{ value: active.id, label: `${active.name} · ${active.repo}` }]);
  expect(goalTargets([{ ...active, name: 'renamed' }])[0].value).toBe(active.id);
});
it('links to the tenant inbox using stable ID with no repository/name inference', () => {
  const url = new URL(cellInboxPath('a/b', 'cell-id'), 'https://cortex.test');
  expect(url.pathname).toBe('/project/a%2Fb/jobs');
  expect(url.searchParams.get('cellId')).toBe('cell-id');
  expect(url.searchParams.get('inbox')).toBe('cell');
});
