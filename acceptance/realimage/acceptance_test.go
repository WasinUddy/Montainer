package realimage_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

const (
	acceptanceImageEnv    = "MONTAINER_ACCEPTANCE_IMAGE"
	expectedVersionEnv    = "MONTAINER_EXPECTED_BEDROCK_VERSION"
	defaultCollectorImage = "otel/opentelemetry-collector-contrib:0.156.0@sha256:125bdbeb7590cc1952c5b3430ecf14063568980c2c93d5b38676cc0446ed8108"
	defaultMinIOImage     = "minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
	defaultLegacyImage    = "ghcr.io/wasinuddy/montainer-stable:1.26.33.1@sha256:e8cafa80a9ec6cd226eb9ea66f3177fd7925b56bee0bfc75556d8c0c3305f965"
)

type suiteHarness struct {
	repoRoot        string
	image           string
	expectedVersion string
	collectorImage  string
	minioImage      string
	legacyImage     string
	probeBinary     string
	clientBinary    string
	clientRequired  bool
}

func TestRealImageAcceptance(t *testing.T) {
	image := strings.TrimSpace(os.Getenv(acceptanceImageEnv))
	if image == "" {
		t.Skipf("set %s to run acceptance tests against a packaged Mojang image", acceptanceImageEnv)
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("the Docker image acceptance harness currently targets Linux and macOS")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("%s is set but Docker is unavailable: %v", acceptanceImageEnv, err)
	}
	expectedVersion := strings.TrimSpace(os.Getenv(expectedVersionEnv))
	if expectedVersion == "" {
		t.Fatalf("set %s to the version used to build %s", expectedVersionEnv, image)
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate real-image acceptance source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	harness := &suiteHarness{
		repoRoot:        repoRoot,
		image:           image,
		expectedVersion: expectedVersion,
		collectorImage:  envOrDefault("MONTAINER_OTEL_COLLECTOR_IMAGE", defaultCollectorImage),
		minioImage:      envOrDefault("MONTAINER_MINIO_IMAGE", defaultMinIOImage),
		legacyImage:     envOrDefault("MONTAINER_LEGACY_IMAGE", defaultLegacyImage),
	}
	tags := os.Getenv("GODOG_TAGS")
	harness.clientRequired = tags == "" || strings.Contains(tags, "@client") || strings.Contains(tags, "@upgrade")
	buildDir, err := os.MkdirTemp("", "montainer-real-image-tools-")
	if err != nil {
		t.Fatalf("create real-image tool directory: %v", err)
	}
	defer os.RemoveAll(buildDir)
	harness.probeBinary = filepath.Join(buildDir, "raknet-probe")
	if err := buildLinuxTool(repoRoot, harness.probeBinary, "./test/fixtures/raknetprobe"); err != nil {
		t.Fatal(err)
	}
	if harness.clientRequired {
		harness.clientBinary = filepath.Join(buildDir, "bedrock-client")
		if err := buildNestedLinuxTool(filepath.Join(repoRoot, "test", "fixtures", "bedrockclient"), harness.clientBinary); err != nil {
			t.Fatal(err)
		}
	}

	state := &scenarioState{suite: harness}
	suite := godog.TestSuite{
		Name:                "montainer-real-image-acceptance",
		ScenarioInitializer: state.initializeScenario,
		Options: &godog.Options{
			Format:      "pretty",
			Paths:       []string{filepath.Join(repoRoot, "acceptance", "realimage", "features")},
			Tags:        os.Getenv("GODOG_TAGS"),
			Strict:      true,
			Concurrency: 1,
			TestingT:    t,
		},
	}
	if status := suite.Run(); status != 0 {
		t.Fatalf("real-image acceptance suite failed with status %d", status)
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (h *suiteHarness) validate() error {
	for name, value := range map[string]string{
		"repository root":          h.repoRoot,
		"candidate image":          h.image,
		"expected Bedrock version": h.expectedVersion,
		"Collector image":          h.collectorImage,
		"MinIO image":              h.minioImage,
		"pre-v3 legacy image":      h.legacyImage,
		"RakNet probe":             h.probeBinary,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must not be blank", name)
		}
	}
	if h.clientRequired && strings.TrimSpace(h.clientBinary) == "" {
		return fmt.Errorf("Bedrock client probe must not be blank")
	}
	return nil
}

func buildLinuxTool(repoRoot, output, pkg string) error {
	command := exec.Command("go", "build", "-trimpath", "-o", output, pkg)
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	combined, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build Linux test tool %s: %w\n%s", pkg, err, combined)
	}
	return nil
}

func buildNestedLinuxTool(moduleDir, output string) error {
	command := exec.Command("go", "build", "-trimpath", "-o", output, ".")
	command.Dir = moduleDir
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	combined, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build nested Linux test tool in %s: %w\n%s", moduleDir, err, combined)
	}
	return nil
}
