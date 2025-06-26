export interface DependencyNode {
  id: string;
  name: string;
  path: string;
  type: string;
  relationships: string[];
}

export interface DependencyEdge {
  id: string;
  source: string;
  target: string;
}

export interface NodePosition {
  nodeId: string;
  x: number;
  y: number;
}

export interface RelationshipGraph {
  nodes: DependencyNode[];
  edges: DependencyEdge[];
}

export interface DependencyGraph {
  nodes: DependencyNode[];
  edges: DependencyEdge[];
}

export interface FileItem {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  type: string;
}