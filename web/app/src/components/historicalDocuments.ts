import { stringifyJobJSON, type HistoryDetail, type ReviewDocumentMetadata } from '@colony2/shared';
import type { ReaderDocument } from './ReviewDocuments';
import { object } from './conversationPresentation';
// Stored identity excludes display name and byte-count metadata, matching the
// retained manifest's producer/ordinal/path identity. Never guess by position.
function referenceIdentity(value: unknown): string | undefined {
  const ref = object(value), key = object(object(ref.stored).key);
  if (ref.kind === 'stored' && typeof key.jobId === 'string' && key.taskOrdinal !== undefined && typeof key.name === 'string') {
    return stringifyJobJSON(['stored', key.jobId, key.taskOrdinal, key.name]);
  }
  if (ref.kind === 'attached' && typeof ref.name === 'string') return stringifyJobJSON(['attached',ref.name]);
  return undefined;
}
export function historicalDocuments(detail: HistoryDetail) {
  const form = object(detail.raw_form), originals = object(form.documents);
  const documents: Record<string, ReaderDocument> = Object.create(null);
  const members: Record<string, string> = Object.create(null);
  for (const [id, raw] of Object.entries(originals)) {
    const doc = object(raw), stored = object(doc.stored), key = object(stored.key);
    documents[id] = {name:typeof doc.name === 'string' ? doc.name : typeof key.name === 'string' ? key.name : id, ...(typeof key.name === 'string' ? {stored:doc.stored} : {})};
    const pointer = `/documents/${id.replace(/~/g,'~0').replace(/\//g,'~1')}`;
    const matches = detail.request_manifest.filter(member => {
      const original = member.original_ref ?? member.ref;
      const identity = referenceIdentity(raw);
      return (identity !== undefined && referenceIdentity(original) === identity) || member.associations?.some(a => a.source === pointer);
    });
    const ids = [...new Set(matches.map(m => m.artifact_id))];
    if (ids.length === 1) {
      members[id] = ids[0];
      // An attached placeholder can resolve to a retained stored reference.
      // Use that actual original filename for format detection, never a key
      // invented from the placeholder's logical attachment name.
      const resolved = object(matches[0].ref);
      const resolvedKey = object(object(resolved.stored).key);
      if (doc.kind !== 'stored' && resolved.kind === 'stored' && typeof resolvedKey.name === 'string') {
        documents[id] = {name: typeof resolved.name === 'string' && resolved.name ? resolved.name : resolvedKey.name, stored:resolved.stored};
      }
    }
  }
  return {documents, members, metadata: object(form.document_metadata) as Record<string, ReviewDocumentMetadata>, overview:typeof form.overview_document_id === 'string' ? form.overview_document_id : undefined, limited:!detail.raw_form || !form.document_metadata};
}
