package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/cucumber/godog"
)

const (
	eventuallyTimeout = 10 * time.Second
	pollInterval      = 50 * time.Millisecond
)

type scenarioState struct {
	suite *suiteHarness

	tmpDir       string
	instanceDir  string
	configDir    string
	resourceDir  string
	logDir       string
	commandsFile string
	startsFile   string
	pidFile      string
	signalsFile  string
	activeFile   string
	overlapsFile string
	gracefulFile string
	listenAddr   string
	baseURL      string
	routePrefix  string
	env          map[string]string

	command     *exec.Cmd
	processDone chan struct{}
	exitMu      sync.Mutex
	exitErr     error
	output      synchronizedBuffer

	collector   *otlpCapture
	websocket   *websocket.Conn
	lastStatus  int
	lastBody    []byte
	lastElapsed time.Duration
}

func (s *scenarioState) initializeScenario(ctx *godog.ScenarioContext) {
	ctx.Before(s.beforeScenario)
	ctx.After(s.afterScenario)

	ctx.Step(`^Montainer uses a controllable fake Bedrock server$`, s.usesFakeBedrock)
	ctx.Step(`^OpenTelemetry log export is disabled$`, s.disableOTel)
	ctx.Step(`^an OTLP HTTP collector is available$`, s.startOTLPCollector)
	ctx.Step(`^the configured OTLP endpoint is unavailable$`, s.configureUnavailableOTLP)
	ctx.Step(`^Montainer is served under the subpath "([^"]*)"$`, s.configureSubpath)
	ctx.Step(`^Montainer starts$`, s.startMontainer)
	ctx.Step(`^the management API eventually becomes healthy$`, s.apiEventuallyHealthy)
	ctx.Step(`^the management API remains healthy$`, s.apiHealthy)
	ctx.Step(`^the server eventually reports "([^"]*)"$`, s.serverEventuallyReports)
	ctx.Step(`^fake Bedrock has started (\d+) times?$`, s.fakeStartedTimes)
	ctx.Step(`^fake Bedrock has eventually started (\d+) times?$`, s.fakeEventuallyStartedTimes)
	ctx.Step(`^I request the server to stop$`, s.requestStop)
	ctx.Step(`^I request the server to start$`, s.requestStart)
	ctx.Step(`^I request the server to restart$`, s.requestRestart)
	ctx.Step(`^I request the server to toggle$`, s.requestToggle)
	ctx.Step(`^the HTTP response status is (\d+)$`, s.responseStatusIs)
	ctx.Step(`^the instance name endpoint returns "([^"]*)"$`, s.instanceNameIs)
	ctx.Step(`^the unprefixed management route "([^"]*)" returns (\d+)$`, s.unprefixedRouteReturns)
	ctx.Step(`^fake Bedrock eventually receives the command "([^"]*)"$`, s.fakeEventuallyReceivedCommand)
	ctx.Step(`^fake Bedrock never overlaps another instance$`, s.fakeNeverOverlaps)
	ctx.Step(`^fake Bedrock takes "([^"]*)" to finish a graceful stop$`, s.fakeTakesToStop)
	ctx.Step(`^I cancel a stop request after fake Bedrock receives it$`, s.cancelStopRequestAfterReceipt)
	ctx.Step(`^fake Bedrock has eventually exited gracefully (\d+) times?$`, s.fakeEventuallyExitedGracefully)
	ctx.Step(`^fake Bedrock received no operating system signal$`, s.fakeReceivedNoSignal)
	ctx.Step(`^fake Bedrock is eventually no longer active$`, s.fakeEventuallyInactive)
	ctx.Step(`^I send the server command "([^"]*)"$`, s.sendServerCommand)
	ctx.Step(`^the local log API eventually contains "([^"]*)"$`, s.localLogsEventuallyContain)
	ctx.Step(`^I am connected to the web log stream$`, s.connectWebLogStream)
	ctx.Step(`^the web log stream eventually contains "([^"]*)"$`, s.webLogStreamEventuallyContains)
	ctx.Step(`^the OTLP collector eventually contains "([^"]*)"$`, s.collectorEventuallyContains)
	ctx.Step(`^the exported log "([^"]*)" has resource attribute "([^"]*)" equal to "([^"]*)"$`, s.exportedResourceAttributeEquals)
	ctx.Step(`^the exported log "([^"]*)" has attribute "([^"]*)" equal to "([^"]*)"$`, s.exportedLogAttributeEquals)
	ctx.Step(`^I terminate Montainer$`, s.terminateMontainer)
	ctx.Step(`^Montainer exits cleanly$`, s.montainerExitsCleanly)
}

func (s *scenarioState) beforeScenario(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
	if err := s.prepare(); err != nil {
		return ctx, err
	}
	return ctx, nil
}

func (s *scenarioState) afterScenario(ctx context.Context, scenario *godog.Scenario, scenarioErr error) (context.Context, error) {
	if scenarioErr != nil {
		fmt.Fprintf(os.Stderr, "\n--- Montainer output for failed scenario %q ---\n%s\n", scenario.Name, s.output.String())
		if s.collector != nil {
			fmt.Fprintf(os.Stderr, "--- OTLP capture diagnostics ---\n%s\n", s.collector.diagnostics())
		}
		fmt.Fprintf(os.Stderr, "--- scenario workspace ---\n%s\n", s.tmpDir)
	}

	cleanupErr := s.cleanup()
	if scenarioErr != nil {
		return ctx, scenarioErr
	}
	return ctx, cleanupErr
}

func (s *scenarioState) prepare() error {
	tmpDir, err := os.MkdirTemp("", "montainer-acceptance-scenario-")
	if err != nil {
		return fmt.Errorf("create scenario directory: %w", err)
	}

	s.tmpDir = tmpDir
	s.instanceDir = filepath.Join(tmpDir, "instance")
	s.configDir = filepath.Join(tmpDir, "configs")
	s.resourceDir = filepath.Join(tmpDir, "resource-packs")
	s.logDir = filepath.Join(tmpDir, "logs")
	s.commandsFile = filepath.Join(tmpDir, "fake-commands.log")
	s.startsFile = filepath.Join(tmpDir, "fake-starts.log")
	s.pidFile = filepath.Join(tmpDir, "fake.pid")
	s.signalsFile = filepath.Join(tmpDir, "fake-signals.log")
	s.activeFile = filepath.Join(tmpDir, "fake-active.pid")
	s.overlapsFile = filepath.Join(tmpDir, "fake-overlaps.log")
	s.gracefulFile = filepath.Join(tmpDir, "fake-graceful-exits.log")
	s.env = make(map[string]string)
	s.command = nil
	s.processDone = nil
	s.exitMu.Lock()
	s.exitErr = nil
	s.exitMu.Unlock()
	s.collector = nil
	s.websocket = nil
	s.lastStatus = 0
	s.lastBody = nil
	s.lastElapsed = 0
	s.output.Reset()

	for _, directory := range []string{
		s.instanceDir,
		filepath.Join(s.instanceDir, "worlds"),
		s.configDir,
		s.resourceDir,
		s.logDir,
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create scenario directory %s: %w", directory, err)
		}
	}

	configFiles := map[string]string{
		"server.properties": "server-name=Montainer Acceptance\nlevel-name=acceptance-world\n",
		"allowlist.json":    "[]\n",
		"permissions.json":  "[]\n",
	}
	for name, contents := range configFiles {
		for _, directory := range []string{s.instanceDir, s.configDir} {
			if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o644); err != nil {
				return fmt.Errorf("write %s fixture: %w", name, err)
			}
		}
	}

	listenAddr, err := unusedLoopbackAddress()
	if err != nil {
		return err
	}
	s.listenAddr = listenAddr
	s.baseURL = "http://" + listenAddr
	s.routePrefix = ""

	s.env = map[string]string{
		"BEDROCK_SERVER_PATH":              s.suite.fakeBin,
		"BEDROCK_EXECUTABLE":               s.suite.fakeBin,
		"BEDROCK_WORK_DIR":                 s.instanceDir,
		"BEDROCK_AUTO_START":               "true",
		"INSTANCE_DIR":                     s.instanceDir,
		"CONFIG_DIR":                       s.configDir,
		"RESOURCE_PACKS_DIR":               s.resourceDir,
		"LOG_DIR":                          s.logDir,
		"LISTEN_ADDR":                      s.listenAddr,
		"HTTP_ADDR":                        s.listenAddr,
		"SUBPATH_URL":                      "/",
		"INSTANCE_NAME":                    "acceptance-instance",
		"GIN_MODE":                         "release",
		"FAKE_BEDROCK_PID_FILE":            s.pidFile,
		"FAKE_BEDROCK_STARTS_FILE":         s.startsFile,
		"FAKE_BEDROCK_COMMANDS_FILE":       s.commandsFile,
		"FAKE_BEDROCK_SIGNALS_FILE":        s.signalsFile,
		"FAKE_BEDROCK_ACTIVE_FILE":         s.activeFile,
		"FAKE_BEDROCK_OVERLAPS_FILE":       s.overlapsFile,
		"FAKE_BEDROCK_GRACEFUL_EXITS_FILE": s.gracefulFile,
		"FAKE_BEDROCK_STOP_DELAY":          "150ms",
		"FAKE_BEDROCK_STARTUP_STDOUT":      "acceptance-bedrock-ready",
		"FAKE_BEDROCK_STARTUP_STDERR":      "",
		"FAKE_BEDROCK_STOPPED_STDOUT":      "acceptance-bedrock-stopped",
		"OTEL_EXPORTER_OTLP_PROTOCOL":      "http/protobuf",
		"OTEL_EXPORTER_OTLP_INSECURE":      "true",
		"OTEL_SERVICE_NAME":                "montainer",
		"OTEL_RESOURCE_ATTRIBUTES":         "service.instance.id=acceptance-instance",
		"OTEL_EXPORTER_OTLP_TIMEOUT":       "1000",
		"OTEL_BSP_EXPORT_TIMEOUT":          "1000",
		"OTEL_BLRP_EXPORT_TIMEOUT":         "1000",
		"OTEL_BLRP_SCHEDULE_DELAY":         "100",
		"AWS_S3_ENDPOINT":                  "",
		"AWS_S3_BUCKET_NAME":               "",
		"AWS_S3_KEY_ID":                    "",
		"AWS_S3_SECRET_KEY":                "",
		"AWS_S3_REGION":                    "",
	}
	s.disableOTel()
	return nil
}

func (s *scenarioState) cleanup() error {
	if s.websocket != nil {
		_ = s.websocket.Close(websocket.StatusNormalClosure, "scenario complete")
		s.websocket = nil
	}

	var cleanupErr error
	if s.processRunning() {
		_ = s.command.Process.Signal(syscall.SIGTERM)
		if _, err := s.waitForExit(3 * time.Second); errors.Is(err, errProcessWaitTimeout) {
			_ = s.command.Process.Kill()
			_, _ = s.waitForExit(2 * time.Second)
		}
	}
	s.killRecordedFakeProcess()

	if s.collector != nil {
		s.collector.close()
		s.collector = nil
	}

	if s.tmpDir != "" && os.Getenv("MONTAINER_ACCEPTANCE_KEEP_TMP") == "" {
		if err := os.RemoveAll(s.tmpDir); err != nil {
			cleanupErr = fmt.Errorf("remove scenario directory: %w", err)
		}
	}
	return cleanupErr
}

func (s *scenarioState) usesFakeBedrock() error {
	if s.env["BEDROCK_SERVER_PATH"] != s.suite.fakeBin {
		return fmt.Errorf("fake Bedrock path is not configured")
	}
	return nil
}

func (s *scenarioState) disableOTel() error {
	delete(s.env, "OTEL_EXPORTER_OTLP_ENDPOINT")
	delete(s.env, "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
	return nil
}

func (s *scenarioState) configureSubpath(prefix string) error {
	if s.command != nil {
		return fmt.Errorf("subpath must be configured before Montainer starts")
	}
	if !strings.HasPrefix(prefix, "/") || prefix == "/" || strings.ContainsAny(prefix, "?#") {
		return fmt.Errorf("subpath must be an absolute non-root URL path, got %q", prefix)
	}
	s.routePrefix = strings.TrimRight(prefix, "/")
	s.env["SUBPATH_URL"] = s.routePrefix
	return nil
}

func (s *scenarioState) startMontainer() error {
	if s.command != nil {
		return fmt.Errorf("Montainer has already been started")
	}

	command := exec.Command(s.suite.montainerBin)
	command.Dir = s.suite.repoRoot
	command.Env = mergeEnvironment(os.Environ(), s.env)
	command.Stdout = &s.output
	command.Stderr = &s.output
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Montainer: %w", err)
	}

	s.command = command
	s.processDone = make(chan struct{})
	go func() {
		err := command.Wait()
		s.exitMu.Lock()
		s.exitErr = err
		s.exitMu.Unlock()
		close(s.processDone)
	}()
	return nil
}

func (s *scenarioState) apiEventuallyHealthy() error {
	return eventually("management API to become healthy", eventuallyTimeout, func() error {
		return s.checkHealth()
	})
}

func (s *scenarioState) apiHealthy() error {
	return s.checkHealth()
}

func (s *scenarioState) checkHealth() error {
	if s.command == nil {
		return fmt.Errorf("Montainer has not started")
	}
	if !s.processRunning() {
		return fmt.Errorf("Montainer exited before becoming healthy: %v\n%s", s.currentExitErr(), s.output.String())
	}
	status, body, _, err := s.httpRequest(http.MethodGet, "/healthz", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("health endpoint returned %d: %s", status, body)
	}
	return nil
}

func (s *scenarioState) serverEventuallyReports(expected string) error {
	wantRunning := strings.EqualFold(expected, "running")
	if !wantRunning && !strings.EqualFold(expected, "stopped") {
		return fmt.Errorf("unsupported server state %q", expected)
	}

	return eventually("server to report "+expected, eventuallyTimeout, func() error {
		status, body, _, err := s.httpRequest(http.MethodGet, "/status", nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("status endpoint returned %d: %s", status, body)
		}
		running, err := runningFromStatus(body)
		if err != nil {
			return err
		}
		if running != wantRunning {
			return fmt.Errorf("is_running is %t", running)
		}
		return nil
	})
}

func (s *scenarioState) fakeStartedTimes(expected int) error {
	actual, err := nonEmptyLineCount(s.startsFile)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("fake Bedrock started %d times, expected %d", actual, expected)
	}
	return nil
}

func (s *scenarioState) fakeEventuallyStartedTimes(expected int) error {
	return eventually(fmt.Sprintf("fake Bedrock to start %d times", expected), eventuallyTimeout, func() error {
		return s.fakeStartedTimes(expected)
	})
}

func (s *scenarioState) requestStop() error {
	return s.storeRequest(http.MethodPost, "/stop", nil)
}

func (s *scenarioState) fakeTakesToStop(rawDuration string) error {
	duration, err := time.ParseDuration(rawDuration)
	if err != nil || duration <= 0 {
		return fmt.Errorf("invalid fake Bedrock stop delay %q", rawDuration)
	}
	if s.command != nil {
		return fmt.Errorf("fake Bedrock stop delay must be configured before Montainer starts")
	}
	s.env["FAKE_BEDROCK_STOP_DELAY"] = duration.String()
	return nil
}

type canceledRequestResult struct {
	status  int
	body    []byte
	elapsed time.Duration
	err     error
}

func (s *scenarioState) cancelStopRequestAfterReceipt() error {
	if !s.processRunning() {
		return fmt.Errorf("Montainer is not running")
	}

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, s.baseURL+s.managementPath("/stop"), nil)
	if err != nil {
		return fmt.Errorf("create cancellable stop request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resultChannel := make(chan canceledRequestResult, 1)
	started := time.Now()
	go func() {
		response, requestErr := client.Do(request)
		result := canceledRequestResult{elapsed: time.Since(started), err: requestErr}
		if response != nil {
			result.status = response.StatusCode
			result.body, _ = io.ReadAll(io.LimitReader(response.Body, 2<<20))
			_ = response.Body.Close()
		}
		resultChannel <- result
	}()

	// The fake records stdin synchronously before beginning its configured
	// delay. Canceling only after that record makes the regression deterministic:
	// the operation has been accepted, but Bedrock is definitely still running.
	if err := s.fakeEventuallyReceivedCommand("stop"); err != nil {
		return err
	}
	cancelRequest()

	select {
	case result := <-resultChannel:
		s.lastStatus = result.status
		s.lastBody = result.body
		s.lastElapsed = result.elapsed
		if result.err == nil {
			return fmt.Errorf("stop request completed before cancellation took effect (HTTP %d: %s)", result.status, result.body)
		}
		if !errors.Is(result.err, context.Canceled) {
			return fmt.Errorf("stop request ended with %v instead of context cancellation", result.err)
		}
		return nil
	case <-time.After(3 * time.Second):
		return fmt.Errorf("canceled stop request did not disconnect within 3 seconds")
	}
}

func (s *scenarioState) requestStart() error {
	return s.storeRequest(http.MethodPost, "/start", nil)
}

func (s *scenarioState) requestRestart() error {
	return s.storeRequest(http.MethodPost, "/restart", nil)
}

func (s *scenarioState) requestToggle() error {
	return s.storeRequest(http.MethodPost, "/toggle", nil)
}

func (s *scenarioState) responseStatusIs(expected int) error {
	if s.lastStatus != expected {
		return fmt.Errorf("HTTP response status was %d, expected %d; body: %s", s.lastStatus, expected, s.lastBody)
	}
	return nil
}

func (s *scenarioState) instanceNameIs(expected string) error {
	status, body, _, err := s.httpRequest(http.MethodGet, "/instance_name", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("instance name endpoint returned %d: %s", status, body)
	}
	var payload struct {
		InstanceName string `json:"instance_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decode instance name response %q: %w", body, err)
	}
	if payload.InstanceName != expected {
		return fmt.Errorf("instance name was %q, expected %q", payload.InstanceName, expected)
	}
	return nil
}

func (s *scenarioState) unprefixedRouteReturns(path string, expected int) error {
	if s.routePrefix == "" {
		return fmt.Errorf("an unprefixed route assertion requires a configured subpath")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("management route must begin with /, got %q", path)
	}
	status, body, _, err := s.rawHTTPRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != expected {
		return fmt.Errorf("unprefixed route %s returned %d, expected %d; body: %s", path, status, expected, body)
	}
	return nil
}

func (s *scenarioState) fakeEventuallyReceivedCommand(expected string) error {
	return eventually("fake Bedrock to receive command "+strconv.Quote(expected), eventuallyTimeout, func() error {
		contents, err := os.ReadFile(s.commandsFile)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no commands recorded yet")
		}
		if err != nil {
			return fmt.Errorf("read fake command record: %w", err)
		}
		for _, command := range strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n") {
			if command == expected {
				return nil
			}
		}
		return fmt.Errorf("command not found in %q", contents)
	})
}

func (s *scenarioState) fakeNeverOverlaps() error {
	contents, err := os.ReadFile(s.overlapsFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read fake Bedrock overlap record: %w", err)
	}
	if strings.TrimSpace(string(contents)) != "" {
		return fmt.Errorf("overlapping fake Bedrock processes were detected: %s", contents)
	}
	return nil
}

func (s *scenarioState) fakeEventuallyExitedGracefully(expected int) error {
	return eventually(fmt.Sprintf("fake Bedrock to exit gracefully %d times", expected), eventuallyTimeout, func() error {
		actual, err := nonEmptyLineCount(s.gracefulFile)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("fake Bedrock exited gracefully %d times, expected %d", actual, expected)
		}
		return nil
	})
}

func (s *scenarioState) fakeReceivedNoSignal() error {
	contents, err := os.ReadFile(s.signalsFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read fake Bedrock signal record: %w", err)
	}
	if strings.TrimSpace(string(contents)) != "" {
		return fmt.Errorf("fake Bedrock received an operating system signal: %s", contents)
	}
	return nil
}

func (s *scenarioState) fakeEventuallyInactive() error {
	return eventually("fake Bedrock active marker to be removed", eventuallyTimeout, func() error {
		_, err := os.Stat(s.activeFile)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect fake Bedrock active marker: %w", err)
		}
		return fmt.Errorf("fake Bedrock active marker still exists")
	})
}

func (s *scenarioState) sendServerCommand(command string) error {
	payload, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return err
	}
	return s.storeRequest(http.MethodPost, "/command", bytes.NewReader(payload))
}

func (s *scenarioState) localLogsEventuallyContain(expected string) error {
	return eventually("local logs to contain "+strconv.Quote(expected), eventuallyTimeout, func() error {
		status, body, _, err := s.httpRequest(http.MethodGet, "/logs?max_lines=500", nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("logs endpoint returned %d: %s", status, body)
		}
		if !bytes.Contains(body, []byte(expected)) {
			return fmt.Errorf("token not present in logs response: %s", body)
		}
		return nil
	})
}

func (s *scenarioState) connectWebLogStream() error {
	websocketURL, err := url.Parse(s.baseURL)
	if err != nil {
		return err
	}
	websocketURL.Scheme = "ws"
	websocketURL.Path = s.managementPath("/ws/stream")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, websocketURL.String(), nil)
	if err != nil {
		if response != nil {
			return fmt.Errorf("connect web log stream (HTTP %d): %w", response.StatusCode, err)
		}
		return fmt.Errorf("connect web log stream: %w", err)
	}
	s.websocket = connection
	return nil
}

func (s *scenarioState) webLogStreamEventuallyContains(expected string) error {
	if s.websocket == nil {
		return fmt.Errorf("web log stream is not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), eventuallyTimeout)
	defer cancel()
	for {
		_, payload, err := s.websocket.Read(ctx)
		if err != nil {
			return fmt.Errorf("read web log stream while waiting for %q: %w", expected, err)
		}
		if bytes.Contains(payload, []byte(expected)) {
			return nil
		}
	}
}

func (s *scenarioState) startOTLPCollector() error {
	if s.collector != nil {
		return fmt.Errorf("OTLP collector already started")
	}
	s.collector = newOTLPCapture()
	s.env["OTEL_EXPORTER_OTLP_ENDPOINT"] = s.collector.endpoint()
	return nil
}

func (s *scenarioState) configureUnavailableOTLP() error {
	endpoint, err := unavailableLoopbackEndpoint()
	if err != nil {
		return err
	}
	s.env["OTEL_EXPORTER_OTLP_ENDPOINT"] = endpoint
	return nil
}

func (s *scenarioState) collectorEventuallyContains(expected string) error {
	if s.collector == nil {
		return fmt.Errorf("OTLP collector is not running")
	}
	return eventually("OTLP collector to contain "+strconv.Quote(expected), eventuallyTimeout, func() error {
		if _, ok := s.collector.find(expected); !ok {
			return fmt.Errorf("log not captured; %s", s.collector.diagnostics())
		}
		return nil
	})
}

func (s *scenarioState) exportedResourceAttributeEquals(body, key, expected string) error {
	log, ok := s.collector.find(body)
	if !ok {
		return fmt.Errorf("exported log %q was not captured", body)
	}
	if actual := log.resourceAttributes[key]; actual != expected {
		return fmt.Errorf("resource attribute %q was %q, expected %q", key, actual, expected)
	}
	return nil
}

func (s *scenarioState) exportedLogAttributeEquals(body, key, expected string) error {
	log, ok := s.collector.find(body)
	if !ok {
		return fmt.Errorf("exported log %q was not captured", body)
	}
	if actual := log.attributes[key]; actual != expected {
		return fmt.Errorf("log attribute %q was %q, expected %q; attributes: %#v", key, actual, expected, log.attributes)
	}
	return nil
}

func (s *scenarioState) terminateMontainer() error {
	if !s.processRunning() {
		return fmt.Errorf("Montainer is not running")
	}
	if err := s.command.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM to Montainer: %w", err)
	}
	return nil
}

func (s *scenarioState) montainerExitsCleanly() error {
	exitErr, waitErr := s.waitForExit(eventuallyTimeout)
	if waitErr != nil {
		return waitErr
	}
	if exitErr != nil {
		return fmt.Errorf("Montainer did not exit cleanly: %v\n%s", exitErr, s.output.String())
	}
	return nil
}

func (s *scenarioState) storeRequest(method, path string, body io.Reader) error {
	status, responseBody, elapsed, err := s.httpRequest(method, path, body)
	s.lastStatus = status
	s.lastBody = responseBody
	s.lastElapsed = elapsed
	return err
}

func (s *scenarioState) httpRequest(method, path string, body io.Reader) (int, []byte, time.Duration, error) {
	return s.rawHTTPRequest(method, s.managementPath(path), body)
}

func (s *scenarioState) rawHTTPRequest(method, path string, body io.Reader) (int, []byte, time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return 0, nil, 0, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	elapsed := time.Since(started)
	if err != nil {
		return 0, nil, elapsed, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return response.StatusCode, nil, elapsed, fmt.Errorf("read %s %s response: %w", method, path, err)
	}
	return response.StatusCode, responseBody, elapsed, nil
}

func (s *scenarioState) managementPath(path string) string {
	if path == "" || path == "/" {
		if s.routePrefix == "" {
			return "/"
		}
		return s.routePrefix + "/"
	}
	return s.routePrefix + "/" + strings.TrimLeft(path, "/")
}

func (s *scenarioState) processRunning() bool {
	if s.command == nil || s.processDone == nil {
		return false
	}
	select {
	case <-s.processDone:
		return false
	default:
		return true
	}
}

func (s *scenarioState) currentExitErr() error {
	s.exitMu.Lock()
	defer s.exitMu.Unlock()
	return s.exitErr
}

var errProcessWaitTimeout = errors.New("timed out waiting for process exit")

func (s *scenarioState) waitForExit(timeout time.Duration) (error, error) {
	if s.processDone == nil {
		return nil, fmt.Errorf("Montainer has not started")
	}
	select {
	case <-s.processDone:
		return s.currentExitErr(), nil
	case <-time.After(timeout):
		return nil, errProcessWaitTimeout
	}
}

func (s *scenarioState) killRecordedFakeProcess() {
	contents, err := os.ReadFile(s.pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil || pid <= 1 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func runningFromStatus(body []byte) (bool, error) {
	var payload struct {
		IsRunning *bool  `json:"is_running"`
		State     string `json:"state"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, fmt.Errorf("decode status response %q: %w", body, err)
	}
	if payload.IsRunning != nil {
		return *payload.IsRunning, nil
	}
	switch strings.ToLower(payload.State) {
	case "running", "starting":
		return true, nil
	case "stopped", "failed", "stopping":
		return false, nil
	default:
		return false, fmt.Errorf("status response has neither is_running nor a recognized state: %s", body)
	}
}

func nonEmptyLineCount(path string) (int, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, nil
}

func unusedLoopbackAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("reserve loopback address: %w", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", fmt.Errorf("release loopback address: %w", err)
	}
	return address, nil
}

func unavailableLoopbackEndpoint() (string, error) {
	address, err := unusedLoopbackAddress()
	if err != nil {
		return "", err
	}
	return "http://" + address, nil
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	merged := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok && !isAcceptanceControlledEnvironment(key) {
			merged[key] = value
		}
	}
	for key, value := range overrides {
		merged[key] = value
	}
	result := make([]string, 0, len(merged))
	for key, value := range merged {
		result = append(result, key+"="+value)
	}
	return result
}

func isAcceptanceControlledEnvironment(key string) bool {
	if strings.HasPrefix(key, "OTEL_") || strings.HasPrefix(key, "FAKE_BEDROCK_") {
		return true
	}
	switch key {
	case "BEDROCK_SERVER_PATH", "BEDROCK_EXECUTABLE", "BEDROCK_WORK_DIR", "BEDROCK_AUTO_START",
		"INSTANCE_DIR", "CONFIG_DIR", "RESOURCE_PACKS_DIR", "LOG_DIR", "LISTEN_ADDR", "HTTP_ADDR",
		"SUBPATH_URL", "INSTANCE_NAME", "GIN_MODE", "AWS_S3_ENDPOINT", "AWS_S3_BUCKET_NAME",
		"AWS_S3_KEY_ID", "AWS_S3_SECRET_KEY", "AWS_S3_REGION":
		return true
	default:
		return false
	}
}

func eventually(description string, timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := check(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s: %w", description, lastErr)
		}
		time.Sleep(pollInterval)
	}
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *synchronizedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buffer.Reset()
}
