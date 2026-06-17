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

  useEffect(() => {
    // Fetch agents from the backend
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
        }
      } catch (err) {
        console.error("Failed to fetch agents:", err);
      }
    };

    fetchAgents();
    // Poll every 5 seconds
    const interval = setInterval(fetchAgents, 5000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="sidebar">
      <div className="sidebar-header">Active Agents</div>
      <ul className="agent-list">
        {agents.length === 0 ? (
          <div style={{ padding: '16px', color: '#94A3B8', fontSize: '0.85rem' }}>No agents found.</div>
        ) : (
          agents.map(agent => (
            <li
              key={agent.ip}
              className={`agent-item ${selectedIp === agent.ip ? 'active' : ''}`}
              onClick={() => onSelect(agent.ip)}
            >
              <div className="agent-info">
                <div className="agent-name">{agent.mac || agent.ip}</div>
                <div className="agent-ip">{agent.ip}</div>
              </div>
              <div className={`status-dot ${agent.online ? 'online' : 'offline'}`} title={agent.online ? "Online" : "Offline"}></div>
            </li>
          ))
        )}
      </ul>
    </div>
  );
};

export default AgentList;
