import React, { useState, useEffect } from 'react';

interface ServiceInfo {
  type: string;
  port: string;
  target?: string;
}

const NetworkPanel: React.FC = () => {
  const [services, setServices] = useState<ServiceInfo[]>([]);
  
  // Socks state
  const [socksPort, setSocksPort] = useState('1080');
  const [isSocksLoading, setIsSocksLoading] = useState(false);
  
  // Fwd state
  const [fwdLocalPort, setFwdLocalPort] = useState('33890');
  const [fwdTarget, setFwdTarget] = useState('127.0.0.1:3389');
  const [isFwdLoading, setIsFwdLoading] = useState(false);

  const fetchServices = async () => {
    try {
      const res = await fetch('/api/services');
      const data = await res.json();
      setServices(data || []);
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => {
    fetchServices();
    const interval = setInterval(fetchServices, 5000);
    return () => clearInterval(interval);
  }, []);

  const hasSocks = services.some(s => s.type === 'socks5');

  const toggleSocks = async () => {
    setIsSocksLoading(true);
    try {
      const method = hasSocks ? 'DELETE' : 'POST';
      const portToUse = hasSocks ? services.find(s => s.type === 'socks5')?.port || socksPort : socksPort;
      await fetch('/api/services/socks', {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ port: portToUse }),
      });
      await fetchServices();
    } catch (e) {
      console.error(e);
    }
    setIsSocksLoading(false);
  };

  const startFwd = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsFwdLoading(true);
    try {
      await fetch('/api/services/fwd', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ localPort: fwdLocalPort, target: fwdTarget }),
      });
      await fetchServices();
      setFwdLocalPort('');
      setFwdTarget('');
    } catch (e) {
      console.error(e);
    }
    setIsFwdLoading(false);
  };

  const stopFwd = async (port: string) => {
    try {
      await fetch('/api/services/fwd', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ localPort: port }),
      });
      await fetchServices();
    } catch (e) {
      console.error(e);
    }
  };

  return (
    <div className="network-panel glass-panel animation-fade-slide-up">
      <div className="panel-header">
        <h2>Network Services</h2>
        <p>Manage active proxies and port forwarding mappings globally.</p>
      </div>

      <div className="service-cards">
        {/* Socks5 Card */}
        <div className="service-card glow-card">
          <div className="card-header">
            <div className="card-title">
              <svg className="icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              </svg>
              Socks5 Proxy
            </div>
            <label className="switch">
              <input type="checkbox" checked={hasSocks} disabled={isSocksLoading} onChange={toggleSocks} />
              <span className="slider round"></span>
            </label>
          </div>
          <div className="card-body">
            <div className="input-group">
              <label>Listen Port</label>
              <input 
                type="text" 
                value={hasSocks ? services.find(s => s.type === 'socks5')?.port : socksPort} 
                onChange={(e) => setSocksPort(e.target.value)}
                disabled={hasSocks || isSocksLoading}
                className="tech-input font-mono"
              />
            </div>
          </div>
        </div>

        {/* Port Forwarding Card */}
        <div className="service-card glow-card">
          <div className="card-header">
            <div className="card-title">
              <svg className="icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="22" y1="2" x2="11" y2="13" />
                <polygon points="22 2 15 22 11 13 2 9 22 2" />
              </svg>
              Port Forwarding
            </div>
          </div>
          
          <div className="card-body">
            <form onSubmit={startFwd} className="fwd-form">
              <div className="input-group flex-group">
                <div className="input-col">
                  <label>Local Port</label>
                  <input type="text" value={fwdLocalPort} onChange={e => setFwdLocalPort(e.target.value)} required className="tech-input font-mono" placeholder="33890" />
                </div>
                <div className="input-col">
                  <label>Target (IP:Port)</label>
                  <input type="text" value={fwdTarget} onChange={e => setFwdTarget(e.target.value)} required className="tech-input font-mono" placeholder="127.0.0.1:3389" />
                </div>
              </div>
              <button type="submit" disabled={isFwdLoading} className="tech-button">
                {isFwdLoading ? 'Adding...' : 'Add Forwarding'}
              </button>
            </form>

            {services.filter(s => s.type === 'fwd').length > 0 && (
              <div className="fwd-list">
                <h4>Active Rules</h4>
                <ul>
                  {services.filter(s => s.type === 'fwd').map(s => (
                    <li key={s.port} className="fwd-item">
                      <span className="font-mono">:{s.port} &rarr; Active</span>
                      <button type="button" onClick={() => stopFwd(s.port)} className="tech-button-danger">Stop</button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default NetworkPanel;
