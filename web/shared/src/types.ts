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

export interface CellPosition {
  cellId: string;
  x: number;
  y: number;
}

export interface RelationshipGraph {
  cells: DependencyCell[];
  edges: DependencyEdge[];
}

export interface DependencyGraph {
  cells: DependencyCell[];
  edges: DependencyEdge[];
}

export interface FileItem {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  type: string;
}