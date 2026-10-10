import type { ReactNode } from 'react';
import { Tabs } from 'antd';

interface AttemptTabsProps {
  attempts: { key: string; number: number }[];
  activeKey?: string | null;
  kind: 'Job attempt' | 'Attempt';
  onChange: (key: string) => void;
  children: ReactNode;
}

export default function AttemptTabs({ attempts, activeKey, kind, onChange, children }: AttemptTabsProps) {
  if (attempts.length < 2) return <>{children}</>;
  return <Tabs
    className="attempt-tabs"
    tabPosition="right"
    size="small"
    tabBarGutter={4}
    activeKey={activeKey || attempts[0].key}
    onChange={onChange}
    items={attempts.map(attempt => ({
      key: attempt.key,
      label: <span aria-label={`${kind} ${attempt.number}`} title={`${kind} ${attempt.number}`}>{attempt.number}</span>,
      children: attempt.key === activeKey ? children : null,
    }))}
  />;
}
