import { useEffect, useRef, useState } from 'react';
import { Alert, Modal, Space, Typography, message } from 'antd';
import { cancelJob, deleteJob } from '@colony2/shared/api';

export type JobAction = 'cancel' | 'delete';

interface Props {
  projectId: string;
  jobIds: string[];
  action: JobAction;
  onClose: () => void;
  onComplete: (succeeded: string[]) => void | Promise<void>;
  onBusyChange?: (busy: boolean) => void;
}

// Mount a fresh dialog for each action so its confirmed IDs cannot change with
// filtering, selection, or a background list refresh.
export default function JobActionDialog({ projectId, jobIds, action, onClose, onComplete, onBusyChange }: Props) {
  const trigger = useRef(document.activeElement as HTMLElement | null);
  useEffect(() => () => {
    if (trigger.current?.isConnected) trigger.current.focus({ preventScroll: true });
  }, []);
  const [pending, setPending] = useState(jobIds);
  const [busy, setBusy] = useState(false);
  const [failures, setFailures] = useState<{ id: string; error: string }[]>([]);
  const verb = action === 'cancel' ? 'Cancel' : 'Delete';
  const label = `${verb} ${pending.length === 1 ? 'job' : `${pending.length} jobs`}`;

  const submit = async () => {
    if (busy || !pending.length) return;
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
          (action === 'cancel' ? cancelJob : deleteJob)(projectId, id)));
        results.forEach((result, index) => {
          if (result.status === 'fulfilled') succeeded.push(ids[index]);
          else failed.push({ id: ids[index], error: result.reason instanceof Error ? result.reason.message : String(result.reason) });
        });
      }
      setFailures(failed);
      setPending(failed.map(result => result.id));
      if (succeeded.length) {
        message.success(action === 'cancel'
          ? `Cancellation requested for ${succeeded.length} job${succeeded.length === 1 ? '' : 's'}`
          : `Deleted ${succeeded.length} job${succeeded.length === 1 ? '' : 's'}`);
        await onComplete(succeeded);
      }
      if (!failed.length) onClose();
    } finally {
      setBusy(false);
      onBusyChange?.(false);
    }
  };

  return <Modal open title={`${label}?`} okText={label} cancelText="Keep jobs"
    onOk={submit} onCancel={onClose} closable={!busy} maskClosable={!busy} keyboard={!busy}
    okButtonProps={{ danger: true, loading: busy, disabled: !pending.length, 'aria-label': label }}
    cancelButtonProps={{ disabled: busy }}>
    <Space direction="vertical" style={{ width: '100%' }}>
      <Typography.Paragraph>
        {action === 'cancel'
          ? 'Cancel these jobs outright, including any pending review or input. Finished jobs remain unchanged.'
          : 'Remove these jobs from job lists and cancel any unfinished work. JobDB retains history and artifacts; deletion does not reclaim storage.'}
      </Typography.Paragraph>
      <ul style={{ maxHeight: 200, overflow: 'auto', overflowWrap: 'anywhere' }}>
        {pending.map(id => <li key={id}>{id}</li>)}
      </ul>
      {failures.length > 0 && <Alert type="error" showIcon message={`${failures.length} job operation${failures.length === 1 ? '' : 's'} failed`}
        description={<><p>Only the failed jobs remain selected. Confirm again to retry.</p>
          {failures.map(failure => <div key={failure.id} style={{ overflowWrap: 'anywhere' }}><strong>{failure.id}:</strong> {failure.error}</div>)}
        </>} />}
    </Space>
  </Modal>;
}
