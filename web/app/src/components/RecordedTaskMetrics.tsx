import { stringifyJobJSON, type RecordedTaskMetrics as TaskReports, type TaskMetric } from '@colony2/shared';

const standardNames: Record<string, string> = {
  c2j_task_duration_seconds: 'Task duration',
  c2j_task_preparation_component_seconds: 'Preparation component',
  c2j_request_duration_seconds: 'Request duration',
  c2j_process_cpu_seconds_total: 'Worker CPU time',
  c2j_process_cpu_observed_seconds_total: 'CPU observation time',
};

function measurement(metric: TaskMetric) {
  const value = metric.delta ? metric.delta.sum : metric.value;
  const unit = metric.unit ? ` ${metric.unit}` : '';
  return (stringifyJobJSON(value) ?? '—') + unit;
}

function metricTitle(metric: TaskMetric) {
  if (metric.name === 'c2j_task_duration_seconds') {
    if (metric.labels?.phase === 'preparation') return 'Preparation time';
    if (metric.labels?.phase === 'execution') return 'Execution time';
  }
  return standardNames[metric.name] ?? metric.name;
}

export default function RecordedTaskMetrics({reports}: {reports?: TaskReports}) {
  if (!reports) return <p>No metrics recorded for this attempt.</p>;
  return <div className="recorded-metrics">
    {(['runtime', 'producer'] as const).map(source => {
      const channel = reports[source];
      const entries = channel.report?.mutations ?? [];
      const ordered = source === 'runtime'
        ? [...entries.filter(m => m.name === 'c2j_task_duration_seconds'), ...entries.filter(m => m.name !== 'c2j_task_duration_seconds')]
        : entries;
      return <section key={source}>
        <h4>{source === 'runtime' ? 'Runtime metrics' : 'Producer metrics'}</h4>
        {channel.diagnostic && <p>{channel.diagnostic}</p>}
        {entries.length === 0 ? <p>{channel.status === 'valid' ? 'No measurements' : channel.status || 'Unavailable'}</p> :
          <dl>{ordered.map((metric, i) => <div key={i}>
            <dt title={metric.name}><span>{source === 'runtime' ? metricTitle(metric) : metric.name}</span>
              {metric.labels && <small>{Object.entries(metric.labels).map(([key, val]) => `${key}=${val}`).join(' · ')}</small>}
            </dt>
            <dd>{measurement(metric)}{metric.delta && Number(String(metric.delta.count)) > 1 && <small>sum of {String(metric.delta.count)} samples</small>}</dd>
          </div>)}</dl>}
      </section>;
    })}
    <details><summary>Full metric reports</summary><pre>{stringifyJobJSON(reports, 2)}</pre></details>
  </div>;
}
