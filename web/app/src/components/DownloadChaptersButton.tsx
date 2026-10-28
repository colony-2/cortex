import { useEffect, useRef, useState } from 'react';
import { Alert, Button } from 'antd';
import { DownloadOutlined } from '@ant-design/icons';
import { getJobChaptersDownload } from '@colony2/shared/api';

function filename(jobId: string): string {
  const safe = Array.from(jobId.replace(/[^\p{L}\p{N}_-]/gu, '_').replace(/^_+|_+$/g, '')).slice(0, 40).join('');
  return `job-${safe || 'export'}-chapters.json`;
}

/** Key by tenant/job at the call site so navigation cancels the previous action. */
export default function DownloadChaptersButton({ projectId, jobId }: { projectId: string; jobId: string }) {
  const request = useRef<AbortController | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => () => { request.current?.abort(); }, []);

  const download = async () => {
    if (!jobId || request.current) return;
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError(null);
    try {
      const blob = await getJobChaptersDownload(projectId, jobId, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      try {
        link.href = url;
        link.download = filename(jobId);
        document.body.appendChild(link);
        link.click();
      } finally {
        link.remove();
        // Give the browser time to consume the URL before releasing it.
        window.setTimeout(() => URL.revokeObjectURL(url), 0);
      }
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Download failed');
    } finally {
      if (!controller.signal.aborted) {
        request.current = null;
        setLoading(false);
      }
    }
  };

  return <div>
    <Button aria-label="Download chapters" icon={<DownloadOutlined />} disabled={!jobId} loading={loading} onClick={download}>Download chapters</Button>
    {error && <Alert type="error" showIcon message="Could not download chapters" description={`${error}. Select Download chapters to retry.`} />}
  </div>;
}
