import { useAssistantSource } from './AssistantContext';
import { useCompact } from '../hooks/useCompact';
import ReviewJobContext from './ReviewJobContext';
import { useEffect, useRef, useState } from 'react';
import { useParams, useNavigate, useSearchParams, useLocation } from 'react-router-dom';
import { reviewReturnSearch } from '../utils/reviewFilters';
import { Card, Button, Space, Typography, message, Spin, Alert } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { inputActivityService, type UserInputDetails, type FormResponse, parseReviewV2 } from '@colony2/shared';
import ReviewDocuments from './ReviewDocuments';
import InputFormRenderer from './InputFormRenderer';
import { adaptInputFormConfig } from '../utils/formAdapter';
import { reviewActionField } from '../utils/reviewForm';
import ReviewV2 from './ReviewV2';
import { resolveReviewDocumentLocation, reviewDocumentSearch, startingReviewDocument } from '../utils/reviewDocumentLocation';
import CancelJobButton from './CancelJobButton';

const { Title, Text } = Typography;

interface InputDetailPageProps {
  projectId: string;
}

export default function InputDetailPage({ projectId }: InputDetailPageProps) {
  const [search] = useSearchParams();
  const {jobId} = useParams<{jobId:string}>();
  const selected=search.get('review_id');
  return selected && jobId ? <ReviewV2 key={JSON.stringify([projectId,jobId,selected])} tenant={projectId} job={jobId} reviewId={selected}/> : <InputRequestPage projectId={projectId}/>;
}
function InputRequestPage({ projectId }: InputDetailPageProps) {
  const compact = useCompact();
  const { jobId } = useParams<{ jobId: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const pageIdentity = JSON.stringify([projectId, jobId]);
  const [loadedIdentity, setLoadedIdentity] = useState<string>();
  const [params] = useSearchParams();
  const reviewsPath = `/project/${encodeURIComponent(projectId)}/reviews${reviewReturnSearch(params)}`;
  const [details, setDetails] = useState<UserInputDetails | null>(null);
  const reviews = details?.form.kind === 'review';
  useAssistantSource(reviews && jobId && details.form.request_id ? { kind: 'review', job_id: jobId, request_id: details.form.request_id } : undefined);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submitBusy = useRef(false);
  const cancelBusy = useRef(false);
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
      if (active) { setDetails(data); setLoadedIdentity(key); }
    }).catch(() => {
      if (active) { setLoadedIdentity(key); setError('Failed to load request details. The request may no longer exist.'); }
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [projectId, jobId]);

  const reviewIdentity = JSON.stringify([projectId, jobId, details?.form.request_id]);
  const selection = !loading && loadedIdentity === pageIdentity && reviews
    ? resolveReviewDocumentLocation(location.search, details.form.request_id!, details.form.documents || {}, details.form.document_metadata, details.form.overview_document_id)
    : undefined;
  const [unavailableWarning, setUnavailableWarning] = useState<string>();
  useEffect(() => { setUnavailableWarning(undefined); }, [reviewIdentity]);
  useEffect(() => {
    if (selection?.kind !== 'selected') return;
    if (selection.unavailable) setUnavailableWarning(reviewIdentity);
    if (selection.search !== location.search) {
      navigate({ pathname: location.pathname, search: selection.search, hash: location.hash }, { replace: true });
    }
  }, [selection?.kind, selection?.kind === 'selected' ? selection.search : undefined, location.pathname, location.search, location.hash, navigate, reviewIdentity]);

  const selectDocument = (id: string) => {
    if (selection?.kind !== 'selected' || id === selection.documentId) return;
    setUnavailableWarning(undefined);
    navigate({ pathname: location.pathname, search: reviewDocumentSearch(location.search, details!.form.request_id!, id), hash: location.hash });
  };
  const openCurrentReview = () => {
    setUnavailableWarning(undefined);
    navigate({ pathname: location.pathname, search: reviewDocumentSearch(location.search, details!.form.request_id!,
      startingReviewDocument(details!.form.documents || {}, details!.form.document_metadata, details!.form.overview_document_id)), hash: location.hash }, { replace: true });
  };

  const handleSubmit = async (response: FormResponse) => {
    if (!jobId || submitBusy.current || cancelBusy.current) return;
    submitBusy.current = true;

    setSubmitting(true);
    setError(null);

    try {
      if (details?.form.kind === 'review') {
        await inputActivityService.submitReview(projectId, jobId, details.form.request_id!, response);
      } else {
        await inputActivityService.submitResponse(projectId, jobId, response);
      }
      message.success('Response submitted successfully');
      navigate(reviewsPath);
    } catch (err) {
      console.error('Failed to submit response:', err);
      if (reviews) setError(err instanceof Error ? err.message : 'Failed to submit review.');
      else message.error('Failed to submit response. Please try again.');
    } finally {
      submitBusy.current = false;
      setSubmitting(false);
    }
  };

  const handleCancel = () => {
    navigate(reviewsPath);
  };

  if (loading || (jobId && loadedIdentity !== pageIdentity)) {
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
          <Button icon={<ArrowLeftOutlined />} onClick={handleCancel} disabled={submitting || cancelling}>
            Back to Reviews
          </Button>
          <Alert message="Error" description={error || 'Input request not found'} type="error" showIcon />
        </Space>
      </div>
    );
  }

  if (details.form.presentation?.type || details.form.response_schema) {
    if(details.form.presentation?.type !== 'review/v2') return <Alert type="error" message="Unsupported input presentation" description={`This client cannot answer ${details.form.presentation?.type ?? 'this structured request'}.`}/>;
    try {
      const request = parseReviewV2(details.form.request);
      if(!details.form.request_id || !details.form.request_schema || !details.form.response_schema)throw new Error('Missing native structured request contract');
      return <ReviewV2 tenant={projectId} job={jobId} reviewId={request.review_id} initial={details}/>;
    } catch(e) {return <Alert type="error" message="Incompatible review request" description={e instanceof Error ? e.message : String(e)}/>;}
  }
  const form = adaptInputFormConfig(details.form, jobId);
  const reviewPrompt = reviewActionField(form)?.label;

  return (
    <div className={reviews ? 'review-page' : undefined} style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
          <Button icon={<ArrowLeftOutlined />} onClick={handleCancel} disabled={submitting || cancelling}>
            Back to Reviews
          </Button>
          {reviews && <CancelJobButton key={`${projectId}/${jobId}`} projectId={projectId} jobId={jobId}
            disabled={submitting} onBusyChange={busy => { cancelBusy.current = busy; setCancelling(busy); }} onCancelled={handleCancel} />}
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

        {reviews && <ReviewJobContext key={`${projectId}/${jobId}`} projectId={projectId} jobId={jobId} compact={compact} relationships />}
        {reviewPrompt && (compact ? <details><summary>Review instructions</summary>
          <section aria-label="Review context" style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{reviewPrompt}</section>
        </details> : <Card title="Review context">
          <section aria-label="Review context" style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{reviewPrompt}</section>
        </Card>)}
        </div>
        {reviews && selection && selection.kind !== 'selected' ? <Alert showIcon type="warning"
          message={selection.kind === 'invalid' ? 'Invalid review link' : 'This linked review is no longer current'}
          description={selection.kind === 'invalid' ? 'The link contains repeated request or document parameters.' : 'The current review has a different request. Open it deliberately to read its documents and respond.'}
          action={<Button onClick={openCurrentReview}>Open current review</Button>} /> : reviews ? <>
        {(selection?.kind === 'selected' && selection.unavailable || unavailableWarning === reviewIdentity) && <Alert showIcon type="warning" message="The linked document is unavailable. Showing the starting document, if one is attached." />}
        <ReviewDocuments key={reviewIdentity} projectId={projectId} jobId={jobId}
          documentMetadata={details.form.document_metadata} overviewDocumentId={details.form.overview_document_id}
          selection={{ documentId: selection?.kind === 'selected' ? selection.documentId : undefined, onSelect: selectDocument }}
          requestId={details.form.request_id!} documents={details.form.documents || {}} response={
            <Card title="Your response"><InputFormRenderer key={JSON.stringify([projectId, jobId, details.form.request_id])} submissionError={error} form={form} reviewPromptShown={!!reviewPrompt} onSubmit={handleSubmit} onCancel={handleCancel} loading={submitting || cancelling} /></Card>
          } /></> : <Card title={form.title}>
          {error && <Alert type="error" showIcon message={error} closable onClose={() => setError(null)} style={{ marginBottom: 16 }} />}
          <InputFormRenderer key={JSON.stringify([projectId, jobId, details.form.request_id])} form={form} reviewPromptShown={!!reviewPrompt} onSubmit={handleSubmit} onCancel={handleCancel} loading={submitting || cancelling} />
        </Card>}
      </Space>
    </div>
  );
}
