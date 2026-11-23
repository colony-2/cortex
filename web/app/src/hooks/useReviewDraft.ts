import { useLayoutEffect, useState, type Dispatch, type SetStateAction } from 'react';
import { getUserEmail } from '@colony2/shared';

// Private, in-memory drafts survive navigation, never reloads or logout.
const drafts = new Map<string, unknown>();
if (typeof window !== 'undefined') window.addEventListener('cortex-ui-identity', () => drafts.clear());
export function useReviewDraft<T>(identity: string | undefined, field: string, initial: T): [T, Dispatch<SetStateAction<T>>] {
  const user = getUserEmail();
  const key = JSON.stringify([user, identity, field]);
  const [value, setValue] = useState<T>(() => user && identity && drafts.has(key) ? drafts.get(key) as T : initial);
  useLayoutEffect(() => { if (user && identity) drafts.set(key, value); }, [user, identity, key, value]);
  return [value, setValue];
}

// A mutable request identity must survive the same navigation as its draft.
export function useReviewDraftRef<T>(identity: string, field: string, initial: T) {
  return useReviewDraft(identity, field, { current: initial })[0];
}
