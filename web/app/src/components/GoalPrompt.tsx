import { useEffect, useState } from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { API_BASE, storedDocumentURL } from '@colony2/shared';
import { getJobInputArtifacts, type InputArtifact } from '@colony2/shared/api';
import { IMAGE_LIMIT, loadDocument } from './review/document';
import { markdownImages, rasterPreview, safeImageName } from '../utils/promptImages';

export function promptImageBinding(projectId: string, jobId: string, name: string, artifacts: readonly InputArtifact[]) {
  if (!safeImageName(name)) return undefined;
  const matches = artifacts.filter(artifact => artifact.name === name);
  if (matches.length !== 1) return undefined;
  const artifact = matches[0];
  if (artifact.kind && !['stored', 'value'].includes(artifact.kind)) return undefined;
  const root = `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/`;
  const key = artifact.kind === 'stored' ? artifact.storedKey : undefined;
  const url = key ? storedDocumentURL(projectId, key) : artifact.contentBinding !== undefined
    ? `${root}${encodeURIComponent(jobId)}/input-document?binding=${encodeURIComponent(artifact.contentBinding)}&ordinal=0`
    : artifact.jobId !== undefined && artifact.taskOrdinal !== undefined
      ? `${root}${encodeURIComponent(artifact.jobId)}/tasks/${artifact.taskOrdinal}/artifacts/${encodeURIComponent(artifact.name)}` : undefined;
  return url ? { url, name, size: artifact.sizeBytes } : undefined;
}
function PromptImage({ binding, alt }: { binding: NonNullable<ReturnType<typeof promptImageBinding>>; alt: string }) {
  const [preview, setPreview] = useState<string>();
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    let owned: string | undefined;
    setPreview(undefined); setFailed(false);
    if (binding.size > IMAGE_LIMIT) { setFailed(true); return; }
    loadDocument(binding.url, binding.name, controller.signal).then(async data => {
      if (data.truncated || data.format !== 'image') throw new Error('Invalid image');
      const url = await rasterPreview(new Blob([data.bytes as BlobPart], { type: data.mime }), controller.signal);
      if (controller.signal.aborted) URL.revokeObjectURL(url);
      else { owned = url; setPreview(url); }
    }).catch(() => { if (!controller.signal.aborted) setFailed(true); });
    return () => { controller.abort(); if (owned) URL.revokeObjectURL(owned); };
  }, [binding.url, binding.name, binding.size]);
  return <span className="prompt-bound-image">{preview ? <img src={preview} alt={alt} /> : <span className="prompt-image-placeholder">{failed ? 'Image unavailable' : 'Loading image…'}: {alt || binding.name}</span>} <a href={binding.url} download={binding.name} aria-label={`Download ${binding.name}`}>Download original image</a></span>;
}
type PromptProps = { projectId: string; jobId: string; text: string; inventory?: InputArtifact[]; waitingForInventory?: boolean };
function OwnedPrompt({ projectId, jobId, text, inventory, waitingForInventory }: PromptProps) {
  const [artifacts, setArtifacts] = useState<InputArtifact[]>([]);
  const hasImages = markdownImages(text).length > 0;
  useEffect(() => {
    if (!hasImages || inventory || waitingForInventory) return;
    const controller = new AbortController();
    getJobInputArtifacts(projectId, jobId, controller.signal, 0).then(result => { if (!controller.signal.aborted) setArtifacts(result.artifacts); }).catch(() => {});
    return () => controller.abort();
  }, [projectId, jobId, hasImages, inventory, waitingForInventory]);
  return <div className="job-prompt-markdown"><Markdown remarkPlugins={[remarkGfm]} skipHtml components={{
    a: ({ children, href }) => /^(https?:|mailto:)/i.test(href || '') ? <a href={href} target="_blank" rel="noopener noreferrer">{children}</a> : <span>{children} (link unavailable)</span>,
    img: ({ src, alt }) => {
      const binding = promptImageBinding(projectId, jobId, src || '', inventory ?? artifacts);
      return binding ? <PromptImage key={binding.url} binding={binding} alt={alt || ''} /> : <span className="prompt-image-placeholder">Image unavailable: {alt || 'unbound image'}</span>;
    },
  }}>{text}</Markdown></div>;
}
export default function GoalPrompt(props: PromptProps) {
  // Remount on ownership changes: stale inventory/URLs can never bind another job's prompt.
  return <OwnedPrompt key={JSON.stringify([props.projectId, props.jobId, props.text])} {...props} />;
}
