import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Button, Checkbox, Empty, Modal } from 'antd';
import { DeleteOutlined } from '@ant-design/icons';
import { discardMessageDraft, messageDraftScope, type useSavedMessageDrafts } from '../hooks/useMessageDrafts';
import { clearReviewDraft, type useSavedReviewDrafts } from '../hooks/useReviewDraft';
import { messagePreview } from '../utils/jobInbox';
import InboxSelectionToolbar from './InboxSelectionToolbar';
import { reviewThreadPath } from '../utils/reviewDocumentLocation';

export default function InboxDraftsPage({ tenant, messages, reviews, sending, openComposer, closeComposer }: {
  tenant: string;
  messages: ReturnType<typeof useSavedMessageDrafts>;
  reviews: ReturnType<typeof useSavedReviewDrafts>;
  sending: ReadonlySet<string>;
  openComposer: (id: string) => void;
  closeComposer: (id: string) => void;
}) {
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [discard, setDiscard] = useState<{ keys: string[]; messages: string[]; reviews: string[] }>();
  const rows = [
    ...messages.map(([id, draft]) => {
      const title = messagePreview(draft.subject) || messagePreview(draft.body) || 'New message';
      return { key: `message:${id}`, title, messageId: id, reviewId: undefined,
        link: <Button type="link" onClick={() => openComposer(id)}>{title}</Button> };
    }),
    ...reviews.map(draft => {
      const title = `Review response · ${messagePreview(draft.title)}`;
      return { key: `review:${draft.identity}`, title, messageId: undefined, reviewId: draft.identity,
        link: <Link state={{ openReviewDraft: true, inboxReturnTo: `/project/${encodeURIComponent(tenant)}/jobs?inbox=drafts` }} to={reviewThreadPath(tenant,draft.job,draft.request,draft.kind === 'review_id' ? 'review_id' : 'request_id')}
          onClick={()=>{if (!sending.has(draft.identity)) closeComposer(draft.identity);}}>{title}</Link> };
    }),
  ];
  const selectedRows = rows.filter(row => selected.has(row.key));
  const beginDiscard = () => setDiscard({
    keys: selectedRows.map(row => row.key),
    messages: selectedRows.flatMap(row => row.messageId ?? []),
    reviews: selectedRows.flatMap(row => row.reviewId ?? []),
  });
  const discardDrafts = () => {
    if (!discard || [...discard.messages,...discard.reviews].some(id => sending.has(id))) return;
    const scope = messageDraftScope(tenant);
    discard.messages.forEach(id => { closeComposer(id); discardMessageDraft(scope, id); });
    discard.reviews.forEach(id => { closeComposer(id); clearReviewDraft(id); });
    setSelected(previous => {
      const next = new Set(previous);
      discard.keys.forEach(key => next.delete(key));
      return next;
    });
    setDiscard(undefined);
  };
  return <div className="app-page inbox-page">
    <div className="inbox-list-toolbar"><h1>Drafts</h1></div>
    <InboxSelectionToolbar items={rows.length} selected={selectedRows.length} kind="drafts"
      onSelect={mode => setSelected(new Set(mode === 'all' ? rows.map(row => row.key) : []))}>
      {selectedRows.length > 0 && <Button type="text" icon={<DeleteOutlined aria-hidden="true" />}
        disabled={selectedRows.some(row => sending.has(row.messageId ?? row.reviewId ?? ''))}
        onClick={beginDiscard}>Discard {selectedRows.length === 1 ? 'draft' : 'drafts'}</Button>}
    </InboxSelectionToolbar>
    <ul className="inbox-job-list inbox-drafts" aria-label="Drafts">
      {rows.map(row => <li key={row.key} className={`inbox-job-row inbox-draft-row${selected.has(row.key) ? ' selected' : ''}`}>
        <Checkbox className="inbox-row-select" aria-label={`Select draft ${row.title}`} checked={selected.has(row.key)} onChange={event => {
          const next = new Set(selected);
          if (event.target.checked) next.add(row.key);
          else next.delete(row.key);
          setSelected(next);
        }} />
        {row.link}
      </li>)}
    </ul>
    {!rows.length && <Empty description="No saved drafts" />}
    {discard && <Modal open title={discard.keys.length === 1 ? 'Discard this draft?' : `Discard ${discard.keys.length} drafts?`}
      okText="Discard" okButtonProps={{ danger: true, disabled: [...discard.messages,...discard.reviews].some(id => sending.has(id)) }}
      cancelText="Keep drafts" onOk={discardDrafts} onCancel={() => setDiscard(undefined)} />}
  </div>;
}
