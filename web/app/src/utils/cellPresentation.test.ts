import { expect, it } from 'vitest';
import { cellLabel } from './cellPresentation';

it.each([undefined, '', '  \n '])('does not infer an identity for %j', name => {
  expect(cellLabel(name)).toBe('Unknown cell');
});
it.each(['self', 'platform'])('preserves explicitly named cell %s', name => {
  expect(cellLabel(name)).toBe(name);
});
