import { describe, expect, it } from 'vitest';
import { resolveReviewDocumentLocation as resolve, reviewDocumentSearch as write } from './reviewDocumentLocation';

const docs = { a: {}, 'b/雪 &+%': {}, 'é': {}, 'e\u0301': {} };
describe('review document URLs', () => {
  it.each(['b/雪 &+%', 'é', 'e\u0301', '%2F', '  spaced  '])('H01 preserves exact identities: %s', id => {
    const request = 'request/雪 &+%2F';
    const search = write('', request, id);
    expect(new URLSearchParams(search).get('document_id')).toBe(id);
    expect(resolve(search, request, { [id]: {} })).toEqual({ kind: 'selected', documentId: id, unavailable: false, search });
  });
  it.each(['?request_id=r&request_id=r', '?document_id=a&document_id=a', '?request_id=r&request_id=other'])('H02 rejects duplicates %s', search => {
    expect(resolve(search, 'r', docs)).toEqual({ kind: 'invalid' });
  });
  it('H03 keeps unrelated repeated values through selection and correction', () => {
    const search = write('?filter=a&filter=b&tenantId=c2&request_id=old&document_id=old', 'r', 'b/雪 &+%');
    expect(new URLSearchParams(search).getAll('filter')).toEqual(['a', 'b']);
    expect(new URLSearchParams(search).get('tenantId')).toBe('c2');
    expect(resolve(search, 'r', docs)).toMatchObject({ kind: 'selected', documentId: 'b/雪 &+%' });
    const correction = resolve('?filter=a&filter=b&document_id=missing', 'r', docs);
    expect(correction).toMatchObject({ unavailable: true, documentId: 'a' });
    if (correction.kind === 'selected') expect(new URLSearchParams(correction.search).getAll('filter')).toEqual(['a', 'b']);
  });
  it('H04 validates request before even a matching document', () => {
    expect(resolve('?request_id=old&document_id=a', 'r', docs)).toEqual({ kind: 'stale' });
    expect(resolve('?request_id=old&document_id=missing', 'r', docs)).toEqual({ kind: 'stale' });
  });
  it.each(['', '?document_id=a', '?request_id=r'])('resolves partial legacy selection %s', search => {
    const result = resolve(search, 'r', docs);
    expect(result).toMatchObject({ kind: 'selected', documentId: 'a', unavailable: false });
    if (result.kind === 'selected') expect(Object.fromEntries(new URLSearchParams(result.search))).toEqual({ request_id: 'r', document_id: 'a' });
  });
  it('retains a valid pair without changing its encoding or unrelated values', () => {
    const search = '?document_id=b%2f%E9%9B%AA%20%26%2B%25&x=%20&request_id=r';
    expect(resolve(search, 'r', docs)).toMatchObject({ search, documentId: 'b/雪 &+%' });
  });
  it('removes missing document identity from empty reviews and preserves its warning', () => {
    expect(resolve('?document_id=old', 'r', {})).toEqual({ kind: 'selected', documentId: undefined, unavailable: true, search: '?request_id=r' });
    expect(resolve('', 'r', {})).toEqual({ kind: 'selected', documentId: undefined, unavailable: false, search: '?request_id=r' });
  });
  it('uses exact membership, including prototype-like and empty IDs', () => {
    expect(resolve('?document_id=constructor', 'r', docs)).toMatchObject({ unavailable: true });
    expect(resolve('?document_id=', 'r', { '': {} })).toMatchObject({ documentId: '', unavailable: false });
  });
});
