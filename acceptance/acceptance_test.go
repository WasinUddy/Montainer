package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cucumber/godog"
)

const acceptanceBinaryEnv = "MONTAINER_ACCEPTANCE_BINARY"

type suiteHarness struct {
	repoRoot     string
	buildDir     string
	montainerBin string
	fakeBin      string
}

func TestAcceptance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Bedrock process acceptance fixture currently targets Linux and macOS")
	}

	harness, err := buildSuiteHarness()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(harness.buildDir)

	state := &scenarioState{suite: harness}
	suite := godog.TestSuite{
		Name:                "montainer-v2-acceptance",
		ScenarioInitializer: state.initializeScenario,
		Options: &godog.Options{
			Format:      "pretty",
			Paths:       []string{filepath.Join(harness.repoRoot, "acceptance", "features")},
			Tags:        os.Getenv("GODOG_TAGS"),
			Strict:      true,
			Concurrency: 1,
			TestingT:    t,
		},
	}

	if status := suite.Run(); status != 0 {
		t.Fatalf("acceptance suite failed with status %d", status)
	}
}

func buildSuiteHarness() (*suiteHarness, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("locate acceptance test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), ".."))
	buildDir, err := os.MkdirTemp("", "montainer-acceptance-build-")
	if err != nil {
		return nil, fmt.Errorf("create acceptance build directory: %w", err)
	}

	harness := &suiteHarness{
		repoRoot: repoRoot,
		buildDir: buildDir,
		fakeBin:  filepath.Join(buildDir, "fake-bedrock"),
	}

	if configured := os.Getenv(acceptanceBinaryEnv); configured != "" {
		harness.montainerBin, err = filepath.Abs(configured)
		if err != nil {
			os.RemoveAll(buildDir)
			return nil, fmt.Errorf("resolve %s: %w", acceptanceBinaryEnv, err)
		}
		if info, statErr := os.Stat(harness.montainerBin); statErr != nil || info.IsDir() {
			os.RemoveAll(buildDir)
			return nil, fmt.Errorf("%s does not identify an executable file: %s", acceptanceBinaryEnv, harness.montainerBin)
		}
	} else {
		harness.montainerBin = filepath.Join(buildDir, "montainer")
		if err := goBuild(repoRoot, harness.montainerBin, "./cmd/montainer"); err != nil {
			os.RemoveAll(buildDir)
			return nil, err
		}
	}

	if err := goBuild(repoRoot, harness.fakeBin, "./test/fixtures/fakebedrock"); err != nil {
		os.RemoveAll(buildDir)
		return nil, err
	}
	return harness, nil
}

func goBuild(repoRoot, output, pkg string) error {
	command := exec.Command("go", "build", "-o", output, pkg)
	command.Dir = repoRoot
	combined, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build %s: %w\n%s", pkg, err, combined)
	}
	return nil
}
