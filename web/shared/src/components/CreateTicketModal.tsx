import React, { useEffect, useState } from 'react';
import { Form, Input, Modal, Select, message } from 'antd';
import type { ManagedCell, Ticket, TicketState } from '@colony2/openapi-client';
import { ActorType, CellsService, TicketsService } from '@colony2/openapi-client';
import { getUserEmail } from '../auth';

const { TextArea } = Input;

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
    const [stages, setStages] = useState<string[]>([]);
    const [states, setStates] = useState<TicketState[]>([]);

    // Fetch data when modal opens
    useEffect(() => {
        if (!open || !projectId) {
            return;
        }

        const fetchData = async () => {
            setLoading(true);
            try {
                const [cellData, stageData, stateData] = await Promise.all([
                    CellsService.getApiProjectsCells(projectId),
                    TicketsService.getApiProjectsTicketsStages(projectId),
                    TicketsService.getApiProjectsTicketsStates(projectId),
                ]);
                setCells(cellData || []);
                setStages(stageData && stageData.length > 0 ? stageData : ['backlog', 'todo', 'doing', 'review', 'done']);
                setStates(stateData && stateData.length > 0 ? stateData : ['waiting_user', 'waiting_dev', 'in_progress', 'blocked', 'completed', 'abandoned'] as TicketState[]);

                // Set default values
                const defaultStage = (stageData && stageData.length > 0 ? stageData[0] : 'backlog');
                const defaultState = (stateData && stateData.length > 0 ? stateData[0] : 'waiting_user');
                const userEmail = getUserEmail();

                form.setFieldsValue({
                    stage: defaultStage,
                    state: defaultState,
                    email: userEmail || undefined,
                });
            } catch (err) {
                console.error('Failed to load ticket creation data:', err);
                message.error('Failed to load form data');
            } finally {
                setLoading(false);
            }
        };

        fetchData();
    }, [open, projectId, form]);

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
            if (onCreated) {
                onCreated(newTicket);
            }
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
        >
            <Form layout="vertical" form={form} onKeyDown={handleKeyDown}>
                <Form.Item
                    label="Title"
                    name="title"
                    rules={[{ required: true, message: 'Please enter a title' }]}
                >
                    <Input placeholder="Ticket title" disabled={loading} />
                </Form.Item>

                <Form.Item label="Description" name="description">
                    <TextArea rows={3} placeholder="Context, acceptance criteria, links…" disabled={loading} />
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
                        loading={loading}
                        disabled={loading}
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
                        loading={loading}
                        disabled={loading}
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
                        loading={loading}
                        disabled={loading}
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
                    <Input placeholder="you@example.com" disabled={loading} />
                </Form.Item>
            </Form>
        </Modal>
    );
};
