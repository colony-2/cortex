import { useId, useRef, useState, type AriaAttributes } from 'react';
import { Alert, Button, Space } from 'antd';
import { ReviewEditor, type ReviewEditorHandle, type ReviewEditorMarkdownFormatInput } from 'critrev/react';
import type { DraftImages } from '../utils/promptImages';
import { useCompact } from '../hooks/useCompact';

type Props = { id?: string; value?: string; onChange?: (value: string) => void; disabled?: boolean; images: DraftImages; locked: () => boolean } & Pick<AriaAttributes, 'aria-label' | 'aria-describedby' | 'aria-invalid' | 'aria-required'>;
const formats: [string, ReviewEditorMarkdownFormatInput][] = [
  ['Heading', { action: 'line', prefix: '## ', suffix: '', richBlock: 'h2' }],
  ['Bold', { action: 'inline', prefix: '**', suffix: '**', richFormat: 'bold' }],
  ['Italic', { action: 'inline', prefix: '*', suffix: '*', richFormat: 'italic' }],
  ['Bullet list', { action: 'line', prefix: '- ', suffix: '', richBlock: 'ul' }],
  ['Numbered list', { action: 'line', prefix: '1. ', suffix: '', richBlock: 'ol' }],
  ['Quote', { action: 'line', prefix: '> ', suffix: '', richBlock: 'quote' }],
  ['Inline code', { action: 'inline', prefix: '`', suffix: '`', richFormat: 'code' }],
  ['Code block', { action: 'block', prefix: '```\n', suffix: '\n```', richBlock: 'code' }],
  ['Link', { action: 'link', url: 'https://example.com', placeholder: 'link text' }],
];
const preservation = 'This Markdown needs source editing to preserve it. Your text is unchanged.';
export default function GoalPromptEditor({ id, value = '', onChange, disabled, images, locked, 'aria-label': label = 'Prompt', ...aria }: Props) {
  const editor = useRef<ReviewEditorHandle>(null);
  const surface = useRef<HTMLDivElement>(null);
  const mixedPaste = useRef(false);
  const [mode, setMode] = useState<'source' | 'wysiwyg'>(() => value ? 'source' : 'wysiwyg');
  const [notice, setNotice] = useState('');
  const compact = useCompact();
  const [moreFormats, setMoreFormats] = useState(false);
  const secondaryId = useId();
  const primaryFormats = new Set(['Bold', 'Italic', 'Bullet list']);
  const formatButton = ([label, command]: typeof formats[number]) => <Button key={label} disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={() => { if (!locked()) editor.current?.applyMarkdownFormat(command); }}>{label}</Button>;
  const switchMode = () => {
    if (locked()) return;
    if (mode === 'source') {
      try { if (!editor.current?.inspectMarkdown(value).exact) { setNotice(preservation); return; } }
      catch { setNotice(preservation); return; }
    }
    setNotice(''); setMode(mode === 'source' ? 'wysiwyg' : 'source');
  };
  const history = (redo: boolean) => {
    if (locked()) return;
    const root = surface.current?.querySelector<HTMLElement>('[contenteditable]');
    root?.focus();
    const apple = /Mac|iPhone|iPad|iPod/.test(navigator.platform);
    root?.dispatchEvent(new KeyboardEvent('keydown', { key: 'z', code: 'KeyZ', ctrlKey: !apple, metaKey: apple, shiftKey: redo, bubbles: true, cancelable: true }));
  };
  return <div className="goal-prompt-editor" ref={surface}>
    <div className="goal-editor-controls">
      <Space><strong>{mode === 'source' ? 'Markdown source' : 'Rich text'}</strong><Button disabled={disabled} onClick={switchMode}>{mode === 'source' ? 'Rich text' : 'Markdown source'}</Button></Space>
      <div role="toolbar" aria-label="Text formatting" className="goal-editor-toolbar">
        {formats.filter(([label]) => !compact || primaryFormats.has(label)).map(formatButton)}
        {compact && <Button disabled={disabled} aria-expanded={moreFormats} aria-controls={secondaryId} onMouseDown={event => event.preventDefault()} onClick={() => { if (!locked()) setMoreFormats(value => !value); }}>More formats</Button>}
        <Button disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={() => history(false)}>Undo</Button>
        <Button disabled={disabled} onMouseDown={event => event.preventDefault()} onClick={() => history(true)}>Redo</Button>
      </div>
      {compact && <div id={secondaryId} role="group" aria-label="More text formats" className="goal-editor-toolbar" hidden={!moreFormats}>
        {formats.filter(([label]) => !primaryFormats.has(label)).map(formatButton)}
      </div>}
    </div>
    {notice && <Alert role="status" type="info" message={notice} />}
    <ReviewEditor {...aria} ref={editor} id={id} aria-label={label} role="textbox" aria-multiline="true" className={`goal-editor-surface ${mode}`} value={{ text: value, suggestions: [], comments: [], highlights: [] }} contentMode={mode} trackChanges={false} readOnly={disabled}
      onPasteCapture={event => { mixedPaste.current = event.clipboardData.types.some(type => type === 'text/html' || type === 'text/plain'); }}
      imagePreviews={images.previews()} onChange={next => { if (!locked()) onChange?.(next.text); }}
      onPasteImages={files => { if (locked()) return { reject: true }; try { setNotice(''); return images.stage(files); } catch (error) { setNotice((error as Error).message); return { reject: true }; } }}
      onPasteImagesResult={outcome => { images.finish(outcome); if (outcome.inserted && mixedPaste.current) setNotice('Pasted images only; accompanying text and HTML were omitted.'); mixedPaste.current = false; }} onError={error => setNotice(error.message)} />
    <div className="goal-editor-help">Paste images directly. PNG, JPEG, GIF and WebP. Markdown is saved with the goal.</div>
  </div>;
}
