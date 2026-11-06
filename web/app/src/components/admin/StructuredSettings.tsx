import { Button, Card, Checkbox, Form, Input, Select, Space, Typography } from 'antd';
import { newDestination, newSecret } from './models';
import type { ActionDraft, GitDraft, GitProfile, HookDraft, HookPolicy, Permissions, PermissionsDraft, Trust } from './models';

function TrustFields({ label, value, onChange }: { label: string; value: Trust; onChange: (value: Trust) => void }) {
  return <>
    <Form.Item label={`${label} — trusted code`}>
      <Select aria-label={`${label} — trusted code`} value={value.mode} onChange={mode => onChange({ mode, ...(mode === 'commit' ? { commit: '' } : {}) })} options={[
        { value: 'commit', label: 'Only a specific commit' }, { value: 'candidate', label: 'Any candidate code' },
      ]} />
    </Form.Item>
    {value.mode === 'commit' ? <Form.Item label={`${label} — commit SHA`}><Input aria-label={`${label} — commit SHA`} value={value.commit || ''} onChange={event => onChange({ ...value, commit: event.target.value })} placeholder="40-character lowercase commit SHA" /></Form.Item>
      : <Typography.Paragraph type="secondary">This grant trusts any candidate code that runs this hook.</Typography.Paragraph>}
  </>;
}
function ActionFields({ label, value, onChange, trust }: { label: string; value: ActionDraft; onChange: (value: ActionDraft) => void; trust: boolean }) {
  return <Card size="small" style={{ marginBottom: 12 }}>
    <Checkbox checked={value.allowed} onChange={event => onChange({ ...value, allowed: event.target.checked })}>{label}</Checkbox>
    {value.allowed && <div style={{ marginTop: 12 }}>
      <Form.Item label={`${label} — target cells`}>
        <Select aria-label={`${label} — target cells`} value={value.restricted ? 'specific' : 'any'} onChange={mode => onChange({ ...value, restricted: mode === 'specific' })} options={[{ value: 'any', label: 'Any permitted cell' }, { value: 'specific', label: 'Specific cells' }]} />
      </Form.Item>
      {value.restricted && <Form.Item label={`${label} — cell names`} help="Enter one cell name per line."><Input.TextArea aria-label={`${label} — cell names`} rows={2} value={value.names} onChange={event => onChange({ ...value, names: event.target.value })} /></Form.Item>}
      {trust && <TrustFields label={label} value={value.trust} onChange={next => onChange({ ...value, trust: next })} />}
    </div>}
  </Card>;
}
function PermissionFields({ value, onChange, limit = false }: { value: PermissionsDraft; onChange: (value: PermissionsDraft) => void; limit?: boolean }) {
  const prefix = limit ? 'Limit: ' : '';
  return <>
    <Typography.Title level={5}>{limit ? 'Secrets allowed by the limit' : 'Secret access'}</Typography.Title>
    {!value.secrets.length && <Typography.Paragraph type="secondary">No secrets allowed.</Typography.Paragraph>}
    {value.secrets.map((secret, index) => {
      const label = `${prefix}Secret ${index + 1}`;
      const update = (next: typeof secret) => onChange({ ...value, secrets: value.secrets.map(row => row.id === secret.id ? next : row) });
      return <Card size="small" key={secret.id} style={{ marginBottom: 12 }}>
        <Form.Item label={`${label} name`}><Input aria-label={`${label} name`} value={secret.name} onChange={event => update({ ...secret, name: event.target.value })} placeholder="e.g. github_token" /></Form.Item>
        {!limit && <TrustFields label={label} value={secret.trust} onChange={trust => update({ ...secret, trust })} />}
        <Button danger onClick={() => onChange({ ...value, secrets: value.secrets.filter(row => row.id !== secret.id) })}>Remove {label.toLowerCase()}</Button>
      </Card>;
    })}
    <Button onClick={() => onChange({ ...value, secrets: [...value.secrets, newSecret()] })} style={{ marginBottom: 16 }}>{limit ? 'Add allowed secret to limit' : 'Add secret access'}</Button>
    <ActionFields label={`${prefix}Submit jobs`} value={value.jobs} onChange={jobs => onChange({ ...value, jobs })} trust={!limit} />
    <ActionFields label={`${prefix}Create cells`} value={value.cells} onChange={cells => onChange({ ...value, cells })} trust={!limit} />
  </>;
}
export function HookFields({ value, onChange }: { value: HookDraft; onChange: (value: HookDraft) => void }) {
  return <>
    <PermissionFields value={value.grant} onChange={grant => onChange({ ...value, grant })} />
    <Checkbox checked={value.limited} onChange={event => onChange({ ...value, limited: event.target.checked })}>Set a permission limit</Checkbox>
    {value.limited && <Card size="small" title="Permission limit" style={{ marginTop: 12 }}>
      <Typography.Paragraph type="secondary">A ceiling on permissions. It must include every grant above; parent limits may restrict access further. Turning this off removes this policy’s ceiling.</Typography.Paragraph>
      <PermissionFields limit value={value.limit} onChange={limit => onChange({ ...value, limit })} />
    </Card>}
  </>;
}
export function GitFields({ value, onChange }: { value: GitDraft[]; onChange: (value: GitDraft[]) => void }) {
  return <>
    <Typography.Paragraph type="secondary">Choose the repositories this profile connects to. Credentials reference a named secret; its value stays in Secrets.</Typography.Paragraph>
    {value.map((entry, index) => {
      const label = `Destination ${index + 1}`;
      const update = (next: GitDraft) => onChange(value.map(row => row.id === entry.id ? next : row));
      return <Card key={entry.id} size="small" title={label} style={{ marginBottom: 16 }}>
        <Form.Item label={`${label} URL`}><Input aria-label={`${label} URL`} placeholder="https://github.com/your-org/" value={entry.url} onChange={event => update({ ...entry, url: event.target.value })} /></Form.Item>
        <Form.Item label={`${label} — rewrite from`} help="Optional: one repository URL or SSH prefix per line to route to this destination."><Input.TextArea aria-label={`${label} — rewrite from`} rows={2} placeholder="git@github.com:your-org/" value={entry.rewrites} onChange={event => update({ ...entry, rewrites: event.target.value })} /></Form.Item>
        <Form.Item label={`${label} — username`}><Input aria-label={`${label} — username`} value={entry.username} onChange={event => update({ ...entry, username: event.target.value })} placeholder="Optional, e.g. x-access-token" /></Form.Item>
        <Form.Item label={`${label} — credential secret name`} help="Supply both a username and secret name, or leave both empty."><Input aria-label={`${label} — credential secret name`} value={entry.secret} onChange={event => update({ ...entry, secret: event.target.value })} placeholder="e.g. github_token" /></Form.Item>
        <Button danger onClick={() => onChange(value.filter(row => row.id !== entry.id))}>Remove destination {index + 1}</Button>
      </Card>;
    })}
    <Button onClick={() => onChange([...value, newDestination()])}>Add repository destination</Button>
  </>;
}
const trustLabel = (value?: Trust) => value?.mode === 'candidate' ? 'Any candidate code' : value?.mode === 'commit' ? `Only commit ${value.commit}` : '';
function PermissionSummary({ value = {}, trust }: { value?: Permissions; trust?: HookPolicy['source_trust'] }) {
  return <div style={{ overflowWrap: 'anywhere' }}>
    <Typography.Paragraph><strong>Secret access:</strong> {!value.secrets?.resolve?.length ? 'None' : null}</Typography.Paragraph>
    {!!value.secrets?.resolve?.length && <ul>{value.secrets.resolve.map(name => <li key={name}>{name}{trust ? ` — ${trustLabel(trust['jobdb.secrets.resolve']?.[name])}` : ''}</li>)}</ul>}
    <Typography.Paragraph><strong>Submit jobs:</strong> {value.jobs?.submit ? `Allowed · ${value.jobs.cellNames?.join(', ') || 'Any permitted cell'}${trust ? ` · ${trustLabel(trust['jobdb.jobs.submit'])}` : ''}` : 'Not allowed'}</Typography.Paragraph>
    <Typography.Paragraph><strong>Create cells:</strong> {value.cells?.create ? `Allowed · ${value.cells.cellNames?.join(', ') || 'Any permitted cell'}${trust ? ` · ${trustLabel(trust['jobdb.cells.create'])}` : ''}` : 'Not allowed'}</Typography.Paragraph>
  </div>;
}
export function HookSummary({ value }: { value: HookPolicy }) {
  return <div style={{ marginTop: 12 }}>
    <PermissionSummary value={value.perms.jobdb} trust={value.source_trust} />
    {value.perms.limit?.jobdb !== undefined ? <Card size="small" title="Permission limit"><PermissionSummary value={value.perms.limit.jobdb} /></Card> : <Typography.Text type="secondary">No additional permission limit.</Typography.Text>}
  </div>;
}
export function GitSummary({ value }: { value: GitProfile }) {
  return <Space direction="vertical" style={{ width: '100%', marginTop: 12, overflowWrap: 'anywhere' }}>
    {value.entries.map((entry, index) => <div key={index}>
      <Typography.Paragraph><strong>Repository destination:</strong> {entry.url}</Typography.Paragraph>
      {!!entry.rewrite_from?.length && <><Typography.Text strong>Rewrite from</Typography.Text><ul>{entry.rewrite_from.map(source => <li key={source}>{source}</li>)}</ul></>}
      <Typography.Paragraph>{entry.secret_ref ? <>Username: {entry.username} · Credential secret: {entry.secret_ref}</> : 'No credentials configured'}</Typography.Paragraph>
    </div>)}
  </Space>;
}
