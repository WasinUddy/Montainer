import React, { useCallback, useEffect, useState } from 'react';
import { Play, Pause, Save, RotateCcw, Star } from 'lucide-react';
import Terminal from './Terminal.jsx';
import { toast, Bounce, ToastContainer } from 'react-toastify';
import 'react-toastify/dist/ReactToastify.css';

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

    const handleGitHubStar = () => {
        // Replace 'your-username/your-repo' with the actual GitHub repository URL
        window.open('https://github.com/wasinuddy/montainer', '_blank');
    };

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
                                onClick={() => {fetch(`${BACKEND_ADDRESS}/restart`, {'method': 'POST'})}}
                                className="p-2 rounded-full bg-yellow-100 hover:bg-yellow-200 text-yellow-700"
                                title="Restart"
                            >
                                <RotateCcw className="w-5 h-5" />
                            </button>

                            <button
                                onClick={() => {
                                    setSaving(true);
                                    fetch(`${BACKEND_ADDRESS}/save`, { 'method': 'POST' })
                                        .then(response => {
                                            if (response.status === 200) {
                                                // Show success toast if the response is OK
                                                toast.success('Data saved successfully!', {
                                                    position: "top-right",
                                                    autoClose: 5000,
                                                    hideProgressBar: false,
                                                    closeOnClick: true,
                                                    pauseOnHover: true,
                                                    draggable: true,
                                                    progress: undefined,
                                                    theme: "light",
                                                    transition: Bounce,
                                                });
                                                setSaving(false);
                                            } else {
                                                return response.json().then(data => {
                                                    throw new Error(data.message || 'Failed to save');
                                                });
                                            }
                                        })
                                        .catch(error => {
                                            // Show error toast if any error occurs
                                            toast.error('Failed to save: ' + error.message, {
                                                position: "top-right",
                                                autoClose: 5000,
                                                hideProgressBar: false,
                                                closeOnClick: true,
                                                pauseOnHover: true,
                                                draggable: true,
                                                progress: undefined,
                                                theme: "light",
                                                transition: Bounce,
                                            });
                                            setSaving(false);
                                        });
                                }}
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

                {/* Footer with GitHub star plea */}
                <div className="border-t border-gray-200 px-6 py-4 bg-gray-50">
                    <div className="flex items-center justify-center">
                        <button
                            onClick={handleGitHubStar}
                            className="flex items-center space-x-2 px-4 py-2 rounded-lg bg-white hover:bg-amber-50 text-gray-600 hover:text-amber-700 border border-gray-200 hover:border-amber-200 transition-all duration-200 shadow-sm hover:shadow-md"
                        >
                            <Star className="w-4 h-4" />
                            <span className="text-sm">If you like Montainer, please star it on GitHub! 🥺</span>
                        </button>
                    </div>
                </div>
            </div>
            <ToastContainer />
        </div>
    );
};

export default App;