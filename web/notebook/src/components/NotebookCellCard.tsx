import { Button, Card, Collapse, Divider, Dropdown, Input, InputNumber, Select, Space, Switch, Tag, Typography } from 'antd';
import type { MenuProps } from 'antd';
import { CaretDownOutlined, CaretRightOutlined, LoadingOutlined, PlayCircleOutlined, PlusOutlined } from '@ant-design/icons';
import type { NotebookCell, NotebookCellKind, OpType } from '../model/notebookModel';
import { kindIcon, kindLabel } from '../ui/kind';
import { statusTagColor } from '../ui/status';

const { Text } = Typography;

function cellStatus(cell: NotebookCell): string | null {
  return cell.storyNode?.status ?? cell.runtime?.status ?? null;
}

function cellOutput(cell: NotebookCell): unknown {
  if (cell.storyNode) return cell.storyNode.output ?? null;
  return cell.runtime?.output ?? null;
}

function toPrettyJson(value: unknown): string {
  if (value === null || value === undefined) return '';
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function opTypeOptions(): Array<{ label: string; value: OpType }> {
  return [
    { value: 'cells.list', label: 'cells.list' },
    { value: 'codex.exec', label: 'codex.exec' },
    { value: 'command_execution', label: 'command_execution' },
    { value: 'input', label: 'input' },
    { value: 'llm_inference2', label: 'llm_inference2' },
    { value: 'sleep', label: 'sleep' },
  ];
}

function allowedChildKinds(kind: NotebookCellKind): NotebookCellKind[] {
  switch (kind) {
    case 'recipe':
      return ['markdown', 'sequence', 'stateMachine', 'op'];
    case 'sequence':
      return ['op', 'sequence', 'stateMachine'];
    case 'stateMachine':
      return ['state'];
    case 'state':
      return ['op', 'sequence', 'stateMachine'];
    case 'markdown':
    case 'op':
    default:
      return [];
  }
}

function addMenuItems(kinds: NotebookCellKind[]): MenuProps['items'] {
  return kinds.map((k) => ({ key: k, label: `Add ${kindLabel(k)}` }));
}

export default function NotebookCellCard(props: {
  cell: NotebookCell;
  depth: number;
  selected: boolean;
  editable?: boolean;
  runnable?: boolean;
  onSelect: () => void;
  onToggleCollapse?: () => void;
  onRun?: () => void;
  onAddChild?: (kind: NotebookCellKind) => void;
  onUpdate?: (next: NotebookCell) => void;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
  onDelete?: () => void;
}): JSX.Element {
  const { cell, depth, selected, editable, runnable, onSelect, onToggleCollapse, onRun, onAddChild, onUpdate, onMoveUp, onMoveDown, onDelete } = props;

  const status = cellStatus(cell);
  const output = cellOutput(cell);

  const canHaveChildren = cell.children.length > 0 || allowedChildKinds(cell.kind).length > 0;
  const caret = cell.collapsed ? <CaretRightOutlined /> : <CaretDownOutlined />;

  const addableKinds = allowedChildKinds(cell.kind);
  const addMenu: MenuProps = {
    items: addMenuItems(addableKinds),
    onClick: (info) => onAddChild?.(info.key as NotebookCellKind),
  };

  const disabled = editable === false;

  const renderKeyValueFields = (
    fields: Array<{ key: string; value: string }>,
    onChange: (next: Array<{ key: string; value: string }>) => void,
  ) => (
    <Space direction="vertical" size={6} style={{ width: '100%' }}>
      {fields.length === 0 ? <Text type="secondary">No fields</Text> : null}
      {fields.map((f, idx) => (
        <Space key={`${cell.id}-kv-${idx}`} wrap style={{ width: '100%' }}>
          <Input
            placeholder="field"
            value={f.key}
            disabled={disabled}
            style={{ width: 180 }}
            onChange={(e) => {
              const next = fields.slice();
              next[idx] = { ...f, key: e.target.value };
              onChange(next);
            }}
          />
          <Input
            placeholder="value"
            value={f.value}
            disabled={disabled}
            style={{ flex: 1, minWidth: 240 }}
            onChange={(e) => {
              const next = fields.slice();
              next[idx] = { ...f, value: e.target.value };
              onChange(next);
            }}
          />
          {!disabled ? (
            <Button
              danger
              size="small"
              onClick={() => {
                const next = fields.slice();
                next.splice(idx, 1);
                onChange(next);
              }}
            >
              Remove
            </Button>
          ) : null}
        </Space>
      ))}
      {!disabled ? (
        <Button size="small" onClick={() => onChange(fields.concat([{ key: '', value: '' }]))}>
          Add field
        </Button>
      ) : null}
    </Space>
  );

  const statusAccent =
    status === 'running'
      ? '#1677ff'
      : status === 'failed'
        ? '#ff4d4f'
        : status === 'succeeded'
          ? '#52c41a'
          : status === 'pending'
            ? '#d9d9d9'
            : undefined;

  const renderEditor = () => {
    switch (cell.kind) {
      case 'markdown':
        return (
          <Input.TextArea
            value={cell.markdown}
            disabled={disabled}
            autoSize={{ minRows: 3, maxRows: 14 }}
            onChange={(e) => onUpdate?.({ ...cell, markdown: e.target.value })}
          />
        );
      case 'recipe':
        return (
          <Space direction="vertical" size={8} style={{ width: '100%' }}>
            <Space>
              <Text type="secondary">Recipe name</Text>
              <Input
                value={cell.name}
                style={{ width: 280 }}
                disabled={disabled}
                onChange={(e) => onUpdate?.({ ...cell, name: e.target.value, title: `recipe: ${e.target.value}` })}
              />
            </Space>
            <Divider style={{ margin: '6px 0' }} />
            <Text type="secondary">Fields</Text>
            {renderKeyValueFields(cell.fields, (next) => onUpdate?.({ ...cell, fields: next }))}
          </Space>
        );
      case 'sequence':
        return (
          <Space direction="vertical" size={8} style={{ width: '100%' }}>
            <Space>
              <Text type="secondary">Sequence</Text>
              <Input
                value={cell.name}
                style={{ width: 280 }}
                disabled={disabled}
                onChange={(e) => onUpdate?.({ ...cell, name: e.target.value, title: `sequence: ${e.target.value}` })}
              />
            </Space>
            <Divider style={{ margin: '6px 0' }} />
            <Text type="secondary">Fields</Text>
            {renderKeyValueFields(cell.fields, (next) => onUpdate?.({ ...cell, fields: next }))}
          </Space>
        );
      case 'stateMachine':
        return (
          <Space direction="vertical" size={8} style={{ width: '100%' }}>
            <Space>
              <Text type="secondary">State machine</Text>
              <Input
                value={cell.machineId}
                style={{ width: 280 }}
                disabled={disabled}
                onChange={(e) =>
                  onUpdate?.({ ...cell, machineId: e.target.value, title: `stateMachine: ${e.target.value}` })
                }
              />
            </Space>
            <Divider style={{ margin: '6px 0' }} />
            <Text type="secondary">Fields</Text>
            {renderKeyValueFields(cell.fields, (next) => onUpdate?.({ ...cell, fields: next }))}
          </Space>
        );
      case 'state':
        return (
          <Space direction="vertical" size={8} style={{ width: '100%' }}>
            <Space>
              <Text type="secondary">State</Text>
              <Input
                value={cell.stateId}
                style={{ width: 280 }}
                disabled={disabled}
                onChange={(e) => onUpdate?.({ ...cell, stateId: e.target.value, title: `state: ${e.target.value}` })}
              />
            </Space>
            <Divider style={{ margin: '4px 0' }} />
            <Text type="secondary">Transitions</Text>
            <Space direction="vertical" size={6} style={{ width: '100%' }}>
              {cell.transitions.map((t, idx) => (
                <Card key={`${cell.id}-t-${idx}`} size="small">
                  <Space wrap>
                    <Select
                      value={t.when}
                      style={{ width: 140 }}
                      disabled={disabled}
                      options={[
                        { value: 'always', label: 'always' },
                        { value: 'expression', label: 'expression' },
                      ]}
                      onChange={(v) => {
                        const next = cell.transitions.slice();
                        next[idx] = { ...t, when: v };
                        onUpdate?.({ ...cell, transitions: next });
                      }}
                    />
                    {t.when === 'expression' ? (
                      <Input
                        value={t.expression ?? ''}
                        placeholder="expression (stub)"
                        style={{ width: 260 }}
                        disabled={disabled}
                        onChange={(e) => {
                          const next = cell.transitions.slice();
                          next[idx] = { ...t, expression: e.target.value };
                          onUpdate?.({ ...cell, transitions: next });
                        }}
                      />
                    ) : null}
                    <Input
                      value={t.toStateId}
                      placeholder="toStateId"
                      style={{ width: 220 }}
                      disabled={disabled}
                      onChange={(e) => {
                        const next = cell.transitions.slice();
                        next[idx] = { ...t, toStateId: e.target.value };
                        onUpdate?.({ ...cell, transitions: next });
                      }}
                    />
                    {!disabled ? (
                      <Button
                        danger
                        size="small"
                        onClick={() => {
                          const next = cell.transitions.slice();
                          next.splice(idx, 1);
                          onUpdate?.({ ...cell, transitions: next });
                        }}
                      >
                        Remove
                      </Button>
                    ) : null}
                  </Space>
                </Card>
              ))}
              {!disabled ? (
                <Button
                  size="small"
                  onClick={() =>
                    onUpdate?.({ ...cell, transitions: cell.transitions.concat([{ when: 'always', toStateId: '' }]) })
                  }
                >
                  Add transition
                </Button>
              ) : null}
            </Space>
          </Space>
        );
      case 'op': {
        const params = cell.params ?? {};
        const setParam = (key: string, value: unknown) => onUpdate?.({ ...cell, params: { ...params, [key]: value } });
        const envValue = params.env;
        const envFields: Array<{ key: string; value: string }> = Array.isArray(envValue)
          ? (envValue as Array<{ key: string; value: string }>)
          : envValue && typeof envValue === 'object' && !Array.isArray(envValue)
            ? Object.entries(envValue as Record<string, unknown>).map(([k, v]) => ({ key: k, value: String(v ?? '') }))
            : [];

        return (
          <Space direction="vertical" size={10} style={{ width: '100%' }}>
            <Space wrap>
              <Text type="secondary">Op type</Text>
              <Select
                value={cell.opType}
                style={{ width: 240 }}
                disabled={disabled}
                options={opTypeOptions()}
                onChange={(v) => onUpdate?.({ ...cell, opType: v })}
              />
            </Space>

            {cell.opType === 'cells.list' ? <Text type="secondary">No inputs</Text> : null}

            {cell.opType === 'sleep' ? (
              <Space wrap>
                <Text type="secondary">duration</Text>
                <Input
                  value={String(params.duration ?? '5s')}
                  style={{ width: 220 }}
                  disabled={disabled}
                  onChange={(e) => setParam('duration', e.target.value)}
                />
              </Space>
            ) : null}

            {cell.opType === 'command_execution' ? (
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Text type="secondary">run</Text>
                <Input.TextArea
                  value={String(params.run ?? '')}
                  autoSize={{ minRows: 2, maxRows: 8 }}
                  disabled={disabled}
                  onChange={(e) => setParam('run', e.target.value)}
                />
                <Space wrap>
                  <Text type="secondary">working_directory</Text>
                  <Input
                    value={String(params.working_directory ?? '')}
                    style={{ width: 320 }}
                    disabled={disabled}
                    onChange={(e) => setParam('working_directory', e.target.value)}
                  />
                  <Text type="secondary">shell</Text>
                  <Select
                    value={String(params.shell ?? 'bash')}
                    style={{ width: 140 }}
                    disabled={disabled}
                    options={[
                      { value: 'bash', label: 'bash' },
                      { value: 'sh', label: 'sh' },
                      { value: 'zsh', label: 'zsh' },
                    ]}
                    onChange={(v) => setParam('shell', v)}
                  />
                </Space>
                <Space wrap>
                  <Text type="secondary">continue_on_error</Text>
                  <Switch
                    checked={Boolean(params.continue_on_error ?? false)}
                    disabled={disabled}
                    onChange={(v) => setParam('continue_on_error', v)}
                  />
                  <Text type="secondary">timeout</Text>
                  <Input
                    value={String(params.timeout ?? '5m')}
                    style={{ width: 140 }}
                    disabled={disabled}
                    onChange={(e) => setParam('timeout', e.target.value)}
                  />
                </Space>
                <Divider style={{ margin: '6px 0' }} />
                <Text type="secondary">env</Text>
                {renderKeyValueFields(envFields, (next) => setParam('env', next))}
              </Space>
            ) : null}

            {cell.opType === 'codex.exec' ? (
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Text type="secondary">prompt</Text>
                <Input.TextArea
                  value={String(params.prompt ?? '')}
                  autoSize={{ minRows: 3, maxRows: 10 }}
                  disabled={disabled}
                  onChange={(e) => setParam('prompt', e.target.value)}
                />
                <Space wrap>
                  <Text type="secondary">model</Text>
                  <Input
                    value={String(params.model ?? 'gpt-5-codex')}
                    style={{ width: 240 }}
                    disabled={disabled}
                    onChange={(e) => setParam('model', e.target.value)}
                  />
                  <Text type="secondary">sessionId</Text>
                  <Input
                    value={String(params.sessionId ?? '')}
                    style={{ width: 240 }}
                    disabled={disabled}
                    onChange={(e) => setParam('sessionId', e.target.value)}
                  />
                </Space>
                <Space wrap>
                  <Text type="secondary">worktree_path</Text>
                  <Input
                    value={String(params.worktree_path ?? '')}
                    style={{ width: 360 }}
                    disabled={disabled}
                    onChange={(e) => setParam('worktree_path', e.target.value)}
                  />
                </Space>
                <Space wrap>
                  <Text type="secondary">cell_relative_path</Text>
                  <Input
                    value={String(params.cell_relative_path ?? '')}
                    style={{ width: 360 }}
                    disabled={disabled}
                    onChange={(e) => setParam('cell_relative_path', e.target.value)}
                  />
                </Space>
                <Divider style={{ margin: '6px 0' }} />
                <Text type="secondary">env</Text>
                {renderKeyValueFields(envFields, (next) => setParam('env', next))}
              </Space>
            ) : null}

            {cell.opType === 'input' ? (
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Text type="secondary">form.title</Text>
                <Input
                  value={String((params.form as any)?.title ?? '')}
                  disabled={disabled}
                  onChange={(e) => setParam('form', { ...(params.form as any), title: e.target.value })}
                />
                <Text type="secondary">form.question</Text>
                <Input.TextArea
                  value={String((params.form as any)?.question ?? '')}
                  autoSize={{ minRows: 2, maxRows: 6 }}
                  disabled={disabled}
                  onChange={(e) => setParam('form', { ...(params.form as any), question: e.target.value })}
                />
                <Space wrap>
                  <Text type="secondary">form.type</Text>
                  <Select
                    value={String((params.form as any)?.type ?? 'short_answer')}
                    style={{ width: 220 }}
                    disabled={disabled}
                    options={[
                      { value: 'short_answer', label: 'short_answer' },
                      { value: 'paragraph_text', label: 'paragraph_text' },
                      { value: 'dropdown', label: 'dropdown' },
                    ]}
                    onChange={(v) => setParam('form', { ...(params.form as any), type: v })}
                  />
                </Space>
              </Space>
            ) : null}

            {cell.opType === 'llm_inference2' ? (
              <Space direction="vertical" size={8} style={{ width: '100%' }}>
                <Space wrap>
                  <Text type="secondary">default_provider</Text>
                  <Select
                    value={String(params.default_provider ?? 'openai')}
                    style={{ width: 160 }}
                    disabled={disabled}
                    options={[
                      { value: 'openai', label: 'openai' },
                      { value: 'anthropic', label: 'anthropic' },
                      { value: 'google', label: 'google' },
                    ]}
                    onChange={(v) => setParam('default_provider', v)}
                  />
                  <Text type="secondary">default_model</Text>
                  <Input
                    value={String(params.default_model ?? 'gpt-4.1')}
                    style={{ width: 220 }}
                    disabled={disabled}
                    onChange={(e) => setParam('default_model', e.target.value)}
                  />
                </Space>
                <Space wrap>
                  <Text type="secondary">temperature</Text>
                  <InputNumber
                    value={typeof params.temperature === 'number' ? params.temperature : Number(params.temperature ?? 0.2)}
                    min={0}
                    max={2}
                    step={0.1}
                    disabled={disabled}
                    onChange={(v) => setParam('temperature', v ?? 0)}
                  />
                  <Text type="secondary">max_tokens</Text>
                  <InputNumber
                    value={typeof params.max_tokens === 'number' ? params.max_tokens : Number(params.max_tokens ?? 512)}
                    min={1}
                    max={8192}
                    step={1}
                    disabled={disabled}
                    onChange={(v) => setParam('max_tokens', v ?? 512)}
                  />
                </Space>
                <Text type="secondary">system_prompt</Text>
                <Input.TextArea
                  value={String(params.system_prompt ?? '')}
                  autoSize={{ minRows: 2, maxRows: 6 }}
                  disabled={disabled}
                  onChange={(e) => setParam('system_prompt', e.target.value)}
                />
                <Text type="secondary">prompt</Text>
                <Input.TextArea
                  value={String(params.prompt ?? '')}
                  autoSize={{ minRows: 3, maxRows: 10 }}
                  disabled={disabled}
                  onChange={(e) => setParam('prompt', e.target.value)}
                />
                <Space wrap>
                  <Text type="secondary">execute_tools</Text>
                  <Switch checked={Boolean(params.execute_tools ?? false)} disabled={disabled} onChange={(v) => setParam('execute_tools', v)} />
                  <Text type="secondary">enable_tool_execution</Text>
                  <Switch
                    checked={Boolean(params.enable_tool_execution ?? false)}
                    disabled={disabled}
                    onChange={(v) => setParam('enable_tool_execution', v)}
                  />
                </Space>
              </Space>
            ) : null}
          </Space>
        );
      }
      default:
        return null;
    }
  };

  return (
    <Card
      size="small"
      style={{
        marginBottom: 12,
        marginLeft: depth * 4,
        borderColor: selected ? '#1677ff' : undefined,
        borderLeft: statusAccent ? `4px solid ${statusAccent}` : undefined,
      }}
      title={
        <Space size={8} onClick={onSelect} style={{ cursor: 'pointer' }}>
          {canHaveChildren ? (
            <Button
              size="small"
              type="text"
              icon={caret}
              onClick={(e) => {
                e.stopPropagation();
                onToggleCollapse?.();
              }}
            />
          ) : (
            <span style={{ width: 24 }} />
          )}
          {kindIcon(cell.kind)}
          <Text strong>{cell.title}</Text>
          <Tag color="default">{kindLabel(cell.kind)}</Tag>
          {status ? (
            <Tag color={statusTagColor(status)} icon={status === 'running' ? <LoadingOutlined /> : undefined}>
              {status.toUpperCase()}
            </Tag>
          ) : null}
        </Space>
      }
      extra={
        <Space>
          {runnable && !disabled ? (
            <Button
              size="small"
              icon={<PlayCircleOutlined />}
              onClick={(e) => {
                e.stopPropagation();
                onRun?.();
              }}
            >
              Run
            </Button>
          ) : null}
          {editable && addableKinds.length > 0 ? (
            <Dropdown menu={addMenu} trigger={['click']}>
              <Button
                size="small"
                icon={<PlusOutlined />}
                onClick={(e) => {
                  e.stopPropagation();
                }}
              >
                Add
              </Button>
            </Dropdown>
          ) : null}
          {onMoveUp ? (
            <Button
              size="small"
              onClick={(e) => {
                e.stopPropagation();
                onMoveUp();
              }}
            >
              Up
            </Button>
          ) : null}
          {onMoveDown ? (
            <Button
              size="small"
              onClick={(e) => {
                e.stopPropagation();
                onMoveDown();
              }}
            >
              Down
            </Button>
          ) : null}
          {onDelete ? (
            <Button
              size="small"
              danger
              onClick={(e) => {
                e.stopPropagation();
                onDelete();
              }}
            >
              Delete
            </Button>
          ) : null}
        </Space>
      }
      onClick={onSelect}
      hoverable
    >
      {renderEditor()}

      {output !== null ? (
        <>
          <Divider style={{ margin: '10px 0' }} />
          <Collapse
            size="small"
            items={[
              ...(output !== null
                ? [
                    {
                      key: 'output',
                      label: 'Output',
                      children: <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{toPrettyJson(output) || '-'}</pre>,
                    },
                  ]
                : []),
            ]}
          />
        </>
      ) : null}
    </Card>
  );
}
