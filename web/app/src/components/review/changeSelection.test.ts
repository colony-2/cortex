import { describe, expect, it } from 'vitest';
import { changeSelection, initialChangeSelection } from './changeSelection';

describe('frozen review commit selection', () => {
  const initial = () => initialChangeSelection('tenant/job/request/context/capture', 100);

  it('starts at All changes and clears a selected anchor on reset', () => {
    expect(initial().range).toBeUndefined();
    expect(initial().anchor).toBeUndefined();
    const selected = changeSelection(initial(), { type: 'select', position: 3 });
    const reset = changeSelection(selected, { type: 'all' });
    expect(reset.range).toBeUndefined();
    expect(reset.anchor).toBeUndefined();
    expect(changeSelection(reset, { type: 'select', position: 7, shift: true }).range).toEqual({ from: 7, through: 7 });
  });

  it('plain selection replaces the range and anchor', () => {
    let state = changeSelection(initial(), { type: 'endpoints', from: 2, through: 8 });
    state = changeSelection(state, { type: 'select', position: 5 });
    expect(state).toMatchObject({ range: { from: 5, through: 5 }, anchor: 5, focus: 5 });
  });

  it('extends in either direction from the last plain selection, retaining full-chain positions across pages', () => {
    let state = changeSelection(initial(), { type: 'select', position: 40 });
    state = changeSelection(state, { type: 'select', position: 85, shift: true });
    expect(state.range).toEqual({ from: 40, through: 85 });
    state = changeSelection(state, { type: 'capture', identity: state.identity, count: 100 });
    state = changeSelection(state, { type: 'select', position: 4, shift: true });
    expect(state.range).toEqual({ from: 4, through: 40 });
    expect(state.anchor).toBe(40);
  });

  it('selects a single row when Shift-click has no anchor', () => {
    expect(changeSelection(initial(), { type: 'select', position: 42, shift: true })).toMatchObject({
      range: { from: 42, through: 42 }, anchor: 42,
    });
  });

  it('moves focus separately, then extends and contracts with Shift+Arrow without wrapping', () => {
    let state = initialChangeSelection('capture', 4);
    state = changeSelection(state, { type: 'arrow', direction: 1 });
    expect(state.focus).toBe(1);
    expect(state.range).toBeUndefined();
    state = changeSelection(state, { type: 'arrow', direction: 1, shift: true });
    expect(state).toMatchObject({ range: { from: 1, through: 2 }, anchor: 1 });
    state = changeSelection(state, { type: 'arrow', direction: 1, shift: true });
    state = changeSelection(state, { type: 'arrow', direction: 1, shift: true });
    expect(state.range).toEqual({ from: 1, through: 3 });
    state = changeSelection(state, { type: 'arrow', direction: -1, shift: true });
    expect(state.range).toEqual({ from: 1, through: 2 });
    for (let i = 0; i < 4; i++) state = changeSelection(state, { type: 'arrow', direction: -1, shift: true });
    expect(state).toMatchObject({ range: { from: 0, through: 1 }, focus: 0, anchor: 1 });
  });

  it('normalizes touch endpoints and uses the displayed From as the next anchor', () => {
    const state = changeSelection(initial(), { type: 'endpoints', from: 9, through: 3 });
    expect(state).toMatchObject({ range: { from: 3, through: 9 }, anchor: 3 });
    expect(changeSelection(state, { type: 'select', position: 12, shift: true }).range).toEqual({ from: 3, through: 12 });
  });

  it.each(['other-context', 'other-request', 'other-tenant'])('resets reading state for %s, preserving the same capture', identity => {
    const selected = changeSelection(initial(), { type: 'select', position: 8 });
    expect(changeSelection(selected, { type: 'capture', identity: selected.identity, count: selected.count })).toBe(selected);
    expect(changeSelection(selected, { type: 'capture', identity, count: 100 })).toEqual(initialChangeSelection(identity, 100));
  });

  it('always yields a contiguous interval and never a disjoint selection', () => {
    let state = initial();
    for (const position of [1, 7, 3, 99, 0]) {
      state = changeSelection(state, { type: 'select', position });
      expect(state.range).toEqual({ from: position, through: position });
    }
  });

  it('does not fabricate commits for empty chains or invalid positions', () => {
    const empty = initialChangeSelection('empty', 0);
    expect(changeSelection(empty, { type: 'arrow', direction: 1, shift: true })).toBe(empty);
    for (const position of [-1, 100, 1.5, NaN]) {
      const state = initial();
      expect(changeSelection(state, { type: 'select', position })).toBe(state);
      expect(changeSelection(state, { type: 'endpoints', from: 0, through: position })).toBe(state);
    }
    expect(() => initialChangeSelection('invalid', -1)).toThrow('Invalid commit count');
  });
});
