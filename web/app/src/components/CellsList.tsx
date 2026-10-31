import { useCompact } from '../hooks/useCompact';
import { useEffect, useMemo, useState } from 'react';
import { Card, Empty, Input, Space, Spin, Table, Tag, Typography, message } from 'antd';
import { listCells } from '@colony2/shared/api';
import type { CortexCell } from '@colony2/shared/types';

interface CellsListProps {
  projectId: string;
}

const { Text, Title } = Typography;

export default function CellsList({ projectId }: CellsListProps) {
  const compact = useCompact();
  const [cells, setCells] = useState<CortexCell[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  useEffect(() => {
    let current = true;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const result = await listCells(projectId);
        if (current) setCells(result);
      } catch (err) {
        if (!current) return;
        console.error('Failed to load cells', err);
        const messageText = err instanceof Error ? err.message : 'Failed to load cells';
        setError(messageText);
        message.error(messageText);
      } finally {
        if (current) setLoading(false);
      }
    };

    setCells([]);
    if (projectId) {
      load();
    }
    return () => { current = false; };
  }, [projectId]);

  const filteredCells = useMemo(() => {
    if (!search) return cells;
    const needle = search.toLowerCase();
    return cells.filter(
      (cell) =>
        cell.name.toLowerCase().includes(needle) ||
        cell.repository_source.toLowerCase().includes(needle) ||
        cell.kind.toLowerCase().includes(needle),
    );
  }, [cells, search]);

  return (
    <div className="app-page" style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <div className="goal-toolbar">
          <div>
            <Title level={3} style={{ margin: 0 }}>
              Cells
            </Title>
            <Text type="secondary">This cell and its c2j-discovered dependents</Text>
          </div>
          <Input.Search
            placeholder="Search cells"
            allowClear
            onSearch={setSearch}
            onChange={(event) => setSearch(event.target.value)}
            style={{ width: 320, maxWidth: '100%' }}
          />
        </div>

        {loading ? (
          <div style={{ minHeight: 320, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Spin size="large" />
          </div>
        ) : error ? (
          <Empty description={error} />
        ) : compact ? <div className="compact-cards" aria-label="Cells">{filteredCells.map(cell => <Card key={cell.id} size="small">
          <Text strong>{cell.name}</Text> <Tag>{cell.kind}</Tag>
          <p>Repository: <Text code>{cell.repository_source}</Text></p><Text>Ref: {cell.git_ref || '—'}</Text>
        </Card>)}{!filteredCells.length && <Empty description="No cells found" />}</div> : (
          <Table
            rowKey="id"
            dataSource={filteredCells}
            pagination={false}
            locale={{ emptyText: 'No cells found' }}
            columns={[
              {
                title: 'Name',
                dataIndex: 'name',
                key: 'name',
                width: 180,
                render: (name: string, cell: CortexCell) => (
                  <Space>
                    <Text strong>{name}</Text>
                    <Tag color={cell.kind === 'self' ? 'blue' : 'default'}>{cell.kind}</Tag>
                  </Space>
                ),
              },
              {
                title: 'Repository',
                dataIndex: 'repository_source',
                key: 'repository_source',
                render: (repo: string) => <Text code>{repo}</Text>,
              },
              {
                title: 'Ref',
                dataIndex: 'git_ref',
                key: 'git_ref',
                width: 140,
                render: (ref?: string) => ref || <Text type="secondary">-</Text>,
              },
            ]}
          />
        )}
      </Space>
    </div>
  );
}
