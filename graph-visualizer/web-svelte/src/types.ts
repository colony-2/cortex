export interface DependencyNode {
  id: string;
  name: string;
  path: string;
  dependencies: string[];
}

export interface DependencyEdge {
  source: string;
  target: string;
}

export interface DependencyGraph {
  nodes: DependencyNode[];
  edges: DependencyEdge[];
}