import type { NotebookCellKind } from '../model/notebookModel';
import {
  BranchesOutlined,
  CodeOutlined,
  FileTextOutlined,
  ForkOutlined,
  NodeIndexOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';

export function kindIcon(kind: NotebookCellKind): ReactNode {
  switch (kind) {
    case 'markdown':
      return <FileTextOutlined />;
    case 'recipe':
      return <FileTextOutlined />;
    case 'sequence':
      return <BranchesOutlined />;
    case 'op':
      return <CodeOutlined />;
    case 'stateMachine':
      return <ForkOutlined />;
    case 'state':
      return <NodeIndexOutlined />;
    default:
      return <FileTextOutlined />;
  }
}

export function kindLabel(kind: NotebookCellKind): string {
  switch (kind) {
    case 'markdown':
      return 'Markdown';
    case 'recipe':
      return 'Recipe';
    case 'sequence':
      return 'Sequence';
    case 'op':
      return 'Op';
    case 'stateMachine':
      return 'State Machine';
    case 'state':
      return 'State';
    default:
      return 'Node';
  }
}
