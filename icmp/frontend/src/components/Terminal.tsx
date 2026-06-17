import React, { useEffect, useRef } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

interface TerminalProps {
  agentIp: string;
}

const Terminal: React.FC<TerminalProps> = ({ agentIp }) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!terminalRef.current) return;

    // Initialize xterm.js
    const term = new XTerm({
      cursorBlink: true,
      fontFamily: "'Fira Code', monospace",
      fontSize: 14,
      theme: {
        background: '#000000',
        foreground: '#F8FAFC',
        cursor: '#22C55E',
        selectionBackground: 'rgba(255, 255, 255, 0.3)',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = term;
    fitAddonRef.current = fitAddon;

    // Connect to WebSocket
    // In dev mode, we connect to the Go backend port (assuming 8080 or dynamically injected)
    // For now we'll construct the WS URL based on current host if running from Go, 
    // or hardcode to 8080 for Vite dev proxy/direct
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let wsHost = window.location.host;
    if (wsHost.includes('5173')) {
       // local vite dev server, fallback to go backend port 8080
       wsHost = 'localhost:8080';
    }
    const wsUrl = `${wsProtocol}//${wsHost}/ws/terminal?ip=${encodeURIComponent(agentIp)}`;
    
    term.writeln(`\x1b[32m[+]\x1b[0m Connecting to ${agentIp} via ${wsUrl}...`);
    
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      term.writeln(`\x1b[32m[+]\x1b[0m Connection established.`);
    };

    ws.onmessage = (event) => {
      term.write(event.data);
    };

    ws.onclose = () => {
      term.writeln(`\r\n\x1b[31m[-]\x1b[0m Connection closed.`);
    };

    ws.onerror = () => {
      term.writeln(`\r\n\x1b[31m[-]\x1b[0m WebSocket Error.`);
    };

    // Handle user input
    let commandBuf = '';
    term.onData((data) => {
      // Enter
      if (data === '\r') {
        term.write('\r\n');
        if (ws.readyState === WebSocket.OPEN) {
          // If empty, send a space so the backend executes an empty line and returns a new prompt
          ws.send(commandBuf.length > 0 ? commandBuf : " ");
        }
        commandBuf = '';
      } 
      // Backspace
      else if (data === '\u007f' || data === '\b') {
        if (commandBuf.length > 0) {
          commandBuf = commandBuf.slice(0, -1);
          term.write('\b \b');
        }
      } 
      // Printable characters (including Paste)
      else if (data >= String.fromCharCode(0x20)) {
        commandBuf += data;
        term.write(data);
      }
    });

    // Handle resize
    const handleResize = () => {
      fitAddon.fit();
    };
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      ws.close();
      term.dispose();
    };
  }, [agentIp]);

  return <div ref={terminalRef} className="xterm-wrapper" />;
};

export default Terminal;
