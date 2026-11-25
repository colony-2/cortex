import type { ReactNode } from 'react';
import Markdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import MarkdownTable from './review/MarkdownTable';
import { storedDocumentURL, type ReviewV2Request } from '@colony2/shared';

type Node = { type: string; value?: string; children?: Node[] };
// Preserve authored prose breaks without interpreting newlines inside fenced
// code, tables or raw HTML. GFM parsing happens before this transform.
function proseBreaks() {
  return (tree: Node) => {
    const visit = (node: Node, prose = false) => {
      if (!node.children || node.type === 'table') return;
      const inside = prose || node.type === 'paragraph' || node.type === 'heading';
      node.children = node.children.flatMap(child => {
        if (inside && child.type === 'text' && child.value?.includes('\n')) {
          return child.value.split('\n').flatMap((value, index) => index ? [{ type: 'break' }, { type: 'text', value }] : [{ type: 'text', value }]);
        }
        visit(child, inside);
        return [child];
      });
    };
    visit(tree);
  };
}

// Stable component types preserve links and keyboard focus across layout changes.
const components: Components = {
  table: MarkdownTable,
  a: ({ children, href }) => /^(https?:|mailto:)/i.test(href || '')
    ? <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>
    : <span>{children} (link unavailable)</span>,
  img: ({ alt }) => <span>Image unavailable: {alt || 'unbound image'}</span>,
};
const remarkPlugins = [remarkGfm, proseBreaks];

export default function ReviewMessageBody({ request, tenant, hasDocument, children }: {
  request: ReviewV2Request;
  tenant: string;
  hasDocument: boolean;
  children?: ReactNode;
}) {
  const lastTurn = request.conversation[request.conversation.length - 1];
  const answer = lastTurn?.role === 'agent' ? lastTurn : undefined;
  const files = new Map<string,string>();
  for (const { name, stored } of answer?.attachments ?? []) {
    const url = storedDocumentURL(tenant, stored.key);
    if (!files.has(url)) files.set(url, (name || stored.key.name).split('/').pop()!);
  }
  const hasPrompt = !!request.question.trim();
  if (!answer?.text.trim() && !files.size && !hasPrompt && !children) return null;

  return <>
    <section aria-label="Review message" className="review-message-body">
      {answer?.text.trim() && <p className="review-message-answer">{answer.text}</p>}
      {Array.from(files, ([url, name]) =>
        <p key={url}><a href={url} download>
          Download {name}
        </a></p>
      )}
      {hasPrompt && <div className="review-v2-question-markdown">
        <Markdown skipHtml remarkPlugins={remarkPlugins} components={components}>{request.question}</Markdown>
      </div>}
      {children}
    </section>
    {hasDocument && <hr className="review-message-divider" />}
  </>;
}
