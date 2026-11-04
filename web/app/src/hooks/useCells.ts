import { useEffect, useState } from 'react';
import { listCells } from '@colony2/shared/api';
import type { CortexCell } from '@colony2/shared/types';

// Pickers traverse registry pages independently of the loaded jobs page.
export function useCells(projectId: string, state: 'ACTIVE' | 'ALL', revision = 0) {
  const [cells, setCells] = useState<CortexCell[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let current = true;
    setCells([]); setLoading(true); setError(undefined);
    (async () => {
      let token: string | undefined;
      const seen = new Set<string>();
      const found = new Map<string, CortexCell>();
      do {
        const page = await listCells(projectId, { state, pageToken: token, pageSize: 100 });
        if (!current) return;
        page.cells.forEach(cell => found.set(cell.id, cell));
        setCells([...found.values()]);
        token = page.nextPageToken;
        if (token && seen.has(token)) throw new Error('Cell registry cursor did not advance');
        if (token) seen.add(token);
      } while (token);
    })().catch(reason => { if (current) setError(reason instanceof Error ? reason.message : 'Could not load cells'); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [projectId, state, revision, retry]);
  return { cells, loading, error, reload: () => setRetry(value => value + 1) };
}
