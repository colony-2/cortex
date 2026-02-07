import { RecipesService } from '@colony2/openapi-client';
import type {
  NotebookBackend,
  NotebookBackendMode,
  NotebookRecipeSummary,
  NotebookRecipeWithContent,
  WorkflowStoryResponse,
} from './types';
import { createStubBackend } from './stubBackend';

const DEFAULT_API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

function normalizeApiBase(apiBase?: string): string {
  const base = (apiBase ?? DEFAULT_API_BASE).trim();
  return base.endsWith('/') ? base.slice(0, -1) : base;
}

async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(`Request failed: ${res.status} ${res.statusText}${text ? ` - ${text}` : ''}`);
  }
  return res.json() as Promise<T>;
}

function createRealBackend(options?: { apiBase?: string }): NotebookBackend {
  const apiBase = normalizeApiBase(options?.apiBase);

  return {
    mode: 'real',
    async listRecipes(projectId: string): Promise<NotebookRecipeSummary[]> {
      const resp = await RecipesService.listRecipes(projectId, 'all');
      const recipes = resp.recipes ?? [];
      return recipes.map((r) => ({
        name: r.name,
        status: r.publishedCommit ? 'published' : 'unpublished',
        published_ref: r.publishedCommit ?? null,
        latest_ref: r.latestCommit ?? null,
        updated_at: r.latestCommitAt ?? null,
      }));
    },
    async getRecipe(projectId: string, recipeName: string, ref?: string): Promise<NotebookRecipeWithContent> {
      const resp = await RecipesService.getRecipe(projectId, recipeName, ref);
      return {
        name: resp.name,
        ref: resp.commitHash ?? null,
        status: resp.isPublished ? 'published' : 'unpublished',
        content: resp.rawYaml,
        updated_at: resp.publishedAt ?? null,
      };
    },
    async getJobStory(projectId: string, jobId: string): Promise<WorkflowStoryResponse> {
      return fetchJson<WorkflowStoryResponse>(
        `${apiBase}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/story`,
      );
    },
    async startRun(): Promise<{ jobId: string }> {
      throw new Error('Starting runs is not implemented on the real backend in this mock UI. Use stub mode.');
    },
    async stepRun(): Promise<WorkflowStoryResponse> {
      throw new Error('Stepping runs is not implemented on the real backend in this mock UI. Use stub mode.');
    },
    async cancelRun(): Promise<void> {
      throw new Error('Cancel is not implemented on the real backend in this mock UI. Use stub mode.');
    },
    async upsertDraftRecipe(): Promise<void> {
      throw new Error('Draft authoring is not implemented on the real backend in this mock UI. Use stub mode.');
    },
  };
}

export function createNotebookBackend(mode: NotebookBackendMode, options?: { apiBase?: string }): NotebookBackend {
  if (mode === 'stub') return createStubBackend();
  return createRealBackend(options);
}
