import { useEffect, useState } from 'react';
import { useParams, useNavigate, Navigate } from 'react-router-dom';
import { Card, Button, Space, Typography, message, Spin, Alert } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { inputActivityService, type UserInputDetails, type FormResponse } from '@colony2/shared';
import ReviewDocuments from './ReviewDocuments';
import InputFormRenderer from './InputFormRenderer';
import { adaptInputFormConfig } from '../utils/formAdapter';

const { Title, Text } = Typography;

interface InputDetailPageProps {
  projectId: string;
  reviews?: boolean;
}

export default function InputDetailPage({ projectId, reviews = false }: InputDetailPageProps) {
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

    let active = true;
    setLoading(true);
    setDetails(null);
    setError(null);
    inputActivityService.getInputDetails(projectId, jobId).then(data => {
      if (active) setDetails(data);
    }).catch(() => {
      if (active) setError(`Failed to load ${reviews ? 'review' : 'input'} details. The request may no longer exist.`);
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [projectId, jobId, reviews]);

  const handleSubmit = async (response: FormResponse) => {
    if (!jobId) return;

    setSubmitting(true);

    try {
      if (details?.form.kind === 'review') {
        await inputActivityService.submitReview(projectId, jobId, details.form.request_id!, response);
      } else {
        await inputActivityService.submitResponse(projectId, jobId, response);
      }
      message.success('Response submitted successfully');
      navigate(reviews ? `/project/${projectId}/reviews` : `/project/${projectId}/jobs/${jobId}/story`);
    } catch (err) {
      console.error('Failed to submit response:', err);
      if (reviews) setError(err instanceof Error ? err.message : 'Failed to submit review.');
      else message.error('Failed to submit response. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = () => {
    navigate(`/project/${projectId}/${reviews ? 'reviews' : 'inputs'}`);
  };

  if (loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>
          <Text type="secondary">{reviews ? 'Loading review...' : 'Loading input request...'}</Text>
        </div>
      </div>
    );
  }

  if (!details || !jobId) {
    return (
      <div style={{ padding: 24 }}>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Button icon={<ArrowLeftOutlined />} onClick={handleCancel}>
            {reviews ? 'Back to Reviews' : 'Back to Inputs'}
          </Button>
          <Alert message="Error" description={error || 'Input request not found'} type="error" showIcon />
        </Space>
      </div>
    );
  }

  const isReview = details.form.kind === 'review';
  if (isReview !== reviews) return <Navigate to={`/project/${projectId}/${isReview ? 'reviews' : 'inputs'}/${jobId}`} replace />;
  const form = adaptInputFormConfig(details.form, jobId);

  return (
    <div style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <Button icon={<ArrowLeftOutlined />} onClick={handleCancel}>
          {reviews ? 'Back to Reviews' : 'Back to Inputs'}
        </Button>

        <Card>
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <div>
              <Title level={4} style={{ margin: 0 }}>
                {reviews ? details.form.title || details.form.question || 'Review' : 'Input Request'}
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

        {error && <Alert type="error" showIcon message={error} closable onClose={() => setError(null)} />}
        {reviews && <ReviewDocuments key={details.form.request_id} projectId={projectId} jobId={jobId}
          requestId={details.form.request_id!} documents={details.form.documents || {}} />}
        <Card title={reviews ? 'Your response' : form.title}>
          <InputFormRenderer key={details.form.request_id || jobId} form={form} onSubmit={handleSubmit} onCancel={handleCancel} loading={submitting} />
        </Card>
      </Space>
    </div>
  );
}
