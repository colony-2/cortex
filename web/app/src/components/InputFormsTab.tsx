import { useState, useEffect } from 'react';
import { List, Typography, Badge, Empty, Spin, message, Divider } from 'antd';
import { ClockCircleOutlined, CheckCircleOutlined, ExclamationCircleOutlined } from '@ant-design/icons';
import type { DependencyCell } from '@vibethis/shared';
import { 
  inputActivityService, 
  type PendingInput, 
  type InputFormDetails,
  type InputEvent,
  type FormResponse 
} from '@vibethis/shared';
import InputFormRenderer from './InputFormRenderer';

const { Title, Text } = Typography;

interface InputFormsTabProps {
  cell: DependencyCell | null;
  cellId?: string;
  initialInputId?: string;
}

export default function InputFormsTab({ cell, cellId, initialInputId }: InputFormsTabProps) {
  const effectiveCellId = cell?.id || cellId;
  const [pendingInputs, setPendingInputs] = useState<PendingInput[]>([]);
  const [completedInputs, setCompletedInputs] = useState<PendingInput[]>([]);
  const [selectedInput, setSelectedInput] = useState<string | null>(null);
  const [currentFormDetails, setCurrentFormDetails] = useState<InputFormDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [formLoading, setFormLoading] = useState(false);

  // Load pending inputs
  const loadPendingInputs = async () => {
    if (!effectiveCellId) return;
    
    try {
      const inputs = await inputActivityService.getPendingInputs(effectiveCellId);
      
      // Sort by creation time (oldest first)
      const sorted = inputs.sort((a, b) => 
        new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
      );
      
      // Separate pending and completed
      const pending = sorted.filter(i => i.status === 'pending');
      const completed = sorted.filter(i => i.status === 'completed');
      
      setPendingInputs(pending);
      setCompletedInputs(completed);
      
      // Auto-select the oldest pending input if none selected
      if (!selectedInput && pending.length > 0) {
        const inputToSelect = initialInputId || pending[0].workflowId;
        setSelectedInput(inputToSelect);
      }
    } catch (error) {
      console.error('Failed to load pending inputs:', error);
      message.error('Failed to load pending inputs');
    } finally {
      setLoading(false);
    }
  };

  // Load form details for selected input
  const loadFormDetails = async (workflowId: string) => {
    setFormLoading(true);
    try {
      const details = await inputActivityService.getInputDetails(workflowId);
      setCurrentFormDetails(details);
    } catch (error) {
      console.error('Failed to load form details:', error);
      message.error('Failed to load form details');
      setCurrentFormDetails(null);
    } finally {
      setFormLoading(false);
    }
  };

  // Handle form submission
  const handleSubmit = async (response: FormResponse) => {
    if (!selectedInput) return;
    
    setSubmitting(true);
    try {
      await inputActivityService.submitResponse(selectedInput, response);
      message.success('Response submitted successfully');
      
      // Remove from pending, add to completed
      setPendingInputs(prev => prev.filter(i => i.workflowId !== selectedInput));
      const submittedInput = pendingInputs.find(i => i.workflowId === selectedInput);
      if (submittedInput) {
        setCompletedInputs(prev => [...prev, { ...submittedInput, status: 'completed' }]);
      }
      
      // Select next pending input if available
      const remaining = pendingInputs.filter(i => i.workflowId !== selectedInput);
      if (remaining.length > 0) {
        setSelectedInput(remaining[0].workflowId);
      } else {
        setSelectedInput(null);
        setCurrentFormDetails(null);
      }
    } catch (error) {
      console.error('Failed to submit response:', error);
      message.error('Failed to submit response');
    } finally {
      setSubmitting(false);
    }
  };

  // Handle form cancellation
  const handleCancel = async () => {
    if (!selectedInput) return;
    
    setSubmitting(true);
    try {
      await inputActivityService.cancelInput(selectedInput);
      message.info('Input cancelled');
      
      // Remove from pending
      setPendingInputs(prev => prev.filter(i => i.workflowId !== selectedInput));
      
      // Select next pending input if available
      const remaining = pendingInputs.filter(i => i.workflowId !== selectedInput);
      if (remaining.length > 0) {
        setSelectedInput(remaining[0].workflowId);
      } else {
        setSelectedInput(null);
        setCurrentFormDetails(null);
      }
    } catch (error) {
      console.error('Failed to cancel input:', error);
      message.error('Failed to cancel input');
    } finally {
      setSubmitting(false);
    }
  };

  // Subscribe to SSE events
  useEffect(() => {
    if (!effectiveCellId) return;
    
    const unsubscribe = inputActivityService.subscribe(effectiveCellId, (event: InputEvent) => {
      if (event.type === 'input_requested' || event.type === 'input_completed' || event.type === 'input_expired') {
        // Reload the list when inputs change
        loadPendingInputs();
      }
    });
    
    return unsubscribe;
  }, [effectiveCellId]);

  // Load pending inputs on mount or when cellId changes
  useEffect(() => {
    loadPendingInputs();
  }, [effectiveCellId]);

  // Load form details when selected input changes
  useEffect(() => {
    if (selectedInput) {
      loadFormDetails(selectedInput);
    } else {
      setCurrentFormDetails(null);
    }
  }, [selectedInput]);

  // Calculate time remaining
  const getTimeRemaining = (expiresAt: string) => {
    const now = new Date().getTime();
    const expiry = new Date(expiresAt).getTime();
    const diff = expiry - now;
    
    if (diff <= 0) return { text: 'Expired', status: 'overdue' };
    
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(minutes / 60);
    const days = Math.floor(hours / 24);
    
    if (days > 0) return { text: `${days} day${days > 1 ? 's' : ''}`, status: 'pending' };
    if (hours > 0) return { text: `${hours} hour${hours > 1 ? 's' : ''}`, status: 'pending' };
    if (minutes > 5) return { text: `${minutes} min`, status: 'pending' };
    return { text: `${minutes} min`, status: 'urgent' };
  };

  // Get status icon
  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'urgent':
        return <ExclamationCircleOutlined style={{ color: '#fa8c16' }} />;
      case 'overdue':
        return <ExclamationCircleOutlined style={{ color: '#f5222d' }} />;
      case 'completed':
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />;
      default:
        return <ClockCircleOutlined style={{ color: '#1890ff' }} />;
    }
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', height: '100%' }}>
      {/* Left Navigation */}
      <div style={{ 
        width: '250px', 
        borderRight: '1px solid #f0f0f0', 
        overflowY: 'auto',
        background: '#fafafa'
      }}>
        <div style={{ padding: '16px' }}>
          <Title level={5}>Pending Inputs ({pendingInputs.length})</Title>
        </div>
        
        <List
          dataSource={pendingInputs}
          renderItem={(item) => {
            const timeInfo = getTimeRemaining(item.expiresAt);
            const isSelected = selectedInput === item.workflowId;
            
            return (
              <List.Item
                style={{ 
                  padding: '12px 16px', 
                  cursor: 'pointer',
                  background: isSelected ? '#e6f7ff' : 'transparent',
                  borderLeft: isSelected ? '3px solid #1890ff' : '3px solid transparent',
                }}
                onClick={() => setSelectedInput(item.workflowId)}
              >
                <List.Item.Meta
                  avatar={getStatusIcon(timeInfo.status)}
                  title={<Text strong={isSelected}>{item.formTitle}</Text>}
                  description={
                    <Text type="secondary" style={{ fontSize: '12px' }}>
                      {timeInfo.text} remaining
                    </Text>
                  }
                />
              </List.Item>
            );
          }}
          locale={{ emptyText: 'No pending inputs' }}
        />
        
        {completedInputs.length > 0 && (
          <>
            <Divider style={{ margin: '8px 0' }} />
            <div style={{ padding: '16px' }}>
              <Title level={5} type="secondary">Completed ({completedInputs.length})</Title>
            </div>
            
            <List
              dataSource={completedInputs}
              renderItem={(item) => (
                <List.Item
                  style={{ 
                    padding: '12px 16px',
                    opacity: 0.6
                  }}
                >
                  <List.Item.Meta
                    avatar={getStatusIcon('completed')}
                    title={<Text>{item.formTitle}</Text>}
                    description={
                      <Text type="secondary" style={{ fontSize: '12px' }}>
                        Completed
                      </Text>
                    }
                  />
                </List.Item>
              )}
            />
          </>
        )}
      </div>
      
      {/* Main Content Area */}
      <div style={{ flex: 1, padding: '24px', overflowY: 'auto' }}>
        {formLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
            <Spin size="large" />
          </div>
        ) : currentFormDetails ? (
          <>
            <Title level={3}>{currentFormDetails.formTitle}</Title>
            {currentFormDetails.context?.workflowName && (
              <Text type="secondary">
                Workflow: {currentFormDetails.context.workflowName}
              </Text>
            )}
            {currentFormDetails.context?.activityName && (
              <Text type="secondary" style={{ marginLeft: '16px' }}>
                Activity: {currentFormDetails.context.activityName}
              </Text>
            )}
            
            <div style={{ marginTop: '24px' }}>
              <InputFormRenderer
                form={currentFormDetails.form}
                context={currentFormDetails.context}
                onSubmit={handleSubmit}
                onCancel={handleCancel}
                loading={submitting}
              />
            </div>
            
            <div style={{ marginTop: '16px' }}>
              <Badge 
                status={getTimeRemaining(currentFormDetails.expiresAt).status === 'urgent' ? 'warning' : 'processing'} 
                text={`Expires in: ${getTimeRemaining(currentFormDetails.expiresAt).text}`} 
              />
            </div>
          </>
        ) : (
          <Empty 
            description={pendingInputs.length === 0 ? "No pending inputs" : "Select an input from the list"} 
            style={{ marginTop: '20%' }}
          />
        )}
      </div>
    </div>
  );
}