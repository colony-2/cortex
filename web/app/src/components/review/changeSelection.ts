// Positions belong to the entire resolved chain, never to a rendered page.
// The shared reader resolves commit IDs to comparison trees and checks membership.
export interface ChangeSelection {
  identity: string;
  count: number;
  range?: { from: number; through: number };
  anchor?: number;
  focus?: number;
}

export type ChangeSelectionAction =
  | { type: 'capture'; identity: string; count: number }
  | { type: 'all' }
  | { type: 'focus'; position: number }
  | { type: 'select'; position: number; shift?: boolean }
  | { type: 'arrow'; direction: -1 | 1; shift?: boolean }
  | { type: 'endpoints'; from: number; through: number };

export function initialChangeSelection(identity: string, count: number): ChangeSelection {
  if (!Number.isSafeInteger(count) || count < 0) throw new Error('Invalid commit count');
  return { identity, count, focus: count ? 0 : undefined };
}

function validPosition(state: ChangeSelection, position: number): boolean {
  return Number.isSafeInteger(position) && position >= 0 && position < state.count;
}

function interval(from: number, through: number) {
  return { from: Math.min(from, through), through: Math.max(from, through) };
}

export function changeSelection(state: ChangeSelection, action: ChangeSelectionAction): ChangeSelection {
  switch (action.type) {
    case 'capture':
      // Identity includes the resolved latest-workspace selection token.
      // Re-rendering or paging the same selection must not discard a range.
      return action.identity === state.identity && action.count === state.count
        ? state : initialChangeSelection(action.identity, action.count);
    case 'all':
      return { ...state, range: undefined, anchor: undefined };
    case 'focus':
      return validPosition(state, action.position) ? { ...state, focus: action.position } : state;
    case 'select': {
      if (!validPosition(state, action.position)) return state;
      const anchor = action.shift ? state.anchor ?? action.position : action.position;
      return { ...state, anchor, focus: action.position, range: interval(anchor, action.position) };
    }
    case 'arrow': {
      if (!state.count) return state;
      const previous = state.focus ?? 0;
      const focus = Math.max(0, Math.min(state.count - 1, previous + action.direction));
      if (!action.shift) return { ...state, focus };
      const anchor = state.anchor ?? previous;
      return { ...state, focus, anchor, range: interval(anchor, focus) };
    }
    case 'endpoints': {
      if (!validPosition(state, action.from) || !validPosition(state, action.through)) return state;
      const range = interval(action.from, action.through);
      return { ...state, range, anchor: range.from, focus: range.through };
    }
  }
}
