import { useState, useEffect, useCallback, useRef } from 'react';
import { ReactFlow, applyNodeChanges, Background, Controls, MiniMap } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { message, Spin, Card, Button, Alert, Collapse } from 'antd';
import { fetchGraph, fetchPositions, savePositions, useInputActivity, type RelationshipGraph, type DependencyCell, type CellPosition, type DependencyEdge } from '@vibethis/shared';
import ProFlowCell from './ProFlowCell';

interface FlowNode {
  id: string;
  position: { x: number; y: number };
  type?: string;
  selectable?: boolean;
  data: {
    title: string;
    description?: string;
    logo?: string;
    [key: string]: any;
  };
}

interface FlowEdge {
  id: string;
  source: string;
  target: string;
  type?: string;
}

export interface GraphFlowProps {
  selectedCellId?: string;
  onCellSelect?: (cell: DependencyCell | null) => void;
}

export default function GraphFlow({ selectedCellId, onCellSelect }: GraphFlowProps) {
  const [nodes, setNodes] = useState<FlowNode[]>([]);
  const [edges, setEdges] = useState<FlowEdge[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedCell, setSelectedCell] = useState<DependencyCell | null>(null);
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const graphRef = useRef<RelationshipGraph | null>(null);
  const isInitialLoad = useRef(true);
  const { pendingInputsByCellId } = useInputActivity();

  const handleCellClick = useCallback((cellId: string) => {
    const cell = graphRef.current?.cells.find((c: DependencyCell) => c.id === cellId) || null;
    setSelectedCell(cell);
    onCellSelect?.(cell);
  }, [onCellSelect]);

  const layoutCells = useCallback((graphData: RelationshipGraph, savedPositions: CellPosition[], selectedCellId?: string, pendingInputsByCellId?: Map<string, any[]>) => {
    // Handle null or undefined cells
    if (!graphData.cells || !Array.isArray(graphData.cells)) {
      setNodes([]);
      setEdges([]);
      return;
    }

    const positionMap = new Map(Array.isArray(savedPositions) ? savedPositions.map(p => [p.cellId, p]) : []);

    // Create Pro Flow cells
    const newNodes: FlowNode[] = graphData.cells.map((cell: DependencyCell, index: number) => {
      const savedPosition = positionMap.get(cell.id);
      
      // Calculate position if not saved
      const x = savedPosition?.x ?? (index % 4) * 250 + 100;
      const y = savedPosition?.y ?? Math.floor(index / 4) * 150 + 100;
      
      // Get pending inputs for this cell
      const cellInputs = pendingInputsByCellId?.get(cell.id) || [];
      const pendingInputs = cellInputs.filter((i: any) => i.status === 'pending');
      const pendingInputCount = pendingInputs.length;
      
      // Calculate urgency
      let inputUrgency: 'pending' | 'urgent' | 'overdue' | undefined = undefined;
      if (pendingInputCount > 0) {
        const now = new Date().getTime();
        let hasOverdue = false;
        let hasUrgent = false;
        
        pendingInputs.forEach(input => {
          const expiresAt = new Date(input.expiresAt).getTime();
          const timeRemaining = expiresAt - now;
          
          if (timeRemaining <= 0) {
            hasOverdue = true;
          } else if (timeRemaining <= 5 * 60 * 1000) { // 5 minutes
            hasUrgent = true;
          }
        });
        
        if (hasOverdue) {
          inputUrgency = 'overdue';
        } else if (hasUrgent) {
          inputUrgency = 'urgent';
        } else {
          inputUrgency = 'pending';
        }
      }
      
      return {
        id: cell.id,
        type: 'custom',
        position: { x, y },
        selectable: true,
        data: {
          title: cell.name,
          name: cell.name,
          type: cell.type,
          dependencies: cell.dependencies,
          logo: '📦',
          selected: selectedCellId === cell.id,
          pendingInputCount,
          inputUrgency
        },
      };
    });

    // Create Pro Flow edges
    const newEdges: FlowEdge[] = graphData.edges && Array.isArray(graphData.edges) 
      ? graphData.edges.map((edge: DependencyEdge) => ({
          id: edge.id,
          source: edge.source,
          target: edge.target,
          type: 'smoothstep',
        }))
      : [];

    setNodes(newNodes);
    setEdges(newEdges);
  }, []); // No dependencies - pure function
  
  // Update nodes when input activity changes
  useEffect(() => {
    if (graphRef.current && !loading) {
      // Get current positions from nodes
      const currentPositions: CellPosition[] = nodes.map(node => ({
        cellId: node.id,
        x: node.position.x,
        y: node.position.y
      }));
      
      // Re-layout with updated input data
      layoutCells(graphRef.current, currentPositions, selectedCellId, pendingInputsByCellId);
    }
  }, [pendingInputsByCellId, loading, nodes, selectedCellId, layoutCells]);

  const onNodesChange = useCallback((changes: any) => {
    setNodes((nds) => {
      const updatedNodes = applyNodeChanges(changes, nds as any) as FlowNode[];
      
      // Check if this was a position change (drag)
      const positionChange = changes.find((c: any) => c.type === 'position' && c.dragging === false);
      if (positionChange) {
        // Save positions after drag ends
        if (saveTimeoutRef.current) {
          clearTimeout(saveTimeoutRef.current);
        }
        
        saveTimeoutRef.current = setTimeout(async () => {
          const positions = updatedNodes.map(n => ({
            cellId: n.id,
            x: n.position.x,
            y: n.position.y
          }));
          
          try {
            await savePositions(positions);
          } catch (err) {
            console.error('Failed to save positions:', err);
            message.error('Failed to save cell positions');
          }
        }, 500);
      }
      
      return updatedNodes;
    });
  }, []);

  useEffect(() => {
    // Listen for cell selection events (for tests and manual triggering)
    const handleCellSelection = (event: CustomEvent) => {
      handleCellClick(event.detail.cellId);
    };
    
    // Listen for dependency update events
    const handleDependencyUpdate = async () => {
      try {
        const [graphData, positions] = await Promise.all([
          fetchGraph(),
          fetchPositions()
        ]);
        
        graphRef.current = graphData;
        layoutCells(graphData, positions, selectedCell?.id, pendingInputsByCellId);
        
        // Update selectedCell with fresh data if one is selected
        if (selectedCell) {
          const updatedCell = graphData.cells.find((c: DependencyCell) => c.id === selectedCell.id);
          if (updatedCell && JSON.stringify(updatedCell) !== JSON.stringify(selectedCell)) {
            setSelectedCell(updatedCell);
            onCellSelect?.(updatedCell);
          }
        }
        
        message.success('Graph updated with new relationships');
      } catch (err) {
        console.error('Failed to refresh graph after relationship update:', err);
        message.error('Failed to refresh graph');
      }
    };
    
    window.addEventListener('cellSelected', handleCellSelection as EventListener);
    window.addEventListener('relationshipsUpdated', handleDependencyUpdate as EventListener);
    
    return () => {
      window.removeEventListener('cellSelected', handleCellSelection as EventListener);
      window.removeEventListener('relationshipsUpdated', handleDependencyUpdate as EventListener);
    };
  }, [handleCellClick, layoutCells, selectedCell?.id, onCellSelect]);

  // Load data only on initial mount
  useEffect(() => {
    if (!isInitialLoad.current) return;
    
    async function loadData() {
      try {
        const [graphData, positions] = await Promise.all([
          fetchGraph(),
          fetchPositions()
        ]);
        
        graphRef.current = graphData;
        layoutCells(graphData, positions, selectedCellId, pendingInputsByCellId);
        setLoading(false);
        isInitialLoad.current = false;
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to load graph';
        console.error('Graph loading error:', err);
        setError(errorMessage);
        setLoading(false);
        message.error(errorMessage);
      }
    }

    loadData();
  }, []); // Empty dependency array - only load on mount
  
  // Handle external cell selection
  useEffect(() => {
    if (selectedCellId && graphRef.current?.cells) {
      const cell = graphRef.current.cells.find((c: DependencyCell) => c.id === selectedCellId);
      if (cell && cell.id !== selectedCell?.id) {
        setSelectedCell(cell);
        // Update cell visual selection state
        setNodes(prevNodes => prevNodes.map(n => ({
          ...n,
          data: {
            ...n.data,
            selected: n.id === selectedCellId
          }
        })));
      }
    } else if (!selectedCellId && selectedCell) {
      setSelectedCell(null);
      // Clear visual selection state
      setNodes(prevNodes => prevNodes.map(n => ({
        ...n,
        data: {
          ...n.data,
          selected: false
        }
      })));
    }
  }, [selectedCellId, selectedCell?.id]); // Include selectedCell?.id to track changes

  if (loading) {
    return (
      <div className="loading-container">
        <Spin size="large" tip="Loading graph..." />
      </div>
    );
  }

  if (error) {
    return (
      <div className="error-container">
        <Card 
          style={{ maxWidth: 600, margin: '0 auto' }}
          title="Failed to Load Graph"
          extra={
            <Button type="primary" onClick={() => window.location.reload()}>
              Retry
            </Button>
          }
        >
          <Alert
            message="Error"
            description={error}
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
          />
          <Collapse ghost>
            <Collapse.Panel header="Troubleshooting Tips" key="1">
              <ul style={{ paddingLeft: '1rem' }}>
                <li>Check if the server is running on the correct port</li>
                <li>Verify the directory contains dependencies.yaml files</li>
                <li>Ensure the .vibestate.db file exists or use the -n flag</li>
                <li>Check the browser console for more details</li>
              </ul>
            </Collapse.Panel>
          </Collapse>
        </Card>
      </div>
    );
  }

  return (
    <div data-testid="react-flow-wrapper" style={{ height: '100%' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={{ custom: ProFlowCell as any }}
        onNodesChange={onNodesChange}
        nodesDraggable={true}
        nodesConnectable={false}
        elementsSelectable={false}
        fitView
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </div>
  );
}