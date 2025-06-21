import React, { useEffect, useRef, useState } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import './NodeBox.css';
import { Node } from '../types';

interface NodeBoxProps {
  node: Node;
  zoomLevel: number;
  onDoubleClick: () => void;
}

const NodeBox: React.FC<NodeBoxProps> = ({ node, zoomLevel, onDoubleClick }) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [terminalInitialized, setTerminalInitialized] = useState(false);

  const showTerminal = zoomLevel > 3;
  const showShimmer = zoomLevel > 1.5 && !showTerminal;

  useEffect(() => {
    // Only initialize terminal when zoom level crosses the threshold
    if (!showTerminal || !terminalRef.current) {
      // Clean up if terminal should not be shown
      if (!showTerminal && terminalInitialized) {
        if (wsRef.current) {
          wsRef.current.close();
          wsRef.current = null;
        }
        if (xtermRef.current) {
          xtermRef.current.dispose();
          xtermRef.current = null;
        }
        setTerminalInitialized(false);
      }
      return;
    }
    
    if (terminalInitialized) return;

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
      },
      rows: 10,
      cols: 40,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    term.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = term;

    const ws = new WebSocket(`ws://localhost:8080/ws/terminal/${node.id}`);
    wsRef.current = ws;

    ws.onopen = () => {
      setTerminalInitialized(true);
    };

    ws.onmessage = (event) => {
      term.write(event.data);
    };

    ws.onerror = (error) => {
      term.write(`\r\nConnection error\r\n`);
    };

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    return () => {
      if (wsRef.current) {
        wsRef.current.close();
      }
      if (xtermRef.current) {
        xtermRef.current.dispose();
      }
      setTerminalInitialized(false);
    };
  }, [showTerminal, node, terminalInitialized]);

  return (
    <div className="node-box" onDoubleClick={onDoubleClick}>
      <div className="node-header">
        {node.name}
      </div>
      <div className="node-content">
        {showTerminal ? (
          <div ref={terminalRef} className="node-terminal" />
        ) : showShimmer ? (
          <div className="shimmer-container">
            <div className="shimmer-line" />
            <div className="shimmer-line" />
            <div className="shimmer-line short" />
          </div>
        ) : (
          <div className="node-path">{node.path}</div>
        )}
      </div>
    </div>
  );
};

export default NodeBox;