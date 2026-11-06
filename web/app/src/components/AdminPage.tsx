import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Alert, Button, Empty, Form, Input, Modal, Space, Spin, Tabs, Tag, Typography } from 'antd';
import { getAdmin, updateAdmin } from '@colony2/shared';
import type { AdminEntry, AdminKind, AdminMutation, AdminSnapshot } from '@colony2/shared';

import { GitFields, GitSummary, HookFields, HookSummary } from './admin/StructuredSettings';
import { gitDraft, gitValue, hookDraft, hookValue } from './admin/models';
import type { GitProfile, HookPolicy } from './admin/models';

const labels: Record<AdminKind, string> = { hooks: 'Hook permissions', configs: 'Configs', git: 'Git', secrets: 'Secrets' };
const kinds = Object.keys(labels) as AdminKind[];


export default function AdminPage({ projectId }: { projectId: string }) {
  return <div className="app-page" style={{ padding: 24 }}>
    <Typography.Title level={3}>Admin</Typography.Title>
    <Typography.Paragraph type="secondary">Manage settings for tenant {projectId}.</Typography.Paragraph>
    <AdminTabs key={projectId} projectId={projectId} />
  </div>;
}

export function AdminTabs({ projectId, cellId }: { projectId: string; cellId?: string }) {
  const [search, setSearch] = useSearchParams();
  const selected = search.get('tab');
  const active = kinds.includes(selected as AdminKind) ? selected as AdminKind : 'hooks';
  // A cell's overview makes no admin request until one of its settings tabs is opened.
  return <Tabs activeKey={active} onChange={tab => setSearch(previous => { const next = new URLSearchParams(previous); next.set('tab', tab); return next; })}
    items={kinds.map(kind => ({ key: kind, label: labels[kind], children: <AdminPanel key={`${projectId}/${cellId || ''}/${kind}`} projectId={projectId} cellId={cellId} kind={kind} /> }))} destroyOnHidden />;
}

export function AdminPanel({ projectId, cellId, kind }: { projectId: string; cellId?: string; kind: AdminKind }) {
  const [data, setData] = useState<AdminSnapshot>();
  const [error, setError] = useState<string>();
  const [reload, setReload] = useState(0);
  const [editor, setEditor] = useState<{ entry?: AdminEntry }>();
  const [pending, setPending] = useState<AdminMutation>();
  const [busy, setBusy] = useState(false);
  const [success, setSuccess] = useState<string>();
  const readOnly = !!cellId && kind !== 'hooks';
  useEffect(() => {
    let current = true;
    setError(undefined); setData(undefined);
    getAdmin(projectId, cellId).then(value => { if (current) setData(value); }).catch(reason => { if (current) setError(reason.message); });
    return () => { current = false; };
  }, [projectId, cellId, reload]);
  const refresh = () => { setSuccess(undefined); setReload(value => value + 1); };
  const change = async (input: AdminMutation) => {
    await updateAdmin(projectId, kind, { ...input, ...(kind !== 'secrets' ? { revision: data!.revision } : {}) }, cellId);
    setSuccess(kind === 'secrets' ? 'Secret update accepted. Stored values are never displayed.' : 'Settings saved.');
    setReload(value => value + 1);
  };
  const remove = async () => {
    setBusy(true);
    try { await change(pending!); setPending(undefined); }
    catch (reason) { setError((reason as Error).message); }
    finally { setBusy(false); }
  };
  const entries: AdminEntry[] = data ? kind === 'secrets' ? data.secretReferences.map(name => ({ name })) : data[kind] : [];
  return <Space direction="vertical" size="middle" style={{ width: '100%' }}>
    {readOnly && <Alert type="info" showIcon message="These settings are tenant-wide" description={<Link to={`/project/${encodeURIComponent(projectId)}/admin?tab=${kind}`}>Manage {labels[kind].toLowerCase()} in Admin</Link>} />}
    {kind === 'hooks' && <Typography.Paragraph type="secondary">{cellId
      ? 'Cell overrides replace tenant defaults for each hook. Clear permissions denies all grants; use tenant default removes the override. Policies follow the cell’s repository.'
      : 'Tenant defaults apply to hooks unless a cell supplies an override. Open a cell to manage its overrides.'}</Typography.Paragraph>}
    {kind === 'configs' && <Typography.Paragraph type="secondary">Named JSON values available to recipes. Keep credentials in Secrets. Git defaults use git-profile-ro and git-profile-rw.</Typography.Paragraph>}
    {kind === 'git' && <Typography.Paragraph type="secondary">Git profiles route repository URLs and reference secrets by name. Profiles named read and write are used by default unless overridden in Configs.</Typography.Paragraph>}
    {kind === 'secrets' && <Alert type="info" showIcon message="Secret values are never displayed" description="JobDB cannot list stored secrets yet. The names below are references from hook permissions and Git profiles, not an inventory or proof that a secret exists. Add or replace a value by name in JobDB’s writable secret store; external providers are managed separately." />}
    {success && <Alert type="success" message={success} showIcon />}
    {error && <Alert type="error" message={error} showIcon action={<Button onClick={refresh}>Refresh settings</Button>} />}
    {!data && !error && <Spin />}
    {data && <>
      <Space wrap>
        <Button onClick={refresh}>Refresh</Button>
        {!readOnly && <Button type="primary" disabled={kind === 'secrets' && !data.secretWrites} onClick={() => setEditor({})}>{kind === 'secrets' ? 'Add or replace secret' : `Add ${kind === 'hooks' ? 'hook policy' : kind === 'git' ? 'Git profile' : 'config'}`}</Button>}
        {kind === 'secrets' && !readOnly && <Button disabled={!data.secretWrites} onClick={() => setPending({ name: '', operation: 'remove' })}>Remove secret by name</Button>}
      </Space>
      {kind === 'secrets' && !data.secretWrites && <Alert type="info" message="Secret writes are unavailable on this connection." />}
      {entries.length === 0 ? <Empty description={kind === 'secrets' ? 'No referenced secret names' : 'No settings configured'} /> : entries.map(entry => <div key={entry.name} style={{ border: '1px solid #d9d9d9', borderRadius: 8, padding: 16, minWidth: 0 }}>
        <Space wrap><Typography.Text strong style={{ overflowWrap: 'anywhere' }}>{entry.name}</Typography.Text>
          {entry.source && <Tag>{entry.source === 'cell' ? 'Cell override' : 'Tenant'}</Tag>}
        </Space>
        {kind === 'hooks' && <HookSummary value={entry.value as HookPolicy} />}
        {kind === 'git' && <GitSummary value={entry.value as GitProfile} />}
        {kind === 'configs' && <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 240, overflow: 'auto' }}>{JSON.stringify((entry as AdminEntry).value, null, 2)}</pre>}
        {!readOnly && <Space wrap style={{ marginTop: 8 }}>
          <Button disabled={kind === 'secrets' && !data.secretWrites} onClick={() => setEditor({ entry })}>{kind === 'secrets' ? 'Replace value' : 'Edit'}</Button>
          {kind === 'hooks' && <Button onClick={() => setPending({ name: entry.name, operation: 'clear' })}>Clear permissions</Button>}
          {!(kind === 'hooks' && cellId && (entry as AdminEntry).source !== 'cell') && <Button danger disabled={kind === 'secrets' && !data.secretWrites} onClick={() => setPending({ name: entry.name, operation: 'remove' })}>{kind === 'hooks' && cellId ? 'Use tenant default' : 'Remove'}</Button>}
        </Space>}
      </div>)}
    </>}
    {editor && <AdminEditor kind={kind} entry={editor.entry} onClose={() => setEditor(undefined)} onSave={async input => {
      if (!editor.entry && kind !== 'secrets' && entries.some(entry => entry.name === input.name)) throw new Error('This name already exists. Use Edit to change it.');
      await change(input);
    }} />}
    {pending && <Modal open title={pending.operation === 'clear' ? 'Clear hook permissions' : kind === 'hooks' && cellId ? 'Use tenant default' : 'Remove setting'} okText="Confirm" okButtonProps={{ danger: true, disabled: !pending.name.trim() }} confirmLoading={busy} onOk={remove} onCancel={() => { if (!busy) setPending(undefined); }} closable={!busy} maskClosable={!busy}>
      <Typography.Paragraph>{pending.operation === 'clear' ? 'This sets an explicit empty policy. The hook will receive no grants from this policy.' : kind === 'hooks' && cellId ? 'Remove the cell override and use the tenant’s permissions for this hook.' : 'Remove this setting? Jobs that depend on it may be affected.'}</Typography.Paragraph>
      {kind === 'secrets' ? <Input aria-label="Secret name to remove" value={pending.name} onChange={event => setPending({ ...pending, name: event.target.value })} /> : <Typography.Text code>{pending.name}</Typography.Text>}
    </Modal>}
  </Space>;
}

function AdminEditor({ kind, entry, onClose, onSave }: { kind: AdminKind; entry?: AdminEntry; onClose: () => void; onSave: (input: AdminMutation) => Promise<void> }) {
  const [name, setName] = useState(entry?.name || '');
  const [value, setValue] = useState(kind === 'configs' ? JSON.stringify(entry ? entry.value : 'value', null, 2) : '');
  const [hook, setHook] = useState(() => hookDraft(kind === 'hooks' ? entry?.value as HookPolicy | undefined : undefined));
  const [git, setGit] = useState(() => gitDraft(kind === 'git' ? entry?.value as GitProfile | undefined : undefined));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const secret = kind === 'secrets';
  const save = async () => {
    let parsed: unknown;
    try { parsed = secret ? value : kind === 'hooks' ? hookValue(hook) : kind === 'git' ? gitValue(git) : JSON.parse(value); }
    catch (reason) { setError(kind === 'configs' ? 'Enter valid JSON.' : (reason as Error).message); return; }
    setBusy(true); setError(undefined);
    // Keep secret drafts only in this mounted form, and discard after any send.
    if (secret) setValue('');
    try { await onSave({ name, operation: 'set', value: parsed }); onClose(); }
    catch (reason) { setError((reason as Error).message); }
    finally { setBusy(false); }
  };
  return <Modal open title={secret ? 'Add or replace secret' : `${entry ? 'Edit' : 'Add'} ${labels[kind].toLowerCase()}`} width={720} okText="Save" onOk={save} confirmLoading={busy} okButtonProps={{ disabled: !name.trim() || (secret && !value) }} onCancel={() => { if (!busy) onClose(); }} closable={!busy} maskClosable={!busy}>
    {error && <Alert type="error" showIcon message={error} />}
    <Form layout="vertical" disabled={busy}>
      <Form.Item label={kind === 'hooks' ? 'Hook path' : 'Name'}><Input aria-label={kind === 'hooks' ? 'Hook path' : 'Name'} value={name} disabled={!!entry || busy} onChange={event => setName(event.target.value)} placeholder={kind === 'hooks' ? '.c2/hooks/build.yaml' : undefined} /></Form.Item>
      {kind === 'hooks' ? <HookFields value={hook} onChange={setHook} /> : kind === 'git' ? <GitFields value={git} onChange={setGit} /> : <Form.Item label={secret ? 'New secret value' : 'Value (JSON)'}>{secret
        ? <Input.Password aria-label="New secret value" autoComplete="new-password" visibilityToggle={false} value={value} disabled={busy} onChange={event => setValue(event.target.value)} />
        : <Input.TextArea aria-label="Value (JSON)" rows={12} value={value} disabled={busy} onChange={event => setValue(event.target.value)} style={{ fontFamily: 'monospace' }} />}</Form.Item>}
    </Form>
    {secret && <Typography.Paragraph type="secondary">The existing value is never retrieved. Saving replaces it. The value is discarded from this form after submission.</Typography.Paragraph>}
  </Modal>;
}
