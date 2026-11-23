import type { Components } from 'react-markdown';

const MarkdownTable: Components['table'] = ({ node: _node, ...props }) => (
  <div className="review-table-scroll" role="region" aria-label="Markdown table" tabIndex={0}>
    <table {...props} />
  </div>
);

export default MarkdownTable;
