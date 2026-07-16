package bedrock

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	logstream "github.com/wasinuddy/montainer/v2/internal/logging"
)

var fakeBedrockPath string

const (
	supervisorHelperModeEnv = "MONT_SUPERVISOR_TEST_HELPER_MODE"
	tailHelperHead          = "tail-output-head"
	tailHelperLines         = 64
	largeHelperLines        = 20_000
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(supervisorHelperModeEnv); mode != "" {
		os.Exit(runSupervisorTestHelper(mode))
	}
	temporary, err := os.MkdirTemp("", "montainer-fakebedrock-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(temporary)
	_, filename, _, _ := runtime.Caller(0)
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	fakeBedrockPath = filepath.Join(temporary, "fakebedrock")
	command := exec.Command("go", "build", "-o", fakeBedrockPath, filepath.Join(repositoryRoot, "test", "fixtures", "fakebedrock"))
	command.Dir = repositoryRoot
	if output, buildErr := command.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build fake Bedrock: %v\n%s", buildErr, output)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func runSupervisorTestHelper(mode string) int {
	switch mode {
	case "tail":
		fmt.Fprintln(os.Stdout, tailHelperHead)
		continueFile := os.Getenv("MONT_SUPERVISOR_TEST_CONTINUE_FILE")
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(continueFile); err == nil {
				break
			}
			if time.Now().After(deadline) {
				fmt.Fprintln(os.Stderr, "timed out waiting for tail-output continuation")
				return 2
			}
			time.Sleep(time.Millisecond)
		}
		for index := 0; index < tailHelperLines; index++ {
			fmt.Fprintf(os.Stdout, "tail-output-%03d-abcdefghijklmnopqrstuvwxyz\n", index)
		}
		if err := os.WriteFile(os.Getenv("MONT_SUPERVISOR_TEST_WRITTEN_FILE"), []byte("done\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write tail-output marker: %v\n", err)
			return 2
		}
		return 0
	case "large":
		stdout := bufio.NewWriterSize(os.Stdout, 64*1024)
		for index := 0; index < largeHelperLines; index++ {
			fmt.Fprintf(stdout, "large-stdout-%05d-abcdefghijklmnopqrstuvwxyz0123456789\n", index)
		}
		if err := stdout.Flush(); err != nil {
			return 2
		}
		stderr := bufio.NewWriterSize(os.Stderr, 64*1024)
		for index := 0; index < largeHelperLines; index++ {
			fmt.Fprintf(stderr, "large-stderr-%05d-abcdefghijklmnopqrstuvwxyz0123456789\n", index)
		}
		if err := stderr.Flush(); err != nil {
			return 2
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown supervisor test helper mode %q\n", mode)
		return 2
	}
}

type recordCollector struct {
	records chan logstream.Record
	mu      sync.Mutex
	stash   []logstream.Record
}

func newRecordCollector() *recordCollector {
	return &recordCollector{records: make(chan logstream.Record, 512)}
}

func (c *recordCollector) Publish(record logstream.Record) { c.records <- record }

func (c *recordCollector) waitBody(t *testing.T, body string) logstream.Record {
	t.Helper()
	c.mu.Lock()
	for index, record := range c.stash {
		if record.Body == body {
			c.stash = append(c.stash[:index], c.stash[index+1:]...)
			c.mu.Unlock()
			return record
		}
	}
	c.mu.Unlock()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case record := <-c.records:
			if record.Body == body {
				return record
			}
			c.mu.Lock()
			c.stash = append(c.stash, record)
			c.mu.Unlock()
		case <-deadline.C:
			t.Fatalf("timed out waiting for log body %q", body)
		}
	}
}

type outputCollector struct {
	blockBody string
	blocked   chan struct{}
	release   chan struct{}
	blockOnce sync.Once

	mu     sync.Mutex
	counts map[logstream.Stream]int
	last   map[logstream.Stream]string
}

func newOutputCollector(blockBody string) *outputCollector {
	return &outputCollector{
		blockBody: blockBody,
		blocked:   make(chan struct{}),
		release:   make(chan struct{}),
		counts:    make(map[logstream.Stream]int),
		last:      make(map[logstream.Stream]string),
	}
}

func (c *outputCollector) Publish(record logstream.Record) {
	if record.Body == c.blockBody {
		c.blockOnce.Do(func() {
			close(c.blocked)
			<-c.release
		})
	}
	if record.Stream != logstream.StreamStdout && record.Stream != logstream.StreamStderr {
		return
	}
	c.mu.Lock()
	c.counts[record.Stream]++
	c.last[record.Stream] = record.Body
	c.mu.Unlock()
}

func (c *outputCollector) output(stream logstream.Stream) (int, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[stream], c.last[stream]
}

func TestSupervisorStartCommandAndGracefulStop(t *testing.T) {
	workingDir := t.TempDir()
	commandsFile := filepath.Join(workingDir, "commands.log")
	collector := newRecordCollector()
	supervisor := newTestSupervisor(t, workingDir, collector, 500*time.Millisecond,
		"FAKE_BEDROCK_COMMANDS_FILE="+commandsFile,
		"FAKE_BEDROCK_STARTUP_STDERR=stderr-ready",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	started := supervisor.Snapshot()
	if started.State != StateRunning || started.PID <= 0 || started.Generation != 1 {
		t.Fatalf("started snapshot = %+v", started)
	}
	stdout := collector.waitBody(t, "Fake Bedrock server started.")
	if stdout.Stream != logstream.StreamStdout || stdout.Attributes["log.iostream"] != "stdout" {
		t.Fatalf("stdout record = %+v", stdout)
	}
	stderr := collector.waitBody(t, "stderr-ready")
	if stderr.Stream != logstream.StreamStderr || stderr.Attributes["log.iostream"] != "stderr" {
		t.Fatalf("stderr record = %+v", stderr)
	}
	if err := supervisor.SendCommand(ctx, "emit hello-from-command"); err != nil {
		t.Fatalf("SendCommand() error = %v", err)
	}
	collector.waitBody(t, "hello-from-command")
	if err := supervisor.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	stopped := supervisor.Snapshot()
	if stopped.State != StateStopped || !stopped.Exited || stopped.ExitCode != 0 || stopped.PID != 0 {
		t.Fatalf("stopped snapshot = %+v", stopped)
	}
	commands, err := os.ReadFile(commandsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(commands) != "emit hello-from-command\nstop\n" {
		t.Fatalf("commands = %q", commands)
	}
}

func TestSupervisorUnexpectedExitBecomesFailed(t *testing.T) {
	collector := newRecordCollector()
	supervisor := newTestSupervisor(t, t.TempDir(), collector, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	collector.waitBody(t, "Fake Bedrock server started.")
	if err := supervisor.SendCommand(ctx, "crash 23"); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Wait(ctx); !errors.Is(err, ErrUnexpectedExit) {
		t.Fatalf("Wait() error = %v, want ErrUnexpectedExit", err)
	}
	snapshot := supervisor.Snapshot()
	if snapshot.State != StateFailed || !snapshot.Exited || snapshot.ExitCode != 23 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestSupervisorDrainsTailOutputBeforeReapingProcess(t *testing.T) {
	workingDir := t.TempDir()
	continueFile := filepath.Join(workingDir, "continue")
	writtenFile := filepath.Join(workingDir, "written")
	collector := newOutputCollector(tailHelperHead)
	supervisor, err := NewSupervisor(SupervisorConfig{
		Executable:       os.Args[0],
		WorkingDir:       workingDir,
		Environment:      []string{supervisorHelperModeEnv + "=tail", "MONT_SUPERVISOR_TEST_CONTINUE_FILE=" + continueFile, "MONT_SUPERVISOR_TEST_WRITTEN_FILE=" + writtenFile},
		ShutdownTimeout:  time.Second,
		LifecycleTimeout: time.Second,
		Publisher:        collector,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	pid := supervisor.Snapshot().PID
	select {
	case <-collector.blocked:
	case <-ctx.Done():
		t.Fatalf("stdout reader did not reach blocked publisher: %v", ctx.Err())
	}
	if err := os.WriteFile(continueFile, []byte("continue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	awaitFile(t, writtenFile)

	// Give the old Wait-before-drain ordering a deterministic chance to reap
	// the exited helper and close its stdout pipe while Publish remains blocked.
	// With the correct ordering the exited child remains waitable until release.
	awaitProcessReaped(pid, 500*time.Millisecond)
	close(collector.release)

	if err := supervisor.Wait(ctx); !errors.Is(err, ErrUnexpectedExit) {
		t.Fatalf("Wait() error = %v, want ErrUnexpectedExit", err)
	}
	count, last := collector.output(logstream.StreamStdout)
	if count != tailHelperLines+1 {
		t.Fatalf("stdout records = %d, want %d; last = %q", count, tailHelperLines+1, last)
	}
	wantLast := fmt.Sprintf("tail-output-%03d-abcdefghijklmnopqrstuvwxyz", tailHelperLines-1)
	if last != wantLast {
		t.Fatalf("last stdout record = %q, want %q", last, wantLast)
	}
}

func TestSupervisorDrainsLargeOutputWithoutDeadlock(t *testing.T) {
	collector := newOutputCollector("")
	supervisor, err := NewSupervisor(SupervisorConfig{
		Executable:       os.Args[0],
		WorkingDir:       t.TempDir(),
		Environment:      []string{supervisorHelperModeEnv + "=large"},
		ShutdownTimeout:  time.Second,
		LifecycleTimeout: time.Second,
		Publisher:        collector,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Wait(ctx); !errors.Is(err, ErrUnexpectedExit) {
		t.Fatalf("Wait() error = %v, want ErrUnexpectedExit", err)
	}
	stdoutCount, stdoutLast := collector.output(logstream.StreamStdout)
	if stdoutCount != largeHelperLines {
		t.Fatalf("stdout records = %d, want %d; last = %q", stdoutCount, largeHelperLines, stdoutLast)
	}
	stderrCount, stderrLast := collector.output(logstream.StreamStderr)
	if stderrCount != largeHelperLines {
		t.Fatalf("stderr records = %d, want %d; last = %q", stderrCount, largeHelperLines, stderrLast)
	}
	if want := fmt.Sprintf("large-stdout-%05d-abcdefghijklmnopqrstuvwxyz0123456789", largeHelperLines-1); stdoutLast != want {
		t.Fatalf("last stdout record = %q, want %q", stdoutLast, want)
	}
	if want := fmt.Sprintf("large-stderr-%05d-abcdefghijklmnopqrstuvwxyz0123456789", largeHelperLines-1); stderrLast != want {
		t.Fatalf("last stderr record = %q, want %q", stderrLast, want)
	}
}

func TestSupervisorStopEscalatesAfterTimeout(t *testing.T) {
	workingDir := t.TempDir()
	commandsFile := filepath.Join(workingDir, "commands.log")
	collector := newRecordCollector()
	supervisor := newTestSupervisor(t, workingDir, collector, 50*time.Millisecond,
		"FAKE_BEDROCK_IGNORE_STOP=true",
		"FAKE_BEDROCK_COMMANDS_FILE="+commandsFile,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	collector.waitBody(t, "Fake Bedrock server started.")
	if err := supervisor.Stop(ctx); !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("Stop() error = %v, want ErrShutdownTimeout", err)
	}
	if supervisor.Snapshot().State != StateStopped {
		t.Fatalf("state = %s, want stopped", supervisor.Snapshot().State)
	}
	commands, err := os.ReadFile(commandsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(commands)) != "stop" {
		t.Fatalf("commands = %q", commands)
	}
}

func TestSupervisorStopContinuesGracefullyAfterCallerCancellation(t *testing.T) {
	workingDir := t.TempDir()
	commandsFile := filepath.Join(workingDir, "commands.log")
	collector := newRecordCollector()
	supervisor := newTestSupervisor(t, workingDir, collector, time.Second,
		"FAKE_BEDROCK_COMMANDS_FILE="+commandsFile,
		"FAKE_BEDROCK_STOP_DELAY=250ms",
	)
	startCtx, startCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer startCancel()
	if err := supervisor.Start(startCtx); err != nil {
		t.Fatal(err)
	}
	collector.waitBody(t, "Fake Bedrock server started.")

	stopCtx, stopCancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- supervisor.Stop(stopCtx)
	}()
	awaitFile(t, commandsFile)
	stopCancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Stop() error after caller cancellation = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() did not finish after Bedrock's graceful shutdown")
	}
	collector.waitBody(t, "Fake Bedrock server stopped.")
	if snapshot := supervisor.Snapshot(); snapshot.State != StateStopped || snapshot.ExitCode != 0 {
		t.Fatalf("stopped snapshot = %+v", snapshot)
	}
}

func TestConcurrentStartsCreateOnlyOneProcess(t *testing.T) {
	workingDir := t.TempDir()
	startsFile := filepath.Join(workingDir, "starts.log")
	supervisor := newTestSupervisor(t, workingDir, newRecordCollector(), time.Second,
		"FAKE_BEDROCK_STARTS_FILE="+startsFile,
	)
	const callers = 16
	errorsByCaller := make(chan error, callers)
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			errorsByCaller <- supervisor.Start(ctx)
		}()
	}
	wait.Wait()
	close(errorsByCaller)
	successes := 0
	for err := range errorsByCaller {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrAlreadyRunning) {
			t.Errorf("Start() error = %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful starts = %d, want 1", successes)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := supervisor.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	starts, err := os.ReadFile(startsFile)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Fields(string(starts)); len(lines) != 1 {
		t.Fatalf("started child PIDs = %q", starts)
	}
}

func TestExclusiveLeaseBlocksLifecycleAndExpires(t *testing.T) {
	supervisor := newTestSupervisor(t, t.TempDir(), newRecordCollector(), time.Second)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	var escaped *Exclusive
	go func() {
		finished <- supervisor.WithExclusive(context.Background(), func(lease *Exclusive) error {
			escaped = lease
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err := supervisor.Start(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked Start() error = %v", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatalf("WithExclusive() error = %v", err)
	}
	if err := escaped.Start(context.Background()); !errors.Is(err, ErrExclusiveExpired) {
		t.Fatalf("escaped lease Start() error = %v", err)
	}
}

func TestAfterStopRunsAfterProcessIsReaped(t *testing.T) {
	workingDir := t.TempDir()
	pidFile := filepath.Join(workingDir, "pid")
	lifecycle := &reapCheckingLifecycle{pidFile: pidFile}
	supervisor, err := NewSupervisor(SupervisorConfig{
		Executable:       fakeBedrockPath,
		WorkingDir:       workingDir,
		Environment:      []string{"FAKE_BEDROCK_PID_FILE=" + pidFile},
		ShutdownTimeout:  time.Second,
		LifecycleTimeout: time.Second,
		Lifecycle:        lifecycle,
		Publisher:        newRecordCollector(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	awaitFile(t, pidFile)
	if err := supervisor.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if lifecycle.afterErr != nil {
		t.Fatalf("AfterStop observed a live process: %v", lifecycle.afterErr)
	}
}

type reapCheckingLifecycle struct {
	pidFile  string
	afterErr error
}

func (*reapCheckingLifecycle) BeforeStart(context.Context) error { return nil }

func (l *reapCheckingLifecycle) AfterStop(context.Context) error {
	contents, err := os.ReadFile(l.pidFile)
	if err != nil {
		return err
	}
	var pid int
	if _, err := fmt.Sscanf(string(contents), "%d", &pid); err != nil {
		return err
	}
	process, err := os.FindProcess(pid)
	if err == nil {
		err = process.Signal(syscall.Signal(0))
	}
	if err == nil {
		l.afterErr = fmt.Errorf("PID %d still exists", pid)
		return l.afterErr
	}
	return nil
}

func newTestSupervisor(t *testing.T, workingDir string, publisher logstream.Publisher, timeout time.Duration, environment ...string) *Supervisor {
	t.Helper()
	supervisor, err := NewSupervisor(SupervisorConfig{
		Executable:       fakeBedrockPath,
		WorkingDir:       workingDir,
		Environment:      environment,
		ShutdownTimeout:  timeout,
		LifecycleTimeout: time.Second,
		Publisher:        publisher,
	})
	if err != nil {
		t.Fatalf("NewSupervisor() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = supervisor.Shutdown(ctx)
	})
	return supervisor
}

func awaitFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func awaitProcessReaped(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(time.Millisecond)
	}
}
