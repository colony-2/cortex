import { useEffect, useMemo, useState } from 'react';
import type { FormInstance } from 'antd';
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ManagedCell, Ticket } from '@colony2/openapi-client';
import { ActorType, CellsService, TicketState, TicketsService } from '@colony2/openapi-client';

const { Title, Text } = Typography;
const { TextArea } = Input;

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
  const [availableStages, setAvailableStages] = useState<string[]>([]);
  const [availableStates, setAvailableStates] = useState<TicketState[]>([]);
  const [cells, setCells] = useState<ManagedCell[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const [ticketData, stageData, stateData, cellData] = await Promise.all([
          TicketsService.getApiProjectsTickets(projectId),
          TicketsService.getApiProjectsTicketsStages(projectId),
          TicketsService.getApiProjectsTicketsStates(projectId),
          CellsService.getApiProjectsCells(projectId),
        ]);
        setTickets(ticketData || []);
        setAvailableStages(
          (stageData && stageData.length > 0 && stageData) || ['backlog', 'todo', 'doing', 'review', 'done'],
        );
        setAvailableStates(stateData && stateData.length > 0 ? stateData : Object.values(TicketState));
        setCells(cellData || []);
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

  const stages = useMemo(() => {
    const stageSet = new Set<string>(
      availableStages.length > 0 ? availableStages : ['backlog', 'todo', 'doing', 'review', 'done'],
    );
    tickets.forEach((ticket) => {
      stageSet.add(ticket.stage || 'backlog');
    });
    return Array.from(stageSet);
  }, [availableStages, tickets]);

  const defaultStage = stages[0] || 'backlog';
  const defaultState = availableStates[0] || TicketState.WAITING_USER;

  const openCreateModal = (stage: string = defaultStage, state: TicketState = defaultState) => {
    form.setFieldsValue({ stage, state });
    setIsCreateOpen(true);
  };

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
    return (
      <div style={{ padding: 16 }}>
        <Alert type="error" showIcon message="Unable to load tickets" description={error} />
      </div>
    );
  }

  return (
    <div style={{ padding: 16 }}>
      <Space style={{ marginBottom: 16, width: '100%', justifyContent: 'space-between' }} align="center">
        <Title level={4} style={{ margin: 0 }}>
          Tickets
        </Title>
        <Button type="primary" onClick={() => openCreateModal(defaultStage, defaultState)}>
          New Ticket
        </Button>
      </Space>

      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 32 }}>
          <Spin size="large" />
        </div>
      ) : (
        <Row gutter={16}>
          {stages.map((stage) => (
            <Col key={stage} span={6} style={{ minWidth: 260, marginBottom: 16 }}>
              <Card title={<StageHeader stage={stage} count={ticketsByStage.get(stage)?.length || 0} />}>
                {ticketsByStage.get(stage)?.length ? (
                  ticketsByStage.get(stage)?.map((ticket) => (
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
                  ))
                ) : (
                  <Empty description="No tickets" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                )}
              </Card>
            </Col>
          ))}
        </Row>
      )}

      <CreateTicketModal
        open={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        onCreated={(ticket) => {
          setTickets((prev) => [...prev, ticket]);
          setAvailableStages((prev) => (prev.includes(ticket.stage) ? prev : [...prev, ticket.stage]));
        }}
        creating={creating}
        form={form}
        defaultStage={defaultStage}
        defaultState={defaultState}
        projectId={projectId}
        cells={cells}
        stages={stages}
        states={availableStates}
        setCreating={setCreating}
      />
    </div>
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

interface CreateTicketModalProps {
  open: boolean;
  onClose: () => void;
  onCreated: (ticket: Ticket) => void;
  creating: boolean;
  form: FormInstance;
  defaultStage: string;
  defaultState: TicketState;
  projectId: string;
  cells: ManagedCell[];
  stages: string[];
  states: TicketState[];
  setCreating: (creating: boolean) => void;
}

function CreateTicketModal({
  open,
  onClose,
  onCreated,
  creating,
  form,
  defaultStage,
  defaultState,
  projectId,
  cells,
  stages,
  states,
  setCreating,
}: CreateTicketModalProps) {
  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      setCreating(true);
      const newTicket = await TicketsService.postApiProjectsTickets(projectId, {
        cell: values.cell,
        title: values.title,
        description: values.description,
        stage: values.stage,
        state: values.state,
        actor: {
          type: ActorType.USER,
          user: {
            email: values.email,
          },
        },
      });
      message.success('Ticket created');
      onCreated(newTicket);
      onClose();
      form.resetFields();
    } catch (err) {
      if ((err as { errorFields?: unknown[] })?.errorFields) {
        return;
      }
      if (err instanceof Error) {
        message.error(err.message);
      } else {
        message.error('Failed to create ticket');
      }
    } finally {
      setCreating(false);
    }
  };

  const handleOpen = () => {
    form.setFieldsValue({
      stage: defaultStage,
      state: defaultState,
    });
  };

  return (
    <Modal
      title="New Ticket"
      open={open}
      onOk={handleSubmit}
      onCancel={onClose}
      okText="Create"
      destroyOnClose
      confirmLoading={creating}
      afterOpenChange={(nextOpen) => {
        if (nextOpen) {
          handleOpen();
        }
      }}
    >
      <Form layout="vertical" form={form}>
        <Form.Item
          label="Title"
          name="title"
          rules={[{ required: true, message: 'Please enter a title' }]}
        >
          <Input placeholder="Ticket title" />
        </Form.Item>

        <Form.Item label="Description" name="description">
          <TextArea rows={3} placeholder="Context, acceptance criteria, links…" />
        </Form.Item>

        <Form.Item
          label="Cell"
          name="cell"
          rules={[{ required: true, message: 'Select a cell' }]}
        >
          <Select
            showSearch
            placeholder="Select cell"
            optionFilterProp="label"
            options={cells.map((cell) => ({
              value: cell.name,
              label: cell.name,
            }))}
          />
        </Form.Item>

        <Form.Item
          label="Stage"
          name="stage"
          rules={[{ required: true, message: 'Select or enter a stage' }]}
        >
          <Select
            showSearch
            placeholder="Stage"
            mode="tags"
            tokenSeparators={[',']}
            options={stages.map((stage) => ({ value: stage, label: stage }))}
          />
        </Form.Item>

        <Form.Item
          label="State"
          name="state"
          rules={[{ required: true, message: 'Select a state' }]}
        >
          <Select
            placeholder="State"
            options={states.map((state) => ({ value: state, label: state.replace(/_/g, ' ') }))}
          />
        </Form.Item>

        <Form.Item
          label="Reporter email"
          name="email"
          rules={[
            { required: true, message: 'Enter your email' },
            { type: 'email', message: 'Enter a valid email' },
          ]}
        >
          <Input placeholder="you@example.com" />
        </Form.Item>
      </Form>
    </Modal>
  );
}
