import Markdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import MarkdownTable from './review/MarkdownTable';

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

export default function ReviewQuestion({ question }: { question: string }) {
  return <article className="review-v2-turn agent">
    <strong>Agent</strong>
    <div className="review-v2-question-markdown"><Markdown skipHtml remarkPlugins={remarkPlugins} components={components}>{question}</Markdown></div>
  </article>;
}
