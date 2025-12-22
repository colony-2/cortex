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
  version?: number;
  name: string;
  gitRepoPath: string;
  gitRepoBranch?: string;
  defaultTicketRecipe?: string;
  createdAt?: string;
  updatedAt?: string;
}
