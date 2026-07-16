import assert from 'node:assert/strict';
import test from 'node:test';

import {
    MINECRAFT_COMMANDS,
    applyMinecraftCommandSuggestion,
    getMinecraftCommandGuide,
    getMinecraftCommandSuggestions,
} from './minecraftCommands.js';

const labels = (suggestions) => suggestions.map(({ label }) => label);

test('ships an offline stable Bedrock command catalog', () => {
    assert.ok(MINECRAFT_COMMANDS.length > 75);
    assert.ok(MINECRAFT_COMMANDS.some(({ name }) => name === 'allowlist'));
    assert.ok(MINECRAFT_COMMANDS.some(({ name }) => name === 'stop'));
    assert.ok(MINECRAFT_COMMANDS.every(({ name }) => !name.startsWith('/')));
});

test('returns at most six command matches with prefixes ahead of substrings', () => {
    const suggestions = getMinecraftCommandSuggestions('set', { limit: 100 });
    assert.equal(suggestions.length, 4);
    assert.deepEqual(labels(suggestions).slice(0, 3).sort(), ['setblock', 'setmaxplayers', 'setworldspawn']);
    assert.equal(suggestions[3].label, 'changesetting');
    assert.ok(getMinecraftCommandSuggestions('').length <= 6);
});

test('preserves an optional leading slash when applying a command completion', () => {
    const suggestion = getMinecraftCommandSuggestions('/gamem')[0];
    assert.equal(suggestion.label, 'gamemode');
    assert.deepEqual(
        applyMinecraftCommandSuggestion('/gamem', suggestion),
        { value: '/gamemode', caret: 9 },
    );
});

test('matches a documented command alias without replacing it with the canonical name', () => {
    const suggestion = getMinecraftCommandSuggestions('/tp')[0];
    assert.equal(suggestion.command, 'teleport');
    assert.equal(suggestion.label, 'tp');
    assert.equal(applyMinecraftCommandSuggestion('/tp', suggestion).value, '/tp');
});

test('returns an alias-aware teleport guide only for an exact command', () => {
    assert.equal(getMinecraftCommandGuide('tp')?.command, 'teleport');
    assert.equal(getMinecraftCommandGuide('tp Steve')?.displayCommand, 'tp');
    assert.equal(getMinecraftCommandGuide('/teleport ')?.displayCommand, '/teleport');
    assert.equal(getMinecraftCommandGuide('/tp @p ~ ~ ~')?.variants.length, 10);
    assert.equal(getMinecraftCommandGuide('t'), null);
    assert.equal(getMinecraftCommandGuide('tpfoo'), null);
    assert.equal(getMinecraftCommandGuide(''), null);
});

test('prioritizes server-console teleport signatures and preserves official rotation order', () => {
    const guide = getMinecraftCommandGuide('tp ');
    assert.equal(
        guide.variants[0].syntax,
        '<victim: target> <destination: x y z> [checkForBlocks: Boolean]',
    );
    assert.equal(
        guide.variants[1].syntax,
        '<victim: target> <destination: target> [checkForBlocks: Boolean]',
    );
    assert.ok(guide.variants.some(({ syntax }) => syntax.includes('[yRot: rotation] [xRot: rotation]')));
});

test('offers safe teleport selector, coordinate, facing, and boolean completions', () => {
    assert.deepEqual(labels(getMinecraftCommandSuggestions('tp @')), ['@a', '@e', '@p', '@r']);
    const coordinates = getMinecraftCommandSuggestions('/tp ~')[0];
    assert.equal(coordinates.label, '~ ~ ~');
    assert.deepEqual(
        applyMinecraftCommandSuggestion('/tp ~', coordinates),
        { value: '/tp ~ ~ ~', caret: 9 },
    );
    assert.deepEqual(labels(getMinecraftCommandSuggestions('tp @p ')), ['@a', '@e', '@p', '@r', '~ ~ ~']);
    assert.deepEqual(labels(getMinecraftCommandSuggestions('tp @p ~ ~ ~ ')), ['facing', 'false', 'true']);
    assert.deepEqual(labels(getMinecraftCommandSuggestions('tp @p ~ ~ ~ facing @')), ['@a', '@e', '@p', '@r']);
    assert.ok(getMinecraftCommandSuggestions('tp ')[0].id.match(/^\S+$/));
});

test('offers contextual values for common dedicated server commands', () => {
    assert.equal(getMinecraftCommandSuggestions('difficulty h')[0].label, 'hard');
    assert.equal(getMinecraftCommandSuggestions('/gamemode c')[0].label, 'creative');
    assert.deepEqual(labels(getMinecraftCommandSuggestions('weather t')), ['thunder']);
    assert.deepEqual(labels(getMinecraftCommandSuggestions('time ')), ['add', 'query', 'set']);
    assert.deepEqual(labels(getMinecraftCommandSuggestions('time set n')).slice(0, 2), ['night', 'noon']);
    assert.deepEqual(labels(getMinecraftCommandSuggestions('save ')), ['hold', 'query', 'resume']);
    assert.deepEqual(labels(getMinecraftCommandSuggestions('allowlist r')), ['reload', 'remove']);
});

test('applies a contextual value at the caret without discarding surrounding text', () => {
    const input = 'time set ni trailing-text';
    const caret = 'time set ni'.length;
    const suggestion = getMinecraftCommandSuggestions(input, { caret })[0];
    assert.deepEqual(
        applyMinecraftCommandSuggestion(input, suggestion),
        { value: 'time set night trailing-text', caret: 'time set night'.length },
    );
});

test('uses command history only as a tie breaker', () => {
    const suggestions = getMinecraftCommandSuggestions('re', {
        history: ['reload', 'reload', 'reloadconfig'],
    });
    assert.equal(suggestions[0].label, 'reload');
    assert.ok(labels(suggestions).includes('replaceitem'));
});

test('returns no contextual suggestions for unsupported or unknown arguments', () => {
    assert.deepEqual(getMinecraftCommandSuggestions('give Steve '), []);
    assert.deepEqual(getMinecraftCommandSuggestions('not-a-command '), []);
});

test('a malformed suggestion is a safe no-op', () => {
    assert.deepEqual(
        applyMinecraftCommandSuggestion('list', null),
        { value: 'list', caret: 4 },
    );
});
