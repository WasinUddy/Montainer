import React, { useState, useEffect } from 'react';
import './App.css';
import Terminal from "./terminal/terminal.jsx";


const App = () => {
  const [initialLogData, setInitialLogData] = useState('');

  // Dynamically determine the base URL
  const getBaseUrl = () => {
    const { protocol, hostname, port, pathname } = window.location;
    const pathSegments = pathname.split('/').filter(segment => segment);
  
    // Construct the base path from the segments, if any exist
    let basePath = '';
    if (pathSegments.length > 0) {
      basePath = '/' + pathSegments.join('/');
    }
  
    // Include the port if it's non-standard
    const portPart = (port && `:${port}`) || '';
  
    return `${protocol}//${hostname}${portPart}${basePath}`;
  };
  

  const baseUrl = getBaseUrl();

  // Fetch initial log data on component mount
  useEffect(() => {
    const fetchLogData = async () => {
      try {
        const res = await fetch(`${baseUrl}/log`);
        const data = await res.json();
        setInitialLogData(data.log.split("\n").slice(-30).join("\n"));
      } catch (error) {
        console.error('Failed to fetch initial log data:', error);
      }
    };

    fetchLogData();
  }, [baseUrl]);

  const handleServerAction = async (action) => {
    try {
      const response = await fetch(`${baseUrl}/${action}`, { method: 'POST' });
      const data = await response.json();
      console.log(data);
    } catch (error) {
console.error(`Failed to ${action} the server:`, error);
    }
  };

  return (
    <div className="container">
        <div className="control-panel">
          <button id="start-button" onClick={() => handleServerAction('start')}>
            <span className="material-icons">play_arrow</span>
          </button>
          <button id="stop-button" onClick={() => handleServerAction('stop')}>
            <span className="material-icons">stop</span>
          </button>
        </div>
        <Terminal initialLogData={initialLogData} baseUrl={baseUrl} />
      </div>
  );
};

export default App;
