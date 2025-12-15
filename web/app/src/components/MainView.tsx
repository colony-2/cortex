import { useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Splitter } from 'antd';
import { GraphFlow } from '@colony2/flowchart';
import SidePanel from './SidePanel';
import type { DependencyCell } from '@colony2/shared';
import { navigateToPath } from '@colony2/shared';

interface MainViewProps {
  projectId: string;
}

export default function MainView({ projectId }: MainViewProps) {
  const { cellId, tab } = useParams<{ cellId?: string; tab?: string; subtab?: string }>();
  const navigate = useNavigate();
  const [selectedCell, setSelectedCell] = useState<DependencyCell | null>(null);

  const handleCellSelect = useCallback((cell: DependencyCell | null) => {
    setSelectedCell(cell);
    if (cell) {
      // Navigate to the cell detail page
      const path = navigateToPath({ projectId, cellId: cell.id, tab: tab || 'inputs' });
      navigate(path);
    }
  }, [navigate, tab, projectId]);

  return (
    <Splitter style={{ height: '100vh' }}>
      <Splitter.Panel defaultSize="50%" min="20%" max="80%">
        <GraphFlow projectId={projectId} selectedCellId={cellId} onCellSelect={handleCellSelect} />
      </Splitter.Panel>
      <Splitter.Panel defaultSize="50%" min="20%" max="80%">
        <SidePanel selectedCell={selectedCell} />
      </Splitter.Panel>
    </Splitter>
  );
}
