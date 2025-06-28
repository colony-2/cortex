import { useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Splitter } from 'antd';
import { GraphFlow } from '@vibethis/flowchart';
import SidePanel from './SidePanel';
import type { DependencyNode } from '@vibethis/shared';
import { navigateToPath } from '@vibethis/shared';

export default function MainView() {
  const { boxId, tab } = useParams<{ boxId?: string; tab?: string; subtab?: string }>();
  const navigate = useNavigate();
  const [selectedNode, setSelectedNode] = useState<DependencyNode | null>(null);

  const handleNodeSelect = useCallback((node: DependencyNode | null) => {
    setSelectedNode(node);
    if (node) {
      // Navigate to the box detail page
      const path = navigateToPath({ boxId: node.id, tab: tab || 'files' });
      navigate(path);
    }
  }, [navigate, tab]);

  return (
    <Splitter style={{ height: '100vh' }}>
      <Splitter.Panel defaultSize="50%" min="20%" max="80%">
        <GraphFlow selectedNodeId={boxId} onNodeSelect={handleNodeSelect} />
      </Splitter.Panel>
      <Splitter.Panel defaultSize="50%" min="20%" max="80%">
        <SidePanel selectedNode={selectedNode} />
      </Splitter.Panel>
    </Splitter>
  );
}