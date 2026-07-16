import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import PropTypes from 'prop-types';
import {
    ArrowDownToLine,
    Check,
    ChevronDown,
    Copy,
    LoaderCircle,
    Pause,
    Play,
    Search,
    SendHorizontal,
    Terminal as TerminalIcon,
} from 'lucide-react';
import { readApiError } from './api.js';
import {
    applyMinecraftCommandSuggestion,
    getMinecraftCommandGuide,
    getMinecraftCommandSuggestions,
} from './minecraftCommands.js';
import { classifyLogLine, filterLogLines } from './ui.js';

const levelFilters = [
    { value: 'all', label: 'All logs' },
    { value: 'info', label: 'Info' },
    { value: 'warning', label: 'Warnings' },
    { value: 'error', label: 'Errors' },
];

const highlightLine = (line, query) => {
    const normalizedQuery = query.trim();
    if (!normalizedQuery) return line || ' ';
    const matchIndex = line.toLowerCase().indexOf(normalizedQuery.toLowerCase());
    if (matchIndex < 0) return line || ' ';
    return (
        <>
            {line.slice(0, matchIndex)}
            <mark>{line.slice(matchIndex, matchIndex + normalizedQuery.length)}</mark>
            {line.slice(matchIndex + normalizedQuery.length)}
        </>
    );
};

const Terminal = ({ logs, serverState, connectionState, backendAddress }) => {
    const [command, setCommand] = useState('');
    const [commandCaret, setCommandCaret] = useState(0);
    const [commandHistory, setCommandHistory] = useState([]);
    const [historyIndex, setHistoryIndex] = useState(-1);
    const [historyDraft, setHistoryDraft] = useState('');
    const [commandError, setCommandError] = useState('');
    const [sending, setSending] = useState(false);
    const [autocompleteOpen, setAutocompleteOpen] = useState(false);
    const [activeSuggestion, setActiveSuggestion] = useState(-1);
    const [query, setQuery] = useState('');
    const [level, setLevel] = useState('all');
    const [following, setFollowing] = useState(true);
    const [paused, setPaused] = useState(false);
    const [frozenLogs, setFrozenLogs] = useState([]);
    const [copied, setCopied] = useState(false);
    const outputRef = useRef(null);
    const inputRef = useRef(null);

    const sourceLogs = paused ? frozenLogs : logs;
    const visibleLogs = useMemo(
        () => filterLogLines(sourceLogs, { level, query }),
        [sourceLogs, level, query],
    );
    const visibleRows = useMemo(() => {
        const occurrences = new Map();
        return visibleLogs.map((line) => {
            const occurrence = occurrences.get(line) || 0;
            occurrences.set(line, occurrence + 1);
            return { line, key: `${line}\u0000${occurrence}` };
        });
    }, [visibleLogs]);
    const suggestions = useMemo(
        () => (autocompleteOpen && /\S/.test(command)
            ? getMinecraftCommandSuggestions(command, {
                caret: commandCaret,
                history: commandHistory,
            })
            : []),
        [autocompleteOpen, command, commandCaret, commandHistory],
    );
    const commandGuide = useMemo(
        () => (autocompleteOpen ? getMinecraftCommandGuide(command, { caret: commandCaret }) : null),
        [autocompleteOpen, command, commandCaret],
    );
    const canSend = serverState === 'running' && !sending;
    const showSuggestions = canSend && suggestions.length > 0;
    const showGuide = canSend && Boolean(commandGuide);
    const showCommandAssist = showGuide || showSuggestions;
    const activeSuggestionId = activeSuggestion >= 0 && suggestions[activeSuggestion]
        ? `command-suggestion-${suggestions[activeSuggestion]?.id}`
        : undefined;
    const commandDescriptionIds = [showGuide ? 'command-guide' : '', commandError ? 'command-error' : '']
        .filter(Boolean)
        .join(' ') || undefined;

    const scrollToLatest = useCallback(() => {
        const output = outputRef.current;
        if (output) output.scrollTop = output.scrollHeight;
        setFollowing(true);
    }, []);

    useEffect(() => {
        if (!following || paused) return undefined;
        const frame = window.requestAnimationFrame(scrollToLatest);
        return () => window.cancelAnimationFrame(frame);
    }, [following, paused, scrollToLatest, visibleRows]);

    useEffect(() => {
        const focusCommand = (event) => {
            const target = event.target;
            const isEditable = target instanceof HTMLElement
                && (target.matches('input, textarea, select') || target.isContentEditable);
            if (event.key === '/' && !isEditable && serverState === 'running') {
                event.preventDefault();
                inputRef.current?.focus();
            }
        };
        window.addEventListener('keydown', focusCommand);
        return () => window.removeEventListener('keydown', focusCommand);
    }, [serverState]);

    const restoreInputFocus = (caret) => {
        window.requestAnimationFrame(() => {
            inputRef.current?.focus();
            inputRef.current?.setSelectionRange(caret, caret);
        });
    };

    const acceptSuggestion = (suggestion) => {
        if (!suggestion) return;
        const result = applyMinecraftCommandSuggestion(command, suggestion);
        setCommand(result.value);
        setCommandCaret(result.caret);
        setHistoryDraft(result.value);
        setHistoryIndex(-1);
        setAutocompleteOpen(false);
        setActiveSuggestion(-1);
        setCommandError('');
        restoreInputFocus(result.caret);
    };

    const handleSubmit = async (event) => {
        event.preventDefault();
        const submittedCommand = command.trim().replace(/^\/(?=\S)/, '');
        if (!submittedCommand || !canSend) return;
        setSending(true);
        setAutocompleteOpen(false);
        setCommandError('');

        try {
            const response = await fetch(`${backendAddress}/command`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ command: submittedCommand }),
            });
            if (!response.ok) throw new Error(await readApiError(response));
            setCommandHistory((previous) => [...previous.slice(-49), submittedCommand]);
            setCommand('');
            setCommandCaret(0);
            setHistoryDraft('');
            setHistoryIndex(-1);
            setActiveSuggestion(-1);
        } catch (requestError) {
            setCommandError(requestError.message);
        } finally {
            setSending(false);
        }
    };

    const showHistoryEntry = (nextIndex) => {
        const nextCommand = nextIndex < 0
            ? historyDraft
            : commandHistory[commandHistory.length - 1 - nextIndex];
        setHistoryIndex(nextIndex);
        setCommand(nextCommand);
        setCommandCaret(nextCommand.length);
        setAutocompleteOpen(false);
        setActiveSuggestion(-1);
        restoreInputFocus(nextCommand.length);
    };

    const handleKeyDown = (event) => {
        if (event.nativeEvent?.isComposing) return;

        if (event.key === 'Escape') {
            if (showCommandAssist) {
                event.preventDefault();
                setAutocompleteOpen(false);
                setActiveSuggestion(-1);
            } else {
                event.currentTarget.blur();
            }
            return;
        }

        if (showSuggestions && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
            event.preventDefault();
            setActiveSuggestion((current) => {
                if (event.key === 'ArrowDown') return current < suggestions.length - 1 ? current + 1 : 0;
                return current > 0 ? current - 1 : suggestions.length - 1;
            });
            return;
        }

        if (showSuggestions && event.key === 'Tab') {
            event.preventDefault();
            acceptSuggestion(suggestions[activeSuggestion >= 0 ? activeSuggestion : 0]);
            return;
        }

        if (showSuggestions && event.key === 'Enter' && activeSuggestion >= 0) {
            event.preventDefault();
            acceptSuggestion(suggestions[activeSuggestion]);
            return;
        }

        if (event.key === 'ArrowUp') {
            if (commandHistory.length === 0) return;
            event.preventDefault();
            if (historyIndex === -1) setHistoryDraft(command);
            showHistoryEntry(Math.min(historyIndex + 1, commandHistory.length - 1));
        } else if (event.key === 'ArrowDown') {
            if (historyIndex === -1) return;
            event.preventDefault();
            showHistoryEntry(historyIndex - 1);
        }
    };

    const handleScroll = () => {
        const output = outputRef.current;
        if (!output || paused) return;
        const isAtBottom = output.scrollHeight - output.scrollTop - output.clientHeight < 40;
        setFollowing(isAtBottom);
    };

    const togglePause = () => {
        if (paused) {
            setPaused(false);
            window.requestAnimationFrame(scrollToLatest);
            return;
        }
        setFrozenLogs([...logs]);
        setPaused(true);
    };

    const copyVisibleLogs = async () => {
        try {
            await navigator.clipboard.writeText(visibleLogs.join('\n'));
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1600);
        } catch {
            setCommandError('Unable to copy logs in this browser.');
        }
    };

    const streamLabel = paused
        ? 'Paused'
        : connectionState === 'connected'
            ? 'Live'
            : connectionState === 'connecting'
                ? 'Syncing'
                : 'Reconnecting';

    return (
        <section className="console-panel" aria-labelledby="console-title">
            <header className="console-header">
                <div className="console-title">
                    <span className="console-icon"><TerminalIcon aria-hidden="true" /></span>
                    <h2 id="console-title">Console</h2>
                </div>
                <div className="console-status" data-state={paused ? 'paused' : connectionState}>
                    <span aria-hidden="true" />
                    {streamLabel} · {sourceLogs.length} lines
                </div>
            </header>

            <div className="console-toolbar">
                <label className="log-search">
                    <Search aria-hidden="true" />
                    <span className="sr-only">Search console logs</span>
                    <input
                        type="search"
                        value={query}
                        onChange={(event) => setQuery(event.target.value)}
                        placeholder="Search logs"
                    />
                    {query && <button type="button" onClick={() => setQuery('')} aria-label="Clear log search">×</button>}
                </label>

                <label className="level-select">
                    <span className="sr-only">Log level</span>
                    <select value={level} onChange={(event) => setLevel(event.target.value)} aria-label="Log level">
                        {levelFilters.map((filter) => <option key={filter.value} value={filter.value}>{filter.label}</option>)}
                    </select>
                    <ChevronDown aria-hidden="true" />
                </label>

                <div className="console-tools">
                    <button
                        type="button"
                        onClick={togglePause}
                        className={paused ? 'active' : ''}
                        aria-label={paused ? 'Resume live logs' : 'Pause live logs'}
                        aria-pressed={paused}
                        title={paused ? 'Resume live logs' : 'Pause live logs'}
                    >
                        {paused ? <Play aria-hidden="true" /> : <Pause aria-hidden="true" />}
                        <span>{paused ? 'Resume' : 'Pause'}</span>
                    </button>
                    <button
                        type="button"
                        onClick={copyVisibleLogs}
                        aria-label={copied ? 'Logs copied' : 'Copy visible logs'}
                        title="Copy visible logs"
                        disabled={visibleLogs.length === 0}
                    >
                        {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
                        <span>{copied ? 'Copied' : 'Copy'}</span>
                    </button>
                </div>
            </div>

            <div className="log-stage">
                <div
                    ref={outputRef}
                    className="log-output"
                    onScroll={handleScroll}
                    role="region"
                    aria-label="Minecraft server logs"
                    tabIndex="0"
                >
                    {visibleRows.length === 0 ? (
                        <div className="log-empty">
                            <Search aria-hidden="true" />
                            <strong>{sourceLogs.length === 0 ? 'Waiting for server output' : 'No matching log lines'}</strong>
                            <span>{sourceLogs.length === 0 ? 'New Bedrock output will appear here.' : 'Try another search or level.'}</span>
                        </div>
                    ) : visibleRows.map(({ line, key }) => (
                        <div className="log-line" data-level={classifyLogLine(line)} key={key}>
                            <code>{highlightLine(line, query)}</code>
                        </div>
                    ))}
                </div>
                {!following && !paused && (
                    <button type="button" className="jump-latest" onClick={scrollToLatest}>
                        <ArrowDownToLine aria-hidden="true" /> Jump to latest
                    </button>
                )}
                {paused && <div className="paused-banner"><Pause aria-hidden="true" /> Live updates paused</div>}
            </div>

            <span className="sr-only" role="status" aria-live="polite" aria-atomic="true">
                {!paused && logs.length > 0 ? `Latest server output: ${logs[logs.length - 1]}` : ''}
            </span>

            <form onSubmit={handleSubmit} className="command-form" aria-busy={sending}>
                {showCommandAssist && (
                    <div className="command-assist">
                        {showGuide && (
                            <section className="command-guide" id="command-guide" aria-label={`${commandGuide.displayCommand} command guide`}>
                                <header className="command-guide-header">
                                    <div>
                                        <strong>{commandGuide.title}</strong>
                                        <span>{commandGuide.description}</span>
                                    </div>
                                    <code>{commandGuide.displayCommand}</code>
                                </header>
                                <div className="command-guide-variants">
                                    {commandGuide.variants.slice(0, 4).map((variant) => (
                                        <div className="command-guide-variant" key={variant.id}>
                                            <code>{commandGuide.displayCommand} {variant.syntax}</code>
                                            <span>{variant.description}</span>
                                        </div>
                                    ))}
                                </div>
                                <p>{'<> required · [] optional · showing 4 common server forms'}</p>
                            </section>
                        )}

                        {showSuggestions && (
                            <div className="autocomplete-list" id="command-suggestions" role="listbox" aria-label="Minecraft command suggestions">
                                {suggestions.map((suggestion, index) => (
                                    <button
                                        type="button"
                                        role="option"
                                        id={`command-suggestion-${suggestion.id}`}
                                        className="autocomplete-option"
                                        aria-selected={index === activeSuggestion}
                                        tabIndex="-1"
                                        key={suggestion.id}
                                        onPointerDown={(event) => event.preventDefault()}
                                        onClick={() => acceptSuggestion(suggestion)}
                                    >
                                        <span className="autocomplete-command">{suggestion.label}</span>
                                        <span className="autocomplete-description">{suggestion.description}</span>
                                    </button>
                                ))}
                            </div>
                        )}
                    </div>
                )}

                <span className="sr-only" role="status" aria-live="polite">
                    {showSuggestions ? `${suggestions.length} command suggestions available. Use arrow keys and Tab to complete.` : ''}
                </span>

                <div className="command-shell">
                    <span className="command-prompt" aria-hidden="true">›</span>
                    <input
                        ref={inputRef}
                        type="text"
                        role="combobox"
                        value={command}
                        onChange={(event) => {
                            const nextCommand = event.target.value;
                            const nextCaret = event.target.selectionStart ?? nextCommand.length;
                            setCommand(nextCommand);
                            setCommandCaret(nextCaret);
                            setHistoryDraft(nextCommand);
                            setHistoryIndex(-1);
                            setAutocompleteOpen(true);
                            setActiveSuggestion(-1);
                            setCommandError('');
                        }}
                        onSelect={(event) => setCommandCaret(event.currentTarget.selectionStart ?? command.length)}
                        onFocus={() => setAutocompleteOpen(true)}
                        onBlur={() => {
                            setAutocompleteOpen(false);
                            setActiveSuggestion(-1);
                        }}
                        onKeyDown={handleKeyDown}
                        placeholder={serverState === 'running' ? "Type a command — try 'help'" : `Commands unavailable while server is ${serverState}`}
                        disabled={serverState !== 'running' || sending}
                        aria-label="Minecraft command"
                        aria-autocomplete="list"
                        aria-expanded={showSuggestions}
                        aria-controls={showSuggestions ? 'command-suggestions' : undefined}
                        aria-activedescendant={activeSuggestionId}
                        aria-describedby={commandDescriptionIds}
                        autoComplete="off"
                    />
                    <button type="submit" disabled={!canSend || !command.trim()}>
                        {sending ? <LoaderCircle className="spin" aria-hidden="true" /> : <SendHorizontal aria-hidden="true" />}
                        <span>{sending ? 'Sending' : 'Run'}</span>
                    </button>
                </div>
                {commandError && <span className="command-error" id="command-error" role="alert">{commandError}</span>}
            </form>
        </section>
    );
};

Terminal.propTypes = {
    logs: PropTypes.arrayOf(PropTypes.string).isRequired,
    serverState: PropTypes.string.isRequired,
    connectionState: PropTypes.string.isRequired,
    backendAddress: PropTypes.string.isRequired,
};

export default Terminal;
