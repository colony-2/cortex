import { Alert, Button, Descriptions, Table, Typography } from 'antd';
import type { TableColumnsType } from 'antd';
import { dateColumn, textColumn } from '../components/tableColumns';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import {
  APIError,
  pulseConfig,
  launchHistory,
  launchInstances,
  pulseLog,
  type NamedInstance,
  type JobLeaseEvent,
} from '@colony2/shared';
import { displayTime, duration, elapsedMinutes, providerNames, usePages, useRead } from './data';
import { jobStoryPath, launchPath, providerPath } from './paths';
import LaunchLog from './LaunchLog';
import { PartialErrors, ReadState } from './ReadState';

function DiscoverLogs({
  tenant,
  launch,
  jobId,
  known,
  logsOnly,
}: {
  tenant: string;
  launch: string;
  jobId?: string;
  known: string[];
  logsOnly: boolean;
}) {
  const lookup = useRead(JSON.stringify([launch, known]), async (signal) => {
    let names = known;
    let configError = '';
    try {
      names = [...new Set([...names, ...providerNames(await pulseConfig(signal))])];
    } catch (e) {
      configError = (e as Error).message;
    }
    const found: string[] = [],
      errors: string[] = configError ? [configError] : [];
    await Promise.all(
      names.map(async (name) => {
        try {
          await pulseLog(name, launch, signal, 1);
          found.push(name);
        } catch (e) {
          if (!(e instanceof APIError && e.status === 404)) errors.push(`${name}: ${(e as Error).message}`);
        }
      }),
    );
    return { found: found.sort(), errors: errors.sort(), count: names.length };
  });
  const matches = lookup.value?.found || [];
  return (
    <>
      <ReadState {...lookup} retry={lookup.refresh} />
      {lookup.value && !matches.length && (
        <p>
          {lookup.value.errors.length
            ? 'Log lookup incomplete.'
            : `No retained log found across ${lookup.value.count} configured providers.`}
        </p>
      )}
      {!!lookup.value?.errors.length && (
        <Alert
          showIcon
          type="warning"
          message={lookup.value.errors.join(' · ')}
          action={<Button onClick={lookup.refresh}>Retry lookup</Button>}
        />
      )}
      {matches.length === 1 && (
        <>
          <p>
            Provider <Link to={providerPath(tenant, matches[0])}>{matches[0]}</Link>
          </p>
          <LaunchLog key={matches[0]} provider={matches[0]} launch={launch} />
        </>
      )}
      {matches.length > 1 && (
        <section>
          <Typography.Title level={4}>Providers with retained output</Typography.Title>
          <ul>
            {!logsOnly &&
              matches.map((name) => (
                <li key={name}>
                  <Link to={launchPath(tenant, launch, { provider: name, jobId }, logsOnly)}>{name}</Link>
                </li>
              ))}
          </ul>
        </section>
      )}
    </>
  );
}
function Details({ instance, tenant }: { instance: NamedInstance; tenant: string }) {
  return (
    <Descriptions
      size="small"
      bordered
      column={{ xs: 1, md: 2 }}
      items={[
        {
          label: 'Provider',
          children: <Link to={providerPath(tenant, instance.provider)}>{instance.provider}</Link>,
        },
        { label: 'Instance ID', children: instance.id },
        { label: 'Status', children: instance.state },
        {
          label: 'Job ID',
          children: (
            <Link to={jobStoryPath(instance.job.tenant_id, instance.job.job_id)}>{instance.job.job_id}</Link>
          ),
        },
        { label: 'Tenant', children: instance.job.tenant_id },
        { label: 'Pulse target', children: instance.job.instance_id },
        { label: 'JobDB', children: instance.job.jobdb },
        { label: 'Launched', children: displayTime(instance.created_at) },
        { label: 'Started', children: displayTime(instance.started_at) },
        { label: 'Run duration', children: duration(elapsedMinutes(instance.started_at)) },
        ...(instance.refs.length ? [{ label: 'References', children: instance.refs.join(', ') }] : []),
      ].map((item) => ({ ...item, key: item.label }))}
    />
  );
}
function RecordedLeases({ tenant, launch, jobId }: { tenant: string; launch: string; jobId?: string }) {
  const leases = usePages(async (signal, token) => {
    const page = await launchHistory(tenant, signal, jobId || '', launch, token);
    return { items: page.events, next: page.nextPageToken };
  });
  const columns: TableColumnsType<JobLeaseEvent> = [
    dateColumn<JobLeaseEvent>('Acquired', 'acquired', (r) => r.occurredAt),
    {
      ...textColumn<JobLeaseEvent>('Job ID', 'job', (r) => r.jobKey.jobId),
      render: (_, r) => (
        <Link to={jobStoryPath(r.jobKey.tenantId, r.jobKey.jobId) + '?tab=leases'}>{r.jobKey.jobId}</Link>
      ),
    },
    textColumn<JobLeaseEvent>('Cell', 'cell', (r) => r.cell),
    textColumn<JobLeaseEvent>('Route', 'route', (r) =>
      [r.route.jobType, r.route.taskType].filter(Boolean).join(' / '),
    ),
    textColumn<JobLeaseEvent>('Lease ID', 'lease', (r) => r.leaseId),
  ];
  return (
    <section>
      <Typography.Title level={4}>Recorded leases</Typography.Title>
      <ReadState {...leases} retry={leases.retry} />
      {leases.items.length > 0 && (
        <Table<JobLeaseEvent>
          size="small"
          aria-label="Recorded launch leases"
          columns={columns}
          rowKey="eventId"
          dataSource={leases.items}
          pagination={false}
          scroll={{ x: 'max-content' }}
        />
      )}
      {!leases.items.length && !leases.busy && !leases.error && <p>No retained acquisitions in {tenant}.</p>}
      {leases.more && (
        <Button disabled={leases.busy} onClick={leases.more}>
          Load older leases
        </Button>
      )}
    </section>
  );
}

function Launch({
  tenant,
  launch,
  provider,
  jobId,
  logsOnly,
}: {
  tenant: string;
  launch: string;
  provider?: string;
  jobId?: string;
  logsOnly: boolean;
}) {
  const result = useRead(launch, (s) => launchInstances(launch, provider, s));
  const matches = (result.value?.items || []).filter(
    (i) => i.launch_id === launch && (!jobId || (i.job.job_id === jobId && i.job.tenant_id === tenant)),
  );
  const providers = [...new Set(matches.map((i) => i.provider))];
  const owner = provider || (providers.length === 1 ? providers[0] : undefined);
  return (
    <>
      <Typography.Title level={2}>
        Launch <code>{launch}</code>
      </Typography.Title>
      <ReadState {...result} retry={result.refresh} />
      <PartialErrors result={result.value} />
      {!logsOnly &&
        matches.map((i) => <Details key={JSON.stringify([i.provider, i.id])} tenant={tenant} instance={i} />)}
      {!result.busy && !matches.length && (
        <>
          <p>
            {result.value?.complete ? 'No active instance listed.' : 'Active instance details unavailable.'}
          </p>
          {!logsOnly && <RecordedLeases key={jobId} tenant={tenant} launch={launch} jobId={jobId} />}
          {provider && (
            <p>
              Provider <Link to={providerPath(tenant, provider)}>{provider}</Link>
            </p>
          )}
        </>
      )}
      {owner ? (
        <LaunchLog key={owner} provider={owner} launch={launch} />
      ) : providers.length > 1 ? (
        <section>
          <Typography.Title level={4}>Select a provider to view logs</Typography.Title>
          {providers.map((p) => (
            <p key={p}>
              <Link to={launchPath(tenant, launch, { provider: p, jobId }, logsOnly)}>{p}</Link>
            </p>
          ))}
        </section>
      ) : (
        !result.busy && (
          <DiscoverLogs
            tenant={tenant}
            launch={launch}
            jobId={jobId}
            logsOnly={logsOnly}
            known={providerNames(undefined, undefined, result.value)}
          />
        )
      )}
    </>
  );
}
export default function LaunchPage({ tenant, logsOnly = false }: { tenant: string; logsOnly?: boolean }) {
  const { launch = '' } = useParams();
  const [q] = useSearchParams();
  const provider = q.get('provider') || undefined;
  const jobId = q.get('jobId') || undefined;
  if (logsOnly && provider)
    return (
      <>
        <Typography.Title level={2}>
          Launch <code>{launch}</code>
        </Typography.Title>
        <Link to={launchPath(tenant, launch, { provider, jobId })}>Instance details</Link>
        <LaunchLog key={JSON.stringify([provider, launch])} provider={provider} launch={launch} />
      </>
    );
  return (
    <Launch
      key={JSON.stringify([tenant, launch, q.get('provider')])}
      tenant={tenant}
      launch={launch}
      provider={provider}
      jobId={jobId}
      logsOnly={logsOnly}
    />
  );
}
