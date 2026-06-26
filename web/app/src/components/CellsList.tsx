import { useEffect, useMemo, useState } from 'react';
import { Empty, Input, Space, Spin, Table, Tag, Typography, message } from 'antd';
import { listCells } from '@colony2/shared/api';
import type { CortexCell } from '@colony2/shared/types';

interface CellsListProps {
  projectId: string;
}

const { Text, Title } = Typography;

export default function CellsList({ projectId }: CellsListProps) {
  const [cells, setCells] = useState<CortexCell[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        setCells(await listCells(projectId));
      } catch (err) {
        console.error('Failed to load cells', err);
        const messageText = err instanceof Error ? err.message : 'Failed to load cells';
        setError(messageText);
        message.error(messageText);
      } finally {
        setLoading(false);
      }
    };

    if (projectId) {
      load();
    }
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
    <div style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'center' }}>
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
            style={{ width: 320 }}
          />
        </div>

        {loading ? (
          <div style={{ minHeight: 320, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Spin size="large" />
          </div>
        ) : error ? (
          <Empty description={error} />
        ) : (
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
