import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Card, Empty, Input, Space, Spin, Table, Tag, Typography, message } from 'antd';
import { syncCells } from '@colony2/shared';

type ManagedCell = {
  id: string;
  name: string;
  workingPath: string;
  populator?: string;
  populatorId?: string;
  updatedAt: string;
  dependencies?: string[];
};

interface CellsListProps {
  projectId: string;
}

const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';
const { Title, Text } = Typography;

export default function CellsList({ projectId }: CellsListProps) {
  const [cells, setCells] = useState<ManagedCell[]>([]);
  const [loading, setLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/cells`);
        if (!response.ok) {
          const text = await response.text().catch(() => '');
          throw new Error(text || 'Failed to load cells');
        }
        const data = await response.json();
        setCells(data || []);
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

  const handleSync = async () => {
    setSyncing(true);
    try {
      await syncCells(projectId);
      message.success('Cells synced successfully');
      // Reload cells after sync
      const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/cells`);
      if (response.ok) {
        const data = await response.json();
        setCells(data || []);
      }
    } catch (err) {
      console.error('Failed to sync cells', err);
      const messageText = err instanceof Error ? err.message : 'Failed to sync cells';
      message.error(messageText);
    } finally {
      setSyncing(false);
    }
  };

  const filteredCells = useMemo(() => {
    if (!search) {
      return cells;
    }
    const needle = search.toLowerCase();
    return cells.filter(
      (cell) =>
        cell.name.toLowerCase().includes(needle) ||
        cell.workingPath.toLowerCase().includes(needle) ||
        (cell.populator && cell.populator.toLowerCase().includes(needle)),
    );
  }, [cells, search]);

  return (
    <Card
      title={
        <Space direction="vertical" size={4}>
          <Title level={4} style={{ margin: 0 }}>
            Cells
          </Title>
          <Text type="secondary">Browse managed cells in this project</Text>
        </Space>
      }
      extra={
        <Space>
          <Button type="primary" onClick={handleSync} loading={syncing}>
            Sync
          </Button>
          <Input.Search
            placeholder="Search by name, path, or populator"
            allowClear
            onSearch={setSearch}
            onChange={(e) => setSearch(e.target.value)}
            style={{ width: 320 }}
          />
        </Space>
      }
      style={{ height: '100%' }}
      styles={{ body: { height: '100%', display: 'flex', flexDirection: 'column' } }}
    >
      {loading ? (
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Spin size="large" />
        </div>
      ) : error ? (
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Empty description={error} />
        </div>
      ) : filteredCells.length === 0 ? (
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Empty description="No cells found" />
        </div>
      ) : (
        <Table
          rowKey="id"
          dataSource={filteredCells}
          pagination={{ pageSize: 10 }}
          onRow={(record) => ({
            onClick: (e) => {
              // Allow Ctrl/Cmd-click to open in new tab
              if (e.ctrlKey || e.metaKey) return;
              if (record) navigate(`/project/${projectId}/cell/${record.id}`);
            },
            style: { cursor: 'pointer' },
          })}
          columns={[
            {
              title: 'Name',
              dataIndex: 'name',
              key: 'name',
              render: (text: string) => <Text strong>{text}</Text>,
            },
            {
              title: 'Path',
              dataIndex: 'workingPath',
              key: 'workingPath',
              render: (text: string) => <Text code>{text}</Text>,
            },
            {
              title: 'Populator',
              dataIndex: 'populator',
              key: 'populator',
              render: (populator?: string, record?: ManagedCell) =>
                populator ? (
                  <Space size="small">
                    <Tag color="purple">{populator}</Tag>
                    {record?.populatorId && <Text type="secondary">#{record.populatorId}</Text>}
                  </Space>
                ) : (
                  <Text type="secondary">—</Text>
                ),
            },
            {
              title: 'Dependencies',
              dataIndex: 'dependencies',
              key: 'dependencies',
              render: (deps?: string[]) => <Text>{deps?.length || 0}</Text>,
              width: 140,
            },
            {
              title: 'Updated',
              dataIndex: 'updatedAt',
              key: 'updatedAt',
              render: (value: string) => <Text type="secondary">{new Date(value).toLocaleString()}</Text>,
              width: 200,
            },
          ]}
        />
      )}
    </Card>
  );
}
