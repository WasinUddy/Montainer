package realimage_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cucumber/godog"
)

const (
	realImageTimeout = 2 * time.Minute
	realImagePoll    = 500 * time.Millisecond
	instanceName     = "real-image-acceptance"
)

var scenarioSequence atomic.Uint64

type requestResult struct {
	status int
	body   []byte
	err    error
}

type backupResult struct {
	Key        string `json:"key"`
	Size       int64  `json:"size"`
	WasRunning bool   `json:"was_running"`
}

type scenarioState struct {
	suite *suiteHarness

	tmpDir      string
	configDir   string
	networkName string
	containers  []string
	candidate   string
	collector   string
	minio       string
	client      string
	baseURL     string
	udpAddress  string
	env         map[string]string
	httpClient  *http.Client

	minioEndpoint string
	minioBucket   string
	minioClient   *s3.Client

	initialGeneration uint64
	concurrentResults []requestResult
	lastBackup        backupResult
	lastAdvertisement bedrockAdvertisement
}

func (s *scenarioState) initializeScenario(ctx *godog.ScenarioContext) {
	ctx.Before(s.beforeScenario)
	ctx.After(s.afterScenario)

	ctx.Step(`^the candidate Montainer image is available$`, s.candidateImageAvailable)
	ctx.Step(`^a real OpenTelemetry Collector is available$`, s.startCollector)
	ctx.Step(`^the configured OpenTelemetry Collector is unavailable$`, s.configureUnavailableCollector)
	ctx.Step(`^log export batching is delayed until shutdown$`, s.delayOTelUntilShutdown)
	ctx.Step(`^S3-compatible MinIO storage is available$`, s.startMinIO)
	ctx.Step(`^I start the candidate with the packaged Bedrock server$`, s.startCandidate)
	ctx.Step(`^the management API eventually becomes healthy$`, s.apiEventuallyHealthy)
	ctx.Step(`^the management API remains healthy$`, s.apiHealthy)
	ctx.Step(`^the packaged Bedrock server eventually reports running$`, s.serverEventuallyRunning)
	ctx.Step(`^the packaged Bedrock server eventually reports stopped$`, s.serverEventuallyStopped)
	ctx.Step(`^the local logs contain the expected Bedrock version$`, s.logsContainExpectedVersion)
	ctx.Step(`^a RakNet client can discover the Bedrock server$`, s.rakNetEventuallyResponds)
	ctx.Step(`^a RakNet client can eventually discover the Bedrock server$`, s.rakNetEventuallyResponds)
	ctx.Step(`^I send the real server command "([^"]*)"$`, s.sendCommand)
	ctx.Step(`^the local logs eventually contain "([^"]*)"$`, s.logsEventuallyContain)
	ctx.Step(`^I request (\d+) stops concurrently$`, s.requestStopsConcurrently)
	ctx.Step(`^I request (\d+) starts concurrently$`, s.requestStartsConcurrently)
	ctx.Step(`^exactly one lifecycle request succeeds and the others conflict$`, s.oneLifecycleRequestSucceeds)
	ctx.Step(`^the process generation increases by (\d+)$`, s.processGenerationIncreasesBy)
	ctx.Step(`^the Collector eventually contains "([^"]*)"$`, s.collectorEventuallyContains)
	ctx.Step(`^the Collector export contains the Montainer service identity$`, s.collectorContainsServiceIdentity)
	ctx.Step(`^I request (\d+) backups concurrently$`, s.requestBackupsConcurrently)
	ctx.Step(`^exactly one backup succeeds and the others conflict$`, s.oneBackupSucceeds)
	ctx.Step(`^the uploaded backup is a valid Montainer archive$`, s.uploadedBackupIsValid)
	ctx.Step(`^I stop the candidate container$`, s.stopCandidate)
	ctx.Step(`^the candidate container exits cleanly$`, s.candidateExitedCleanly)
	ctx.Step(`^the virtual Bedrock player joins$`, s.startVirtualClient)
	ctx.Step(`^the virtual Bedrock player receives the teleport$`, s.virtualClientReceivesTeleport)
}

func (s *scenarioState) beforeScenario(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
	if err := s.prepare(); err != nil {
		return ctx, err
	}
	return ctx, nil
}

func (s *scenarioState) afterScenario(ctx context.Context, scenario *godog.Scenario, scenarioErr error) (context.Context, error) {
	if scenarioErr != nil {
		fmt.Fprintf(os.Stderr, "\n--- real-image diagnostics for %q ---\n", scenario.Name)
		s.printDiagnostics()
		fmt.Fprintf(os.Stderr, "--- scenario workspace ---\n%s\n", s.tmpDir)
	}
	cleanupErr := s.cleanup()
	if scenarioErr != nil {
		return ctx, scenarioErr
	}
	return ctx, cleanupErr
}

func (s *scenarioState) prepare() error {
	if err := s.suite.validate(); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp("", "montainer-real-image-")
	if err != nil {
		return fmt.Errorf("create real-image scenario directory: %w", err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(scenarioSequence.Add(1), 36)
	s.tmpDir = tmpDir
	s.configDir = filepath.Join(tmpDir, "configs")
	s.networkName = "montainer-real-" + suffix
	s.containers = nil
	s.candidate = ""
	s.collector = ""
	s.minio = ""
	s.client = ""
	s.baseURL = ""
	s.udpAddress = ""
	s.httpClient = &http.Client{Timeout: 90 * time.Second}
	s.minioEndpoint = ""
	s.minioBucket = "montainer-real-image"
	s.minioClient = nil
	s.initialGeneration = 0
	s.concurrentResults = nil
	s.lastBackup = backupResult{}
	s.lastAdvertisement = bedrockAdvertisement{}
	s.env = map[string]string{
		"INSTANCE_NAME":                   instanceName,
		"BEDROCK_AUTO_START":              "true",
		"GIN_MODE":                        "release",
		"SHUTDOWN_TIMEOUT":                "30s",
		"LIFECYCLE_TIMEOUT":               "20s",
		"BACKUP_TIMEOUT":                  "3m",
		"LOG_HISTORY_SIZE":                "2000",
		"OTEL_EXPORTER_OTLP_PROTOCOL":     "http/protobuf",
		"OTEL_EXPORTER_OTLP_INSECURE":     "true",
		"OTEL_SERVICE_NAME":               "montainer",
		"OTEL_RESOURCE_ATTRIBUTES":        "service.instance.id=" + instanceName + ",deployment.environment.name=real-image-acceptance",
		"OTEL_BLRP_SCHEDULE_DELAY":        "100",
		"OTEL_BLRP_EXPORT_TIMEOUT":        "2000",
		"OTEL_EXPORTER_OTLP_LOGS_TIMEOUT": "2000",
	}

	if err := os.MkdirAll(s.configDir, 0o777); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.Chmod(s.configDir, 0o777); err != nil {
		return fmt.Errorf("make config directory writable by the container: %w", err)
	}
	fixtures := map[string]string{
		"server.properties": strings.Join([]string{
			"server-name=Montainer Real Image Acceptance",
			"gamemode=creative",
			"force-gamemode=true",
			"difficulty=peaceful",
			"allow-cheats=true",
			"max-players=4",
			"online-mode=false",
			"allow-list=false",
			"server-port=19132",
			"server-portv6=19133",
			"enable-lan-visibility=true",
			"level-name=acceptance-world",
			"view-distance=4",
			"tick-distance=4",
			"player-idle-timeout=0",
		}, "\n") + "\n",
		"allowlist.json":   "[]\n",
		"permissions.json": "[]\n",
	}
	for name, contents := range fixtures {
		if err := os.WriteFile(filepath.Join(s.configDir, name), []byte(contents), 0o666); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		if err := os.Chmod(filepath.Join(s.configDir, name), 0o666); err != nil {
			return fmt.Errorf("make %s writable by the container: %w", name, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := s.docker(ctx, "network", "create", s.networkName); err != nil {
		return err
	}
	return nil
}

func (s *scenarioState) cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var cleanupErr error

	if s.candidate != "" {
		_, _ = s.docker(ctx, "stop", "--time", "45", s.candidate)
	}
	for index := len(s.containers) - 1; index >= 0; index-- {
		if _, err := s.docker(ctx, "rm", "--force", "--volumes", s.containers[index]); err != nil &&
			!strings.Contains(err.Error(), "No such container") && cleanupErr == nil {
			cleanupErr = err
		}
	}
	if s.networkName != "" {
		if _, err := s.docker(ctx, "network", "rm", s.networkName); err != nil && cleanupErr == nil {
			cleanupErr = err
		}
	}
	if s.tmpDir != "" && os.Getenv("MONTAINER_ACCEPTANCE_KEEP_TMP") == "" {
		if err := os.RemoveAll(s.tmpDir); err != nil && cleanupErr == nil {
			cleanupErr = fmt.Errorf("remove scenario directory: %w", err)
		}
	}
	return cleanupErr
}

func (s *scenarioState) printDiagnostics() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, container := range s.containers {
		output, err := s.docker(ctx, "logs", "--tail", "500", container)
		fmt.Fprintf(os.Stderr, "--- docker logs %s ---\n%s\n", container, output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read logs: %v\n", err)
		}
	}
}
