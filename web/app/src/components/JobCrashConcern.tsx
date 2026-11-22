import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Modal, message } from 'antd';
import { APIError, clearCrashConcern } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';

type Props = {
  projectId: string; jobId: string; job?: RecipeJob; status?: 'loading' | 'loaded' | 'failed';
  refresh: () => Promise<RecipeJob | undefined>;
  pause: () => void; resume: () => void;
};
export default function JobCrashConcern({ projectId, jobId, job, status, refresh, pause, resume }: Props) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const [error, setError] = useState<string>();
  const pending = useRef(false);
  const alive = useRef(true);
  const panel = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const concern = status === 'loaded' && job?.status === 'CRASH_CONCERN';
  // Retain the trigger during a background read so refresh does not discard
  // keyboard focus. Use aria-disabled plus a click guard during that read;
  // native disabling also drops browser focus. Clearing requires fresh metadata.
  const showConcern = (status === 'loaded' || status === 'loading') && job?.status === 'CRASH_CONCERN';
  const close = () => { if (!pending.current) setOpen(false); };
  const clear = async () => {
    if (pending.current || !concern) return;
    pending.current = true; setAttempted(true); setBusy(true); setError(undefined); pause();
    try {
      await clearCrashConcern(projectId, jobId);
      if (!alive.current) return;
      setOpen(false);
      message.success('Crash concern cleared');
    } catch (failure) {
      if (!alive.current) return;
      setOpen(false);
      const diagnostic = failure instanceof Error ? failure.message : 'Clear request failed';
      setError(failure instanceof APIError && failure.status >= 400 && failure.status < 500
        ? `${diagnostic}. Status is being refreshed before another clear.`
        : `${diagnostic}. The clear outcome may be unknown. Refresh status before trying again.`);
    } finally {
      if (alive.current) {
        await refresh();
        if (alive.current) { pending.current = false; setBusy(false); resume(); panel.current?.focus(); }
      }
    }
  };
  return <div ref={panel} tabIndex={-1} className="job-crash-concern" aria-label="Crash concern status">
    {error && <Alert type="error" showIcon message="Could not confirm clearing" description={error} />}
    {busy && <div role="status">{open ? 'Clearing crash concern…' : 'Refreshing current job status…'}</div>}
    {status === 'failed' && <Alert type="warning" showIcon message={attempted ? 'Current job status unavailable' : 'Current crash concern status unavailable'}
      action={<Button onClick={() => void refresh()}>Refresh status</Button>} />}
    {status === 'loading' && !busy && <div role="status">Checking current crash concern status…</div>}
    {showConcern && <Alert type="warning" showIcon message={concern ? 'Crash concern' : 'Last observed: Crash concern'} description={<div className="job-crash-concern-content">
      <p>Workers repeatedly lost their lease on this job. Automatic execution is paused. Resolve the worker issue before clearing this concern.</p>
      <Button ref={trigger} disabled={busy} aria-disabled={!concern} className={!concern ? 'ant-btn-disabled' : undefined}
        onClick={() => { if (concern) setOpen(true); }}>Clear crash concern</Button>
    </div>} />}
    <Modal className="crash-concern-dialog" title="Clear crash concern?" open={open} onCancel={close} closable={!busy} keyboard={!busy} maskClosable={!busy}
      focusTriggerAfterClose={false} afterClose={() => { if (!pending.current) (trigger.current ?? panel.current)?.focus(); }}
      footer={<Button aria-label="Clear crash concern" autoFocus type="primary" loading={busy} disabled={busy || !concern} onClick={() => void clear()}>Clear crash concern</Button>}>
      {busy && <p role="status">Clearing crash concern…</p>}
      <p>Tenant: <strong>{projectId}</strong><br />Job: <strong>{jobId}</strong></p>
      <p>This resets the consecutive expired-lease count. Workers may pick up this job again when its scheduling conditions allow. Existing history is preserved.</p>
    </Modal>
  </div>;
}
