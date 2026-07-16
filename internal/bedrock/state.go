// Package bedrock supervises the Minecraft Bedrock child process independently
// from the HTTP transport.
package bedrock

import "time"

// State is the authoritative child-process lifecycle state.
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateFailed   State = "failed"
)

// Snapshot is an immutable point-in-time view of the supervisor.
type Snapshot struct {
	State      State
	PID        int
	Generation uint64
	StartedAt  time.Time
	StoppedAt  time.Time
	Exited     bool
	ExitCode   int
	LastError  string
}

// IsRunning matches the legacy API's boolean status while retaining richer
// internal states for v2 callers.
func (s Snapshot) IsRunning() bool {
	return s.State == StateRunning || s.State == StateStarting || s.State == StateStopping
}
