import { Card, Descriptions, Empty, List, Space, Tag, Typography } from 'antd';
import dayjs from 'dayjs';
import type { NotebookCell } from '../model/notebookModel';
import { statusTagColor } from '../ui/status';

const { Text } = Typography;

function formatTimestamp(ts?: string | null): string {
  if (!ts) return '-';
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss');
}

function toPrettyJson(value: unknown): string {
  if (value === null || value === undefined) return '';
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export default function NotebookInspector(props: { cell: NotebookCell | null }): JSX.Element {
  const node = props.cell?.storyNode;
  if (!props.cell) {
    return (
      <Card size="small" title="Inspector">
        <Empty description="Select a cell" />
      </Card>
    );
  }

  const status = node?.status ?? props.cell.runtime?.status ?? null;
  const output = node ? (node.output ?? null) : (props.cell.runtime?.output ?? null);

  return (
    <Card size="small" title="Inspector">
      <Descriptions size="small" column={1} bordered>
        <Descriptions.Item label="Title">{props.cell.title}</Descriptions.Item>
        <Descriptions.Item label="Kind">{props.cell.kind}</Descriptions.Item>
        <Descriptions.Item label="Status">
          {status ? <Tag color={statusTagColor(status)}>{status.toUpperCase()}</Tag> : '-'}
        </Descriptions.Item>
        <Descriptions.Item label="Cell ID">
          <Text code style={{ wordBreak: 'break-all' }}>
            {props.cell.id}
          </Text>
        </Descriptions.Item>
        <Descriptions.Item label="Path">
          {node?.path ? (
            <Text code style={{ wordBreak: 'break-all' }}>
              {node.path.join(' / ')}
            </Text>
          ) : (
            '-'
          )}
        </Descriptions.Item>
        <Descriptions.Item label="Started">{formatTimestamp(node?.started_at)}</Descriptions.Item>
        <Descriptions.Item label="Finished">{formatTimestamp(node?.finished_at)}</Descriptions.Item>
      </Descriptions>

      <Space direction="vertical" style={{ width: '100%', marginTop: 12 }} size={10}>
        <Card size="small" title="Config">
          <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>
            {toPrettyJson(
              (() => {
                switch (props.cell.kind) {
                  case 'recipe':
                    return { name: props.cell.name, fields: props.cell.fields };
                  case 'sequence':
                    return { name: props.cell.name, fields: props.cell.fields };
                  case 'op':
                    return { opType: props.cell.opType, params: props.cell.params };
                  case 'stateMachine':
                    return { machineId: props.cell.machineId, fields: props.cell.fields };
                  case 'state':
                    return { stateId: props.cell.stateId, transitions: props.cell.transitions };
                  case 'markdown':
                    return { markdown: props.cell.markdown };
                  default:
                    return null;
                }
              })(),
            ) || '-'}
          </pre>
        </Card>
        <Card size="small" title="Output">
          <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{toPrettyJson(output) || '-'}</pre>
        </Card>
        {props.cell.runtime?.error ? (
          <Card size="small" title="Error">
            <Text type="danger">{props.cell.runtime.error}</Text>
          </Card>
        ) : null}
      </Space>

      {node ? (
        <Space direction="vertical" style={{ width: '100%', marginTop: 12 }} size={10}>
          <Card size="small" title="Input">
            <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{toPrettyJson(node.input ?? null) || '-'}</pre>
          </Card>
          <Card size="small" title="Artifacts">
            <List
              size="small"
              dataSource={node.artifact_keys ?? []}
              locale={{ emptyText: 'No artifacts' }}
              renderItem={(a) => (
                <List.Item>
                  <Space direction="vertical" size={0}>
                    <Text code>{a.name}</Text>
                    <Text type="secondary">
                      taskOrdinal={a.taskOrdinal}
                      {typeof a.sizeBytes === 'number' ? ` • ${a.sizeBytes.toLocaleString()} bytes` : ''}
                    </Text>
                  </Space>
                </List.Item>
              )}
            />
          </Card>
        </Space>
      ) : null}
    </Card>
  );
}
