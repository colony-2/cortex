import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Empty, Input, List, Modal, Select, Space, Switch, Typography, message } from 'antd';
import { Link, useNavigate } from 'react-router-dom';
import { API_BASE, getRestartPoints, parseJobJSON, stringifyJobJSON, type RestartContext, type RestartScope } from '@colony2/shared';
import RestartContextPreview from './RestartContextPreview';
import JobActionDialog from './JobActionDialog';
import { buildContextPatch, type PatchLine, type ScopePatchLine } from '../utils/restartPatch';
import type { StoryNode } from '../utils/storyTree';
import { confirmsRecoveryCompletion, type JobStatusInput } from '../utils/jobStatus';
import { createRequestId as newId } from '@colony2/shared/cellProtocol';
import dayjs from 'dayjs';

const { Text } = Typography;
const scopeKey = (scope: RestartScope) => JSON.stringify([scope.scope_id, scope.chapter_ordinal]);

export default function RestartJobDialog({ projectId, jobId, node: restartNode, job, returnTo, filterSearch = '', sourceError, onRefresh, onClose }: {
  projectId: string; jobId: string; node?: StoryNode | null; job?: JobStatusInput; returnTo?: string; filterSearch?: string; sourceError?: string;
  onRefresh?: () => Promise<unknown>; onClose: () => void;
}) {
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const navigate = useNavigate();
  const restartBusy = useRef(false);
  const [restartScopes, setPoints] = useState<RestartScope[]>();
  const [pointsError, setPointsError] = useState('');
  const [read, retry] = useState(0);
  useEffect(() => {
    const abort = new AbortController();
    setPointsError('');
    getRestartPoints(projectId, jobId, abort.signal).then(points => {
      if (!abort.signal.aborted) setPoints(points);
    }).catch(error => { if (!abort.signal.aborted) setPointsError(String(error)); });
    return () => abort.abort();
  }, [projectId, jobId, read]);
  const [restartSelection, setRestartSelection] = useState<string>();
  const [restartSubmitting, setRestartSubmitting] = useState(false);
  const [drafts, setDrafts] = useState<Record<string, { job: PatchLine[]; scope: ScopePatchLine[] }>>({});
  const [inspection, setInspection] = useState<{ key: string; snapshot?: RestartContext }>();
  const [applyPatch, setApplyPatch] = useState(false);

  const storageKey = `json-patch-v2:cortex:restart:${projectId}:${jobId}`;
  type SavedRestart = { payload: string; request: unknown };
  const [savedRestart, setSavedRestart] = useState<SavedRestart | null>(() => {
    try {
      const value = sessionStorage.getItem(storageKey);
      const saved = value ? parseJobJSON(value) as SavedRestart : null;
      return saved && typeof saved.payload === 'string' && saved.request && typeof saved.request === 'object' && (saved.request as {goal?: {mode?: string}}).goal?.mode === 'new' ? saved : null;
    } catch { return null; }
  });
  const [restartError, setRestartError] = useState<string>();
  const [cancelSource, setCancelSource] = useState(false);
  const [cancellationBusy, setCancellationBusy] = useState(false);
  const [cancellationResult, setCancellationResult] = useState<string>();
  const [cancellationError, setCancellationError] = useState<string>();

  const initialScope = restartNode?.restart_scope_id;
  const initialOrdinal = restartNode?.restart_chapter_ordinal ?? (initialScope ? undefined : 0);
  const restartScope = restartSelection ? restartScopes?.find(scope => scopeKey(scope) === restartSelection)
    : restartScopes?.find(scope => initialScope ? scope.scope_id === initialScope &&
        (restartNode?.restart_chapter_ordinal == null || scope.chapter_ordinal === initialOrdinal) : scope.chapter_ordinal === initialOrdinal);
  const restartChapterOrdinal = restartScope?.chapter_ordinal ?? initialOrdinal;
  const selectedScopeID = restartScope?.scope_id ?? initialScope;
  const restartScopeID = restartScopes && restartScopes.filter(point => point.scope_id === selectedScopeID).length > 1
    ? undefined : selectedScopeID;
  const submissionRestart = restartChapterOrdinal === 0;
  const restartable = !!restartScope;
  const draftKey = JSON.stringify([projectId, jobId, restartScopeID, restartChapterOrdinal]);
  const jobPatchLines = drafts[draftKey]?.job || [];
  const scopePatchLines = drafts[draftKey]?.scope || [];
  const setJobPatchLines = (update: PatchLine[] | ((lines: PatchLine[]) => PatchLine[])) => setDrafts(prev => {
    const draft = prev[draftKey] || { job: [], scope: [] };
    return { ...prev, [draftKey]: { ...draft, job: typeof update === 'function' ? update(draft.job) : update } };
  });
  const setScopePatchLines = (update: ScopePatchLine[] | ((lines: ScopePatchLine[]) => ScopePatchLine[])) => setDrafts(prev => {
    const draft = prev[draftKey] || { job: [], scope: [] };
    return { ...prev, [draftKey]: { ...draft, scope: typeof update === 'function' ? update(draft.scope) : update } };
  });
  const onSnapshot = useCallback((snapshot?: RestartContext) => setInspection({ key: draftKey, snapshot }), [draftKey]);
  let generatedPatch: ReturnType<typeof buildContextPatch> | undefined;
  let patchError: string | undefined;
  const patchRequested = applyPatch && !submissionRestart;
  const snapshot = inspection?.key === draftKey ? inspection.snapshot : undefined;
  if (patchRequested && snapshot) {
    try { generatedPatch = buildContextPatch(snapshot, jobPatchLines, scopePatchLines); }
    catch (error) { patchError = error instanceof Error ? error.message : 'Cannot generate patch'; }
  }


  const sendRestart = async (saved: SavedRestart) => {
    const response = await fetch(
      `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/restart`,
      { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: stringifyJobJSON(saved.request) }
    );
    if (!response.ok) {
      const diagnostic = await response.text().catch(() => 'Unknown error');
      throw new Error(`${response.status} ${response.statusText}${diagnostic ? ` - ${diagnostic}` : ''}`);
    }
    const data = (await response.json()) as { job_id: string };
    if (!alive.current) return;
    sessionStorage.removeItem(storageKey);
    setSavedRestart(null);
    message.success('Restarted job');
    onClose();
    navigate(`/project/${projectId}/jobs/${data.job_id}/story${filterSearch}`);
  };

  const submitRestart = async (replay = false) => {
    if (restartBusy.current) return;
    if (!replay && patchRequested && !snapshot) { setRestartError('Load the selected restart context before submitting a patch.'); return; }
    if (!replay && patchError) { setRestartError(patchError); return; }
    if (replay && !savedRestart) return;
    restartBusy.current = true;
    setRestartSubmitting(true);
    setRestartError(undefined);
    try {
      if (replay) {
        await sendRestart(savedRestart!);
      } else {
        const payload = { ...(restartScopeID ? { scope_id: restartScopeID } : { chapter_ordinal: restartChapterOrdinal }), goal: { mode: 'new' }, ...(generatedPatch ? { patch: generatedPatch } : {}) };
        const serialized = stringifyJobJSON(payload)!;
        if (savedRestart?.payload === serialized) { await sendRestart(savedRestart); return; }
        if (!alive.current) return;
        const saved = { payload: serialized, request: { ...payload, request_id: newId() } };
        sessionStorage.setItem(storageKey, stringifyJobJSON(saved)!);
        setSavedRestart(saved);
        await sendRestart(saved);
      }
    } catch (e) {
      if (alive.current) setRestartError(e instanceof Error ? e.message : 'Restart failed');
    } finally {
      restartBusy.current = false;
      if (alive.current) setRestartSubmitting(false);
    }
  };

  const sourceFinished = confirmsRecoveryCompletion(job);
  const loadMetadata = async () => { await onRefresh?.(); };
  return <>
      <Modal
        title="Restart Job"
        open
        onCancel={() => {
          if (restartBusy.current || cancellationBusy) return;
          onClose();
        }}
        width={900}
        okText="Restart"
        okButtonProps={{ 'aria-label': 'Restart', 'aria-busy': restartSubmitting }}
        onOk={() => void submitRestart()}
      >
        <Space direction="vertical" style={{ width: '100%', marginBottom: 16 }}>
          {sourceError && <Alert type="info" message="Source details unavailable" description={sourceError} />}
          {cancellationResult && <Alert type="info" message={cancellationResult} description={sourceFinished ? 'Source completion is now confirmed.' : 'Cancellation remains pending until fresh job detail confirms completion.'} />}
          {cancellationError && <Alert type="error" message="Source cancellation failed" description={cancellationError} />}
          {restartError && <Alert type="error" message="Restart failed" description={restartError} />}
          <Space wrap>
            {onRefresh && <Button disabled={restartSubmitting || cancellationBusy} onClick={() => void loadMetadata()}>Refresh source</Button>}
            {!sourceFinished && <Button disabled={restartSubmitting || cancellationBusy} onClick={() => setCancelSource(true)}>Cancel source</Button>}
            {returnTo && <Link to={returnTo}>Back to inbox</Link>}
          </Space>
          {savedRestart && <Card size="small" title="Saved restart request">
            <Typography.Paragraph>The following saved request will be retried unchanged, independently of the current editor and source completion.</Typography.Paragraph>
            <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 180, overflow: 'auto' }}>{stringifyJobJSON(savedRestart.request)}</pre>
            <Button onClick={() => void submitRestart(true)}>Retry saved request</Button>
          </Card>}
        </Space>
        {restartScopes ? (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Alert
              type="info"
              showIcon
              message="Restart as a new goal"
              description={
                <div>
                  {submissionRestart
                    ? 'Restart from the original submission and inputs. Setup and recipe resolution run again, so floating recipe references may resolve to a newer version. The source job is not cancelled.'
                    : 'Choose where to resume execution. Completed work before that point is retained. The source job is not cancelled. A new patch replaces any previous patch at this same restart point.'}
                </div>
              }
            />

            {!restartScopes.length && <Alert type="info" message="No recorded restart points are available." />}
            {!restartable && restartNode && <Alert type="info" showIcon
              message={restartNode?.restart_unavailable_reason || 'No recorded restart point is available for this node.'}
              description="Choose a restart point below." />}
            {restartChapterOrdinal === restartNode?.restart_chapter_ordinal && restartNode?.restart_relationship === 'containing' &&
              <Alert type="info" showIcon message="This restarts the containing scope, including its work before this task." />}
            {restartChapterOrdinal === restartNode?.restart_chapter_ordinal && restartNode?.restart_relationship === 'body' &&
              <Text type="secondary">Restart from the recipe’s executable body.</Text>}
            <Select
              aria-label="Restart point"
              placeholder="Choose a restart point"
              style={{ width: '100%' }}
              value={restartScope ? scopeKey(restartScope) : undefined}
              disabled={restartSubmitting || cancellationBusy} onChange={setRestartSelection}
              options={restartScopes.map(scope => ({
                value: scopeKey(scope),
                label: `${scope.title || scope.path?.join(" / ") || "Restart point"} · Chapter ${scope.chapter_ordinal} · ${scope.started_at ? dayjs(scope.started_at).format('YYYY-MM-DD HH:mm:ss') : '-'}`,
              }))}
              showSearch
              optionFilterProp="label"
            />

            <Card title="Context Patching" size="small"><fieldset disabled={restartSubmitting || cancellationBusy} style={{ border: 0, padding: 0, margin: 0 }}>
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                  <Text type="secondary">
                    Apply additional context over the existing context before resuming execution.
                  </Text>
                  <Space>
                    <Text type="secondary">Enable</Text>
                    <Switch checked={applyPatch && !submissionRestart} disabled={submissionRestart} onChange={setApplyPatch} />
                  </Space>
                </Space>

                {submissionRestart ? (
                  <Text type="secondary">Submission context is read-only. No context patch will be applied.</Text>
                ) : !applyPatch ? (
                  <Text type="secondary">No context patch will be applied.</Text>
                ) : (
                  <Space direction="vertical" style={{ width: '100%' }} size="large">
                    <RestartContextPreview onSnapshot={onSnapshot} projectId={projectId} jobId={jobId} scopeId={restartScopeID} chapterOrdinal={restartable && !restartScopeID ? restartChapterOrdinal : undefined} />
                    <Card
                      size="small"
                      title="Job Patch (global)"
                      extra={
                        <Button
                          size="small"
                          onClick={() =>
                            setJobPatchLines((prev) => [
                              ...prev,
                              { id: newId(), path: '', value: '' },
                            ])
                          }
                        >
                          Add line
                        </Button>
                      }
                    >
                      <Text type="secondary">
                        Each line edits a value under <Text code>context</Text>. Use dot
                        paths like <Text code>git.author</Text>. Values accept JSON (e.g.{' '}
                        <Text code>"james@example.com"</Text>, <Text code>true</Text>,{' '}
                        <Text code>123</Text>, <Text code>null</Text>) or raw text.
                      </Text>
                      <div style={{ marginTop: 12 }}>
                        {jobPatchLines.length === 0 ? (
                          <Empty description="No job patch lines" />
                        ) : (
                          <List
                            size="small"
                            dataSource={jobPatchLines}
                            renderItem={(line) => (
                              <List.Item
                                actions={[
                                  <Button
                                    key="remove"
                                    size="small"
                                    onClick={() =>
                                      setJobPatchLines((prev) => prev.filter((p) => p.id !== line.id))
                                    }
                                  >
                                    Remove
                                  </Button>,
                                ]}
                              >
                                <Space style={{ width: '100%' }}>
                                  <Text style={{ width: 80 }} type="secondary">
                                    Path
                                  </Text>
                                  <Input
                                    style={{ flex: 1, padding: 6 }}
                                    value={line.path}
                                    placeholder='e.g. git.author'
                                    onChange={(e) =>
                                      setJobPatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, path: e.target.value } : p))
                                      )
                                    }
                                  />
                                  <Text style={{ width: 60 }} type="secondary">
                                    Value
                                  </Text>
                                  <Input
                                    style={{ flex: 1, padding: 6 }}
                                    value={line.value}
                                    placeholder='e.g. "james@example.com"'
                                    onChange={(e) =>
                                      setJobPatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, value: e.target.value } : p))
                                      )
                                    }
                                  />
                                </Space>
                              </List.Item>
                            )}
                          />
                        )}
                      </div>
                    </Card>

                    <Card
                      size="small"
                      title="Scoped Patches (local outputs)"
                      extra={
                        <Button
                          size="small"
                          onClick={() =>
                            setScopePatchLines((prev) => [
                              ...prev,
                              {
                                id: newId(),
                                container: 'sequence',
                                scopeId: '',
                                path: '',
                                value: '',
                              },
                            ])
                          }
                        >
                          Add line
                        </Button>
                      }
                    >
                      <Text type="secondary">
                        Each line edits a value under a scope container’s outputs.
                        Container is <Text code>sequence</Text> or <Text code>states</Text>; id is the
                        step/state id; path is under outputs (dot path).
                      </Text>
                      <div style={{ marginTop: 12 }}>
                        {scopePatchLines.length === 0 ? (
                          <Empty description="No scoped patch lines" />
                        ) : (
                          <List
                            size="small"
                            dataSource={scopePatchLines}
                            renderItem={(line) => (
                              <List.Item
                                actions={[
                                  <Button
                                    key="remove"
                                    size="small"
                                    onClick={() =>
                                      setScopePatchLines((prev) => prev.filter((p) => p.id !== line.id))
                                    }
                                  >
                                    Remove
                                  </Button>,
                                ]}
                              >
                                <Space style={{ width: '100%' }} wrap>
                                  <Text style={{ width: 80 }} type="secondary">
                                    Container
                                  </Text>
                                  <Select
                                    value={line.container}
                                    style={{ width: 140 }}
                                    options={[
                                      { label: 'sequence', value: 'sequence' },
                                      { label: 'states', value: 'states' },
                                    ]}
                                    onChange={(v) =>
                                      setScopePatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, container: v } : p))
                                      )
                                    }
                                  />
                                  <Text style={{ width: 30 }} type="secondary">
                                    ID
                                  </Text>
                                  <Input
                                    style={{ width: 160 }}
                                    value={line.scopeId}
                                    placeholder='e.g. build'
                                    onChange={(e) =>
                                      setScopePatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, scopeId: e.target.value } : p))
                                      )
                                    }
                                  />
                                  <Text style={{ width: 40 }} type="secondary">
                                    Path
                                  </Text>
                                  <Input
                                    style={{ width: 200 }}
                                    value={line.path}
                                    placeholder='e.g. image_tag'
                                    onChange={(e) =>
                                      setScopePatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, path: e.target.value } : p))
                                      )
                                    }
                                  />
                                  <Text style={{ width: 50 }} type="secondary">
                                    Value
                                  </Text>
                                  <Input
                                    style={{ flex: 1, minWidth: 180 }}
                                    value={line.value}
                                    placeholder='e.g. "v2"'
                                    onChange={(e) =>
                                      setScopePatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, value: e.target.value } : p))
                                      )
                                    }
                                  />
                                </Space>
                              </List.Item>
                            )}
                          />
                        )}
                      </div>
                    </Card>

                    <Card size="small" title="Patch payload">
                      {patchError ? <Alert type="error" message={patchError} /> : <pre aria-label="Patch payload">{stringifyJobJSON(generatedPatch, 2)}</pre>}
                    </Card>
                  </Space>
                )}
              </Space>
            </fieldset></Card>
          </Space>
        ) : (
          pointsError ? <Alert type="error" message="Could not load restart points" description={pointsError} action={<Button onClick={() => retry(n => n + 1)}>Retry points</Button>} /> : <p role="status">Loading restart points…</p>
        )}
      </Modal>
      {cancelSource && <JobActionDialog projectId={projectId} jobIds={[jobId]} action="cancel" job={job}
        onBusyChange={setCancellationBusy}
        onFailure={failures => { if (alive.current) { setCancellationError(failures.map(failure => failure.error).join('; ')); void loadMetadata(); } }}
        onClose={() => { if (alive.current) { setCancelSource(false); void loadMetadata(); } }}
        onComplete={async () => {
          if (!alive.current) return;
          setCancellationError(undefined);
          setCancellationResult('Cancellation requested for this source');
          await loadMetadata();
        }} />}

  </>;
}
