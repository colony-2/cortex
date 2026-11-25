import { useEffect, useState } from 'react';
import { getRecipeSource, type RecipeGitSource } from '@colony2/shared';

export default function RecipeSource({projectId, jobId, terminal}: {projectId: string; jobId: string; terminal: boolean}) {
  const [source, setSource] = useState<RecipeGitSource>();
  const [error, setError] = useState(false);
  const [revision, retry] = useState(0);
  useEffect(() => {
    const abort = new AbortController();
    setError(false);
    getRecipeSource(projectId, jobId, abort.signal).then(value => {
      if (!abort.signal.aborted) setSource(value);
    }).catch(() => { if (!abort.signal.aborted) setError(true); });
    return () => abort.abort();
  }, [projectId, jobId, terminal, revision]);
  return <section className="job-environment" aria-label="Recipe source">
    <h2>Recipe source</h2>
    {error ? <p>Recipe source unavailable. <button onClick={() => retry(n => n + 1)}>Retry</button></p>
      : !source ? <p role="status">Loading recipe source…</p>
      : source.path ? <dl>
        <div><dt>Recipe Git path</dt><dd><code>{source.path}</code></dd></div>
        <div><dt>Commit hash</dt><dd><code>{source.commit || 'Not recorded'}</code></dd></div>
      </dl> : <p>No Git source recorded.</p>}
  </section>;
}
