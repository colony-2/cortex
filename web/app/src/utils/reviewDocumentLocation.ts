import type { ReviewDocumentMetadata } from '@colony2/shared';

const utf8 = new TextEncoder();
function compareDocumentIds(a: string, b: string): number {
  const left = utf8.encode(a), right = utf8.encode(b);
  for (let i = 0; i < Math.min(left.length, right.length); i++) {
    if (left[i] !== right[i]) return left[i] - right[i];
  }
  return left.length - right.length;
}
function displayOrder(order: unknown): number {
  return typeof order === 'number' && Number.isInteger(order) && order >= 0 && order <= 1_000_000 ? order : Infinity;
}
function roleOrder(role: unknown): number {
  return role === 'primary' ? 0 : role === 'supporting' ? 1 : role === 'diagnostic' ? 2 : 3;
}
export function orderedReviewDocumentIds(documents: Record<string, unknown>, metadata: Record<string, ReviewDocumentMetadata>): string[] {
  return Object.keys(documents).sort((a, b) =>
    roleOrder(metadata[a]?.role) - roleOrder(metadata[b]?.role) ||
    displayOrder(metadata[a]?.order) - displayOrder(metadata[b]?.order) || compareDocumentIds(a, b));
}
export function startingReviewDocument(documents: Record<string, unknown>, metadata: Record<string, ReviewDocumentMetadata> = {}, overview?: string): string | undefined {
  if (overview !== undefined && Object.prototype.hasOwnProperty.call(documents, overview)) return overview;
  const ids = orderedReviewDocumentIds(documents, metadata);
  return ids.find(id => metadata[id]?.role === 'primary') ?? ids.sort(compareDocumentIds)[0];
}

export function reviewDocumentSearch(search: string, requestId: string, documentId?: string): string {
  const params = new URLSearchParams(search);
  params.set('request_id', requestId);
  if (documentId === undefined) params.delete('document_id');
  else params.set('document_id', documentId);
  return `?${params.toString()}`;
}

export type ReviewDocumentLocation =
  | { kind: 'invalid' | 'stale' }
  | { kind: 'selected'; documentId: string | undefined; search: string; unavailable: boolean };

// Call only after the current request's details have loaded. Request identity must
// be checked before membership, even when another request has the same document ID.
export function resolveReviewDocumentLocation(search: string, requestId: string, documents: Record<string, unknown>, metadata: Record<string, ReviewDocumentMetadata> = {}, overview?: string): ReviewDocumentLocation {
  const params = new URLSearchParams(search);
  if (params.getAll('request_id').length > 1 || params.getAll('document_id').length > 1) return { kind: 'invalid' };
  if (params.has('request_id') && params.get('request_id') !== requestId) return { kind: 'stale' };
  const requested = params.get('document_id');
  const unavailable = requested !== null && !Object.prototype.hasOwnProperty.call(documents, requested);
  const documentId = requested !== null && !unavailable ? requested : startingReviewDocument(documents, metadata, overview);
  // Valid, explicit pairs are left byte-for-byte intact.
  const canonical = params.has('request_id') && (documentId === undefined ? !params.has('document_id') : requested === documentId);
  return { kind: 'selected', documentId, unavailable, search: canonical ? search : reviewDocumentSearch(search, requestId, documentId) };
}
