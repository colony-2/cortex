import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Card, Button, Space, Typography, message, Spin, Alert } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { inputActivityService, type UserInputDetails, type FormResponse } from '@colony2/shared';
import InputFormRenderer from './InputFormRenderer';
import { adaptInputFormConfig } from '../utils/formAdapter';

const { Title, Text } = Typography;

interface InputDetailPageProps {
  projectId: string;
}

export default function InputDetailPage({ projectId }: InputDetailPageProps) {
  const { jobId } = useParams<{ jobId: string }>();
  const navigate = useNavigate();
  const [details, setDetails] = useState<UserInputDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!jobId) {
      setError('No job ID provided');
      setLoading(false);
      return;
    }

    loadDetails();
  }, [projectId, jobId]);

  const loadDetails = async () => {
    if (!jobId) return;

    setLoading(true);
    setError(null);

    try {
      const data = await inputActivityService.getInputDetails(projectId, jobId);
      setDetails(data);
    } catch (err) {
      console.error('Failed to load input details:', err);
      setError('Failed to load input details. The input request may no longer exist.');
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async (response: FormResponse) => {
    if (!jobId) return;

    setSubmitting(true);

    try {
      await inputActivityService.submitResponse(projectId, jobId, response);
      message.success('Response submitted successfully');
      navigate(`/project/${projectId}/workflows/${jobId}`);
    } catch (err) {
      console.error('Failed to submit response:', err);
      message.error('Failed to submit response. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = () => {
    navigate(`/project/${projectId}/inputs`);
  };

  if (loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>
          <Text type="secondary">Loading input request...</Text>
        </div>
      </div>
    );
  }

  if (error || !details || !jobId) {
    return (
      <div style={{ padding: 24 }}>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Button icon={<ArrowLeftOutlined />} onClick={handleCancel}>
            Back to Inputs
          </Button>
          <Alert message="Error" description={error || 'Input request not found'} type="error" showIcon />
        </Space>
      </div>
    );
  }

  const form = adaptInputFormConfig(details.form, jobId);

  return (
    <div style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <Button icon={<ArrowLeftOutlined />} onClick={handleCancel}>
          Back to Inputs
        </Button>

        <Card>
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <div>
              <Title level={4} style={{ margin: 0 }}>
                Input Request
              </Title>
              <Text type="secondary">Job ID: {details.jobId}</Text>
            </div>

            <div>
              <Text type="secondary">Status: </Text>
              <Text strong>{details.status}</Text>
            </div>

            <div>
              <Text type="secondary">Started: </Text>
              <Text>{new Date(details.startTime).toLocaleString()}</Text>
            </div>
          </Space>
        </Card>

        <Card title={form.title}>
          <InputFormRenderer form={form} onSubmit={handleSubmit} onCancel={handleCancel} loading={submitting} />
        </Card>
      </Space>
    </div>
  );
}
