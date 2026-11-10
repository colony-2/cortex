import type { KeyboardEvent } from 'react';

export const jobDetailTabs = [
  ['conversation', 'Conversation'],
  ['relationships', 'Relationships'],
  ['input-artifacts', 'Input artifacts'],
  ['summary', 'Summary'],
  ['execution', 'Steps'],
] as const;
export type JobDetailTab = typeof jobDetailTabs[number][0];

export function jobDetailTab(search: URLSearchParams): JobDetailTab {
  if (search.has('occurrenceId') || search.has('requestId')) return 'conversation';
  if (search.has('tab')) {
    return jobDetailTabs.find(([key]) => key === search.get('tab'))?.[0] || 'conversation';
  }
  if (search.get('input') === '1') return 'conversation';
  if (['nodeId', 'path', 'attempt', 'taskOrdinal'].some(key => search.has(key))) return 'execution';
  return 'conversation';
}

export default function JobDetailTabs({ activeTab, onChange }: {
  activeTab: JobDetailTab; onChange: (tab: JobDetailTab) => void;
}) {
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const direction = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0;
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? jobDetailTabs.length - 1
      : direction ? (index + direction + jobDetailTabs.length) % jobDetailTabs.length : undefined;
    if (next === undefined) return;
    event.preventDefault();
    onChange(jobDetailTabs[next][0]);
    document.getElementById(`job-tab-${jobDetailTabs[next][0]}`)?.focus();
  };
  return <div role="tablist" aria-label="Job detail" className="job-detail-tabs">
    {jobDetailTabs.map(([key, label], index) => <button key={key} type="button" role="tab"
      id={`job-tab-${key}`} aria-controls={`job-panel-${key}`} aria-selected={activeTab === key}
      tabIndex={activeTab === key ? 0 : -1} onClick={() => onChange(key)} onKeyDown={event => onKeyDown(event, index)}>
      {label}
    </button>)}
  </div>;
}
