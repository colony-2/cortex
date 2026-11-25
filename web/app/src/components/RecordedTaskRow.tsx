import { FrownOutlined, LoadingOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useEffect, useState } from 'react';
import { compactDuration, waitsForUser, type TaskInterval, type ResourceWait } from '../utils/recordedTaskTiming';
import type { StoryNode } from '../utils/storyTree';

function taskLabel(node: StoryNode) {
  const nodeId = node.path[node.path.length - 1];
  // A recorded op can be a namespace (e.g. "input") for a more specific task.
  let op = node.op_type && !node.title.startsWith(node.op_type + ':') ? node.op_type : node.title;
  if (op === 'extension_execution' || op.startsWith('extension_execution:')) {
    const input = node.input;
    if (input && typeof input === 'object' && 'selector' in input && typeof input.selector === 'string') {
      op = input.selector;
    }
  }
  if (op.startsWith('nix:') && op.includes('#')) op = op.slice(op.lastIndexOf('#') + 1) || op;
  return nodeId ? `${nodeId}:${op}` : op;
}

function RunningDuration({start}: {start: number}) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  return <>{compactDuration(now - start)}</>;
}

function TraceTime({start, showDate, timestamp, estimated}: {start: number; showDate: boolean; timestamp?: string | null; estimated?: boolean}) {
  if (!Number.isFinite(start)) return <>—</>;
  const iso = timestamp ?? new Date(start).toISOString();
  return <time dateTime={iso} title={estimated ? `Estimated start · ${iso}` : iso}>{dayjs(start).format(showDate ? 'MM/DD HH:mm' : 'HH:mm')}</time>;
}

export function ResourceWaitRow({gap, showDate}: {gap: ResourceWait; showDate: boolean}) {
  return <tr className="recorded-resource-wait" title="Estimated from the gap between recorded work intervals">
    <td><TraceTime start={gap.start} showDate={showDate} estimated /></td>
    <td>waiting for resources</td>
    <td>{compactDuration(gap.duration)}</td>
  </tr>;
}

export default function RecordedTaskRow({task, showDate, selected, onSelect}: {task: TaskInterval; showDate: boolean; selected: boolean; onSelect: () => void}) {
  const {node, start, estimated, duration} = task;
  const userWait = waitsForUser(node);
  const label = userWait ? 'wait for user' : taskLabel(node);
  return <tr className={selected ? 'selected' : undefined} onClick={onSelect}>
    <td><TraceTime start={start} showDate={showDate} timestamp={estimated ? undefined : node.started_at} estimated={estimated} /></td>
    <td><button type="button" className={userWait ? 'recorded-task-user-wait' : undefined} aria-pressed={selected}>
      <span>{label}{(node.attempt ?? 1) > 1 && ` (${node.attempt})`}</span>
      {node.status === 'failed' && <FrownOutlined aria-label="Failed" className="recorded-task-failed" />}
      {node.status === 'running' && <LoadingOutlined aria-label="Running" spin className="recorded-task-running" />}
    </button></td>
    <td title={task.measured === undefined ? 'Elapsed attempt time' : 'Measured preparation and execution'}>{node.status === 'running' && Number.isFinite(start) && !node.finished_at
      ? <RunningDuration start={start} /> : compactDuration(duration)}</td>
  </tr>;
}
