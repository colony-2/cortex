import { expect, it } from 'vitest';
import { cellLabel } from './cellPresentation';
it.each([undefined, '', '  \n '])('rejects missing cell names %j', name => {
  expect(() => cellLabel(name as string)).toThrow('required cell_name');
});
it.each(['self', 'platform'])('preserves canonical name %s', name => expect(cellLabel(name)).toBe(name));
