import type { LosslessNumber } from 'lossless-json';

// Stored int64 values outside JavaScript's safe range stay exact.
export type StoredInteger = number | LosslessNumber;

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
  title?: string;
  metadata?: Record<string, unknown>;
  schema_hash?: string;
  blocking_wait_summary?: { waitId: string; deadline?: string };
  goal_id?: string;
  other_goal_ids?: string[];
  prompt?: string;
  blocking_wait?: DependencyWait;
  tenant_id: string;
  job_id: string;
  status: RecipeJobStatus;
  completion_status?: string;
  completion_detail?: string;
  store: 'ACTIVE' | 'ARCHIVED';
  job_type: string;
  recipe: string;
  repo?: string;
  cell_id?: string;
  cell_name?: string;
  git_ref?: string;
  input_hash?: string;
  parent?: {
    goal_id?: string;
    tenant_id?: string;
    job_id?: string;
    job_type?: string;
    op_type?: string;
    op_step?: string;
    op_task_type?: string;
    cell_name?: string;
    repo?: string;
    git_ref?: string;
    invocation_path?: string;
    invocation_seq?: StoredInteger;
    invocation_hash?: string;
  };
  submitted_at?: string;
  created_at: string;
  available_at: string;
  archived_at?: string;
  lease_expires_at?: string;
  expires_at?: string;
  next_route?: { jobType: string; taskType?: string };
  task_wait?: {
    inputOrdinal: StoredInteger;
    outputOrdinal: StoredInteger;
    inputHash: string;
    resumeJobType: string;
  };
  client_payload?: unknown;
  client_payload_revision?: StoredInteger;
  execution?: {
    status: string;
    source: string;
    published: boolean;
    diagnostic?: string;
    demand?: Record<string, unknown>;
    initial?: Record<string, unknown>;
  };
  wait_for?: string[];
  cancel_requested?: boolean;
}

export interface DependencyWait {
  waitId: string;
  state: string;
  deadline?: string;
  dependencies: {
    target: { goalId?: string; jobId?: string };
    condition: string;
    state: string;
    satisfiedByJobId?: string;
    blockedReason?: string;
  }[];
}

// Stored fields are available in lists; memberships and wait explanations need inspection.
export type RecipeJobSummary = Omit<RecipeJob, 'other_goal_ids' | 'blocking_wait'>;

export interface ListRecipeJobsResponse {
  jobs: RecipeJobSummary[];
  next_page_token?: string;
}

export interface SubmitRecipeJobRequest {
  cell?: string;
  repo?: string;
  recipe?: string;
  inputs?: Record<string, unknown>;
  job_id?: string;
}

export interface RestartScope {
  chapter_ordinal: number;
  scope_id: string;
  started_at: string;
  title?: string;
  path?: string[];
}

export interface RestartContext {
  project_id: string;
  job_id: string;
  chapter_ordinal: number;
  scope_id: string;
  job: Record<string, unknown>;
  visible: Record<string, unknown>;
  redacted_paths: string[] | null;
}
