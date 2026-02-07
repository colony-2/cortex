export type NotebookBackendMode = 'stub' | 'real';

export type RecipePublishStatus = 'published' | 'unpublished';

export interface NotebookRecipeSummary {
  name: string;
  status: RecipePublishStatus;
  published_ref: string | null;
  latest_ref: string | null;
  updated_at: string | null;
}

export interface NotebookRecipeWithContent {
  name: string;
  ref: string | null;
  status: RecipePublishStatus;
  content: string;
  updated_at: string | null;
}

export type JobStatus =
  | 'running'
  | 'completed'
  | 'failed'
  | 'canceled'
  | 'terminated'
  | 'timed_out'
  | 'unknown';

export type NodeStatus =
  | 'pending'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'canceled'
  | 'skipped'
  | 'unknown';

export type StoryKind =
  | 'recipe'
  | 'sequence'
  | 'op'
  | 'opStep'
  | 'stateMachine'
  | 'state'
  | 'transitionEval'
  | 'contextPatch';

export interface ArtifactKey {
  jobId: string;
  taskOrdinal: number;
  name: string;
  sizeBytes?: number;
}

export type TransitionDecision = { kind: 'state'; to_state_id: string } | { kind: 'fallthrough' };

export interface TransitionEvaluation {
  expression: string;
  result: boolean;
  to_state_id: string;
}

export interface StoryNode {
  kind: StoryKind;
  title: string;
  status: NodeStatus;
  started_at?: string | null;
  finished_at?: string | null;
  path: string[];
  invoke_seq?: number;
  input?: unknown | null;
  output?: unknown | null;
  artifact_keys?: ArtifactKey[];
  children?: StoryNode[];
  attempt?: number;
  prior_attempts?: StoryNode[];
  task_ordinal?: number | null;
  restart_from_ordinal?: number | null;

  evaluations?: TransitionEvaluation[];
  decision?: TransitionDecision;
}

export interface WorkflowStoryResponse {
  job_id: string;
  invocation_sequence?: number;
  recipe?: unknown;
  status: JobStatus;
  started_at?: string | null;
  finished_at?: string | null;
  root?: StoryNode | null;
}

export interface NotebookBackend {
  mode: NotebookBackendMode;
  listRecipes(projectId: string): Promise<NotebookRecipeSummary[]>;
  getRecipe(projectId: string, recipeName: string, ref?: string): Promise<NotebookRecipeWithContent>;
  getJobStory(projectId: string, jobId: string): Promise<WorkflowStoryResponse>;
  startRun(projectId: string, recipeName: string, context?: unknown): Promise<{ jobId: string }>;
  stepRun(projectId: string, jobId: string): Promise<WorkflowStoryResponse>;
  cancelRun(projectId: string, jobId: string): Promise<void>;
  upsertDraftRecipe(projectId: string, recipeName: string, content: string): Promise<void>;
}

