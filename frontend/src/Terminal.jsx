import React, { useState, useEffect, useRef } from "react";
import { Terminal as TerminalIcon, SendHorizontal } from "lucide-react";

const Terminal = ({ logData, setLogData }) => {
    const [command, setCommand] = useState("");
    const [commandHistory, setCommandHistory] = useState([]);
    const [historyIndex, setHistoryIndex] = useState(-1);
    const outputRef = useRef(null);

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

    // Auto-scroll to bottom when new content is added
    useEffect(() => {
        if (outputRef.current) {
            outputRef.current.scrollTop = outputRef.current.scrollHeight;
        }
    }, [logData]);

    const handleSubmit = async (e) => {
        e.preventDefault();
        if (!command.trim()) return;

        // Add command to history
        setCommandHistory(prev => [...prev, command]);

        try {
            // Send command to server
            const response = await fetch(`${BACKEND_ADDRESS}/command`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ 'command': command })
            });

            if (!response.ok) {
                const errorData = await response.json();
                throw new Error(errorData.detail);
            }

            const result = await response.json();
            console.log('Command sent successfully:', result);
        } catch (error) {
            console.error('Error sending command:', error);
        }

        // Clear the command input after submission
        setCommand("");
        setHistoryIndex(-1);
    };

    const handleKeyDown = (e) => {
        if (e.key === "ArrowUp") {
            e.preventDefault();
            setHistoryIndex(prev => {
                const newIndex = Math.min(prev + 1, commandHistory.length - 1);
                if (newIndex >= 0) {
                    setCommand(commandHistory[commandHistory.length - 1 - newIndex]);
                    return newIndex;
                }
                return prev;
            });
        } else if (e.key === "ArrowDown") {
            e.preventDefault();
            setHistoryIndex(prev => {
                const newIndex = Math.max(prev - 1, -1);
                if (newIndex >= 0) {
                    setCommand(commandHistory[commandHistory.length - 1 - newIndex]);
                } else {
                    setCommand("");
                }
                return newIndex;
            });
        }
    };

    // Limit the number of lines displayed while preserving history
    const maxLines = 1000; // Increased for better history retention
    const displayedLogData = logData.split('\n').slice(-maxLines).join('\n');

    return (
        <div className="bg-gray-900 rounded-lg overflow-hidden m-2 shadow-xl border border-gray-800">
            {/* Terminal Header */}
            <div className="flex items-center justify-between px-4 py-2 bg-gray-800 border-b border-gray-700">
                <div className="flex items-center space-x-2">
                    <TerminalIcon className="w-5 h-5 text-gray-400"/>
                    <span className="text-gray-400 font-medium">Minecraft Console</span>
                </div>
                <div className="flex space-x-2">
                    <div className="w-3 h-3 rounded-full bg-red-500 hover:bg-red-600 transition-colors cursor-pointer" />
                    <div className="w-3 h-3 rounded-full bg-yellow-500 hover:bg-yellow-600 transition-colors cursor-pointer" />
                    <div className="w-3 h-3 rounded-full bg-green-500 hover:bg-green-600 transition-colors cursor-pointer" />
                </div>
            </div>

            {/* Terminal Output */}
            <div
                ref={outputRef}
                className="p-4 h-[40rem] font-mono text-sm text-gray-300 bg-gradient-to-b from-gray-900 to-gray-800 overflow-y-auto"
            >
                <pre className="whitespace-pre-wrap break-words">{displayedLogData}</pre>
            </div>

            {/* Command Input */}
            <form onSubmit={handleSubmit} className="border-t border-gray-800 p-2 bg-gray-850">
                <div className="flex items-center space-x-2 bg-gray-800 rounded px-3 py-2 focus-within:ring-2 focus-within:ring-blue-500">
                    <span className="text-gray-500">/</span>
                    <input
                        type="text"
                        value={command}
                        onChange={(e) => setCommand(e.target.value)}
                        onKeyDown={handleKeyDown}
                        placeholder="Enter Minecraft command..."
                        className="flex-1 bg-transparent border-none outline-none text-gray-300 placeholder-gray-500"
                        aria-label="Command input"
                    />
                    <button
                        type="submit"
                        className="p-1 hover:bg-gray-700 rounded transition-colors"
                        aria-label="Send command"
                    >
                        <SendHorizontal className="w-4 h-4 text-gray-400" />
                    </button>
                </div>
            </form>
        </div>
    );
};

export default Terminal;
