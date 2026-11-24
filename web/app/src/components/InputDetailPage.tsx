import { hasThreadInput, useThreadInput, ThreadSnapshotContext } from '../hooks/useThreadInput';
import { useContext } from 'react';
import { clearReviewDraft } from '../hooks/useReviewDraft';
import { useCompact } from '../hooks/useCompact';
import ReviewJobContext from './ReviewJobContext';
import { useEffect, useRef, useState } from 'react';
import { useParams, useNavigate, useSearchParams, useLocation, Navigate } from 'react-router-dom';
import { reviewReturnSearch } from '../utils/reviewFilters';
import { Card, Button, Space, Typography, message, Spin, Alert } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { inputActivityService, useInputActivity, getJob, type FormResponse, type RecipeJob, parseReviewV2 } from '@colony2/shared';
import ReviewWorkspace from './ReviewWorkspace';
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
  return selected !== null && jobId ? <ReviewV2 key={JSON.stringify([projectId,jobId,selected])} tenant={projectId} job={jobId} reviewId={selected}/> : <InputRequestReader key={JSON.stringify([projectId,jobId])} projectId={projectId}/>;
}
export function InputRequestReader({ projectId, embedded = false, metadata }: InputDetailPageProps & { embedded?: boolean; metadata?: RecipeJob }) {
  const compact = useCompact();
  const {setCurrentProjectId} = useInputActivity();
  useEffect(() => {setCurrentProjectId(projectId);}, [projectId, setCurrentProjectId]);
  const { jobId } = useParams<{ jobId: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const [params] = useSearchParams();
  const reviewsPath = `/project/${encodeURIComponent(projectId)}/reviews${reviewReturnSearch(params)}`;
  const thread = useContext(ThreadSnapshotContext);
  const unified = hasThreadInput(thread);
  const selectedInput = useThreadInput(projectId, jobId ?? '');
  const details = selectedInput.details ?? selectedInput.readableDetails;
  const [jobError, setJobError] = useState<string>();
  const [jobRetry, setJobRetry] = useState(0);
  const [jobChecking, setJobChecking] = useState(true);
  const reviews = details?.form.kind === 'review';
  const pageClassName = [reviews && 'review-page', embedded && 'embedded-review'].filter(Boolean).join(' ');
  const loading = jobChecking || selectedInput.loading || selectedInput.discovery.status === 'loading';
  const [submitting, setSubmitting] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submitBusy = useRef(false);
  const cancelBusy = useRef(false);
  useEffect(() => {
    if (unified) {setJobChecking(false); setJobError(undefined); return;}
    let active = true;
    setJobChecking(true); setJobError(undefined);
    if (!jobId) {setJobChecking(false); setJobError('No job ID provided'); return;}
    getJob(projectId, jobId ?? '').then(() => {
      if (active) setJobChecking(false);
    }, error => { if (active) {setJobChecking(false); setJobError(error instanceof Error ? error.message : String(error));} });
    return () => { active = false; };
  }, [projectId, jobId, jobRetry, unified]);

  const refresh = () => {setJobRetry(value => value + 1); void selectedInput.refresh().catch(() => {});};
  const requestIdentity = JSON.stringify([projectId, jobId, details?.form.request_id ?? selectedInput.readableVersion]);
  // Advertised details establish the reader; optional job context loads independently.
  const selection = reviews
    ? resolveReviewDocumentLocation(location.search, details.form.request_id!, details.form.documents || {}, details.form.document_metadata, details.form.overview_document_id)
    : undefined;
  const [unavailableWarning, setUnavailableWarning] = useState<string>();
  useEffect(() => { setUnavailableWarning(undefined); }, [requestIdentity]);
  useEffect(() => {
    if (selection?.kind !== 'selected') return;
    if (selection.unavailable) setUnavailableWarning(requestIdentity);
    if (selection.search !== location.search) {
      navigate({ pathname: location.pathname, search: selection.search, hash: location.hash }, { replace: true, state: location.state });
    }
  }, [selection?.kind, selection?.kind === 'selected' ? selection.search : undefined, location.pathname, location.search, location.hash, navigate, requestIdentity]);

  const selectDocument = (id: string) => {
    if (selection?.kind !== 'selected' || id === selection.documentId) return;
    setUnavailableWarning(undefined);
    navigate({ pathname: location.pathname, search: reviewDocumentSearch(location.search, details!.form.request_id!, id), hash: location.hash }, { state: location.state });
  };
  const openCurrentReview = () => {
    setUnavailableWarning(undefined);
    navigate({ pathname: location.pathname, search: reviewDocumentSearch(location.search, details!.form.request_id!,
      startingReviewDocument(details!.form.documents || {}, details!.form.document_metadata, details!.form.overview_document_id)), hash: location.hash }, { replace: true, state: location.state });
  };

  const handleSubmit = async (response: FormResponse) => {
    if (!jobId || !selectedInput.details || submitBusy.current || cancelBusy.current) return;
    submitBusy.current = true;

    setSubmitting(true);
    setError(null);

    try {
      if (details?.form.kind === 'review') {
        await inputActivityService.submitReview(projectId, jobId, details.form.request_id!, response);
        clearReviewDraft(requestIdentity);
      } else {
        await inputActivityService.submitResponse(projectId, jobId, response);
      }
      message.success('Response submitted successfully');
      if (embedded) { await selectedInput.refresh().catch(() => {}); } else navigate(reviewsPath);
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
    navigate(embedded ? `/project/${encodeURIComponent(projectId)}/jobs` : reviewsPath);
  };

  if (!details && loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>
          <Text type="secondary">Checking pending requests…</Text>
        </div>
      </div>
    );
  }

  if (!details || !jobId) {
    return (
      <div className={pageClassName} style={{ padding: embedded ? 0 : 24 }}>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Button icon={<ArrowLeftOutlined />} onClick={handleCancel} disabled={submitting || cancelling}>
            Back to Reviews
          </Button>
          <Alert message={jobError ? 'Could not load job' : selectedInput.error ? 'Could not load this request' : selectedInput.discovery.status === 'unavailable' ? 'Pending request availability is unavailable.' : 'No outstanding input request.'}
            description={jobError || selectedInput.error || selectedInput.discovery.error}
            type={jobError || selectedInput.error ? 'error' : selectedInput.discovery.status === 'unavailable' ? 'warning' : 'info'} showIcon
            action={<Button onClick={refresh}>Refresh</Button>} />
          {jobId && <Button onClick={() => navigate(`/project/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/story`)}>Open job</Button>}
        </Space>
      </div>
    );
  }

  if (details.form.presentation?.type || details.form.response_schema) {
    if(details.form.presentation?.type !== 'review/v2') return <Alert type="error" message="Unsupported input presentation" description={`This client cannot answer ${details.form.presentation?.type ?? 'this structured request'}.`}/>;
    try {
      const request = parseReviewV2(details.form.request);
      if(!details.form.request_id || !details.form.request_schema || !details.form.response_schema)throw new Error('Missing native structured request contract');
      const next = new URLSearchParams(location.search); next.set('review_id', request.review_id);
      return embedded ? <ReviewV2 key={JSON.stringify([projectId,jobId,request.review_id])} tenant={projectId} job={jobId} reviewId={request.review_id} initial={details} embedded metadata={metadata} /> : <Navigate replace to={{pathname:location.pathname,search:'?'+next,hash:location.hash}}/>;
    } catch(e) {return <Alert type="error" message="Incompatible review request" description={e instanceof Error ? e.message : String(e)}/>;}
  }
  const form = adaptInputFormConfig(details.form, jobId);
  const reviewPrompt = reviewActionField(form)?.label;

  return (
    <div className={pageClassName} style={{ padding: embedded ? 0 : 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        {!embedded && <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
          <Button icon={<ArrowLeftOutlined />} onClick={handleCancel} disabled={submitting || cancelling}>
            Back to Reviews
          </Button>
          {reviews && <CancelJobButton key={`${projectId}/${jobId}`} projectId={projectId} jobId={jobId}
            disabled={submitting} onBusyChange={busy => { cancelBusy.current = busy; setCancelling(busy); }} onCancelled={handleCancel} />}
        </Space>}

        {!selectedInput.details && <Alert showIcon type={selectedInput.error ? 'error' : selectedInput.discovery.status === 'unavailable' ? 'warning' : 'info'}
          message={selectedInput.error ? 'Could not load this request' : selectedInput.discovery.status === 'unavailable' ? 'Pending request availability is unavailable.' : selectedInput.loading ? 'Checking pending requests…' : 'No outstanding input request.'}
          description={selectedInput.error || selectedInput.discovery.error}
          action={<Button onClick={refresh}>Refresh</Button>} />}
        {!embedded && <div className={compact && reviews ? "review-secondary" : undefined}>
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
        </div>}
        {embedded && reviews && <Title level={4} style={{ margin: 0 }}>{details.form.title || 'Review'}</Title>}
        {reviews && selection && selection.kind !== 'selected' ? <Alert showIcon type="warning"
          message={selection.kind === 'invalid' ? 'Invalid review link' : 'This linked review is no longer current'}
          description={selection.kind === 'invalid' ? 'The link contains repeated request or document parameters.' : 'The current review has a different request. Open it deliberately to read its documents and respond.'}
          action={<Button onClick={openCurrentReview}>Open current review</Button>} /> : reviews ? <>
        {(selection?.kind === 'selected' && selection.unavailable || unavailableWarning === requestIdentity) && <Alert showIcon type="warning" message="The linked document is unavailable. Showing the starting document, if one is attached." />}
        <ReviewWorkspace key={requestIdentity} projectId={projectId} jobId={jobId}
          documentMetadata={details.form.document_metadata} overviewDocumentId={details.form.overview_document_id}
          selection={{ documentId: selection?.kind === 'selected' ? selection.documentId : undefined, onSelect: selectDocument }}
          requestId={details.form.request_id!} documents={details.form.documents || {}} response={
            <Card title="Your response"><InputFormRenderer key={requestIdentity} submissionError={error} form={form} draftIdentity={requestIdentity} reviewPromptShown={!embedded && !!reviewPrompt} onSubmit={handleSubmit} onCancel={handleCancel} loading={submitting || cancelling || !selectedInput.details} /></Card>
          } /></> : <Card title={form.title}>
          {error && <Alert type="error" showIcon message={error} closable onClose={() => setError(null)} style={{ marginBottom: 16 }} />}
          <InputFormRenderer key={requestIdentity} form={form} draftIdentity={requestIdentity} reviewPromptShown={!embedded && !!reviewPrompt} onSubmit={handleSubmit} onCancel={handleCancel} loading={submitting || cancelling || !selectedInput.details} />
        </Card>}
      </Space>
    </div>
  );
}
