import React, { useEffect, useRef, useState } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

interface TerminalProps {
  agentIp: string;
}

const Terminal: React.FC<TerminalProps> = ({ agentIp }) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting');

  useEffect(() => {
    if (!terminalRef.current) return;
    setStatus('connecting');

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

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let wsHost = window.location.host;
    if (wsHost.includes('5173')) {
       wsHost = 'localhost:8080';
    }
    const wsUrl = `${wsProtocol}//${wsHost}/ws/terminal?ip=${encodeURIComponent(agentIp)}`;
    
    term.writeln(`\x1b[32m[+]\x1b[0m Opening session for ${agentIp}...`);
    
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      setStatus('connected');
      term.writeln(`\x1b[32m[+]\x1b[0m Connection established.`);
    };

    ws.onmessage = (event) => {
      term.write(event.data);
    };

    ws.onclose = () => {
      setStatus('disconnected');
      term.writeln(`\r\n\x1b[31m[-]\x1b[0m Connection closed.`);
    };

    ws.onerror = () => {
      setStatus('error');
      term.writeln(`\r\n\x1b[31m[-]\x1b[0m WebSocket Error.`);
    };

    let commandBuf = '';
    term.onData((data) => {
      if (data === '\r') {
        term.write('\r\n');
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(commandBuf.length > 0 ? commandBuf : " ");
        } else {
          term.writeln('\x1b[33m[!]\x1b[0m Session is disconnected.');
        }
        commandBuf = '';
      } 
      else if (data === '\u007f' || data === '\b') {
        if (commandBuf.length > 0) {
          commandBuf = commandBuf.slice(0, -1);
          term.write('\b \b');
        }
      } 
      else if (data >= String.fromCharCode(0x20)) {
        commandBuf += data;
        term.write(data);
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
  }, [agentIp]);

  return (
    <section className="terminal-session" aria-label={`Terminal session for ${agentIp}`}>
      <div className="terminal-toolbar">
        <span className={`terminal-status ${status}`}></span>
        <span className="terminal-agent">{agentIp}</span>
        <span className="terminal-state">{status}</span>
      </div>
      <div ref={terminalRef} className="xterm-wrapper" />
    </section>
  );
};

export default Terminal;
