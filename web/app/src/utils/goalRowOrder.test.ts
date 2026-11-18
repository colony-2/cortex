import { expect, it } from 'vitest';
import { orderGoalRows } from './goalRowOrder';

const fixtures = [
  { key: 'A-old', cell: 'alpha', time: 8, status: 'Ready' },
  { key: 'B-new', cell: 'beta', time: 11, status: 'Ready' },
  { key: 'B-old', cell: 'beta', time: 9, status: 'Done' },
  { key: 'A-new', cell: 'alpha', time: 10, status: 'Done' },
];
const cell = (a: typeof fixtures[number], b: typeof fixtures[number]) => a.cell.localeCompare(b.cell);
const keys = (rows: readonly { key: string }[]) => rows.map(row => row.key);

it('GS01–03 prioritizes each new column and keeps preceding ties in either direction', () => {
  const time = orderGoalRows(fixtures, new Map(), (a, b) => a.time - b.time, 'descend');
  expect(keys(time.rows)).toEqual(['B-new', 'A-new', 'B-old', 'A-old']);
  const ascending = orderGoalRows(fixtures, time.positions, cell);
  expect(keys(ascending.rows)).toEqual(['A-new', 'A-old', 'B-new', 'B-old']);
  const descending = orderGoalRows(fixtures, ascending.positions, cell, 'descend');
  expect(keys(descending.rows)).toEqual(['B-new', 'B-old', 'A-new', 'A-old']);
  const status = orderGoalRows(fixtures, descending.positions, (a, b) => a.status.localeCompare(b.status));
  expect(keys(status.rows)).toEqual(['B-old', 'A-new', 'B-new', 'A-old']);
  expect(keys(orderGoalRows(fixtures, status.positions, cell).rows)).toEqual(['A-new', 'A-old', 'B-old', 'B-new']);
});

it('GS09 reconciles surviving identities and appends tied newcomers in response order', () => {
  const before = orderGoalRows(fixtures, new Map(), (a, b) => b.time - a.time);
  const rows = [fixtures[0], { ...fixtures[0], key: 'new-1' }, fixtures[3], { ...fixtures[0], key: 'new-2' }];
  const after = orderGoalRows(rows, before.positions, cell);
  expect(keys(after.rows)).toEqual(['A-new', 'A-old', 'new-1', 'new-2']);
  expect([...after.positions.keys()]).not.toContain('B-new');
});

it('GS10 handles empty, singleton and equal rows without mutating inputs or ranks', () => {
  const rows = Object.freeze(fixtures.map(row => Object.freeze({ ...row })));
  const positions = new Map([['B-old', 0], ['A-old', 1], ['B-new', 2], ['A-new', 3]]);
  const snapshot = [...positions];
  for (const direction of ['ascend', 'descend'] as const) {
    expect(keys(orderGoalRows(rows, positions, () => 0, direction).rows)).toEqual(['B-old', 'A-old', 'B-new', 'A-new']);
    expect(orderGoalRows([], positions, cell, direction).rows).toEqual([]);
    expect(orderGoalRows([rows[0]], positions, cell, direction).rows).toEqual([rows[0]]);
  }
  expect([...positions]).toEqual(snapshot);
  expect(keys(rows)).toEqual(['A-old', 'B-new', 'B-old', 'A-new']);
  expect(keys(orderGoalRows(rows, positions).rows)).toEqual(keys(rows));
});
