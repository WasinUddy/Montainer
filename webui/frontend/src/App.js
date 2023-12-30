import React, { useState, useEffect } from 'react';
import './App.css';
import Terminal from "./terminal/terminal.jsx";

const App = () => {
  const [initialLogData, setInitialLogData] = useState('');

  // Fetch initial log data on component mount
  useEffect(() => {
    const fetchLogData = async () => {
      try {
        const res = await fetch("/log");
        const data = await res.json();
        setInitialLogData(data.log.split("\n").slice(-30).join("\n"));
      } catch (error) {
        console.error('Failed to fetch initial log data:', error);
      }
    };

    fetchLogData();
  }, []);

  const handleServerAction = async (action) => {
    try {
      const response = await fetch(`/${action}`, { method: 'POST' });
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
      <Terminal initialLogData={initialLogData} />
    </div>
  );
};

export default App;
