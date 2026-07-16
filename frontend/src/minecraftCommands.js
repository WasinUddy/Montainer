/**
 * Offline Minecraft Bedrock command catalog.
 *
 * Command names are based on Microsoft's stable Bedrock command reference and
 * deliberately omit the chat-style leading slash. The console can therefore
 * preserve whichever style the operator starts typing.
 */
const commandRows = [
    ['aimassist', 'Enable aim assistance.'],
    ['allowlist', 'Manage the dedicated server allowlist.'],
    ['camera', 'Change a player camera perspective.'],
    ['camerashake', 'Apply or stop a camera shake.'],
    ['changesetting', 'Change a dedicated server setting at runtime.'],
    ['clear', 'Clear items from a player inventory.'],
    ['clearspawnpoint', 'Remove a player spawn point.'],
    ['clone', 'Copy blocks from one region to another.'],
    ['controlscheme', 'Set or clear a player control scheme.'],
    ['damage', 'Apply damage to selected entities.'],
    ['daylock', 'Lock or unlock the day-night cycle.'],
    ['deop', 'Revoke operator status from a player.'],
    ['dialogue', 'Open NPC dialogue for a player.'],
    ['difficulty', 'Set the world difficulty.'],
    ['effect', 'Add or clear entity status effects.'],
    ['enchant', 'Enchant a player selected item.'],
    ['event', 'Trigger an entity event.'],
    ['execute', 'Run a command in another execution context.'],
    ['fill', 'Fill a region with blocks.'],
    ['fog', 'Add or remove fog settings.'],
    ['function', 'Run a behavior-pack function.'],
    ['gamemode', 'Set a player game mode.'],
    ['gamerule', 'Read or change a game rule.'],
    ['gametest', 'Manage GameTest framework tests.'],
    ['give', 'Give an item to a player.'],
    ['help', 'List commands or show command usage.'],
    ['hud', 'Show or hide HUD elements.'],
    ['inputpermission', 'Change a player input permissions.'],
    ['kick', 'Disconnect a player from the server.'],
    ['kill', 'Kill selected entities.'],
    ['list', 'List players currently on the server.'],
    ['locate', 'Find a nearby biome or structure.'],
    ['loot', 'Place loot in the world or an inventory.'],
    ['me', 'Send an action-style chat message.'],
    ['mobevent', 'Enable or disable a mob event.'],
    ['music', 'Control music playback.'],
    ['op', 'Grant operator status to a player.'],
    ['packstack', 'Print the active resource and behavior packs.'],
    ['particle', 'Create a particle emitter.'],
    ['permission', 'Reload and apply dedicated server permissions.'],
    ['place', 'Place a feature or jigsaw structure.'],
    ['playanimation', 'Play an entity animation.'],
    ['playsound', 'Play a sound for selected players.'],
    ['project', 'Manage an Editor project.'],
    ['recipe', 'Lock or unlock player recipes.'],
    ['reload', 'Reload behavior-pack functions and scripts.'],
    ['reloadconfig', 'Reload server configuration files.'],
    ['reloadpacketlimitconfig', 'Reload packet-limit configuration.'],
    ['replaceitem', 'Replace an item in an inventory.'],
    ['ride', 'Manage entity riders and mounts.'],
    ['save', 'Control dedicated server world saving.'],
    ['say', 'Send a message to all players.'],
    ['schedule', 'Schedule a function to run later.'],
    ['scoreboard', 'Manage scoreboard objectives and players.'],
    ['script', 'Use script debugging commands.'],
    ['scriptevent', 'Send an event to scripts.'],
    ['sendshowstoreoffer', 'Open a Marketplace offer for a player.'],
    ['setblock', 'Replace a block at a position.'],
    ['setmaxplayers', 'Set the maximum player count.'],
    ['setworldspawn', 'Set the default world spawn.'],
    ['spawnpoint', 'Set a player spawn point.'],
    ['spreadplayers', 'Spread entities across random positions.'],
    ['stop', 'Gracefully stop the dedicated server.'],
    ['stopsound', 'Stop a sound for selected players.'],
    ['structure', 'Save or load a structure.'],
    ['summon', 'Summon an entity.'],
    ['tag', 'Manage entity tags.'],
    ['teleport', 'Teleport entities to another location.', ['tp']],
    ['tell', 'Send a private message.'],
    ['tellraw', 'Send a JSON-formatted message.'],
    ['testfor', 'Count entities matching a selector.'],
    ['testforblock', 'Test the block at a position.'],
    ['testforblocks', 'Compare blocks in two regions.'],
    ['tickingarea', 'Manage ticking areas.'],
    ['time', 'Add, set, or query world time.'],
    ['title', 'Display a title for selected players.'],
    ['titleraw', 'Display a JSON-formatted title.'],
    ['toggledownfall', 'Toggle precipitation.'],
    ['transfer', 'Transfer a player to another server.'],
    ['weather', 'Set or query the weather.'],
    ['wsserver', 'Connect to a WebSocket server.'],
    ['xp', 'Add or remove player experience.'],
];

const commonCommandOrder = [
    'help', 'list', 'say', 'allowlist', 'op', 'deop', 'kick', 'save', 'stop',
    'permission', 'reload', 'reloadconfig', 'setmaxplayers', 'gamemode',
    'difficulty', 'gamerule', 'time', 'weather', 'give', 'teleport', 'effect',
    'enchant', 'kill', 'summon',
];

const commonCommandRank = new Map(commonCommandOrder.map((name, index) => [name, index]));

/**
 * Stable, offline command metadata used by the autocomplete UI.
 *
 * @type {ReadonlyArray<Readonly<{name: string, description: string, aliases: ReadonlyArray<string>, rank: number}>>}
 */
export const MINECRAFT_COMMANDS = Object.freeze(commandRows.map(([name, description, aliases = []]) => Object.freeze({
    name,
    description,
    aliases: Object.freeze(aliases),
    rank: commonCommandRank.get(name) ?? commonCommandOrder.length + 1,
})));

const values = (...entries) => Object.freeze(entries.map(([value, description, aliases = []]) => Object.freeze({
    value,
    description,
    aliases: Object.freeze(aliases),
})));

const teleportDestinations = values(
    ['@p', 'Nearest player.'],
    ['@a', 'All players.'],
    ['@e', 'All entities.'],
    ['@r', 'Random player.'],
    ['~ ~ ~', 'Relative x, y, and z coordinates.'],
);

const booleanValues = values(
    ['false', 'Allow teleporting into occupied blocks.'],
    ['true', 'Cancel if the destination is occupied.'],
);

const teleportContinuations = values(
    ['facing', 'Face a position or entity after teleporting.'],
    ['false', 'Allow teleporting into occupied blocks.'],
    ['true', 'Cancel if the destination is occupied.'],
);

const contextualValues = Object.freeze({
    allowlist: Object.freeze({
        1: values(
            ['add', 'Add a player to the allowlist.'],
            ['remove', 'Remove a player from the allowlist.'],
            ['off', 'Disable the allowlist.'],
            ['list', 'List allowlisted players.'],
            ['reload', 'Reload the allowlist file.'],
            ['on', 'Enable the allowlist.'],
        ),
    }),
    difficulty: Object.freeze({
        1: values(
            ['peaceful', 'Peaceful difficulty.', ['p', '0']],
            ['easy', 'Easy difficulty.', ['e', '1']],
            ['normal', 'Normal difficulty.', ['n', '2']],
            ['hard', 'Hard difficulty.', ['h', '3']],
        ),
    }),
    gamemode: Object.freeze({
        1: values(
            ['default', 'Use the default game mode.', ['d']],
            ['creative', 'Creative mode.', ['c']],
            ['spectator', 'Spectator mode.'],
            ['survival', 'Survival mode.', ['s']],
            ['adventure', 'Adventure mode.', ['a']],
        ),
    }),
    save: Object.freeze({
        1: values(
            ['query', 'Check whether saving is ready.'],
            ['hold', 'Pause automatic world saving.'],
            ['resume', 'Resume automatic world saving.'],
        ),
    }),
    time: Object.freeze({
        1: values(
            ['add', 'Advance world time by ticks.'],
            ['set', 'Set world time.'],
            ['query', 'Read a world time value.'],
        ),
        '2:set': values(
            ['day', 'Set time to day.'],
            ['sunrise', 'Set time to sunrise.'],
            ['noon', 'Set time to noon.'],
            ['sunset', 'Set time to sunset.'],
            ['night', 'Set time to night.'],
            ['midnight', 'Set time to midnight.'],
        ),
        '2:query': values(
            ['daytime', 'Query the current daytime.'],
            ['gametime', 'Query total elapsed game time.'],
            ['day', 'Query the elapsed day count.'],
        ),
    }),
    weather: Object.freeze({
        1: values(
            ['clear', 'Clear precipitation.'],
            ['rain', 'Start rain or snow.'],
            ['thunder', 'Start a thunderstorm.'],
            ['query', 'Read the current weather.'],
        ),
    }),
});

const teleportGuideVariants = Object.freeze([
    Object.freeze({
        id: 'player-position',
        syntax: '<victim: target> <destination: x y z> [checkForBlocks: Boolean]',
        description: 'Move a target to coordinates.',
    }),
    Object.freeze({
        id: 'player-target',
        syntax: '<victim: target> <destination: target> [checkForBlocks: Boolean]',
        description: 'Move a target to another entity.',
    }),
    Object.freeze({
        id: 'player-facing-position',
        syntax: '<victim: target> <destination: x y z> facing <lookAtPosition: x y z> [checkForBlocks: Boolean]',
        description: 'Move a target and face coordinates.',
    }),
    Object.freeze({
        id: 'player-facing-entity',
        syntax: '<victim: target> <destination: x y z> facing <lookAtEntity: target> [checkForBlocks: Boolean]',
        description: 'Move a target and face an entity.',
    }),
    Object.freeze({
        id: 'player-rotation',
        syntax: '<victim: target> <destination: x y z> [yRot: rotation] [xRot: rotation] [checkForBlocks: Boolean]',
        description: 'Move a target with an optional rotation.',
    }),
    Object.freeze({
        id: 'self-position',
        syntax: '<destination: x y z> [checkForBlocks: Boolean]',
        description: 'Move the command executor to coordinates.',
    }),
    Object.freeze({
        id: 'self-rotation',
        syntax: '<destination: x y z> [yRot: rotation] [xRot: rotation] [checkForBlocks: Boolean]',
        description: 'Move the command executor with a rotation.',
    }),
    Object.freeze({
        id: 'self-facing-position',
        syntax: '<destination: x y z> facing <lookAtPosition: x y z> [checkForBlocks: Boolean]',
        description: 'Move the command executor and face coordinates.',
    }),
    Object.freeze({
        id: 'self-facing-entity',
        syntax: '<destination: x y z> facing <lookAtEntity: target> [checkForBlocks: Boolean]',
        description: 'Move the command executor and face an entity.',
    }),
    Object.freeze({
        id: 'self-target',
        syntax: '<destination: target>',
        description: 'Move the command executor to another entity.',
    }),
]);

const commandGuides = Object.freeze({
    teleport: Object.freeze({
        title: 'Teleport usage',
        description: 'Move a player or entity to coordinates or another target.',
        variants: teleportGuideVariants,
    }),
});

const clampCaret = (input, caret) => {
    const requested = Number.isInteger(caret) ? caret : input.length;
    return Math.min(Math.max(requested, 0), input.length);
};

const completionContext = (input, requestedCaret) => {
    const caret = clampCaret(input, requestedCaret);
    const beforeCaret = input.slice(0, caret);
    const matches = [...beforeCaret.matchAll(/\S+/g)];
    const startsNewToken = beforeCaret.length === 0 || /\s$/.test(beforeCaret);

    if (startsNewToken) {
        return {
            caret,
            fragment: '',
            replaceStart: caret,
            replaceEnd: caret,
            tokenIndex: matches.length,
            tokens: matches.map((match) => match[0]),
        };
    }

    const current = matches[matches.length - 1];
    let replaceEnd = caret;
    while (replaceEnd < input.length && !/\s/.test(input[replaceEnd])) replaceEnd += 1;

    return {
        caret,
        fragment: beforeCaret.slice(current.index),
        replaceStart: current.index,
        replaceEnd,
        tokenIndex: matches.length - 1,
        tokens: matches.map((match) => match[0]),
    };
};

const matchValue = (query, value, aliases = []) => {
    if (!query) return { score: 3, matchedValue: value };
    const candidates = [value, ...aliases];
    let best;
    candidates.forEach((candidate, index) => {
        let score = Number.POSITIVE_INFINITY;
        if (candidate === query) score = 0;
        else if (candidate.startsWith(query)) score = 1;
        else if (candidate.includes(query)) score = 2;
        const match = { score, matchedValue: candidate, alias: index > 0 };
        if (!best || match.score < best.score) best = match;
    });
    return best;
};

const commandHistoryCounts = (history) => {
    const counts = new Map();
    history.forEach((entry) => {
        if (typeof entry !== 'string') return;
        const firstToken = entry.trim().split(/\s+/, 1)[0].replace(/^\//, '').toLowerCase();
        const command = MINECRAFT_COMMANDS.find(({ name, aliases }) => name === firstToken || aliases.includes(firstToken));
        if (command) counts.set(command.name, (counts.get(command.name) ?? 0) + 1);
    });
    return counts;
};

const coordinateToken = (token) => /^(?:[~^](?:-?\d*(?:\.\d+)?)?|-?\d+(?:\.\d+)?)$/.test(token ?? '');

const teleportCandidates = (context) => {
    const completed = context.tokens.slice(1, context.tokenIndex).map((token) => token.toLowerCase());
    if (context.tokenIndex === 1) return teleportDestinations;

    const facingIndex = completed.lastIndexOf('facing');
    if (facingIndex >= 0) {
        const facingArguments = completed.slice(facingIndex + 1);
        if (facingArguments.length === 0) return teleportDestinations;
        if (facingArguments.length === 1 && !coordinateToken(facingArguments[0])) return booleanValues;
        if (facingArguments.length >= 3 && facingArguments.slice(-3).every(coordinateToken)) return booleanValues;
        return undefined;
    }

    if (context.tokenIndex === 2 && completed[0] && !coordinateToken(completed[0])) {
        return teleportDestinations;
    }

    if (completed.length >= 3 && completed.slice(-3).every(coordinateToken)) {
        return teleportContinuations;
    }

    if (completed.length === 2 && completed.every((token) => !coordinateToken(token))) {
        return booleanValues;
    }

    return undefined;
};

const enumCandidates = (context) => {
    if (context.tokenIndex < 1) return undefined;
    const commandToken = context.tokens[0]?.replace(/^\//, '').toLowerCase();
    const command = MINECRAFT_COMMANDS.find(({ name, aliases }) => name === commandToken || aliases.includes(commandToken));
    if (!command) return undefined;
    if (command.name === 'teleport') {
        return { command: command.name, entries: teleportCandidates(context) };
    }
    const commandContexts = contextualValues[command.name];
    if (!commandContexts) return undefined;
    const previousValue = context.tokens[context.tokenIndex - 1]?.toLowerCase();
    return {
        command: command.name,
        entries: commandContexts[`${context.tokenIndex}:${previousValue}`] ?? commandContexts[context.tokenIndex],
    };
};

const safeIdPart = (value) => encodeURIComponent(value).replaceAll('%', '').replace(/[^a-zA-Z0-9_-]/g, '-');

/**
 * Return an alias-aware usage guide for an exact command name.
 *
 * Partial command names intentionally return no guide so ordinary command
 * autocomplete stays compact while the operator is still choosing a command.
 *
 * @param {string} input Current command input.
 * @param {{caret?: number}} [options]
 * @returns {null|{command: string, displayCommand: string, title: string, description: string, variants: ReadonlyArray<{id: string, syntax: string, description: string}>}}
 */
export const getMinecraftCommandGuide = (input, options = {}) => {
    const source = typeof input === 'string' ? input : '';
    const caret = clampCaret(source, options.caret);
    const firstToken = source.slice(0, caret).match(/^\s*(\/?\S+)/)?.[1];
    if (!firstToken) return null;

    const normalized = firstToken.replace(/^\//, '').toLowerCase();
    const command = MINECRAFT_COMMANDS.find(({ name, aliases }) => name === normalized || aliases.includes(normalized));
    const guide = command ? commandGuides[command.name] : undefined;
    if (!guide) return null;

    return {
        command: command.name,
        displayCommand: firstToken,
        title: guide.title,
        description: guide.description,
        variants: guide.variants,
    };
};

/**
 * Return up to six prefix-first command or contextual enum suggestions.
 *
 * `caret` allows completion in the middle of a command. `history` is optional
 * and only breaks otherwise equal command matches.
 *
 * @param {string} input Current command input.
 * @param {{caret?: number, history?: string[], limit?: number}} [options]
 * @returns {Array<{id: string, kind: 'command'|'value', label: string, description: string, command: string, insertText: string, replaceStart: number, replaceEnd: number}>}
 */
export const getMinecraftCommandSuggestions = (input, options = {}) => {
    const source = typeof input === 'string' ? input : '';
    const context = completionContext(source, options.caret);
    const requestedLimit = Number.isInteger(options.limit) ? options.limit : 6;
    const limit = Math.min(Math.max(requestedLimit, 0), 6);
    if (limit === 0) return [];

    if (context.tokenIndex === 0) {
        const slash = context.fragment.startsWith('/') ? '/' : '';
        const query = context.fragment.replace(/^\//, '').toLowerCase();
        const historyCounts = commandHistoryCounts(Array.isArray(options.history) ? options.history : []);

        return MINECRAFT_COMMANDS
            .map((command) => ({ command, match: matchValue(query, command.name, command.aliases) }))
            .filter(({ match }) => Number.isFinite(match.score))
            .sort((left, right) => left.match.score - right.match.score
                || (historyCounts.get(right.command.name) ?? 0) - (historyCounts.get(left.command.name) ?? 0)
                || left.command.rank - right.command.rank
                || left.command.name.localeCompare(right.command.name))
            .slice(0, limit)
            .map(({ command, match }) => {
                const label = match.alias ? match.matchedValue : command.name;
                return {
                    id: `command-${command.name}-${label}`,
                    kind: 'command',
                    label,
                    description: command.description,
                    command: command.name,
                    insertText: `${slash}${label}`,
                    replaceStart: context.replaceStart,
                    replaceEnd: context.replaceEnd,
                };
            });
    }

    const contextual = enumCandidates(context);
    if (!contextual?.entries) return [];
    const query = context.fragment.toLowerCase();
    return contextual.entries
        .map((entry) => ({ entry, match: matchValue(query, entry.value, entry.aliases) }))
        .filter(({ match }) => Number.isFinite(match.score))
        .sort((left, right) => left.match.score - right.match.score
            || left.entry.value.localeCompare(right.entry.value))
        .slice(0, limit)
        .map(({ entry }) => ({
            id: `value-${contextual.command}-${context.tokenIndex}-${safeIdPart(entry.value)}`,
            kind: 'value',
            label: entry.value,
            description: entry.description,
            command: contextual.command,
            insertText: entry.value,
            replaceStart: context.replaceStart,
            replaceEnd: context.replaceEnd,
        }));
};

/**
 * Apply a suggestion returned by `getMinecraftCommandSuggestions`.
 *
 * The returned caret belongs immediately after the inserted text; text before
 * and after the replacement range is preserved unchanged.
 *
 * @param {string} input Current command input.
 * @param {{insertText: string, replaceStart: number, replaceEnd: number}} suggestion
 * @returns {{value: string, caret: number}}
 */
export const applyMinecraftCommandSuggestion = (input, suggestion) => {
    const source = typeof input === 'string' ? input : '';
    if (!suggestion || typeof suggestion.insertText !== 'string') {
        return { value: source, caret: source.length };
    }
    const start = clampCaret(source, suggestion.replaceStart);
    const end = Math.max(start, clampCaret(source, suggestion.replaceEnd));
    const value = `${source.slice(0, start)}${suggestion.insertText}${source.slice(end)}`;
    return { value, caret: start + suggestion.insertText.length };
};
