# Montainer frontend

Montainer's React/Vite frontend is a light, console-first interface for the Go management API. It provides lifecycle and backup controls, searchable live Bedrock logs, and offline Minecraft command assistance.

## Local development

Use Node.js 22 and start the Go backend on `http://127.0.0.1:8000`. Vite proxies the management HTTP routes and `/ws` WebSocket connection to that address.

```bash
npm ci
npm run dev
```

Open the Vite URL shown in the terminal. To change the development backend, update `server.proxy` in `vite.config.js`.

## Validation

```bash
npm run lint
npm test
npm run build
```

Tests use Node's built-in test runner for API compatibility, UI helpers, log filtering, the 82-command Bedrock catalog, contextual suggestions, teleport usage guidance, and caret-safe completion.

The production build is written to `../web/dist`. The root multi-stage Dockerfile builds these assets and copies them into `/app/dist`, where the Go process serves them at `/` or below `SUBPATH_URL`.

## Command assistance

`src/minecraftCommands.js` contains the deterministic offline catalog. It intentionally does not issue hidden `help` commands to the server. Command names, aliases, contextual values, suggestion replacement ranges, and usage guides should remain pure data/logic so they can be covered without a browser.

When adding or changing a guide:

- keep chat-style `/` optional because the dedicated-server console does not require it;
- do not insert placeholder text that could be accidentally submitted;
- keep suggestion IDs valid for `aria-activedescendant`;
- preserve text outside the active caret replacement range; and
- update `src/minecraftCommands.test.js` with both suggestion and application cases.

See the [root README](../README.md) for deployment, API, OpenTelemetry, persistence, and acceptance-test documentation.
