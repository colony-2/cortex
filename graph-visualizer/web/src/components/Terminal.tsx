import React, { useEffect, useRef } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import './Terminal.css';
import { Node } from '../types';

interface TerminalProps {
  node: Node;
  onClose: () => void;
}

const Terminal: React.FC<TerminalProps> = ({ node, onClose }) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!terminalRef.current) return;

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    term.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = term;

    const ws = new WebSocket(`ws://localhost:8080/ws/terminal/${node.id}`);
    wsRef.current = ws;

    ws.onopen = () => {
      term.write(`Connected to ${node.name} (${node.path})\r\n$ `);
    };

    ws.onmessage = (event) => {
      term.write(event.data);
    };

    ws.onerror = (error) => {
      term.write(`\r\nConnection error: ${error}\r\n`);
    };

    ws.onclose = () => {
      term.write('\r\nConnection closed\r\n');
    };

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    const handleResize = () => {
      fitAddon.fit();
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      ws.close();
      term.dispose();
    };
  }, [node]);

  return (
    <div className="terminal-container">
      <div className="terminal-header">
        <span>Terminal - {node.name}</span>
        <button onClick={onClose} className="close-button">×</button>
      </div>
      <div ref={terminalRef} className="terminal-content"></div>
    </div>
  );
};

export default Terminal;