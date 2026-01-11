import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { EditOutlined, PlusOutlined, ArrowLeftOutlined } from '@ant-design/icons';
import type { ManagedCell, Ticket } from '@colony2/openapi-client';
import { ActorType, CellsService, TicketState, TicketsService } from '@colony2/openapi-client';
import { TicketDetailModal } from '@colony2/shared';

const { Title, Text } = Typography;
const { TextArea } = Input;

interface CellDetailPageProps {
  projectId: string;
}

export default function CellDetailPage({ projectId }: CellDetailPageProps) {
  const { cellId } = useParams<{ cellId: string }>();
  const navigate = useNavigate();
  const [cell, setCell] = useState<ManagedCell | null>(null);
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(false);
  const [ticketsLoading, setTicketsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isEditOpen, setIsEditOpen] = useState(false);
  const [isCreateTicketOpen, setIsCreateTicketOpen] = useState(false);
  const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null);
  const [editForm] = Form.useForm();
  const [ticketForm] = Form.useForm();

  const [availableStages, setAvailableStages] = useState<string[]>([]);
  const [availableStates, setAvailableStates] = useState<TicketState[]>([]);

  useEffect(() => {
    const loadCell = async () => {
      if (!cellId) return;

      setLoading(true);
      setError(null);
      try {
        const cellData = await CellsService.getApiProjectsCells1(projectId, cellId);
        setCell(cellData);
      } catch (err) {
        console.error('Failed to load cell', err);
        const errorMsg = err instanceof Error ? err.message : 'Failed to load cell';
        setError(errorMsg);
        message.error(errorMsg);
      } finally {
        setLoading(false);
      }
    };

    loadCell();
  }, [projectId, cellId]);

  useEffect(() => {
    const loadTickets = async () => {
      if (!cellId) return;

      setTicketsLoading(true);
      try {
        const [ticketData, stageData, stateData] = await Promise.all([
          TicketsService.getApiProjectsTickets(projectId, undefined, undefined, undefined, undefined, [cellId]),
          TicketsService.getApiProjectsTicketsStages(projectId),
          TicketsService.getApiProjectsTicketsStates(projectId),
        ]);
        setTickets(ticketData || []);
        setAvailableStages(stageData || ['backlog', 'todo', 'doing', 'review', 'done']);
        setAvailableStates(stateData || Object.values(TicketState));
      } catch (err) {
        console.error('Failed to load tickets', err);
      } finally {
        setTicketsLoading(false);
      }
    };

    loadTickets();
  }, [projectId, cellId]);

  const handleEdit = () => {
    if (!cell) return;
    editForm.setFieldsValue({
      name: cell.name,
      description: cell.description || '',
      workingPath: cell.workingPath,
    });
    setIsEditOpen(true);
  };

  const handleEditSubmit = async () => {
    if (!cell || !cellId) return;

    try {
      const values = await editForm.validateFields();
      await CellsService.patchApiProjectsCells(projectId, cellId, {
        name: values.name,
        description: values.description || undefined,
        workingPath: values.workingPath,
      });
      message.success('Cell updated successfully');

      // Reload cell data
      const updatedCell = await CellsService.getApiProjectsCells1(projectId, cellId);
      setCell(updatedCell);
      setIsEditOpen(false);
    } catch (err) {
      if ((err as { errorFields?: unknown[] })?.errorFields) {
        return;
      }
      console.error('Failed to update cell', err);
      const errorMsg = err instanceof Error ? err.message : 'Failed to update cell';
      message.error(errorMsg);
    }
  };

  const handleCreateTicket = () => {
    if (!cell) return;
    ticketForm.setFieldsValue({
      cell: cell.name,
      stage: availableStages[0] || 'backlog',
      state: availableStates[0] || TicketState.WAITING_USER,
    });
    setIsCreateTicketOpen(true);
  };

  const handleCreateTicketSubmit = async () => {
    if (!cell) return;

    try {
      const values = await ticketForm.validateFields();
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
      message.success('Ticket created successfully');
      setTickets((prev) => [...prev, newTicket]);
      setIsCreateTicketOpen(false);
      ticketForm.resetFields();
    } catch (err) {
      if ((err as { errorFields?: unknown[] })?.errorFields) {
        return;
      }
      console.error('Failed to create ticket', err);
      const errorMsg = err instanceof Error ? err.message : 'Failed to create ticket';
      message.error(errorMsg);
    }
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <Spin size="large" />
      </div>
    );
  }

  if (error || !cell) {
    return (
      <div style={{ padding: 24 }}>
        <Alert type="error" showIcon message="Unable to load cell" description={error || 'Cell not found'} />
      </div>
    );
  }

  return (
    <div style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        {/* Header with back button */}
        <div>
          <Button
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate(`/project/${projectId}/cells/list`)}
            style={{ marginBottom: 16 }}
          >
            Back to Cells
          </Button>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Title level={2} style={{ margin: 0 }}>
              {cell.name}
            </Title>
            <Button type="primary" icon={<EditOutlined />} onClick={handleEdit}>
              Edit Cell
            </Button>
          </div>
        </div>

        {/* Cell Properties */}
        <Card title="Cell Properties" bordered>
          <Descriptions bordered column={2}>
            <Descriptions.Item label="Name" span={2}>
              <Text strong>{cell.name}</Text>
            </Descriptions.Item>
            <Descriptions.Item label="Description" span={2}>
              {cell.description || <Text type="secondary">No description</Text>}
            </Descriptions.Item>
            <Descriptions.Item label="Working Path" span={2}>
              <Text code>{cell.workingPath}</Text>
            </Descriptions.Item>
            <Descriptions.Item label="Populator">
              {cell.populator ? <Tag color="purple">{cell.populator}</Tag> : <Text type="secondary">None</Text>}
            </Descriptions.Item>
            <Descriptions.Item label="Populator ID">
              {cell.populatorId || <Text type="secondary">N/A</Text>}
            </Descriptions.Item>
            <Descriptions.Item label="Dependencies" span={2}>
              {cell.dependencies && cell.dependencies.length > 0 ? (
                <Space size="small" wrap>
                  {cell.dependencies.map((dep: string) => (
                    <Tag key={dep}>{dep}</Tag>
                  ))}
                </Space>
              ) : (
                <Text type="secondary">No dependencies</Text>
              )}
            </Descriptions.Item>
            <Descriptions.Item label="Created At">
              {new Date(cell.createdAt).toLocaleString()}
            </Descriptions.Item>
            <Descriptions.Item label="Updated At">
              {new Date(cell.updatedAt).toLocaleString()}
            </Descriptions.Item>
            <Descriptions.Item label="Cell ID" span={2}>
              <Text code>{cell.id}</Text>
            </Descriptions.Item>
          </Descriptions>
        </Card>

        {/* Tickets Section */}
        <Card
          title="Tickets"
          extra={
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateTicket}>
              Create Ticket
            </Button>
          }
          bordered
        >
          {ticketsLoading ? (
            <div style={{ display: 'flex', justifyContent: 'center', padding: 32 }}>
              <Spin />
            </div>
          ) : tickets.length === 0 ? (
            <Empty description="No tickets found for this cell" />
          ) : (
            <Table
              rowKey="id"
              dataSource={tickets}
              pagination={{ pageSize: 10 }}
              onRow={(record) => ({
                onClick: (e) => {
                  // Allow Ctrl/Cmd-click to open in new tab
                  if (e.ctrlKey || e.metaKey) return;
                  setSelectedTicket(record);
                },
                style: { cursor: 'pointer' },
              })}
              columns={[
                {
                  title: 'Title',
                  dataIndex: 'title',
                  key: 'title',
                  render: (text: string) => <Text strong>{text}</Text>,
                },
                {
                  title: 'Stage',
                  dataIndex: 'stage',
                  key: 'stage',
                  render: (stage: string) => <Tag color="blue">{stage}</Tag>,
                },
                {
                  title: 'State',
                  dataIndex: 'state',
                  key: 'state',
                  render: (state: TicketState) => (
                    <Tag color={state === TicketState.WORKING ? 'green' : 'default'}>
                      {state.replace(/_/g, ' ')}
                    </Tag>
                  ),
                },
                {
                  title: 'Description',
                  dataIndex: 'description',
                  key: 'description',
                  ellipsis: true,
                  render: (desc?: string) => desc || <Text type="secondary">—</Text>,
                },
                {
                  title: 'Updated',
                  dataIndex: 'updatedAt',
                  key: 'updatedAt',
                  render: (value: string) => new Date(value).toLocaleString(),
                  width: 200,
                },
              ]}
            />
          )}
        </Card>
      </Space>

      {/* Edit Cell Modal */}
      <Modal
        title="Edit Cell"
        open={isEditOpen}
        onOk={handleEditSubmit}
        onCancel={() => {
          setIsEditOpen(false);
          editForm.resetFields();
        }}
        okText="Save"
        width={600}
      >
        <Form layout="vertical" form={editForm}>
          <Form.Item
            label="Name"
            name="name"
            rules={[{ required: true, message: 'Please enter a cell name' }]}
          >
            <Input placeholder="Cell name" />
          </Form.Item>
          <Form.Item label="Description" name="description">
            <TextArea rows={3} placeholder="Cell description" />
          </Form.Item>
          <Form.Item
            label="Working Path"
            name="workingPath"
            rules={[{ required: true, message: 'Please enter a working path' }]}
          >
            <Input placeholder="/path/to/cell" />
          </Form.Item>
        </Form>
      </Modal>

      {/* Create Ticket Modal */}
      <Modal
        title="Create Ticket"
        open={isCreateTicketOpen}
        onOk={handleCreateTicketSubmit}
        onCancel={() => {
          setIsCreateTicketOpen(false);
          ticketForm.resetFields();
        }}
        okText="Create"
        width={600}
      >
        <Form layout="vertical" form={ticketForm}>
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
            rules={[{ required: true, message: 'Cell is required' }]}
          >
            <Input disabled />
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
              options={availableStages.map((stage) => ({ value: stage, label: stage }))}
            />
          </Form.Item>
          <Form.Item
            label="State"
            name="state"
            rules={[{ required: true, message: 'Select a state' }]}
          >
            <Select
              placeholder="State"
              options={availableStates.map((state) => ({ value: state, label: state.replace(/_/g, ' ') }))}
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

      {/* Ticket Detail Modal */}
      <TicketDetailModal
        visible={selectedTicket !== null}
        onClose={() => setSelectedTicket(null)}
        ticket={selectedTicket}
        projectId={projectId}
      />
    </div>
  );
}
