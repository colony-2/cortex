import React, { useEffect, useState } from 'react';
import { Modal, Descriptions, Timeline, Tag, Spin, Alert, Tabs } from 'antd';
import type {
    Ticket,
    TicketEvent,
    Actor,
} from '@colony2/openapi-client';
import { TicketsService } from '@colony2/openapi-client';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';

dayjs.extend(relativeTime);

export interface TicketDetailModalProps {
    /** Whether the modal is visible */
    visible: boolean;
    /** Callback when modal is closed */
    onClose: () => void;
    /** Ticket to display details for */
    ticket: Ticket | null;
    /** Project ID (required for fetching events) */
    projectId: string;
}

export const TicketDetailModal: React.FC<TicketDetailModalProps> = ({
    visible,
    onClose,
    ticket,
    projectId,
}) => {
    const [events, setEvents] = useState<TicketEvent[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // Fetch events when ticket changes
    useEffect(() => {
        if (!ticket || !visible) {
            setEvents([]);
            setError(null);
            return;
        }

        const fetchEvents = async () => {
            setLoading(true);
            setError(null);
            try {
                const fetchedEvents = await TicketsService.getApiProjectsTicketsEvents(
                    projectId,
                    ticket.id,
                );
                // Sort by timestamp descending (newest first)
                const sorted = [...fetchedEvents].sort(
                    (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
                );
                setEvents(sorted);
            } catch (err) {
                console.error('Failed to fetch ticket events:', err);
                setError('Failed to load ticket events. Please try again.');
            } finally {
                setLoading(false);
            }
        };

        fetchEvents();
    }, [ticket, projectId, visible]);

    if (!ticket) {
        return null;
    }

    return (
        <Modal
            title={`Ticket: ${ticket.title}`}
            open={visible}
            onCancel={onClose}
            footer={null}
            width={800}
            afterClose={() => setEvents([])}
        >
            <Tabs
                defaultActiveKey="details"
                items={[
                    {
                        key: 'details',
                        label: 'Details',
                        children: <TicketDetails ticket={ticket} />,
                    },
                    {
                        key: 'events',
                        label: `Events (${events.length})`,
                        children: loading ? (
                            <div style={{ textAlign: 'center', padding: '40px 0' }}>
                                <Spin size="large" />
                            </div>
                        ) : error ? (
                            <Alert type="error" message={error} showIcon />
                        ) : (
                            <TicketEventsTimeline events={events} />
                        ),
                    },
                ]}
            />
        </Modal>
    );
};

// Sub-component: Ticket Details Tab
const TicketDetails: React.FC<{ ticket: Ticket }> = ({ ticket }) => {
    return (
        <Descriptions bordered column={1} size="small">
            <Descriptions.Item label="ID">{ticket.id}</Descriptions.Item>
            <Descriptions.Item label="Cell">
                <Tag color="blue">{ticket.cellName}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Stage">
                <Tag color="blue">{ticket.stage}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="State">
                <Tag color={getStateColor(ticket.state)}>{ticket.state}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Description">
                {ticket.description || <span style={{ color: '#999' }}>No description</span>}
            </Descriptions.Item>
            <Descriptions.Item label="Creator">
                {formatActor(ticket.creator)}
            </Descriptions.Item>
            <Descriptions.Item label="Created">
                {dayjs(ticket.createdAt).format('YYYY-MM-DD HH:mm:ss')}
                {' '}
                <span style={{ color: '#999' }}>({dayjs(ticket.createdAt).fromNow()})</span>
            </Descriptions.Item>
            <Descriptions.Item label="Updated">
                {dayjs(ticket.updatedAt).format('YYYY-MM-DD HH:mm:ss')}
                {' '}
                <span style={{ color: '#999' }}>({dayjs(ticket.updatedAt).fromNow()})</span>
            </Descriptions.Item>
            {ticket.completedAt && (
                <Descriptions.Item label="Completed">
                    {dayjs(ticket.completedAt).format('YYYY-MM-DD HH:mm:ss')}
                    {' '}
                    <span style={{ color: '#999' }}>({dayjs(ticket.completedAt).fromNow()})</span>
                </Descriptions.Item>
            )}
        </Descriptions>
    );
};

// Sub-component: Events Timeline Tab
const TicketEventsTimeline: React.FC<{ events: TicketEvent[] }> = ({ events }) => {
    if (events.length === 0) {
        return (
            <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
                No events found for this ticket.
            </div>
        );
    }

    return (
        <Timeline
            style={{ marginTop: 16 }}
            items={events.map(event => ({
                color: getEventColor(event),
                children: <EventItem event={event} />,
            }))}
        />
    );
};

// Sub-component: Individual Event Item
const EventItem: React.FC<{ event: TicketEvent }> = ({ event }) => {
    const renderEventDetails = () => {
        switch (event.kind) {
            case 'workflow':
                if (!event.workflowPayload) return null;
                return (
                    <div>
                        <strong>Workflow {event.workflowPayload.type}</strong>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            Workflow ID: <code>{event.workflowPayload.workflowId}</code>
                        </div>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            Run ID: <code>{event.workflowPayload.runId}</code>
                        </div>
                    </div>
                );

            case 'ticket':
                if (!event.ticketPayload) return null;
                return (
                    <div>
                        <strong>Ticket updated</strong>
                        {event.ticketPayload.stage && (
                            <div style={{ fontSize: '12px' }}>Stage: {event.ticketPayload.stage}</div>
                        )}
                        {event.ticketPayload.state && (
                            <div style={{ fontSize: '12px' }}>State: {event.ticketPayload.state}</div>
                        )}
                        {event.ticketPayload.notes && (
                            <div style={{ fontSize: '12px', fontStyle: 'italic' }}>
                                "{event.ticketPayload.notes}"
                            </div>
                        )}
                    </div>
                );

            case 'markdown_doc':
                if (!event.markdownPayload) return null;
                return (
                    <div>
                        <strong>Document {event.markdownPayload.type}</strong>
                        <div style={{ fontSize: '12px' }}>{event.markdownPayload.name}</div>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            <code>{event.markdownPayload.path}</code>
                        </div>
                    </div>
                );

            case 'changeset':
                if (!event.changesetPayload) return null;
                return (
                    <div>
                        <strong>Code {event.changesetPayload.type}</strong>
                        <div style={{ fontSize: '12px' }}>{event.changesetPayload.commit_message}</div>
                        <div style={{ fontSize: '12px', color: '#666' }}>
                            <code>{event.changesetPayload.path}</code>
                        </div>
                    </div>
                );

            case 'reset':
                if (!event.resetPayload) return null;
                return (
                    <div>
                        <strong>History reset</strong>
                        <div style={{ fontSize: '12px' }}>{event.resetPayload.reason}</div>
                    </div>
                );

            default:
                return <div>Unknown event type</div>;
        }
    };

    return (
        <div>
            {renderEventDetails()}
            <div style={{ fontSize: '11px', color: '#999', marginTop: 4 }}>
                {dayjs(event.timestamp).format('YYYY-MM-DD HH:mm:ss')} ({dayjs(event.timestamp).fromNow()})
                {' · '}
                {formatActor(event.actor)}
            </div>
        </div>
    );
};

// Helper: Get color for ticket state
function getStateColor(state: string): string {
    switch (state) {
        case 'working':
            return 'green';
        case 'waiting_user':
            return 'orange';
        case 'waiting_dependency':
            return 'blue';
        case 'waiting_capacity':
            return 'purple';
        default:
            return 'default';
    }
}

// Helper: Get color for event timeline dot
function getEventColor(event: TicketEvent): string {
    switch (event.kind) {
        case 'workflow':
            if (event.workflowPayload?.type === 'completed') return 'green';
            if (event.workflowPayload?.type === 'failed') return 'red';
            return 'blue';
        case 'ticket':
            return 'gray';
        case 'changeset':
            return 'purple';
        case 'markdown_doc':
            return 'cyan';
        case 'reset':
            return 'orange';
        default:
            return 'gray';
    }
}

// Helper: Format actor for display
function formatActor(actor: Actor): string {
    if (actor.type === 'user' && actor.user) {
        return actor.user.email;
    }
    if (actor.type === 'agent' && actor.agent) {
        return `${actor.agent.cell} (agent)`;
    }
    return 'Unknown';
}
