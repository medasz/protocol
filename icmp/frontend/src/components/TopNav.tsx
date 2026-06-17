import React from 'react';

const TopNav: React.FC = () => {
  return (
    <nav className="top-nav">
      <div className="brand">
        <span className="brand-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M4 17L10 11L4 5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
            <path d="M12 19H20" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
          </svg>
        </span>
        ICMP C2 Dashboard
      </div>
    </nav>
  );
};

export default TopNav;
