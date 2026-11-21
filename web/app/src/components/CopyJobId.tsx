import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { Button, Input, Tooltip, message } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { copyText, copyTextCompatibility } from '../utils/copyText';

type Feedback = 'idle' | 'pending' | 'copied' | 'modern-failure' | 'compatibility-failure';
export default function CopyJobId({ jobId, title, before, after, compact = false }: {
  jobId: string; title: ReactNode; before?: ReactNode; after?: ReactNode; compact?: boolean;
}) {
  const [feedback, setFeedback] = useState<Feedback>('idle');
  const [announcement, setAnnouncement] = useState(0);
  const attempt = useRef(0);
  const button = useRef<HTMLButtonElement>(null);
  const recovery = useRef<HTMLDivElement>(null);
  const focusAfterRetry = useRef(false);
  const id = useId();
  useEffect(() => () => { attempt.current++; }, [jobId]);
  useLayoutEffect(() => {
    if (!focusAfterRetry.current) return;
    focusAfterRetry.current = false;
    if (feedback === 'copied') button.current?.focus({ preventScroll: true });
    else recovery.current?.querySelector('textarea')?.focus({ preventScroll: true });
  }, [feedback]);
  const complete = (current: number, outcome: Feedback) => {
    if (current !== attempt.current) return;
    setFeedback(outcome);
    if (outcome === 'copied') {
      setAnnouncement(current);
      message.success('Job ID copied');
    }
  };
  const copy = () => {
    const current = ++attempt.current;
    setFeedback('pending');
    const result = copyText(jobId);
    if (result.method === 'compatibility') complete(current, result.copied ? 'copied' : 'compatibility-failure');
    else void result.completion.then(
      () => complete(current, 'copied'),
      () => complete(current, 'modern-failure'),
    );
  };
  const failed = feedback === 'modern-failure' || feedback === 'compatibility-failure';
  const mac = /Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent);
  return <div className={`goal-copy-region${compact ? ' goal-copy-compact' : ''}`}>
    <div className={compact ? 'goal-card-summary' : 'goal-copy-header'}>
      {before}
      <div className="goal-title-with-copy">{title}
        <Tooltip title={`Copy job ID ${jobId}`} trigger={['hover', 'focus']}>
          <Button ref={button} className="goal-copy-id" type="text" size="small" aria-label={`Copy job ID ${jobId}`}
            icon={<CopyOutlined aria-hidden="true" />} onClick={event => { event.stopPropagation(); copy(); }} />
        </Tooltip>
      </div>
      {after}
    </div>
    <div className="goal-copy-recovery" ref={recovery} onClick={event => event.stopPropagation()}>
      <div className="goal-copy-outcome">
        <div className="goal-copy-feedback" role="status" aria-live="polite" aria-atomic="true">
          {/* Static Ant Design messages have no live region. Keep one announcement
              here; remount it for repeated synchronous compatibility copies. */}
          {feedback === 'copied' && <span key={announcement} className="sr-only">Job ID copied</span>}
          {failed && 'Could not copy job ID.'}
        </div>
        {feedback === 'modern-failure' && <Button className="goal-copy-retry" size="small" onClick={event => {
          event.stopPropagation();
          const current = ++attempt.current;
          focusAfterRetry.current = document.activeElement === event.currentTarget;
          complete(current, copyTextCompatibility(jobId) ? 'copied' : 'compatibility-failure');
        }}>Copy another way</Button>}
      </div>
      {failed && <div className="goal-copy-manual">
        <label htmlFor={id}>Job ID</label>
        <Input.TextArea id={id} className="goal-copy-manual-id" value={jobId} readOnly autoSize={{ minRows: 1 }}
          aria-describedby={`${id}-hint`} onFocus={event => event.currentTarget.select()} />
        <div id={`${id}-hint`}>Select the job ID and press {mac ? '⌘C' : 'Ctrl+C'} to copy.</div>
      </div>}
    </div>
  </div>;
}
