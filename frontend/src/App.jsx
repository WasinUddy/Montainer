import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    AlertTriangle,
    Archive,
    Box,
    CheckCircle2,
    Github,
    LoaderCircle,
    Play,
    Power,
    RotateCcw,
    Wifi,
    WifiOff,
} from 'lucide-react';
import Terminal from './Terminal.jsx';
import { getBaseUrl, getWebSocketUrl, readApiError } from './api.js';
import { formatBytes, formatUptime } from './ui.js';
import './App.css';

const initialStatus = {
    state: 'unknown',
    is_running: false,
    pid: 0,
    generation: 0,
    started_at: '',
    stopped_at: '',
    exit_code: 0,
    last_error: '',
    dropped_logs: { file: 0, otlp: 0 },
};

const stateLabels = {
    unknown: 'Checking',
    starting: 'Starting',
    running: 'Running',
    stopping: 'Stopping',
    stopped: 'Stopped',
    failed: 'Needs attention',
};

const App = () => {
    const [serverStatus, setServerStatus] = useState(initialStatus);
    const [connectionState, setConnectionState] = useState('connecting');
    const [managerReachable, setManagerReachable] = useState(null);
    const [instanceName, setInstanceName] = useState('Montainer instance');
    const [logs, setLogs] = useState([]);
    const [pendingAction, setPendingAction] = useState('');
    const [operationNotice, setOperationNotice] = useState(null);
    const [now, setNow] = useState(Date.now());
    const logsRef = useRef([]);
    const backendAddress = useMemo(getBaseUrl, []);

    const storeLogs = useCallback((nextLogs) => {
        const normalized = Array.isArray(nextLogs) ? nextLogs.filter((line) => typeof line === 'string') : [];
        logsRef.current = normalized;
        setLogs(normalized);
    }, []);

    const loadStatus = useCallback(async (signal) => {
        try {
            const response = await fetch(`${backendAddress}/status`, { signal });
            if (!response.ok) throw new Error(await readApiError(response));
            const payload = await response.json();
            setServerStatus((current) => ({
                ...current,
                ...payload,
                dropped_logs: { ...current.dropped_logs, ...payload.dropped_logs },
            }));
            setManagerReachable(true);
            return payload;
        } catch (error) {
            if (error.name !== 'AbortError') setManagerReachable(false);
            return null;
        }
    }, [backendAddress]);

    const loadLogs = useCallback(async (signal) => {
        const requestLogs = async (path) => {
            const response = await fetch(`${backendAddress}${path}`, { signal });
            return response;
        };

        try {
            let response = await requestLogs('/logs?max_lines=250');
            if (response.status === 422) response = await requestLogs('/logs');
            if (!response.ok) throw new Error(await readApiError(response));
            const payload = await response.json();
            storeLogs(payload.logs);
        } catch (error) {
            if (error.name !== 'AbortError' && logsRef.current.length === 0) storeLogs([]);
        }
    }, [backendAddress, storeLogs]);

    useEffect(() => {
        const controller = new AbortController();
        let statusRequestInFlight = false;

        const refreshStatus = async () => {
            if (statusRequestInFlight) return;
            statusRequestInFlight = true;
            await loadStatus(controller.signal);
            statusRequestInFlight = false;
        };

        refreshStatus();
        loadLogs(controller.signal);
        const statusTimer = window.setInterval(refreshStatus, 5000);
        const clockTimer = window.setInterval(() => setNow(Date.now()), 1000);

        fetch(`${backendAddress}/instance_name`, { signal: controller.signal })
            .then((response) => response.ok ? response.json() : Promise.reject(new Error('Unable to load instance name')))
            .then((payload) => setInstanceName(payload.instance_name || 'Montainer instance'))
            .catch((error) => {
                if (error.name !== 'AbortError') setInstanceName('Montainer instance');
            });

        return () => {
            controller.abort();
            window.clearInterval(statusTimer);
            window.clearInterval(clockTimer);
        };
    }, [backendAddress, loadLogs, loadStatus]);

    useEffect(() => {
        let cancelled = false;
        let socket;
        let reconnectTimer;
        let logRefreshTimer;
        let logRefreshInFlight = false;
        let logRefreshQueued = false;
        let attempt = 0;
        const logController = new AbortController();

        const refreshLogs = async () => {
            logRefreshTimer = undefined;
            if (cancelled) return;

            logRefreshQueued = false;
            logRefreshInFlight = true;
            await loadLogs(logController.signal);
            logRefreshInFlight = false;

            if (logRefreshQueued && !cancelled) scheduleLogRefresh();
        };

        const scheduleLogRefresh = () => {
            if (logRefreshTimer || logRefreshInFlight) {
                logRefreshQueued = true;
                return;
            }
            logRefreshTimer = window.setTimeout(refreshLogs, 200);
        };

        const connect = () => {
            if (cancelled) return;
            setConnectionState('connecting');
            const currentSocket = new WebSocket(getWebSocketUrl(backendAddress));
            socket = currentSocket;

            currentSocket.onmessage = (event) => {
                if (cancelled || socket !== currentSocket) return;
                try {
                    const message = JSON.parse(event.data);
                    if (typeof message.is_running !== 'boolean') return;
                    attempt = 0;
                    setConnectionState('connected');
                    setManagerReachable(true);
                    setServerStatus((current) => ({
                        ...current,
                        is_running: message.is_running,
                        state: message.state || (message.is_running ? 'running' : 'stopped'),
                    }));
                    if (logsRef.current.length === 0 && Array.isArray(message.logs)) storeLogs(message.logs);
                    scheduleLogRefresh();
                } catch (error) {
                    console.error('Invalid server stream message:', error);
                }
            };

            currentSocket.onerror = () => currentSocket.close();
            currentSocket.onclose = () => {
                if (cancelled || socket !== currentSocket) return;
                setConnectionState('disconnected');
                const backoff = Math.min(750 * 2 ** attempt, 10000);
                const jitter = 0.85 + Math.random() * 0.3;
                attempt += 1;
                reconnectTimer = window.setTimeout(connect, backoff * jitter);
            };
        };

        connect();
        return () => {
            cancelled = true;
            logController.abort();
            window.clearTimeout(reconnectTimer);
            window.clearTimeout(logRefreshTimer);
            socket?.close();
        };
    }, [backendAddress, loadLogs, storeLogs]);

    const runAction = async ({ name, path, progress, success }) => {
        setPendingAction(name);
        setOperationNotice({ type: 'progress', message: progress });
        try {
            const response = await fetch(`${backendAddress}${path}`, { method: 'POST' });
            if (!response.ok) throw new Error(await readApiError(response));
            const payload = await response.json();
            const message = typeof success === 'function' ? success(payload) : success;
            setOperationNotice({ type: 'success', message });
        } catch (error) {
            const backupUnknown = name === 'Backup' && error instanceof TypeError;
            setOperationNotice({
                type: 'error',
                message: backupUnknown
                    ? 'The backup result is unknown because the connection was lost. Server state has been refreshed.'
                    : `${name} failed: ${error.message}`,
            });
        } finally {
            await Promise.all([loadStatus(), loadLogs()]);
            setPendingAction('');
        }
    };

    const state = serverStatus.state || 'unknown';
    const running = state === 'running';
    const transitional = state === 'starting' || state === 'stopping';
    const stateKnown = state !== 'unknown';
    const actionsDisabled = pendingAction !== '' || transitional || !stateKnown;
    const primaryAction = running
        ? {
            name: 'Stop',
            path: '/stop',
            progress: 'Stopping Bedrock gracefully…',
            success: 'Bedrock stopped cleanly.',
            label: 'Stop',
            Icon: Power,
            tone: 'danger',
        }
        : {
            name: 'Start',
            path: '/start',
            progress: 'Starting Bedrock and synchronizing server files…',
            success: 'Bedrock started successfully.',
            label: state === 'failed' ? 'Try again' : 'Start',
            Icon: Play,
            tone: 'primary',
        };
    const PrimaryIcon = pendingAction === primaryAction.name ? LoaderCircle : primaryAction.Icon;

    const connection = connectionState === 'connected'
        ? { label: 'Connected', detail: 'Live updates connected', Icon: Wifi, tone: 'live' }
        : connectionState === 'connecting'
            ? { label: 'Connecting', detail: 'Synchronizing state', Icon: LoaderCircle, tone: 'pending' }
            : managerReachable
                ? { label: 'Reconnecting', detail: 'HTTP fallback active', Icon: WifiOff, tone: 'pending' }
                : { label: 'Offline', detail: 'Showing last known state', Icon: WifiOff, tone: 'offline' };
    const ConnectionIcon = connection.Icon;

    const uptime = formatUptime(serverStatus.started_at, state, now);
    const fileDrops = Number(serverStatus.dropped_logs?.file || 0);
    const otlpDrops = Number(serverStatus.dropped_logs?.otlp || 0);
    const healthWarnings = [
        serverStatus.last_error,
        fileDrops > 0 ? `${fileDrops} local log ${fileDrops === 1 ? 'record was' : 'records were'} dropped.` : '',
        otlpDrops > 0 ? `${otlpDrops} OTLP log ${otlpDrops === 1 ? 'record was' : 'records were'} dropped.` : '',
    ].filter(Boolean);

    return (
        <div className="app-shell">
            <header className="app-header">
                <a className="brand" href={backendAddress || '/'} aria-label="Montainer dashboard home">
                    <span className="brand-mark"><Box aria-hidden="true" /></span>
                    <strong>Montainer</strong>
                </a>

                <div className="header-actions">
                    <div
                        className="connection-chip"
                        data-tone={connection.tone}
                        title={connection.detail}
                        role="status"
                        aria-label={`${connection.label}: ${connection.detail}`}
                    >
                        <ConnectionIcon className={connectionState === 'connecting' ? 'spin' : ''} aria-hidden="true" />
                        <span>{connection.label}</span>
                    </div>
                    <a className="github-link" href="https://github.com/wasinuddy/montainer" target="_blank" rel="noreferrer" aria-label="View Montainer on GitHub">
                        <Github aria-hidden="true" />
                    </a>
                </div>
            </header>

            <main className="dashboard">
                <section className="server-panel" aria-labelledby="instance-title">
                    <div className="server-summary">
                        <div className="server-title-row">
                            <h1 id="instance-title">{instanceName}</h1>
                            <span className="state-pill" data-state={state} role="status">
                                <span className="state-dot" aria-hidden="true" />
                                {stateLabels[state] || state}
                            </span>
                        </div>
                        <div className="server-meta" aria-label="Server runtime details">
                            <span>Uptime <strong>{uptime}</strong></span>
                            <span>PID <strong>{serverStatus.pid || '—'}</strong></span>
                            <span>Generation <strong>#{serverStatus.generation || 0}</strong></span>
                        </div>
                    </div>

                    <div className="server-actions" aria-busy={pendingAction !== ''}>
                        <button
                            type="button"
                            className={`server-action action-${primaryAction.tone}`}
                            disabled={actionsDisabled}
                            onClick={() => runAction(primaryAction)}
                        >
                            <PrimaryIcon className={pendingAction === primaryAction.name ? 'spin' : ''} aria-hidden="true" />
                            <span>{transitional ? `${stateLabels[state]}…` : primaryAction.label}</span>
                        </button>
                        <button
                            type="button"
                            className="server-action action-secondary"
                            disabled={actionsDisabled || !running}
                            onClick={() => runAction({
                                name: 'Restart',
                                path: '/restart',
                                progress: 'Restarting Bedrock gracefully…',
                                success: 'Bedrock restarted successfully.',
                            })}
                        >
                            {pendingAction === 'Restart' ? <LoaderCircle className="spin" aria-hidden="true" /> : <RotateCcw aria-hidden="true" />}
                            <span>Restart</span>
                        </button>
                        <button
                            type="button"
                            className="server-action action-secondary"
                            disabled={actionsDisabled}
                            onClick={() => runAction({
                                name: 'Backup',
                                path: '/save',
                                progress: 'Creating and uploading a consistent world snapshot…',
                                success: (payload) => payload.backup
                                    ? `Backup uploaded: ${payload.backup.key} (${formatBytes(payload.backup.size)}).`
                                    : 'Backup uploaded successfully.',
                            })}
                        >
                            {pendingAction === 'Backup' ? <LoaderCircle className="spin" aria-hidden="true" /> : <Archive aria-hidden="true" />}
                            <span>Backup</span>
                        </button>
                    </div>

                    {healthWarnings.length > 0 && (
                        <div className="server-alert" role="alert">
                            <AlertTriangle aria-hidden="true" />
                            <span>{healthWarnings.join(' ')}</span>
                        </div>
                    )}

                    {operationNotice && (
                        <div className="operation-notice" data-type={operationNotice.type} role={operationNotice.type === 'error' ? 'alert' : 'status'}>
                            {operationNotice.type === 'progress' && <LoaderCircle className="spin" aria-hidden="true" />}
                            {operationNotice.type === 'success' && <CheckCircle2 aria-hidden="true" />}
                            {operationNotice.type === 'error' && <AlertTriangle aria-hidden="true" />}
                            <span>{operationNotice.message}</span>
                            <button type="button" onClick={() => setOperationNotice(null)} aria-label="Dismiss operation message">×</button>
                        </div>
                    )}
                </section>

                <Terminal
                    logs={logs}
                    serverState={state}
                    connectionState={connectionState}
                    backendAddress={backendAddress}
                />
            </main>
        </div>
    );
};

export default App;
