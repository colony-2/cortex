import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import JobDetailTabs, { jobDetailTab, jobDetailTabs, type JobDetailTab } from './JobDetailTabs';

afterEach(cleanup);
describe('CT-004,006 tab URL precedence', () => {
  it.each([
    ['', 'conversation'], ['?tab=input-artifacts', 'conversation'], ['?tab=unknown&nodeId=n', 'conversation'], ['?nodeId=n', 'execution'],
    ['?path=root&attempt=2', 'execution'], ['?taskOrdinal=2', 'execution'], ['?attempt=2', 'execution'],
    ['?input=1&taskOrdinal=2', 'conversation'], ['?tab=summary&input=1&nodeId=n', 'execution'], ['?tab=leases', 'runtime'], ['?tab=env', 'runtime'], ['?tab=runtime', 'runtime'], ['?tab=relationships', 'dag'], ['?tab=env&requestId=r', 'conversation'], ['?tab=env&occurrenceId=o', 'conversation'],
    ['?requestId=opaque%2Fid&tab=execution&input=1', 'conversation'],
    ['?requestId=&tab=execution', 'conversation'], ['?tab=dag', 'dag'], ['?occurrenceId=x&tab=dag', 'conversation'],
  ])('%s selects %s', (search, expected) => expect(jobDetailTab(new URLSearchParams(search))).toBe(expected));
});
it('CT-007 supports arrows, wraparound, Home and End with a single tab stop', () => {
  function Host() {
    const [tab, setTab] = useState<JobDetailTab>('conversation');
    return <JobDetailTabs activeTab={tab} onChange={setTab} />;
  }
  render(<Host />);
  const tabs = screen.getAllByRole('tab');
  expect(tabs.map(tab => tab.textContent)).toEqual(['Thread', 'Execution', 'Graph', 'Trace']);
  const press = (key: string, expected: number) => {
    fireEvent.keyDown(screen.getByRole('tab', { selected: true }), { key });
    expect(tabs[expected]).toHaveAttribute('aria-selected', 'true');
    expect(tabs[expected]).toHaveFocus();
    expect(tabs.filter(tab => tab.tabIndex === 0)).toEqual([tabs[expected]]);
  };
  press('ArrowLeft', 3); press('ArrowRight', 0); press('ArrowRight', 1); press('End', 3); press('Home', 0); press('ArrowRight', 1); press('ArrowRight', 2);
  expect(tabs[1]).toHaveAttribute('aria-controls', 'job-panel-runtime');
});
it('selects a clicked tab', () => {
  const change = vi.fn(); render(<JobDetailTabs activeTab="conversation" onChange={change} />);
  fireEvent.click(screen.getByRole('tab', { name: 'Graph' }));
  expect(change).toHaveBeenCalledWith('dag');
});
