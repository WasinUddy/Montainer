const zeroTimestamp = /^0*1-01-01T/;

export const formatDuration = (milliseconds) => {
    if (!Number.isFinite(milliseconds) || milliseconds < 0) return '—';
    const totalSeconds = Math.floor(milliseconds / 1000);
    const days = Math.floor(totalSeconds / 86400);
    const hours = Math.floor((totalSeconds % 86400) / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;

    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    if (minutes > 0) return `${minutes}m ${seconds}s`;
    return `${seconds}s`;
};

export const formatUptime = (startedAt, state, now = Date.now()) => {
    if (state !== 'running') return '—';
    const started = Date.parse(startedAt);
    if (!Number.isFinite(started)) return 'Starting…';
    return formatDuration(Math.max(0, now - started));
};

export const formatDateTime = (value) => {
    if (!value || zeroTimestamp.test(value)) return 'not yet';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'not yet';
    return new Intl.DateTimeFormat(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    }).format(date);
};

export const formatBytes = (bytes) => {
    const value = Number(bytes);
    if (!Number.isFinite(value) || value < 0) return 'unknown size';
    if (value < 1024) return `${value} B`;
    const units = ['KiB', 'MiB', 'GiB', 'TiB'];
    let size = value / 1024;
    let unit = units[0];
    for (let index = 1; index < units.length && size >= 1024; index += 1) {
        size /= 1024;
        unit = units[index];
    }
    return `${size >= 10 ? size.toFixed(0) : size.toFixed(1)} ${unit}`;
};

export const classifyLogLine = (line) => {
    const normalized = String(line).toUpperCase();
    if (/\b(FATAL|ERROR)\b/.test(normalized)) return 'error';
    if (/\bWARN(?:ING)?\b/.test(normalized)) return 'warning';
    if (/\bINFO\b/.test(normalized)) return 'info';
    if (/\bDEBUG\b/.test(normalized)) return 'debug';
    return 'neutral';
};

export const filterLogLines = (logs, { level = 'all', query = '' } = {}) => {
    const normalizedQuery = query.trim().toLowerCase();
    return logs.filter((line) => {
        if (level !== 'all' && classifyLogLine(line) !== level) return false;
        return !normalizedQuery || line.toLowerCase().includes(normalizedQuery);
    });
};
