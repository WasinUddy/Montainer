import assert from 'node:assert/strict';
import test from 'node:test';

import {
    classifyLogLine,
    filterLogLines,
    formatBytes,
    formatDuration,
    formatUptime,
} from './ui.js';

test('formats runtime and backup values for compact dashboard metrics', () => {
    assert.equal(formatDuration(5_000), '5s');
    assert.equal(formatDuration(3_665_000), '1h 1m');
    assert.equal(formatUptime('2026-07-16T00:00:00Z', 'running', Date.parse('2026-07-16T00:02:05Z')), '2m 5s');
    assert.equal(formatUptime('2026-07-16T00:00:00Z', 'stopped'), '—');
    assert.equal(formatBytes(1536), '1.5 KiB');
});

test('classifies and filters raw Bedrock log lines conservatively', () => {
    const logs = [
        '[INFO] Server started',
        '[WARN] Allow list is empty',
        '[ERROR] Unknown command',
        'CREATING VANILLA WORLD',
    ];
    assert.equal(classifyLogLine(logs[0]), 'info');
    assert.equal(classifyLogLine(logs[1]), 'warning');
    assert.equal(classifyLogLine(logs[2]), 'error');
    assert.equal(classifyLogLine(logs[3]), 'neutral');
    assert.deepEqual(filterLogLines(logs, { level: 'warning' }), [logs[1]]);
    assert.deepEqual(filterLogLines(logs, { query: 'server' }), [logs[0]]);
});
