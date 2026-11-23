import { useEffect, useMemo, useState } from 'react';
import { Collapse, Descriptions, Table } from 'antd';
import type { TableColumnsType } from 'antd';
import { Link } from 'react-router-dom';
import type { JobLeaseEvent } from '@colony2/shared/types';
import { launchPath } from '../runners/paths';
import { dateColumn, numberColumn, textColumn } from './tableColumns';

interface Lease {
  id: string;
  launch: string;
  acquired?: string;
  lastRenewal?: string;
  expiry: string;
  renewals: number;
  events: JobLeaseEvent[];
}

// JobDB pages arrive newest first, with an event-ID tie breaker. The first
// renewal is authoritative even when it shortens the previous expiry.
function groupLeases(events: JobLeaseEvent[]) {
  const leases = new Map<string, Lease>();
  for (const event of events) {
    let lease = leases.get(event.leaseId);
    if (!lease) {
      lease = {
        id: event.leaseId,
        launch: event.workerId,
        expiry: event.leaseExpiresAt,
        renewals: 0,
        events: [],
      };
      leases.set(lease.id, lease);
    }
    lease.events.push(event);
    if (event.eventType === 'lease_acquired') lease.acquired = event.occurredAt;
    else {
      if (!lease.renewals) {
        lease.lastRenewal = event.occurredAt;
        lease.expiry = event.leaseExpiresAt;
      }
      lease.renewals++;
    }
  }
  return [...leases.values()];
}

function recordedTime(raw?: string) {
  return raw && !raw.startsWith('0001-') ? Date.parse(raw) : NaN;
}

function elapsed(seconds?: number) {
  if (seconds === undefined) return '—';
  const s = Math.floor(seconds);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
  return `${Math.floor(s / 3600)}h ${Math.floor(s / 60) % 60}m`;
}

function EventDetails({ event }: { event: JobLeaseEvent }) {
  const fields = {
    'Event ID': event.eventId,
    'Occurred at (UTC)': event.occurredAt,
    'Lease expires at (UTC)': event.leaseExpiresAt,
    'Tenant ID': event.jobKey.tenantId,
    'Job ID': event.jobKey.jobId,
    'Lease ID': event.leaseId,
    'Historical cell ID': event.cellId,
    'Historical cell name': event.cell,
    'Historical cell repository': event.cellRepo,
    'Primary goal': event.goalId,
    'Original job type': event.jobType,
    'Route job type': event.route.jobType,
    'Route task type': event.route.taskType,
    ...(event.eventType === 'lease_acquired'
      ? { 'Acquisition source': event.acquisitionSource, 'Previous lease ID': event.previousLeaseId }
      : { 'Previous lease expiry (UTC)': event.previousLeaseExpiresAt }),
  };
  return (
    <Descriptions
      size="small"
      column={{ xs: 1, sm: 2 }}
      items={Object.entries(fields).map(([label, value]) => ({
        key: label,
        label,
        children: value || 'Not recorded',
      }))}
    />
  );
}

export default function LeaseTable({
  events,
  projectId,
  jobId,
}: {
  events: JobLeaseEvent[];
  projectId: string;
  jobId: string;
}) {
  const leases = useMemo(() => groupLeases(events), [events]);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const end = leases.reduce(
      (latest, r) => (r.acquired ? Math.max(latest, recordedTime(r.expiry) || 0) : latest),
      0,
    );
    setNow(Date.now());
    if (end <= Date.now()) return;
    const timer = window.setInterval(() => {
      const current = Date.now();
      setNow(current);
      if (current >= end) window.clearInterval(timer);
    }, 1000);
    return () => window.clearInterval(timer);
  }, [leases]);
  const duration = (row: Lease) => {
    const ms = Math.min(now, recordedTime(row.expiry)) - recordedTime(row.acquired);
    return Number.isFinite(ms) ? Math.max(0, ms / 1000) : undefined;
  };
  const columns: TableColumnsType<Lease> = [
    dateColumn<Lease>('Time acquired', 'acquired', (r) => r.acquired),
    dateColumn<Lease>('Last renewal', 'renewed', (r) => r.lastRenewal),
    {
      ...numberColumn<Lease>('Duration', 'duration', 'Minimum seconds', duration),
      render: (_, r) => elapsed(duration(r)),
    },
    dateColumn<Lease>('Expiry', 'expiry', (r) => r.expiry),
    {
      ...textColumn<Lease>('Launch ID', 'launch', (r) => r.launch),
      render: (_, r) => <Link to={launchPath(projectId, r.launch, { jobId })}>{r.launch}</Link>,
    },
    numberColumn<Lease>('Renewals', 'renewals', 'Minimum renewals', (r) => r.renewals),
  ];
  return (
    <Table<Lease>
      size="small"
      aria-label="Recorded leases"
      rowKey="id"
      columns={columns}
      dataSource={leases}
      showSorterTooltip={{ styles: { root: { pointerEvents: 'none' } } }}
      pagination={false}
      scroll={{ x: 'max-content' }}
      expandable={{
        expandedRowRender: (lease) => (
          <Collapse
            size="small"
            items={lease.events.map((event) => ({
              key: event.eventId,
              label: `${event.eventType === 'lease_acquired' ? 'Lease acquired' : 'Lease renewed'} · ${event.eventId}`,
              children: <EventDetails event={event} />,
            }))}
          />
        ),
      }}
    />
  );
}
