export interface DependencyCell {
  id: string;
  name: string;
  path: string;
  type: string;
  dependencies: string[];
}

export interface DependencyEdge {
  id: string;
  source: string;
  target: string;
}

export interface RelationshipGraph {
  cells: DependencyCell[];
  edges: DependencyEdge[];
}

export interface Project {
  id: string;
  name: string;
  tenant_id?: string;
}

export interface CortexCell {
  id: string;
  project_id: string;
  tenant_id: string;
  name: string;
  repo: string;
  repository_source: string;
  git_ref?: string;
  kind: 'self' | 'dependent' | string;
}

export type RecipeJobStatus =
  | 'READY'
  | 'EXPIRED'
  | 'PENDING_JOBS'
  | 'AWAITING_FUTURE'
  | 'ACTIVE'
  | 'CRASH_CONCERN'
  | 'CANCELLED'
  | 'COMPLETED';

export interface RecipeJob {
  tenant_id: string;
  job_id: string;
  status: RecipeJobStatus;
  store: 'ACTIVE' | 'ARCHIVED';
  job_type: string;
  recipe: string;
  repo?: string;
  cell_id?: string;
  cell_name?: string;
  git_ref?: string;
  input_hash?: string;
  submitted_at?: string;
  created_at: string;
  available_at: string;
  archived_at?: string;
  lease_expires_at?: string;
  expires_at?: string;
  next_need?: string;
  task_wait_next?: string;
  wait_for?: string[];
  cancel_requested?: boolean;
}

export interface ListRecipeJobsResponse {
  jobs: RecipeJob[];
  next_page_token?: string;
}

export interface SubmitRecipeJobRequest {
  cell?: string;
  repo?: string;
  recipe?: string;
  inputs?: Record<string, unknown>;
  job_id?: string;
}
