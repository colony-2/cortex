import React, { useEffect, useState } from 'react';
import GraphVisualization from './components/GraphVisualization';
import { Graph } from './types';
import './App.css';

function App() {
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchGraph();
  }, []);

  const fetchGraph = async () => {
    try {
      const response = await fetch('http://localhost:8080/api/graph');
      if (!response.ok) {
        throw new Error('Failed to fetch graph data');
      }
      const data = await response.json();
      setGraph(data);
      setLoading(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
      setLoading(false);
    }
  };

  if (loading) {
    return <div className="loading">Loading graph...</div>;
  }

  if (error) {
    return <div className="error">Error: {error}</div>;
  }

  return (
    <div className="App">
      <GraphVisualization graph={graph} />
    </div>
  );
}

export default App;