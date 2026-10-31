import { expect, it } from 'vitest';
import type { CortexCell } from '@colony2/shared/types';
import { goalTargets, initialGoalTarget, normalizeGoalRepository, selfTarget } from './goalTarget';

const cell = (name: string, repository_source: string, kind = 'dependent') => ({ name, repository_source, kind } as CortexCell);
it('merges known identities without confusing unknown names, self, or duplicate repositories', () => {
  const choices = goalTargets([cell('self', 'https://github.com/colony-2/cortex.git', 'self'), cell('a', 'https://github.com/colony-2/a.git'), cell('a', 'https://github.com/colony-2/other.git')],
    [{ cell_name: 'a', repo: 'https://github.com/colony-2/a' }, { cell_name: 'literal name' }, { cell_name: '  ' }], ['saved', '']);
  expect(choices.map(choice => choice.cell)).toEqual([undefined, 'https://github.com/colony-2/a.git', 'https://github.com/colony-2/other.git', 'literal name', 'saved']);
  expect(choices.filter(choice => choice.name === 'a').map(choice => choice.label)).toEqual(['a · https://github.com/colony-2/a.git', 'a · https://github.com/colony-2/other.git']);
  expect(initialGoalTarget(choices, ['a'])).toBe(selfTarget);
  expect(initialGoalTarget(choices, ['literal name']).cell).toBe('literal name');
  for (const filters of [null, [], [''], ['a', 'saved'], ['missing']]) expect(initialGoalTarget(choices, filters)).toBe(selfTarget);
});
it.each(['github/colony-2/tool', 'github.com/colony-2/tool', 'https://github.com/colony-2/tool', ' github/colony-2/tool.git/ '])('normalizes %s', input => {
  expect(normalizeGoalRepository(input)).toBe('https://github.com/colony-2/tool.git');
});
it.each(['', 'github/colony-2/', 'github/colony-2/..', 'github/colony-2/.git', 'github/colony-2/a b', 'github/colony-2/a/path', 'https://user@github.com/colony-2/a', 'https://evil.test/colony-2/a', 'github/colony-2/a?x', 'github/colony-2/a#main', 'git@github.com:colony-2/a'])('rejects %s', input => {
  expect(() => normalizeGoalRepository(input)).toThrow();
});
