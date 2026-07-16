package bedrock

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	logstream "github.com/wasinuddy/montainer/v2/internal/logging"
)

var (
	ErrAlreadyRunning    = errors.New("bedrock server is already running")
	ErrNotRunning        = errors.New("bedrock server is not running")
	ErrInvalidCommand    = errors.New("bedrock command must be one non-empty line")
	ErrShutdownTimeout   = errors.New("bedrock server did not stop before the shutdown timeout")
	ErrUnexpectedExit    = errors.New("bedrock server exited unexpectedly")
	ErrExclusiveExpired  = errors.New("exclusive supervisor lease has expired")
	ErrExclusiveCallback = errors.New("exclusive supervisor callback must not be nil")
)

// Lifecycle runs filesystem synchronization around a confirmed process
// lifetime. AfterStop is never invoked until exec.Cmd.Wait has returned.
type Lifecycle interface {
	BeforeStart(context.Context) error
	AfterStop(context.Context) error
}

// SupervisorConfig describes the Bedrock process and its lifecycle behavior.
type SupervisorConfig struct {
	Executable       string
	WorkingDir       string
	Args             []string
	Environment      []string
	ShutdownTimeout  time.Duration
	LifecycleTimeout time.Duration
	Publisher        logstream.Publisher
	Lifecycle        Lifecycle
}

type Supervisor struct {
	cfg SupervisorConfig

	// operation is a context-aware mutex shared by every public mutation and
	// by backup maintenance windows.
	operation  chan struct{}
	writeMu    sync.Mutex
	mu         sync.RWMutex
	snapshot   Snapshot
	process    *managedProcess
	lastResult error
}

type managedProcess struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	done       chan struct{}
	readers    sync.WaitGroup
	generation uint64

	result       error
	expectedStop bool
}

// NewSupervisor validates configuration and creates a stopped supervisor.
func NewSupervisor(cfg SupervisorConfig) (*Supervisor, error) {
	if strings.TrimSpace(cfg.Executable) == "" {
		return nil, fmt.Errorf("bedrock executable must not be blank")
	}
	if strings.TrimSpace(cfg.WorkingDir) == "" {
		return nil, fmt.Errorf("bedrock working directory must not be blank")
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 15 * time.Second
	}
	if cfg.LifecycleTimeout <= 0 {
		cfg.LifecycleTimeout = 15 * time.Second
	}
	s := &Supervisor{
		cfg:       cfg,
		operation: make(chan struct{}, 1),
		snapshot: Snapshot{
			State: StateStopped,
		},
	}
	s.operation <- struct{}{}
	return s, nil
}

// Start launches Bedrock. It does not bind the child lifetime to ctx; ctx only
// bounds acquiring the operation lease and pre-start lifecycle work.
func (s *Supervisor) Start(ctx context.Context) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	return s.start(ctx)
}

// Stop sends Bedrock's stop command, waits for confirmed exit, and escalates
// to a process kill after ShutdownTimeout. Once the command has been written,
// caller cancellation no longer interrupts the confirmed-stop sequence.
func (s *Supervisor) Stop(ctx context.Context) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	return s.stop(ctx, false)
}

// ForceStop kills the active child and waits for the process to be reaped.
func (s *Supervisor) ForceStop(ctx context.Context) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	return s.stop(ctx, true)
}

// Restart performs a confirmed graceful stop followed by a new start under a
// single operation lease, so no other lifecycle operation can interleave.
func (s *Supervisor) Restart(ctx context.Context) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if s.active() {
		if err := s.stop(ctx, false); err != nil {
			return err
		}
	}
	return s.start(ctx)
}

// SendCommand writes one command atomically. A stop command is routed through
// Stop so state and post-stop synchronization remain correct.
func (s *Supervisor) SendCommand(ctx context.Context, command string) error {
	if strings.EqualFold(strings.TrimSpace(command), "stop") {
		return s.Stop(ctx)
	}
	if err := validateCommand(command); err != nil {
		return err
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	return s.sendCommand(ctx, command)
}

// Shutdown is idempotent and intended for application signal handling.
func (s *Supervisor) Shutdown(ctx context.Context) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if !s.active() {
		return nil
	}
	return s.stop(ctx, false)
}

// Wait waits for the current process to exit and returns its terminal result.
// An unexpected clean exit returns ErrUnexpectedExit.
func (s *Supervisor) Wait(ctx context.Context) error {
	s.mu.RLock()
	process := s.process
	state := s.snapshot.State
	lastResult := s.lastResult
	s.mu.RUnlock()
	if process == nil {
		if state == StateFailed {
			return lastResult
		}
		return nil
	}
	select {
	case <-process.done:
		return process.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Snapshot returns the authoritative state derived from the actual child.
func (s *Supervisor) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

// Exclusive is valid only for the duration of a WithExclusive callback.
// Backup services use it to keep stop -> snapshot -> optional restart atomic
// relative to API lifecycle calls.
type Exclusive struct {
	s      *Supervisor
	active atomic.Bool
}

// WithExclusive runs callback while holding the same context-aware lease used
// by Start, Stop, Restart, ForceStop, and SendCommand.
func (s *Supervisor) WithExclusive(ctx context.Context, callback func(*Exclusive) error) error {
	if callback == nil {
		return ErrExclusiveCallback
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	lease := &Exclusive{s: s}
	lease.active.Store(true)
	defer lease.active.Store(false)
	return callback(lease)
}

func (e *Exclusive) Start(ctx context.Context) error {
	if err := e.valid(); err != nil {
		return err
	}
	return e.s.start(ctx)
}

func (e *Exclusive) Stop(ctx context.Context) error {
	if err := e.valid(); err != nil {
		return err
	}
	return e.s.stop(ctx, false)
}

func (e *Exclusive) ForceStop(ctx context.Context) error {
	if err := e.valid(); err != nil {
		return err
	}
	return e.s.stop(ctx, true)
}

func (e *Exclusive) Restart(ctx context.Context) error {
	if err := e.valid(); err != nil {
		return err
	}
	if e.s.active() {
		if err := e.s.stop(ctx, false); err != nil {
			return err
		}
	}
	return e.s.start(ctx)
}

func (e *Exclusive) SendCommand(ctx context.Context, command string) error {
	if err := e.valid(); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(command), "stop") {
		return e.s.stop(ctx, false)
	}
	if err := validateCommand(command); err != nil {
		return err
	}
	return e.s.sendCommand(ctx, command)
}

func (e *Exclusive) Snapshot() (Snapshot, error) {
	if err := e.valid(); err != nil {
		return Snapshot{}, err
	}
	return e.s.Snapshot(), nil
}

func (e *Exclusive) valid() error {
	if e == nil || e.s == nil || !e.active.Load() {
		return ErrExclusiveExpired
	}
	return nil
}

func (s *Supervisor) start(ctx context.Context) error {
	s.mu.Lock()
	if s.process != nil || s.snapshot.State == StateRunning || s.snapshot.State == StateStarting || s.snapshot.State == StateStopping {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	s.snapshot.State = StateStarting
	s.snapshot.LastError = ""
	s.snapshot.Exited = false
	s.lastResult = nil
	snapshot := s.snapshot
	s.mu.Unlock()
	s.publishState(snapshot, "")

	if s.cfg.Lifecycle != nil {
		if err := s.runLifecycle(ctx, s.cfg.Lifecycle.BeforeStart); err != nil {
			s.failStart(fmt.Errorf("prepare bedrock server: %w", err))
			return fmt.Errorf("prepare bedrock server: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		s.failStart(err)
		return err
	}

	cmd := exec.Command(s.cfg.Executable, s.cfg.Args...)
	cmd.Dir = s.cfg.WorkingDir
	cmd.Env = mergeEnvironment(os.Environ(), s.cfg.Environment)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.failStart(fmt.Errorf("open bedrock stdout: %w", err))
		return fmt.Errorf("open bedrock stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		s.failStart(fmt.Errorf("open bedrock stderr: %w", err))
		return fmt.Errorf("open bedrock stderr: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		s.failStart(fmt.Errorf("open bedrock stdin: %w", err))
		return fmt.Errorf("open bedrock stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		s.failStart(fmt.Errorf("start bedrock server: %w", err))
		return fmt.Errorf("start bedrock server: %w", err)
	}

	s.mu.Lock()
	generation := s.snapshot.Generation + 1
	process := &managedProcess{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		done:       make(chan struct{}),
		generation: generation,
	}
	s.process = process
	s.snapshot.State = StateRunning
	s.snapshot.PID = cmd.Process.Pid
	s.snapshot.Generation = generation
	s.snapshot.StartedAt = time.Now().UTC()
	s.snapshot.StoppedAt = time.Time{}
	s.snapshot.Exited = false
	s.snapshot.ExitCode = 0
	s.snapshot.LastError = ""
	snapshot = s.snapshot
	s.mu.Unlock()
	s.publishState(snapshot, "")

	process.readers.Add(2)
	go s.readPipe(process, stdout, logstream.StreamStdout)
	go s.readPipe(process, stderr, logstream.StreamStderr)
	go s.waitProcess(process)
	return nil
}

func (s *Supervisor) stop(ctx context.Context, force bool) error {
	s.mu.Lock()
	process := s.process
	if process == nil {
		s.mu.Unlock()
		return ErrNotRunning
	}
	process.expectedStop = true
	s.snapshot.State = StateStopping
	snapshot := s.snapshot
	s.mu.Unlock()
	s.publishState(snapshot, "")

	if force {
		if err := process.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill bedrock server: %w", err)
		}
		// Once a kill has been issued, do not release the lifecycle lease until
		// Wait has reaped the child. This preserves the one-process invariant
		// even if the request context is cancelled at that exact moment.
		<-process.done
		return process.result
	}

	if err := s.writeLine(ctx, process, "stop", StateStopping); err != nil {
		_ = process.cmd.Process.Kill()
		<-process.done
		return fmt.Errorf("send stop command: %w", err)
	}

	timer := time.NewTimer(s.cfg.ShutdownTimeout)
	defer timer.Stop()
	select {
	case <-process.done:
		return process.result
	case <-timer.C:
		killErr := process.cmd.Process.Kill()
		<-process.done
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(ErrShutdownTimeout, fmt.Errorf("kill bedrock server: %w", killErr))
		}
		return ErrShutdownTimeout
	}
}

func (s *Supervisor) sendCommand(ctx context.Context, command string) error {
	s.mu.RLock()
	process := s.process
	state := s.snapshot.State
	s.mu.RUnlock()
	if process == nil || state != StateRunning {
		return ErrNotRunning
	}
	return s.writeLine(ctx, process, command, StateRunning)
}

func (s *Supervisor) writeLine(ctx context.Context, process *managedProcess, command string, requiredState State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.RLock()
	current := s.process
	state := s.snapshot.State
	s.mu.RUnlock()
	if current != process || state != requiredState {
		return ErrNotRunning
	}
	_, err := io.WriteString(process.stdin, command+"\n")
	if err != nil {
		return fmt.Errorf("write bedrock stdin: %w", err)
	}
	return nil
}

func (s *Supervisor) waitProcess(process *managedProcess) {
	// StdoutPipe and StderrPipe require their readers to reach EOF before Wait:
	// Wait closes the read ends after the child exits and can otherwise discard
	// output that is still buffered in the operating-system pipes.
	process.readers.Wait()
	waitErr := process.cmd.Wait()
	_ = process.stdin.Close()

	var lifecycleErr error
	if s.cfg.Lifecycle != nil {
		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.LifecycleTimeout)
		lifecycleErr = s.cfg.Lifecycle.AfterStop(ctx)
		cancel()
		if lifecycleErr != nil {
			lifecycleErr = fmt.Errorf("sync bedrock data after stop: %w", lifecycleErr)
		}
	}

	s.mu.Lock()
	expected := process.expectedStop
	if s.process == process {
		s.process = nil
		s.snapshot.PID = 0
		s.snapshot.StoppedAt = time.Now().UTC()
		s.snapshot.Exited = true
		s.snapshot.ExitCode = exitCode(process.cmd.ProcessState)
		switch {
		case lifecycleErr != nil:
			process.result = lifecycleErr
			s.lastResult = process.result
			s.snapshot.State = StateFailed
			s.snapshot.LastError = lifecycleErr.Error()
		case expected:
			process.result = nil
			s.lastResult = nil
			s.snapshot.State = StateStopped
			s.snapshot.LastError = ""
		case waitErr != nil:
			process.result = errors.Join(ErrUnexpectedExit, waitErr)
			s.lastResult = process.result
			s.snapshot.State = StateFailed
			s.snapshot.LastError = process.result.Error()
		default:
			process.result = ErrUnexpectedExit
			s.lastResult = process.result
			s.snapshot.State = StateFailed
			s.snapshot.LastError = ErrUnexpectedExit.Error()
		}
	}
	snapshot := s.snapshot
	s.mu.Unlock()
	s.publishState(snapshot, snapshot.LastError)
	close(process.done)
}

func (s *Supervisor) readPipe(process *managedProcess, reader io.Reader, stream logstream.Stream) {
	defer process.readers.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		timestamp := time.Now().UTC()
		record := logstream.NewRecord(timestamp, stream, scanner.Text())
		record.Attributes["minecraft.server.state"] = string(StateRunning)
		record.Attributes["process.pid"] = strconv.Itoa(process.cmd.Process.Pid)
		record.Attributes["minecraft.server.generation"] = strconv.FormatUint(process.generation, 10)
		s.publish(record)
	}
	if err := scanner.Err(); err != nil {
		record := logstream.NewRecord(time.Now().UTC(), logstream.StreamSystem, "failed reading bedrock "+string(stream)+": "+err.Error())
		record.Attributes["minecraft.server.state"] = string(s.Snapshot().State)
		s.publish(record)
	}
}

func (s *Supervisor) failStart(err error) {
	s.mu.Lock()
	s.snapshot.State = StateFailed
	s.snapshot.PID = 0
	s.snapshot.Exited = false
	s.snapshot.LastError = err.Error()
	s.lastResult = err
	snapshot := s.snapshot
	s.mu.Unlock()
	s.publishState(snapshot, err.Error())
}

func (s *Supervisor) publishState(snapshot Snapshot, stateError string) {
	record := logstream.NewRecord(
		time.Now().UTC(),
		logstream.StreamSystem,
		"bedrock server state changed to "+string(snapshot.State),
	)
	record.Attributes["minecraft.server.state"] = string(snapshot.State)
	record.Attributes["minecraft.server.generation"] = strconv.FormatUint(snapshot.Generation, 10)
	if snapshot.PID != 0 {
		record.Attributes["process.pid"] = strconv.Itoa(snapshot.PID)
	}
	if stateError != "" {
		record.Attributes["error.type"] = "bedrock.lifecycle"
		record.Attributes["error.message"] = stateError
	}
	s.publish(record)
}

func (s *Supervisor) publish(record logstream.Record) {
	if s.cfg.Publisher != nil {
		s.cfg.Publisher.Publish(record)
	}
}

func (s *Supervisor) active() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.process != nil
}

func (s *Supervisor) runLifecycle(ctx context.Context, fn func(context.Context) error) error {
	lifecycleCtx, cancel := context.WithTimeout(ctx, s.cfg.LifecycleTimeout)
	defer cancel()
	return fn(lifecycleCtx)
}

func (s *Supervisor) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.operation:
		return nil
	}
}

func (s *Supervisor) release() { s.operation <- struct{}{} }

func validateCommand(command string) error {
	if strings.TrimSpace(command) == "" || strings.ContainsAny(command, "\r\n") {
		return ErrInvalidCommand
	}
	return nil
}

func exitCode(state *os.ProcessState) int {
	if state == nil {
		return -1
	}
	return state.ExitCode()
}

func mergeEnvironment(base, overrides []string) []string {
	positions := make(map[string]int, len(base)+len(overrides))
	merged := make([]string, 0, len(base)+len(overrides))
	for _, entry := range append(append([]string(nil), base...), overrides...) {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		if position, exists := positions[key]; exists {
			merged[position] = entry
			continue
		}
		positions[key] = len(merged)
		merged = append(merged, entry)
	}
	return merged
}
