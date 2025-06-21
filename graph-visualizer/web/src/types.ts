export interface Node {
  id: string;
  name: string;
  path: string;
  dependencies: string[];
}

export interface Edge {
  source: string;
  target: string;
}

export interface Graph {
  nodes: Node[];
  edges: Edge[];
}