import React, { useEffect, useMemo, useState } from 'react';
import { Button, Form, Input, Modal, Select, Space, message } from 'antd';
import type { ManagedCell, Ticket } from '@colony2/openapi-client';
import { ActorType, CellsService, TicketState, TicketsService } from '@colony2/openapi-client';
import { getUserEmail } from '../auth';

const { TextArea } = Input;
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

export interface CreateTicketModalProps {
    /** Whether the modal is visible */
    open: boolean;
    /** Callback when modal is closed */
    onClose: () => void;
    /** Project ID (required for creating ticket) */
    projectId: string;
    /** Optional callback when ticket is created */
    onCreated?: (ticket: Ticket) => void;
}

export const CreateTicketModal: React.FC<CreateTicketModalProps> = ({
    open,
    onClose,
    projectId,
    onCreated,
}) => {
    const [form] = Form.useForm();
    const [creating, setCreating] = useState(false);
    const [loading, setLoading] = useState(false);
    const [cells, setCells] = useState<ManagedCell[]>([]);
    const [hasDraft, setHasDraft] = useState(false);
    const draftKey = useMemo(() => `${DRAFT_KEY_PREFIX}:${projectId}`, [projectId]);
    const lastCellKey = useMemo(() => `${LAST_CELL_KEY_PREFIX}:${projectId}`, [projectId]);

    useEffect(() => {
        if (!open || !projectId) {
            setCells([]);
            return;
        }

        const fetchCells = async () => {
            setLoading(true);
            try {
                const cellData = await CellsService.getApiProjectsCells(projectId);
                setCells(cellData || []);
            } catch (err) {
                console.error('Failed to load ticket creation data:', err);
                message.error('Failed to load form data');
            } finally {
                setLoading(false);
            }
        };

        fetchCells();
    }, [projectId]);

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
            if (onCreated) {
                onCreated(newTicket);
            }
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
        const payload = {
            title: allValues.title?.trim() || '',
            description: allValues.description?.trim() || '',
        };
        const hasContent = Boolean(payload.title || payload.description);
        setHasDraft(hasContent);
        if (!hasContent) {
            storage.removeItem(draftKey);
            return;
        }
        storage.setItem(draftKey, JSON.stringify(payload));
    };

    const handleCellChange = (value: string) => {
        const storage = getStorage();
        if (!storage) {
            return;
        }
        storage.setItem(lastCellKey, value);
    };

    const handleClearDraft = () => {
        const storage = getStorage();
        if (storage) {
            storage.removeItem(draftKey);
        }
        form.setFieldsValue({ title: undefined, description: undefined });
        setHasDraft(false);
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
            destroyOnHidden
            style={{ top: 24 }}
            width="90vw"
            styles={{ body: { maxHeight: '80vh', overflowY: 'auto' } }}
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
                        loading={loading}
                        disabled={loading}
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
                    <Input placeholder="Ticket title" disabled={loading} />
                </Form.Item>

                <Form.Item label="Description" name="description">
                    <TextArea
                        rows={12}
                        placeholder="Write in markdown... (Context, acceptance criteria, links)"
                        disabled={loading}
                    />
                </Form.Item>
            </Form>
        </Modal>
    );
};
