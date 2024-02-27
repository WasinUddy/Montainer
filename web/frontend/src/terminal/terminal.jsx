import React, { useEffect, useState } from 'react';
import "./terminal.css";

export default function Terminal({ initialLogData, baseUrl }) {
    const [logData, setLogData] = useState(initialLogData || "");
    const [command, setCommand] = useState("");

    // Interval to fetch logs
    useEffect(() => {
        const interval = setInterval(() => {
            fetch(`${baseUrl}/log`)
                .then(res => res.json())
                .then(data => {
                    // capture only last 10 lines
                    data.log = data.log.split("\n").slice(-30).join("\n");
                    setLogData(data.log);
                })
                .catch(err => console.log(err));
        }, 750);

        return () => clearInterval(interval);
    }, [baseUrl]);

    const handleInputChange = (event) => {
        setCommand(event.target.value);
    }

    const handleKeyPress = (event) => {
        if (event.key === "Enter") {
            event.preventDefault(); // Prevent the default action (form submission)
    
            // Construct the URL with the command as a query parameter
            const url = `${baseUrl}/command?cmd=${encodeURIComponent(command)}`;
    
            // POST request to the server
            fetch(url, { method: 'POST' })
                .catch((error) => console.error('Error:', error));
    
            setCommand(""); // Clear the command input after sending
        }
    }
    
    return (
        <div className="terminal-container">
            <pre className="terminal-output">
                {logData}
            </pre>
            <input type="text" placeholder="Enter command" value={command} onChange={handleInputChange} onKeyPress={handleKeyPress}/>
        </div>
    )
}