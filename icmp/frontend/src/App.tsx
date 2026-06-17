import { useState } from 'react';
import TopNav from './components/TopNav';
import AgentList from './components/AgentList';
import Terminal from './components/Terminal';

function App() {
  const [selectedIp, setSelectedIp] = useState<string | null>(null);

  return (
    <div className="app-container">
      <TopNav />
      <AgentList onSelect={setSelectedIp} selectedIp={selectedIp} />
      <main className="main-content">
        {selectedIp ? (
          <div className="terminal-container">
            <Terminal agentIp={selectedIp} key={selectedIp} />
          </div>
        ) : (
          <div className="placeholder-view">
            <svg className="placeholder-icon" width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" strokeLinecap="round" strokeLinejoin="round">
              <rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect>
              <line x1="8" y1="21" x2="16" y2="21"></line>
              <line x1="12" y1="17" x2="12" y2="21"></line>
            </svg>
            <h2>Select an agent to start a session</h2>
            <p>Listen for incoming ICMP heartbeats or ping a remote agent to wake it up.</p>
          </div>
        )}
      </main>
    </div>
  );
}

export default App;
