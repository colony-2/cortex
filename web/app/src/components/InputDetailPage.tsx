import { useCompact } from '../hooks/useCompact';
import ReviewJobContext from './ReviewJobContext';
import { useEffect, useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Card, Button, Space, Typography, message, Spin, Alert } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { inputActivityService, type UserInputDetails, type FormResponse } from '@colony2/shared';
import ReviewDocuments from './ReviewDocuments';
import InputFormRenderer from './InputFormRenderer';
import { adaptInputFormConfig } from '../utils/formAdapter';
import { reviewActionField } from '../utils/reviewForm';
import CancelJobButton from './CancelJobButton';

const { Title, Text } = Typography;

interface InputDetailPageProps {
  projectId: string;
}

export default function InputDetailPage({ projectId }: InputDetailPageProps) {
  const compact = useCompact();
  const { jobId } = useParams<{ jobId: string }>();
  const navigate = useNavigate();
  const [details, setDetails] = useState<UserInputDetails | null>(null);
  const reviews = details?.form.kind === 'review';
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const formRequest = useRef<{ key: string; promise: Promise<UserInputDetails> }>();

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
    const key = JSON.stringify([projectId, jobId]);
    if (formRequest.current?.key !== key) formRequest.current = { key, promise: inputActivityService.getInputDetails(projectId, jobId, true) };
    formRequest.current.promise.then(data => {
      if (active) setDetails(data);
    }).catch(() => {
      if (active) setError('Failed to load request details. The request may no longer exist.');
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [projectId, jobId]);

  const handleSubmit = async (response: FormResponse) => {
    if (!jobId) return;

    setSubmitting(true);
    setError(null);

    try {
      if (details?.form.kind === 'review') {
        await inputActivityService.submitReview(projectId, jobId, details.form.request_id!, response);
      } else {
        await inputActivityService.submitResponse(projectId, jobId, response);
      }
      message.success('Response submitted successfully');
      navigate(`/project/${projectId}/reviews`);
    } catch (err) {
      console.error('Failed to submit response:', err);
      if (reviews) setError(err instanceof Error ? err.message : 'Failed to submit review.');
      else message.error('Failed to submit response. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = () => {
    navigate(`/project/${projectId}/reviews`);
  };

  if (loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>
          <Text type="secondary">Loading request...</Text>
        </div>
      </div>
    );
  }

  if (!details || !jobId) {
    return (
      <div className={reviews ? 'review-page' : undefined} style={{ padding: 24 }}>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Button icon={<ArrowLeftOutlined />} onClick={handleCancel}>
            Back to Reviews
          </Button>
          <Alert message="Error" description={error || 'Input request not found'} type="error" showIcon />
        </Space>
      </div>
    );
  }

  const form = adaptInputFormConfig(details.form, jobId);
  const reviewPrompt = reviewActionField(form)?.label;

  return (
    <div className={reviews ? 'review-page' : undefined} style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
          <Button icon={<ArrowLeftOutlined />} onClick={handleCancel}>
            Back to Reviews
          </Button>
          {reviews && <CancelJobButton key={`${projectId}/${jobId}`} projectId={projectId} jobId={jobId}
            disabled={submitting} onBusyChange={setCancelling} onCancelled={handleCancel} />}
        </Space>

        <div className={compact && reviews ? "review-secondary" : undefined}>
        {compact && reviews ? <Card>
          <Title level={4} style={{ margin: 0 }}>{details.form.title || details.form.question || 'Review'}</Title>
          <details><summary>Request details</summary>
            <p>Job ID: {details.jobId}</p><p>Input status: {details.status}</p>
            <p>Input started: {new Date(details.startTime).toLocaleString()}</p>
          </details>
        </Card> : (<Card style={{ overflowWrap: 'anywhere' }}>
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <div>
              <Title level={4} style={{ margin: 0 }}>
                {reviews ? details.form.title || details.form.question || 'Review' : 'Input Request'}
              </Title>
              <Text type="secondary">Job ID: {details.jobId}</Text>
            </div>

            <div>
              <Text type="secondary">{reviews ? 'Input status: ' : 'Status: '}</Text>
              <Text strong>{details.status}</Text>
            </div>

            <div>
              <Text type="secondary">{reviews ? 'Input started: ' : 'Started: '}</Text>
              <Text>{new Date(details.startTime).toLocaleString()}</Text>
            </div>
          </Space>
        </Card>)}

        {reviews && <details className="review-goal-context" open={!compact || undefined}>
          <summary>Goal context</summary>
          <ReviewJobContext key={`${projectId}/${jobId}`} projectId={projectId} jobId={jobId} />
        </details>}
        {reviewPrompt && (compact ? <details><summary>Review instructions</summary>
          <section aria-label="Review context" style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{reviewPrompt}</section>
        </details> : <Card title="Review context">
          <section aria-label="Review context" style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{reviewPrompt}</section>
        </Card>)}
        </div>
        {reviews && <ReviewDocuments key={details.form.request_id} projectId={projectId} jobId={jobId}
          requestId={details.form.request_id!} documents={details.form.documents || {}} />}
        <Card title={reviews ? 'Your response' : form.title}>
          {error && <Alert type="error" showIcon message={error} closable onClose={() => setError(null)} style={{ marginBottom: 16 }} />}
          <InputFormRenderer key={details.form.request_id || jobId} form={form} reviewPromptShown={!!reviewPrompt} onSubmit={handleSubmit} onCancel={handleCancel} loading={submitting || cancelling} />
        </Card>
      </Space>
    </div>
  );
}
