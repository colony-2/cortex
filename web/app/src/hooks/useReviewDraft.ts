import { useLayoutEffect, useState, useSyncExternalStore, type Dispatch, type SetStateAction } from 'react';
import { getUserEmail } from '@colony2/shared';

// Drafts live in this browser tab and are cleared when the account changes.
const drafts = new Map<string, Map<string, unknown>>();
const listeners = new Set<() => void>();
let revision = 0;
function changed() {
  revision++;
  listeners.forEach(listener => listener());
}
function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}
if (typeof window !== 'undefined') window.addEventListener('cortex-ui-identity', () => {
  drafts.clear();
  changed();
});

export function useReviewDraft<T>(identity: string | undefined, field: string, initial: T): [T, Dispatch<SetStateAction<T>>] {
  const user = getUserEmail();
  const key = JSON.stringify([user, identity]);
  const [value, setValue] = useState<T>(() => {
    const fields = drafts.get(key);
    return fields?.has(field) ? fields.get(field) as T : initial;
  });
  useLayoutEffect(() => {
    if (!user || !identity || getUserEmail() !== user) return;
    const fields = drafts.get(key) ?? new Map();
    if (fields.has(field) && Object.is(fields.get(field), value)) return;
    fields.set(field, value);
    drafts.set(key, fields);
    changed();
  }, [user, identity, key, field, value]);
  return [value, setValue];
}

// A mutable request identity survives navigation with its draft.
export function useReviewDraftRef<T>(identity: string, field: string, initial: T) {
  return useReviewDraft(identity, field, { current: initial })[0];
}

export function clearReviewDraft(identity: string) {
  if (drafts.delete(JSON.stringify([getUserEmail(), identity]))) changed();
}

function hasContent(value: unknown): boolean {
  if (Array.isArray(value)) return value.some(hasContent);
  return typeof value === 'string' ? !!value.trim() : value !== undefined && value !== null;
}

export function useSavedReviewDrafts(tenant: string) {
  useSyncExternalStore(subscribe, () => revision);
  const user = getUserEmail();
  return [...drafts].flatMap(([key, fields]) => {
    const [owner, identity] = JSON.parse(key);
    const kind = fields.get('review-kind');
    if (!user || owner !== user || !identity?.startsWith('[') || !kind) return [];
    const [scope, job, request] = JSON.parse(identity);
    if (scope !== tenant || !request || (kind !== 'review_id' && kind !== 'input_request_id')) return [];
    const values = Object.values(fields.get('legacy-values') ?? {});
    const uploads = Object.values(fields.get('legacy-files') ?? {});
    const feedback = fields.get('feedback');
    if (![feedback, fields.get('files'), ...values, ...uploads, ...Object.values(fields.get('settings') ?? {})].some(hasContent)) return [];
    const title = typeof feedback === 'string' && feedback.trim() ? feedback.trim() : values.find(value => typeof value === 'string' && value.trim());
    return [{ identity, job: String(job), request: String(request), kind, title: typeof title === 'string' ? title : 'Review response' }];
  });
}
