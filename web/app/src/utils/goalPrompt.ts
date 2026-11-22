import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkGfm from 'remark-gfm';

const parser = unified().use(remarkParse).use(remarkGfm);

// Empty headings, rules, quotes, lists and code blocks have no prompt content.
// Inspect meaning without rewriting the Markdown that will be submitted.
export function hasPromptContent(markdown: string): boolean {
  type Node = { type: string; value?: string; children?: readonly Node[] };
  const containsContent = (node: Node): boolean => {
    if (node.type === 'image' || node.type === 'imageReference') return true;
    if (node.type === 'code' || node.type === 'inlineCode') return !!node.value?.trim();
    if (node.type === 'text') return !!node.value?.replace(/[\s*_~`]/g, '');
    return node.children?.some(containsContent) ?? false;
  };
  return containsContent(parser.parse(markdown));
}
