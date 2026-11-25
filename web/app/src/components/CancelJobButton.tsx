import { useState } from 'react';
import { Button } from 'antd';
import { CancelJobIcon } from './JobActionIcons';
import JobActionDialog from './JobActionDialog';

interface Props {
  projectId: string;
  jobId: string;
  disabled?: boolean;
  iconOnly?: boolean;
  onCancelled: () => void | Promise<void>;
  onBusyChange?: (busy: boolean) => void;
}

export default function CancelJobButton({ projectId, jobId, disabled, iconOnly, onCancelled, onBusyChange }: Props) {
  const [open, setOpen] = useState(false);
  const [cancelled, setCancelled] = useState(false);
  return <>
    {iconOnly ? <CancelJobIcon disabled={disabled || cancelled} onClick={() => setOpen(true)} /> : <Button danger disabled={disabled || cancelled} onClick={() => setOpen(true)}>Cancel job</Button>}
    {open && <JobActionDialog projectId={projectId} jobIds={[jobId]} action="cancel" disabled={disabled}
      onClose={() => setOpen(false)} onBusyChange={onBusyChange}
      onComplete={async () => { setCancelled(true); await onCancelled(); }} />}
  </>;
}
