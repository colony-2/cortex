/** Reconcile a loaded page by stable row identity, then sort using prior ranks for ties. */
export function orderGoalRows<T extends { key: string }>(
  rows: readonly T[],
  previous: ReadonlyMap<string, number>,
  compare?: (a: T, b: T) => number,
  direction: 'ascend' | 'descend' = 'ascend',
) {
  // Surviving rows keep their order; new identities follow in response order.
  const survivors = rows.filter(row => previous.has(row.key))
    .sort((a, b) => previous.get(a.key)! - previous.get(b.key)!);
  const preceding = [...survivors, ...rows.filter(row => !previous.has(row.key))];
  const priorRowNum = new Map(preceding.map((row, index) => [row.key, index]));
  const ordered = compare ? [...preceding].sort((a, b) => {
    const primary = compare(a, b);
    return primary ? primary * (direction === 'descend' ? -1 : 1)
      : priorRowNum.get(a.key)! - priorRowNum.get(b.key)!;
  }) : [...rows];
  return { rows: ordered, positions: new Map(ordered.map((row, index) => [row.key, index])) };
}
