import type { ComponentProps } from 'react';
import { Tabs } from 'antd';
import ReviewDocuments from './ReviewDocuments';
import ReviewChanges from './ReviewChanges';

// The parent keys this reader by tenant/job/request. Panels stay mounted after
// their first visit; the response remains a stable sibling of both panels.
export default function ReviewWorkspace({ response, ...documents }: ComponentProps<typeof ReviewDocuments>) {
  return <div className="review-workspace">
    <Tabs defaultActiveKey="documents" items={[
      { key: 'documents', label: 'Documents', children: <ReviewDocuments {...documents} /> },
      { key: 'changes', label: 'Changes', children: <ReviewChanges projectId={documents.projectId} jobId={documents.jobId} /> },
    ]} />
    <div className="review-workspace-response">{response}</div>
  </div>;
}
