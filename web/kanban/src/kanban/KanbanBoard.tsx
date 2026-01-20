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
const DRAFT_KEY_PREFIX = 'new-ticket-draft';
const LAST_CELL_KEY_PREFIX = 'new-ticket-last-cell';
const getStorage = () => {
  try {
    if (typeof window === 'undefined') {
      return null;
    }
    const storage = window.localStorage;
    if (!storage || typeof storage.getItem !== 'function') {
      return null;
    }
    return storage;
  } catch {
    return null;
  }
};

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
  const location = useLocation();
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [filteredTickets, setFilteredTickets] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [availableStages, setAvailableStages] = useState<string[]>([]);
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
        const [ticketData, stageData, cellData] = await Promise.all([
          TicketsService.getApiProjectsTickets(projectId),
          TicketsService.getApiProjectsTicketsStages(projectId),
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

  const columns: TableColumnsType<Ticket> = [
    {
      title: 'Title',
      dataIndex: 'title',
      key: 'title',
      width: '50%',
      render: (title: string, record: Ticket) => {
        const cellLabel = record.cellName || 'unknown';
        return <span>{`[${cellLabel}] ${title}`}</span>;
      },
    },
    {
      title: 'Stage',
      dataIndex: 'stage',
      key: 'stage',
      width: '20%',
      filters: availableStages.map((stage) => ({ text: stage, value: stage })),
      onFilter: (value, record) => record.stage === value,
      render: (stage: string) => <Tag color={stageColor[stage] || 'default'}>{stage}</Tag>,
    },
    {
      title: 'Updated',
      dataIndex: 'updated',
      key: 'updated',
      width: '30%',
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
        projectId={projectId}
        cells={cells}
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
  projectId: string;
  cells: ManagedCell[];
  setCreating: (creating: boolean) => void;
}

function CreateTicketModal({
  open,
  onClose,
  onCreated,
  creating,
  form,
  projectId,
  cells,
  setCreating,
}: CreateTicketModalProps) {
  const [hasDraft, setHasDraft] = useState(false);
  const draftKey = `${DRAFT_KEY_PREFIX}:${projectId}`;
  const lastCellKey = `${LAST_CELL_KEY_PREFIX}:${projectId}`;

  useEffect(() => {
    if (!open || !projectId) {
      return;
    }
    const storage = getStorage();
    if (!storage) {
      setHasDraft(false);
      return;
    }
    const raw = storage.getItem(draftKey);
    const lastCell = storage.getItem(lastCellKey);
    if (!raw) {
      setHasDraft(false);
    } else {
      try {
        const draft = JSON.parse(raw) as { title?: string; description?: string };
        form.setFieldsValue({
          title: draft.title,
          description: draft.description,
        });
        setHasDraft(Boolean(draft.title || draft.description));
      } catch {
        setHasDraft(false);
      }
    }
    if (lastCell) {
      form.setFieldsValue({ cell: lastCell });
    }
  }, [open, projectId, form, draftKey, lastCellKey]);

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      setCreating(true);
      const userEmail = getUserEmail();
      const newTicket = await TicketsService.postApiProjectsTickets(projectId, {
        cell: values.cell,
        title: values.title,
        description: values.description,
        stage: 'open',
        state: TicketState.WAITING_USER,
        actor: {
          type: ActorType.USER,
          user: {
            email: userEmail || '',
          },
        },
      });
      message.success('Ticket created');
      onCreated(newTicket);
      onClose();
      form.resetFields();
      const storage = getStorage();
      if (storage) {
        storage.removeItem(draftKey);
      }
      setHasDraft(false);
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

  const handleValuesChange = (_changed: unknown, allValues: { title?: string; description?: string }) => {
    const storage = getStorage();
    if (!storage) {
      return;
    }
    persistDraft(allValues.title, allValues.description, storage);
  };

  const handleClearDraft = () => {
    const storage = getStorage();
    if (storage) {
      storage.removeItem(draftKey);
    }
    form.setFieldsValue({ title: undefined, description: undefined });
    setHasDraft(false);
  };

  const persistDraft = (title?: string, description?: string, storage?: Storage | null) => {
    const activeStorage = storage ?? getStorage();
    if (!activeStorage) {
      return;
    }
    const payload = {
      title: title?.trim() || '',
      description: description?.trim() || '',
    };
    const hasContent = Boolean(payload.title || payload.description);
    setHasDraft(hasContent);
    if (!hasContent) {
      activeStorage.removeItem(draftKey);
      return;
    }
    activeStorage.setItem(draftKey, JSON.stringify(payload));
  };

  const handleCellChange = (value: string) => {
    const storage = getStorage();
    if (!storage) {
      return;
    }
    storage.setItem(lastCellKey, value);
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
      onCancel={onClose}
      destroyOnClose
      style={{ top: 24 }}
      width="90vw"
      bodyStyle={{ maxHeight: '80vh', overflowY: 'auto' }}
      footer={
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Button onClick={handleClearDraft} disabled={!hasDraft}>
            Clear
          </Button>
          <Space>
            <Button onClick={onClose}>Cancel</Button>
            <Button type="primary" onClick={handleSubmit} loading={creating}>
              Create
            </Button>
          </Space>
        </div>
      }
    >
      <Form
        layout="vertical"
        form={form}
        onKeyDown={handleKeyDown}
        onValuesChange={handleValuesChange}
      >
        <Form.Item
          label="Cell"
          name="cell"
          rules={[{ required: true, message: 'Select a cell' }]}
        >
          <Select
            showSearch
            placeholder="Select cell"
            optionFilterProp="label"
            onChange={handleCellChange}
            options={cells.map((cell) => ({
              value: cell.name,
              label: cell.name,
            }))}
          />
        </Form.Item>

        <Form.Item
          label="Title"
          name="title"
          rules={[{ required: true, message: 'Please enter a title' }]}
        >
          <Input placeholder="Ticket title" />
        </Form.Item>

        <Form.Item label="Description" name="description">
          <TextArea rows={12} placeholder="Write in markdown... (Context, acceptance criteria, links)" />
        </Form.Item>
      </Form>
    </Modal>
  );
}
