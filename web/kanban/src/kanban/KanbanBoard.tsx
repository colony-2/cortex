import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import type { FormInstance, TableColumnsType } from 'antd';
import {
  Alert,
  Button,
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
import type { ManagedCell, Ticket } from '@colony2/openapi-client';
import { ActorType, CellsService, TicketState, TicketsService } from '@colony2/openapi-client';
import { TicketDetailModal, getUserEmail } from '@colony2/shared';

const { Title } = Typography;
const { TextArea } = Input;
const { Search } = Input;

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

const stateColor: Record<string, string> = {
  [TicketState.WAITING_USER]: 'orange',
  [TicketState.WAITING_DEPENDENCY]: 'blue',
  [TicketState.WAITING_CAPACITY]: 'purple',
  [TicketState.WORKING]: 'green',
  // Legacy/custom states
  'waiting_dev': 'blue',
  'in_progress': 'processing',
  'blocked': 'red',
  'completed': 'success',
  'abandoned': 'default',
};

export default function KanbanBoard({ projectId }: KanbanBoardProps) {
  const location = useLocation();
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [filteredTickets, setFilteredTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [availableStages, setAvailableStages] = useState<string[]>([]);
  const [availableStates, setAvailableStates] = useState<TicketState[]>([]);
  const [cells, setCells] = useState<ManagedCell[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [selectedTicket, setSelectedTicket] = useState<Ticket | null>(null);
  const [searchText, setSearchText] = useState('');
  const [form] = Form.useForm();

  // Open create modal if navigated from header button
  useEffect(() => {
    const state = location.state as { openCreateModal?: boolean } | null;
    if (state?.openCreateModal) {
      setIsCreateOpen(true);
      // Clear the state immediately so it doesn't reopen on close
      window.history.replaceState({}, document.title);
    }
  }, [location.state]); // Only depend on location.state, not isCreateOpen

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
        const sortedTickets = (ticketData || []).sort((a, b) => {
          const dateA = new Date(a.createdAt || 0).getTime();
          const dateB = new Date(b.createdAt || 0).getTime();
          return dateB - dateA;
        });
        setTickets(sortedTickets);
        setFilteredTickets(sortedTickets);
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

  useEffect(() => {
    if (!searchText) {
      setFilteredTickets(tickets);
      return;
    }
    const lowerSearch = searchText.toLowerCase();
    const filtered = tickets.filter((ticket) => {
      return (
        ticket.title?.toLowerCase().includes(lowerSearch) ||
        ticket.description?.toLowerCase().includes(lowerSearch) ||
        ticket.cellName?.toLowerCase().includes(lowerSearch) ||
        ticket.id?.toLowerCase().includes(lowerSearch) ||
        ticket.stage?.toLowerCase().includes(lowerSearch) ||
        ticket.state?.toLowerCase().includes(lowerSearch)
      );
    });
    setFilteredTickets(filtered);
  }, [searchText, tickets]);

  const defaultStage = availableStages[0] || 'backlog';
  const defaultState = availableStates[0] || TicketState.WAITING_USER;

  const columns: TableColumnsType<Ticket> = [
    {
      title: 'Title',
      dataIndex: 'title',
      key: 'title',
      width: '30%',
      render: (title: string) => <span style={{ fontWeight: 500 }}>{title}</span>,
    },
    {
      title: 'Cell',
      dataIndex: 'cellName',
      key: 'cellName',
      width: '20%',
      filters: Array.from(new Set(tickets.map((t) => t.cellName).filter(Boolean))).map((name) => ({
        text: name!,
        value: name!,
      })),
      onFilter: (value, record) => record.cellName === value,
      render: (cellName: string) => cellName && <Tag color="blue">{cellName}</Tag>,
    },
    {
      title: 'Stage',
      dataIndex: 'stage',
      key: 'stage',
      width: '15%',
      filters: availableStages.map((stage) => ({ text: stage, value: stage })),
      onFilter: (value, record) => record.stage === value,
      render: (stage: string) => <Tag color={stageColor[stage] || 'default'}>{stage}</Tag>,
    },
    {
      title: 'State',
      dataIndex: 'state',
      key: 'state',
      width: '15%',
      filters: availableStates.map((state) => ({ text: state.replace(/_/g, ' '), value: state })),
      onFilter: (value, record) => record.state === value,
      render: (state: TicketState) => (
        <Tag color={stateColor[state] || 'default'}>{state.replace(/_/g, ' ')}</Tag>
      ),
    },
    {
      title: 'Updated',
      dataIndex: 'updated',
      key: 'updated',
      width: '20%',
      sorter: (a, b) => {
        const dateA = new Date(a.updatedAt || 0).getTime();
        const dateB = new Date(b.updatedAt || 0).getTime();
        return dateA - dateB;
      },
      defaultSortOrder: 'descend',
      render: (updated: string) => updated && new Date(updated).toLocaleString(),
    },
  ];

  const openCreateModal = () => {
    form.setFieldsValue({ stage: defaultStage, state: defaultState });
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
        <Space>
          <Search
            placeholder="Search tickets..."
            allowClear
            style={{ width: 300 }}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
          />
          <Button type="primary" onClick={openCreateModal}>
            New Ticket
          </Button>
        </Space>
      </Space>

      <Table
        columns={columns}
        dataSource={filteredTickets}
        rowKey="id"
        loading={loading}
        pagination={{
          pageSize: 20,
          showSizeChanger: true,
          showTotal: (total) => `Total ${total} tickets`,
        }}
        onRow={(record) => ({
          onClick: (e) => {
            // Allow Ctrl/Cmd-click to open in new tab
            if (e.ctrlKey || e.metaKey) return;
            setSelectedTicket(record);
          },
          style: { cursor: 'pointer' },
        })}
      />

      <CreateTicketModal
        open={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        onCreated={(ticket) => {
          const newTickets = [ticket, ...tickets].sort((a, b) => {
            const dateA = new Date(a.createdAt || 0).getTime();
            const dateB = new Date(b.createdAt || 0).getTime();
            return dateB - dateA;
          });
          setTickets(newTickets);
          setAvailableStages((prev) => (prev.includes(ticket.stage) ? prev : [...prev, ticket.stage]));
        }}
        creating={creating}
        form={form}
        defaultStage={defaultStage}
        defaultState={defaultState}
        projectId={projectId}
        cells={cells}
        stages={availableStages}
        states={availableStates}
        setCreating={setCreating}
      />

      <TicketDetailModal
        visible={selectedTicket !== null}
        onClose={() => setSelectedTicket(null)}
        ticket={selectedTicket}
        projectId={projectId}
      />
    </div>
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
    const userEmail = getUserEmail();
    form.setFieldsValue({
      stage: defaultStage,
      state: defaultState,
      email: userEmail || undefined,
    });
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      handleSubmit();
    }
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
      <Form layout="vertical" form={form} onKeyDown={handleKeyDown}>
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
