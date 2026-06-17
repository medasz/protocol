import React, { useEffect, useState } from 'react';

export interface Agent {
  ip: string;
  mac: string;
  lastSeen: number;
  online: boolean;
}

interface AgentListProps {
  onSelect: (ip: string) => void;
  selectedIp: string | null;
}

const AgentList: React.FC<AgentListProps> = ({ onSelect, selectedIp }) => {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchAgents = async () => {
      try {
        let apiUrl = '/api/agents';
        if (window.location.host.includes('5173')) {
          apiUrl = 'http://localhost:8080/api/agents';
        }
        const res = await fetch(apiUrl);
        if (res.ok) {
          const data = await res.json();
          setAgents(data || []);
          setError(null);
        } else {
          setError(`Agent API returned ${res.status}`);
        }
      } catch (err) {
        console.error('Failed to fetch agents:', err);
        setError('Agent API unavailable');
      } finally {
        setLoading(false);
      }
    };

    fetchAgents();
    const interval = setInterval(fetchAgents, 5000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="sidebar glass-panel">
      <div className="sidebar-header">Active Agents</div>
      <ul className="agent-list" role="listbox" aria-label="Available agents">
        {loading ? (
          <li className="agent-empty">Loading agents...</li>
        ) : error ? (
          <li className="agent-empty error">{error}</li>
        ) : agents.length === 0 ? (
          <li className="agent-empty">Waiting for slave heartbeats.</li>
        ) : (
          agents.map((agent) => (
            <li
              key={agent.ip}
              className={`agent-item ${selectedIp === agent.ip ? 'active' : ''}`}
              role="option"
              aria-selected={selectedIp === agent.ip}
              tabIndex={0}
              onClick={() => onSelect(agent.ip)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onSelect(agent.ip);
                }
              }}
            >
              <div className="agent-info">
                <div className="agent-name">{agent.mac || agent.ip}</div>
                <div className="agent-ip">{agent.ip}</div>
                <div className="agent-last-seen">{formatLastSeen(agent.lastSeen)}</div>
              </div>
              <div
                className={`status-dot ${agent.online ? 'online' : 'offline'}`}
                title={agent.online ? 'Online' : 'Offline'}
              ></div>
            </li>
          ))
        )}
      </ul>
    </div>
  );
};

function formatLastSeen(lastSeen: number): string {
  if (!lastSeen) {
    return 'Not seen yet';
  }
  const deltaMs = Date.now() - lastSeen;
  if (deltaMs < 5000) {
    return 'Seen just now';
  }
  if (deltaMs < 60000) {
    return `Seen ${Math.floor(deltaMs / 1000)}s ago`;
  }
  return `Seen ${Math.floor(deltaMs / 60000)}m ago`;
}

export default AgentList;
