import assert from 'node:assert/strict';
import test from 'node:test';

import { getBaseUrl, getWebSocketUrl, readApiError } from './api.js';

test('builds root and subpath backend URLs from the served page', () => {
    assert.equal(getBaseUrl({ origin: 'https://example.test', pathname: '/' }), 'https://example.test');
    assert.equal(
        getBaseUrl({ origin: 'https://example.test', pathname: '/servers/friends/' }),
        'https://example.test/servers/friends',
    );
    assert.equal(
        getWebSocketUrl('https://example.test/servers/friends'),
        'wss://example.test/servers/friends/ws/stream',
    );
});

test('reads Gin and compatibility API error envelopes', async () => {
    const nested = new Response(JSON.stringify({ detail: { message: 'server is stopped' } }), {
        status: 409,
        headers: { 'Content-Type': 'application/json' },
    });
    assert.equal(await readApiError(nested), 'server is stopped');

    const simple = new Response(JSON.stringify({ detail: 'command is required' }), {
        status: 422,
        headers: { 'Content-Type': 'application/json' },
    });
    assert.equal(await readApiError(simple), 'command is required');
});
