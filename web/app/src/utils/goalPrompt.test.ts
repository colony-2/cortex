import { expect, it } from 'vitest';
import { hasPromptContent } from './goalPrompt';

it('rejects empty Markdown structures and formatting without changing source', () => {
  for (const source of ['', ' \n\t', '---', '***', '## ', '> ', '- ', '1. ', '```\n\n```', '** **', '~~ ~~']) {
    expect(hasPromptContent(source), JSON.stringify(source)).toBe(false);
  }
});

it('accepts authored text, literal code, and image-only Markdown', () => {
  for (const source of ['Build this.', '## Plan', '**Important**', '[Task](https://example.com)', '`**`', '```\n---\n```', '![](pasted.png)', '![diagram][image]\n\n[image]: pasted.png']) {
    expect(hasPromptContent(source), JSON.stringify(source)).toBe(true);
  }
});
