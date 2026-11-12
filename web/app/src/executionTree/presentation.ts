import type { TreeNode } from '@colony2/shared';
import { abbreviatePrompt } from '../utils/goalPresentation';
export { jobStatus, isFinishedJob } from '../utils/jobStatus';

export const headline = (node: TreeNode) => node.title?.trim() || abbreviatePrompt(node.prompt || '') || 'Untitled job';
// Ant Design's light tag palette, also used by the goal list's status tags.
export const statusPalette: Record<string, [string, string, string]> = {
  blue: ['#e6f4ff','#91caff','#0958d9'], gold: ['#fffbe6','#ffe58f','#ad8b00'],
  cyan: ['#e6fffb','#87e8de','#08979c'], purple: ['#f9f0ff','#d3adf7','#531dab'],
  orange: ['#fff7e6','#ffd591','#d46b08'], green: ['#f6ffed','#b7eb8f','#389e0d'],
  red: ['#fff1f0','#ffa39e','#cf1322'], default: ['#fafafa','#d9d9d9','#595959'],
};
