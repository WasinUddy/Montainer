import React, { useCallback, useEffect, useState } from 'react';
import { Play, Pause, Save } from 'lucide-react';
import Terminal from './Terminal.jsx';

const App = () => {
    const [running, setRunning] = useState(false);
    const [saving, setSaving] = useState(false);
    const [ws, setWs] = useState(null);
    const [logData, setLogData] = useState(''); // New state for logs

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

    const BACKEND_ADDRESS = getBaseUrl(); // Use the dynamic base URL

    const connectWebSocket = useCallback(() => {
        // Construct the WebSocket URL using the dynamic base URL
        const wsUrl = `${BACKEND_ADDRESS.replace(/^http/, 'ws')}/ws/stream`;
        const ws = new WebSocket(wsUrl);

        ws.onopen = () => {
            console.log('Connected to server');
        };

        ws.onclose = () => {
            console.log('Disconnected from server');
        };

        ws.onmessage = (event) => {
            let msg = JSON.parse(event.data); // Parse the JSON string

            // Update server status
            if (msg.is_running !== running) {
                setRunning(msg.is_running);
            }

            // Update log data
            if (msg.logs) {
                setLogData(msg.logs.join('\n'));
            }
        };

        return ws;
    }, [running, logData]);

    useEffect(() => {
        const ws = connectWebSocket();
        setWs(ws);

        return () => {
            ws.close();
        };
    }, [connectWebSocket]);

    return (
        <div className="min-h-screen bg-gray-100 p-4">
            <div className="max-w-7xl mx-auto bg-white rounded-lg shadow-lg">
                <div className="p-6">
                    <div className="flex items-center justify-between">
                        <div className="flex items-center space-x-4">
                            <h1 className="text-2xl font-semibold text-gray-800">Montainer Web Manager</h1>
                            <span
                                className={`px-3 py-1 text-sm font-medium rounded-full transition-colors duration-200 ${
                                    running ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'
                                }`}
                            >
                                {running ? 'Running' : 'Stopped'}
                            </span>
                        </div>

                        <div className="flex items-center space-x-3">
                            <button
                                onClick={() => {fetch(`${BACKEND_ADDRESS}/toggle`, {'method': 'POST'})}}
                                className={`p-2 rounded-full transition-colors duration-200 ${
                                    running ? 'bg-red-100 hover:bg-red-200 text-red-700' : 'bg-green-100 hover:bg-green-200 text-green-700'
                                }`}
                                title={running ? 'Stop' : 'Start'}
                            >
                                {running ? <Pause className="w-5 h-5" /> : <Play className="w-5 h-5" />}
                            </button>

                            <button
                                onClick={() => {}}
                                disabled={saving}
                                className={`p-2 rounded-full transition-all duration-200 ${
                                    saving ? 'bg-gray-100 text-gray-400' : 'bg-blue-100 hover:bg-blue-200 text-blue-700'
                                } ${saving ? 'cursor-not-allowed' : 'cursor-pointer'}`}
                                title="Save"
                            >
                                <Save className={`w-5 h-5 ${saving ? 'animate-pulse' : ''}`} />
                            </button>
                        </div>
                    </div>
                    <Terminal logData={logData} setLogData={setLogData}/>
                </div>
            </div>
        </div>
    );
};

export default App;
