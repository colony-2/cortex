import { useEffect, useRef, useState } from 'react';
import { Alert, Input, Modal, Space, Typography, message } from 'antd';
import { APIError, cancelJob, archiveJob, getJob, DEFAULT_CANCELLATION_REASON } from '@colony2/shared/api';

import type { RecipeJobSummary } from '@colony2/shared/types';
import { isFinishedJob } from '../utils/jobStatus';

export type JobAction = 'cancel' | 'archive';

interface Props {
  projectId: string;
  scopeText?: string;
  onSettled?: (succeeded: string[], failures: { id: string; error: string }[]) => Promise<boolean>;
  focusFallback?: () => void;
  jobIds: string[];
  action: JobAction;
  onClose: () => void;
  onComplete: (succeeded: string[]) => void | Promise<void>;
  onFailure?: (failures: { id: string; error: string }[]) => void;
  immediate?: boolean;
  disabled?: boolean;
  job?: RecipeJobSummary;
  onBusyChange?: (busy: boolean) => void;
}

// Mount a fresh dialog for each action so its confirmed IDs cannot change with
// filtering, selection, or a background list refresh.
export default function JobActionDialog({ projectId, jobIds, action, onClose, onComplete, onFailure, onBusyChange, scopeText, onSettled, focusFallback, disabled = false, immediate = false, job }: Props) {
  const trigger = useRef(document.activeElement as HTMLElement | null);
  useEffect(() => () => {
    if (trigger.current?.isConnected) trigger.current.focus({ preventScroll: true });
    else focusFallback?.();
  }, []);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const started = useRef(false);
  const submitting = useRef(false);
  const [pending, setPending] = useState([...jobIds]);
  const [busy, setBusy] = useState(false);
  const [failures, setFailures] = useState<{ id: string; error: string }[]>([]);
  const [single] = useState(jobIds.length === 1);
  const [reason, setReason] = useState('');
  const [accepted, setAccepted] = useState(!!job?.status && (isFinishedJob(job) ? job.completion_status === 'cancelled' : !!job.cancel_requested));
  const [needsReason, setNeedsReason] = useState(action === 'cancel' || !job?.status || !isFinishedJob(job));
  const [resolving, setResolving] = useState(single && action === 'archive' && !job?.status);
  const [statusError, setStatusError] = useState<string>();
  const resolveStatus = async () => {
    setResolving(true);
    setStatusError(undefined);
    try {
      const detail = await getJob(projectId, jobIds[0]);
      if (!mounted.current) return;
      if (!detail.status) throw new Error('Job status is unavailable. Retry before archiving.');
      setNeedsReason(!isFinishedJob(detail));
      setAccepted(isFinishedJob(detail) ? detail.completion_status === 'cancelled' : !!detail.cancel_requested);
    } catch (error) { if (mounted.current) setStatusError(error instanceof Error ? error.message : String(error)); }
    finally { if (mounted.current) setResolving(false); }
  };
  useEffect(() => { if (single && action === 'archive' && !job?.status) void resolveStatus(); }, []);
  const showReason = single && needsReason && !resolving && !statusError;
  const count = Array.from(reason.trim()).length;
  const verb = action === 'cancel' ? 'Cancel' : 'Archive';
  const label = `${verb} ${pending.length === 1 ? 'job' : `${pending.length} jobs`}`;

  const submit = async () => {
    if (disabled || submitting.current || !pending.length || resolving || statusError || (showReason && count > 4000)) return;
    submitting.current = true;
    setBusy(true);
    onBusyChange?.(true);
    const succeeded: string[] = [];
    const failed: { id: string; error: string }[] = [];
    try {
      // Bound concurrency and settle every request: one failed job must not
      // disguise successful operations or prevent the rest from being tried.
      for (let offset = 0; offset < pending.length; offset += 5) {
        const ids = pending.slice(offset, offset + 5);
        const results = await Promise.allSettled(ids.map(id =>
          (action === 'cancel' ? cancelJob : archiveJob)(projectId, id, showReason ? reason.trim() || DEFAULT_CANCELLATION_REASON : undefined)));
        if (!mounted.current) return;
        results.forEach((result, index) => {
          if (result.status === 'fulfilled') succeeded.push(ids[index]);
          else {
            if (result.reason instanceof APIError && result.reason.code === 'archive_failed_after_cancel') setAccepted(true);
            failed.push({ id: ids[index], error: result.reason instanceof Error ? result.reason.message : String(result.reason) });
          }
        });
      }
      setFailures(failed);
      if (failed.length) onFailure?.(failed);
      setPending(failed.map(result => result.id));
      const resolved = await onSettled?.(succeeded, failed);
      if (succeeded.length) {
        message.success(action === 'cancel'
          ? `Cancellation requested for ${succeeded.length} job${succeeded.length === 1 ? '' : 's'}`
          : `Archived ${succeeded.length} job${succeeded.length === 1 ? '' : 's'}`);
        await onComplete(succeeded);
      }
      if (mounted.current && (!failed.length || resolved)) onClose();
    } finally {
      submitting.current = false;
      if (mounted.current) { setBusy(false); onBusyChange?.(false); }
    }
  };

  useEffect(() => {
    if (immediate && !started.current) {
      started.current = true;
      void submit();
    }
  }, []); // The mounted operation is an immutable click-time snapshot.

  if (immediate && !failures.length) return <span role="status" aria-live="polite">Archiving jobs…</span>;

  return <Modal open zIndex={1100} title={failures.length ? `${verb} failed jobs` : `${label}?`} okText={label} cancelText={failures.length ? "Close" : "Keep jobs"}
    onOk={submit} onCancel={onClose} closable={!busy} maskClosable={!busy} keyboard={!busy}
    okButtonProps={{ danger: true, loading: busy, disabled: disabled || busy || resolving || !!statusError || (showReason && count > 4000) || !pending.length, 'aria-label': label }}
    cancelButtonProps={{ disabled: busy }}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Typography.Paragraph>
        {scopeText || (action === 'cancel'
          ? 'Cancel these jobs outright, including any pending review or input. Finished jobs remain unchanged.'
          : 'Move these jobs out of the inbox and cancel any unfinished work. History and artifacts remain available in All jobs. Jobs cannot currently be restored to the inbox.')}
      </Typography.Paragraph>
      {scopeText && <Typography.Paragraph>Tenant: {projectId}<br />Job: {jobIds[0]}</Typography.Paragraph>}
      {resolving && <Typography.Paragraph role="status">Checking job status…</Typography.Paragraph>}
      {statusError && <Alert type="error" message={statusError} action={<button onClick={resolveStatus}>Retry status check</button>} />}
      {showReason && <div>
        <label htmlFor="cancellation-reason">Cancellation message (optional)</label>
        <Input.TextArea id="cancellation-reason" value={reason} onChange={event => setReason(event.target.value)} disabled={busy || (action === 'archive' && accepted)} autoSize={{ minRows: 3, maxRows: 8 }} />
        <Typography.Paragraph type="secondary">Saved with the cancellation. Leave blank to use the default message. {count.toLocaleString()} / 4,000</Typography.Paragraph>
      </div>}
      {single && action === 'archive' && accepted && <Alert type="info" message="Cancellation has already been accepted. Its saved message is retained; only archiving remains." />}
      <ul style={{ maxHeight: 200, overflow: 'auto', overflowWrap: 'anywhere' }}>
        {pending.map(id => <li key={id}>{id}</li>)}
      </ul>
      {failures.length > 0 && <Alert type="error" showIcon message={`${failures.length} job operation${failures.length === 1 ? '' : 's'} failed`}
        description={<><p>{onSettled ? 'Status was checked again. Retry is available only while the job is unfinished and no cancellation is recorded.' : 'Only the failed jobs remain selected. Retry to complete the operation.'}</p>
          {failures.map(failure => <div key={failure.id} style={{ overflowWrap: 'anywhere' }}><strong>{failure.id}:</strong> {failure.error}</div>)}
        </>} />}
    </Space>
  </Modal>;
}
