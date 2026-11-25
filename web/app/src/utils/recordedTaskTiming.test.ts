import { describe, expect, it } from 'vitest';
import { recordedTimeline, workerDurationMs } from './recordedTaskTiming';
import type { StoryNode } from './storyTree';

function task(start: number, end: number): StoryNode {
  return {kind:'task', path:[], title:'work', status:'succeeded',
    started_at:new Date(start * 1000).toISOString(), finished_at:new Date(end * 1000).toISOString()};
}

describe('recorded task timing', () => {
  it('finds only uncovered time across overlapping intervals, including short hidden tasks', () => {
    const tasks = [task(0, 60), task(5, 12), task(62, 67), task(80, 100)];
    const rows = recordedTimeline(tasks, false);
    expect(rows.filter(row => row.kind === 'wait')).toEqual([{kind:'wait', start:67000, duration:13000}]);
    expect(rows.filter(row => row.kind === 'task')).toHaveLength(2);
  });

  it('avoids inferring waits across unknown timings or job attempts', () => {
    const unknown = {...task(1, 2), started_at:null};
    const rows = recordedTimeline([task(0, 1), unknown, task(50, 60), {...task(90, 100), job_attempt:2}], true);
    expect(rows.some(row => row.kind === 'wait')).toBe(false);
  });

  it('uses measured preparation for a failed attempt that never entered execution', () => {
    const failed = task(0, 60);
    failed.metrics = {runtime:{status:'valid', phases:{execution:'not_started'}, report:{version:1, mutations:[
      {name:'c2j_task_duration_seconds', kind:'histogram', op:'observe', labels:{phase:'preparation'}, value:0.25},
    ]}}, producer:{status:'absent'}};
    expect(workerDurationMs(failed)).toBe(250);
    delete failed.metrics.runtime.phases;
    expect(workerDurationMs(failed)).toBeUndefined();
  });
});
