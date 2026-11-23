import { Button, Card, Collapse, Descriptions, Statistic, Tag, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { Link, Route, Routes, useParams } from 'react-router-dom';
import {
  pulseConfig,
  pulseInstances,
  pulseStatus,
  providerInstances,
  type PulseHealth,
  type PulseProvider,
} from '@colony2/shared';
import {
  catalog,
  displayTime,
  providerNames,
  readProvider,
  readProviders,
  usePages,
  useRead,
  type RunnerProvider,
} from './data';
import { instancesPath, providerPath, runnersPath } from './paths';
import { InstancesPage, InstancesTable } from './InstancesPage';
import LaunchPage from './LaunchPage';
import { PartialErrors, ReadState } from './ReadState';
import './runners.css';

function fields(value: unknown) {
  if (Array.isArray(value) && value.every((v) => typeof v === 'string')) return value.join(', ') || 'None';
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2);
}
function ConfiguredCatalog({ settings }: { settings?: Record<string, unknown> }) {
  const entries = catalog(settings);
  return entries.length ? (
    <>
      <Typography.Text type="secondary">From provider configuration</Typography.Text>
      <Descriptions
        size="small"
        column={1}
        items={entries.map(([key, value]) => ({ key, label: key, children: fields(value) }))}
      />
    </>
  ) : (
    <Typography.Text type="secondary">Bases and capabilities unreported.</Typography.Text>
  );
}
const discoveryMessage: Record<Exclude<PulseProvider['discovery'], 'known'>, string> = {
  unknown: 'Provider has no catalog information.',
  unavailable: 'Provider unavailable for catalog discovery.',
  timeout: 'Catalog discovery timed out.',
  invalid: 'Provider returned an invalid catalog.',
  error: 'Catalog discovery failed.',
};

function Catalog({ provider }: { provider: RunnerProvider }) {
  if (provider.discovery === undefined) return <ConfiguredCatalog settings={provider.settings} />;
  if (provider.discovery !== 'known')
    return <Typography.Text type="secondary">{discoveryMessage[provider.discovery]}</Typography.Text>;
  return (
    <>
      <Typography.Text type="secondary">
        {provider.bases_complete ? 'Complete catalog' : 'Partial catalog'}
      </Typography.Text>
      {provider.bases.length ? (
        <Descriptions
          size="small"
          column={1}
          items={provider.bases.map((base) => ({
            key: base.name,
            label: base.name,
            children: base.capabilities.length ? (
              <span>
                {base.capabilities.map((capability) => (
                  <Tag key={capability}>{capability}</Tag>
                ))}
              </span>
            ) : (
              <Typography.Text type="secondary">No optional capabilities</Typography.Text>
            ),
          }))}
        />
      ) : (
        <div>No bases advertised.</div>
      )}
    </>
  );
}

function ProviderHealth({ health }: { health?: PulseHealth }) {
  return <Tag>{health?.state || 'Health unreported'}</Tag>;
}
function Overview({ tenant }: { tenant: string }) {
  const status = useRead('status', pulseStatus);
  const providers = useRead('providers', readProviders);
  const instances = useRead('instances', (s) => pulseInstances(s));
  const names = providerNames(undefined, providers.value, instances.value);
  const reads = [status, providers, instances];
  const refresh = () => reads.forEach((read) => read.refresh());
  const errors = [...new Set(reads.map((read) => read.error).filter(Boolean))];
  return (
    <>
      <div className="runner-heading">
        <Typography.Title level={2}>Runners</Typography.Title>
        <Button icon={<ReloadOutlined />} onClick={refresh}>
          Refresh
        </Button>
      </div>
      {status.value && (
        <p>
          Pulse {status.value.version} · {status.value.status} · {status.value.providers} providers · Last
          poll {displayTime(status.value.poll.last_finished)}
        </p>
      )}
      {errors.map((error) => (
        <ReadState key={error} error={error} retry={refresh} />
      ))}
      <PartialErrors result={instances.value} />
      <div className="runner-grid">
        {names.map((name) => {
          const provider = providers.value?.find((item) => item.name === name);
          const faulty = instances.value?.errors.some((e) => e.provider === name);
          const count = instances.value?.items.filter((i) => i.provider === name).length;
          return (
            <article key={name}>
              <Card
                size="small"
                title={<Link to={providerPath(tenant, name)}>{name}</Link>}
                extra={<ProviderHealth health={provider?.health} />}
              >
                {provider?.type && <Typography.Text type="secondary">{provider.type}</Typography.Text>}
                <Statistic
                  title="Active instances"
                  value={count ?? '—'}
                  prefix={faulty && count !== undefined ? '≥' : undefined}
                />
                {provider && <Catalog provider={provider} />}
              </Card>
            </article>
          );
        })}
      </div>
      {!names.length && (
        <p>{providers.busy || instances.busy ? 'Loading providers…' : 'No providers available.'}</p>
      )}
    </>
  );
}
function Provider({ tenant, name }: { tenant: string; name: string }) {
  const config = useRead('config', pulseConfig);
  const provider = useRead(name, (signal) => readProvider(name, signal));
  const instances = usePages(async (signal, token) => {
    const page = await providerInstances(name, signal, token);
    return { items: page.items.map((i) => ({ ...i, provider: name })), next: page.next_page_token };
  });
  const settings = config.value?.providers?.[name];
  const snapshot = provider.value?.health;
  const targets = config.value?.targets?.filter((t) => t.launch_services?.some((p) => p.name === name));
  return (
    <>
      <div className="runner-heading">
        <Typography.Title level={2}>{name}</Typography.Title>
        <div>
          {provider.value?.type && <Tag>{provider.value.type}</Tag>}
          <ProviderHealth health={snapshot} />
        </div>
      </div>
      <ReadState {...config} retry={config.refresh} />
      <ReadState {...provider} retry={provider.refresh} />
      <div className="runner-grid">
        <Card size="small" title="Bases and capabilities" loading={provider.busy && !provider.value}>
          {provider.value && <Catalog provider={provider.value} />}
        </Card>
        <Card size="small" title="Provider health" loading={provider.busy && !provider.value}>
          {snapshot ? (
            <Descriptions
              size="small"
              column={1}
              items={Object.entries(snapshot)
                .filter(([k]) => k !== 'name')
                .map(([key, value]) => ({
                  key,
                  label: key.replace(/_/g, ' '),
                  children: value === '' ? '—' : String(value),
                }))}
            />
          ) : (
            <Typography.Text type="secondary">No health snapshot reported.</Typography.Text>
          )}
        </Card>
      </div>
      {targets && targets.length > 0 && (
        <section>
          <Typography.Title level={4}>Targets</Typography.Title>
          <ul>
            {targets.map((t, i) => (
              <li key={i}>
                {t.instance_id} · {t.jobdb} · Priority{' '}
                {t.launch_services?.find((p) => p.name === name)?.priority}
              </li>
            ))}
          </ul>
        </section>
      )}
      {(settings || config.value?.defaults) && (
        <Collapse
          size="small"
          items={[
            ...(settings
              ? [
                  {
                    key: 'provider',
                    label: 'Provider configuration',
                    children: <pre>{JSON.stringify(settings, null, 2)}</pre>,
                  },
                ]
              : []),
            ...(config.value?.defaults
              ? [
                  {
                    key: 'defaults',
                    label: 'Controller launch defaults',
                    children: <pre>{JSON.stringify(config.value.defaults, null, 2)}</pre>,
                  },
                ]
              : []),
          ]}
        />
      )}
      <Typography.Title level={4}>Active instances</Typography.Title>
      <ReadState {...instances} retry={instances.retry} />
      <InstancesTable tenant={tenant} rows={instances.items} empty={!instances.busy && !instances.error} />
      {instances.more && (
        <Button disabled={instances.busy} onClick={instances.more}>
          Load more instances
        </Button>
      )}
    </>
  );
}
function ProviderRoute({ tenant }: { tenant: string }) {
  const { provider = '' } = useParams();
  return <Provider key={provider} tenant={tenant} name={provider} />;
}
export default function RunnersPage() {
  const { projectId = '' } = useParams();
  return (
    <div className="app-page runners-page" key={projectId}>
      <nav aria-label="Runner navigation">
        <Link to={runnersPath(projectId)}>Runners overview</Link>
        <Link to={instancesPath(projectId)}>Instances</Link>
      </nav>
      <Routes>
        <Route index element={<Overview tenant={projectId} />} />
        <Route path="instances" element={<InstancesPage tenant={projectId} />} />
        <Route path="providers/:provider" element={<ProviderRoute tenant={projectId} />} />
        <Route path="launches/:launch/logs" element={<LaunchPage tenant={projectId} logsOnly />} />
        <Route path="launches/:launch" element={<LaunchPage tenant={projectId} />} />
        <Route path="*" element={<p>Runner page not found.</p>} />
      </Routes>
    </div>
  );
}
