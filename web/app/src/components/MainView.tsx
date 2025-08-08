import { useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Splitter } from 'antd';
import { GraphFlow } from '@vibethis/flowchart';
import SidePanel from './SidePanel';
import type { DependencyCell } from '@vibethis/shared';
import { navigateToPath } from '@vibethis/shared';

export default function MainView() {
  const { cellId, tab } = useParams<{ cellId?: string; tab?: string; subtab?: string }>();
  const navigate = useNavigate();
  const [selectedCell, setSelectedCell] = useState<DependencyCell | null>(null);

  const handleCellSelect = useCallback((cell: DependencyCell | null) => {
    setSelectedCell(cell);
    if (cell) {
      // Navigate to the cell detail page
      const path = navigateToPath({ cellId: cell.id, tab: tab || 'files' });
      navigate(path);
    }
  }, [navigate, tab]);

  return (
    <Splitter style={{ height: '100vh' }}>
      <Splitter.Panel defaultSize="50%" min="20%" max="80%">
        <GraphFlow selectedCellId={cellId} onCellSelect={handleCellSelect} />
      </Splitter.Panel>
      <Splitter.Panel defaultSize="50%" min="20%" max="80%">
        <SidePanel selectedCell={selectedCell} />
      </Splitter.Panel>
    </Splitter>
  );
}