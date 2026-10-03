import { useEffect, useState } from 'react';
import { Alert, Button, Card, Empty, Radio, Select, Space, Spin, Typography } from 'antd';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { inputActivityService, type ReviewDocument } from '@colony2/shared';

interface Props {
  projectId: string;
  jobId: string;
  requestId: string;
  documents: Record<string, ReviewDocument>;
}

export default function ReviewDocuments({ projectId, jobId, requestId, documents }: Props) {
  const ids = Object.keys(documents).sort();
  const [selection, setSelection] = useState(ids[0]);
  const selected = selection && documents[selection] ? selection : ids[0];
  const [mode, setMode] = useState<'rendered' | 'source'>('rendered');
  const [content, setContent] = useState<{ url: string; text: string }>();
  const [error, setError] = useState<string>();
  const [retry, setRetry] = useState(0);
  const doc = documents[selected];
  const name = doc?.name || doc?.stored.key.name || selected;
  const markdown = /\.(md|markdown)$/i.test(name || '');
  const url = selected ? inputActivityService.reviewDocumentURL(projectId, jobId, requestId, selected) : '';

  useEffect(() => {
    setContent(undefined);
    setError(undefined);
    if (!url || !markdown) return;
    const controller = new AbortController();
    fetch(url, { signal: controller.signal }).then(async response => {
      if (!response.ok) throw new Error('Could not load this document. It may be unavailable or the review may have changed.');
      const text = await response.text();
      if (!controller.signal.aborted) setContent({ url, text });
    }).catch(error => {
      if (!controller.signal.aborted) setError(error.message);
    });
    return () => controller.abort();
  }, [url, markdown, retry]);

  return (
    <Card title={`Documents (${ids.length})`} extra={<Typography.Text type="secondary">Read only</Typography.Text>}>
      {!selected ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No documents attached" /> : (
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <div className="review-document-toolbar">
            <Select aria-label="Review document" value={selected} onChange={setSelection} style={{ minWidth: 220, maxWidth: '100%' }}
              options={ids.map(id => ({ value: id, label: `${id} · ${documents[id].name || documents[id].stored.key.name}` }))} />
            <Space wrap>
              {markdown && <Radio.Group aria-label="Document display" value={mode} onChange={event => setMode(event.target.value)} optionType="button"
                options={[{ label: 'Rendered', value: 'rendered' }, { label: 'Markdown', value: 'source' }]} />}
              <Button href={url} download={name}>Download</Button>
            </Space>
          </div>
          {!markdown ? <Alert type="info" showIcon message="Preview is available for Markdown documents. Download this file to view it." />
            : error ? <Alert type="error" showIcon message={error} action={<Button onClick={() => setRetry(value => value + 1)}>Retry</Button>} />
            : !content || content.url !== url ? <Spin aria-label="Loading document" />
            : mode === 'source' ? <pre className="review-markdown-source" aria-label="Markdown source">{content.text}</pre>
            : <article className="review-markdown" aria-label="Rendered document">
              <Markdown remarkPlugins={[remarkGfm]} skipHtml components={{
                a: ({ children, href }) => <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>,
              }}>{content.text}</Markdown>
            </article>}
        </Space>
      )}
    </Card>
  );
}
