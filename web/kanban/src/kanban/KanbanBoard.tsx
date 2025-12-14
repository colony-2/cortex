import { useEffect, useMemo, useState } from 'react';
import { Card, Col, Empty, Row, Space, Spin, Tag, Typography } from 'antd';
import type { Ticket } from '@vibethis/openapi-client';
import { TicketsService } from '@vibethis/openapi-client';

const { Title, Text } = Typography;

interface KanbanBoardProps {
  projectId: string;
}

const stageColor: Record<string, string> = {
  backlog: 'default',
  todo: 'blue',
  doing: 'gold',
  review: 'purple',
  done: 'green',
};

export default function KanbanBoard({ projectId }: KanbanBoardProps) {
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const data = await TicketsService.getApiProjectsTickets(projectId);
        setTickets(data || []);
      } catch (err) {
        console.error('Failed to load tickets', err);
        setError(err instanceof Error ? err.message : 'Failed to load tickets');
      } finally {
        setLoading(false);
      }
    };

    if (projectId) {
      load();
    }
  }, [projectId]);

  const ticketsByStage = useMemo(() => {
    const grouped = new Map<string, Ticket[]>();
    tickets.forEach((ticket) => {
      const stage = ticket.stage || 'backlog';
      const list = grouped.get(stage) || [];
      list.push(ticket);
      grouped.set(stage, list);
    });
    return grouped;
  }, [tickets]);

  const stages = Array.from(ticketsByStage.keys());

  if (!projectId) {
    return <Empty description="Select a project to view tickets" />;
  }

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 32 }}>
        <Spin size="large" />
      </div>
    );
  }

  if (error) {
    return <Empty description={error} />;
  }

  if (tickets.length === 0) {
    return <Empty description="No tickets" />;
  }

  return (
    <Row gutter={16} style={{ padding: 16 }}>
      {stages.map((stage) => (
        <Col key={stage} span={6} style={{ minWidth: 260 }}>
          <Card title={<StageHeader stage={stage} count={ticketsByStage.get(stage)?.length || 0} /> }>
            {ticketsByStage.get(stage)?.map((ticket) => (
              <Card key={ticket.id} size="small" style={{ marginBottom: 8 }}>
                <Title level={5} style={{ marginBottom: 4 }}>
                  {ticket.title}
                </Title>
                <Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>
                  {ticket.state || 'unknown'}
                </Text>
                {ticket.cellName && (
                  <div style={{ marginTop: 4 }}>
                    <Tag color="blue" style={{ marginBottom: 4 }}>
                      {ticket.cellName}
                    </Tag>
                  </div>
                )}
              </Card>
            )) || <Empty description="No tickets" />}          
          </Card>
        </Col>
      ))}
    </Row>
  );
}

function StageHeader({ stage, count }: { stage: string; count: number }) {
  return (
    <Space align="center" size="small">
      <Tag color={stageColor[stage] || 'default'}>{stage}</Tag>
      <Text strong>{count}</Text>
    </Space>
  );
}
