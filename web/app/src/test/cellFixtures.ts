import type { CortexCell } from '@colony2/shared/types';
export const jobIdentity = { cell_id: 'cell-platform', cell_name: 'platform', repo: 'github.com/acme/platform' };
export const registryCell = (name = 'platform', tenantId = 'tenant'): CortexCell => ({
  id: `cell-${name}`, name, tenantId, repo: `github.com/acme/${name}`, state: 'ACTIVE',
  description: '', metadata: {}, aliases: [], revision: 'r1', createdAt: '2026-11-04T00:00:00Z', updatedAt: '2026-11-04T00:00:00Z',
});
