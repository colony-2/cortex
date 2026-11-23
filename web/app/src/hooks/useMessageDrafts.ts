import { useSyncExternalStore } from 'react';
import { getUserEmail } from '@colony2/shared';

export type MessageDraft = {
  recipient?: string;
  subject: string;
  body: string;
  files: File[];
  pasted: File[];
  attempt?: string;
};

const drafts = new Map<string, Map<string, MessageDraft>>();
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

export function messageDraftScope(tenant: string) {
  return JSON.stringify([getUserEmail(), tenant]);
}

export function getMessageDraft(tenant: string, id: string): MessageDraft | undefined {
  return drafts.get(messageDraftScope(tenant))?.get(id);
}

export function saveMessageDraft(scope: string, id: string, draft: MessageDraft) {
  if (JSON.parse(scope)[0] !== getUserEmail()) return;
  if (!draft.recipient && !draft.subject.trim() && !draft.body.trim() && !draft.files.length && !draft.pasted.length && !draft.attempt) {
    discardMessageDraft(scope, id);
    return;
  }
  const messages = drafts.get(scope) ?? new Map<string, MessageDraft>();
  messages.set(id, draft);
  drafts.set(scope, messages);
  changed();
}

export function discardMessageDraft(scope: string, id: string) {
  if (drafts.get(scope)?.delete(id)) changed();
}

export function useSavedMessageDrafts(tenant: string) {
  useSyncExternalStore(subscribe, () => revision);
  return [...(drafts.get(messageDraftScope(tenant)) ?? [])];
}
