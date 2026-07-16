# Montainer v2 acceptance tests

This suite treats Montainer as a black box. It builds the real
`cmd/montainer` executable, launches it with isolated directories, and replaces
only the Mojang executable with `test/fixtures/fakebedrock`. The same HTTP,
WebSocket, process, and OTLP boundaries used in production are exercised.

## Run the suite

From the repository root:

```bash
go test -v -count=1 ./acceptance
```

Run one business area by selecting its Gherkin tag:

```bash
GODOG_TAGS='@lifecycle' go test -v -count=1 ./acceptance
GODOG_TAGS='@logging' go test -v -count=1 ./acceptance
GODOG_TAGS='@otel' go test -v -count=1 ./acceptance
```

The suite normally builds `./cmd/montainer`. To exercise an already-built
artifact (for example, the exact CI release binary), set:

```bash
MONTAINER_ACCEPTANCE_BINARY=/absolute/path/to/montainer \
  go test -v -count=1 ./acceptance
```

Set `MONTAINER_ACCEPTANCE_KEEP_TMP=1` to preserve per-scenario instance,
configuration, process-recording, and log files for debugging. Failed scenarios
also print the Montainer child-process output and OTLP capture diagnostics.

## Covered behavior

- automatic startup, stop/start, graceful restart, and unexpected process exit;
- completion of an accepted graceful stop after its HTTP client disconnects;
- subpath-scoped health, status, instance, lifecycle, command, and WebSocket routes;
- local log retrieval and the WebSocket stream with no collector configured;
- stdout and stderr ingestion;
- real OTLP/HTTP protobuf export and resource/log attributes;
- continued local operation while an OTLP endpoint is unavailable; and
- exporter flushing during graceful application shutdown.

The fake Bedrock process records every start, stdin command, OS signal, and
completed graceful exit. It also accepts test-only commands: `emit TEXT` writes
to stdout, `emiterr TEXT` writes to stderr, `crash CODE` exits unexpectedly,
and `stop` performs a graceful exit.
Additional timing and failure behavior can be configured with the
`FAKE_BEDROCK_*` environment variables documented in its source.
