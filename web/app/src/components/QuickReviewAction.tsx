import { Button, Tooltip } from 'antd';
import { CheckOutlined } from '@ant-design/icons';
import type { ReactNode } from 'react';
import './reviewV2.css';

export default function QuickReviewAction({ label, send, disabled, children }: {
  label: string; send?: () => void; disabled?: boolean; children: ReactNode;
}) {
  if (!send) return <>{children}</>;
  const hint = `Send ${label} without feedback`;
  return <span className="review-split-action">
    {children}
    <Tooltip title={hint} trigger={['hover', 'focus']}>
      <Button className="review-quick-send" aria-label={hint} disabled={disabled}
        icon={<CheckOutlined aria-hidden />} onClick={send} />
    </Tooltip>
  </span>;
}
