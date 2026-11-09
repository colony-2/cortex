import { describe, expect, it } from 'vitest';
import { resolveUIBuild } from './build-info';
describe('UI provenance', () => {
 it('preserves explicit archive inputs without git', () => expect(resolveUIBuild({ CORTEX_UI_VERSION: '2.3.4', CORTEX_UI_COMMIT: 'f'.repeat(40) }, () => { throw Error(); })).toEqual({ version: '2.3.4', commit: 'f'.repeat(40) }));
 it('marks dirty source', () => expect(resolveUIBuild({}, args => args[0] === 'status' ? ' M src/App.tsx' : 'a'.repeat(40))).toEqual({ version: null, commit: 'a'.repeat(40) + '-dirty' }));
 it('keeps clean revision', () => expect(resolveUIBuild({}, args => args[0] === 'status' ? '' : 'a'.repeat(40)).commit).toBe('a'.repeat(40)));
 it('reports unavailable without git', () => expect(resolveUIBuild({}, () => { throw Error(); })).toEqual({ version: null, commit: null }));
});
