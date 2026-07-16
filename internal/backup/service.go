package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wasinuddy/montainer/v2/internal/bedrock"
	"github.com/wasinuddy/montainer/v2/internal/storage"
)

var (
	ErrNotConfigured = errors.New("backup object storage is not configured")
	ErrInProgress    = errors.New("a backup is already in progress")
)

type Result struct {
	Key        string `json:"key"`
	Size       int64  `json:"size"`
	WasRunning bool   `json:"was_running"`
}

type Service struct {
	supervisor      *bedrock.Supervisor
	store           storage.ObjectStore
	paths           Paths
	instanceName    string
	recoveryTimeout time.Duration
	now             func() time.Time
	operation       sync.Mutex
}

type Options struct {
	Supervisor      *bedrock.Supervisor
	Store           storage.ObjectStore
	Paths           Paths
	InstanceName    string
	RecoveryTimeout time.Duration
}

func NewService(options Options) (*Service, error) {
	if options.Supervisor == nil {
		return nil, fmt.Errorf("backup supervisor is required")
	}
	if strings.TrimSpace(options.Paths.InstanceDir) == "" || strings.TrimSpace(options.Paths.ConfigDir) == "" {
		return nil, fmt.Errorf("backup instance and config directories are required")
	}
	if options.RecoveryTimeout <= 0 {
		options.RecoveryTimeout = 30 * time.Second
	}
	return &Service{
		supervisor:      options.Supervisor,
		store:           options.Store,
		paths:           options.Paths,
		instanceName:    options.InstanceName,
		recoveryTimeout: options.RecoveryTimeout,
		now:             time.Now,
	}, nil
}

func (s *Service) Configured() bool { return s != nil && s.store != nil }

// Save briefly stops a running server under the supervisor's exclusive lease,
// snapshots its files, restores its prior running state, and performs the
// slower compression/upload after releasing the lifecycle lease.
func (s *Service) Save(ctx context.Context) (Result, error) {
	if !s.Configured() {
		return Result{}, ErrNotConfigured
	}
	if !s.operation.TryLock() {
		return Result{}, ErrInProgress
	}
	defer s.operation.Unlock()

	workDir, err := os.MkdirTemp("", "montainer-backup-*")
	if err != nil {
		return Result{}, fmt.Errorf("create backup workspace: %w", err)
	}
	defer os.RemoveAll(workDir)
	stageDir := filepath.Join(workDir, "snapshot")
	archivePath := filepath.Join(workDir, "backup.zip")

	var wasRunning bool
	var restartErr error
	snapshotErr := s.supervisor.WithExclusive(ctx, func(server *bedrock.Exclusive) error {
		snapshot, err := server.Snapshot()
		if err != nil {
			return err
		}
		wasRunning = snapshot.IsRunning()
		if wasRunning {
			if err := server.Stop(ctx); err != nil {
				s.recoverRunningState(server, &restartErr)
				return fmt.Errorf("stop server for backup: %w", err)
			}
		}

		stageErr := stageSnapshot(ctx, stageDir, s.paths)
		if wasRunning {
			s.recoverRunningState(server, &restartErr)
		}
		return stageErr
	})
	if snapshotErr != nil {
		return Result{WasRunning: wasRunning}, errors.Join(snapshotErr, restartErr)
	}

	if err := createZIP(ctx, stageDir, archivePath); err != nil {
		return Result{WasRunning: wasRunning}, errors.Join(err, restartErr)
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return Result{WasRunning: wasRunning}, errors.Join(fmt.Errorf("open backup archive: %w", err), restartErr)
	}
	defer archive.Close()
	info, err := archive.Stat()
	if err != nil {
		return Result{WasRunning: wasRunning}, errors.Join(fmt.Errorf("inspect backup archive: %w", err), restartErr)
	}

	key := fmt.Sprintf("%s_%d_backup.zip", sanitizeName(s.instanceName), s.now().UTC().UnixMilli())
	result := Result{Key: key, Size: info.Size(), WasRunning: wasRunning}
	if err := s.store.Upload(ctx, key, archive, info.Size()); err != nil {
		return result, errors.Join(err, restartErr)
	}
	return result, restartErr
}

func (s *Service) recoverRunningState(server *bedrock.Exclusive, destination *error) {
	recoveryCtx, cancel := context.WithTimeout(context.Background(), s.recoveryTimeout)
	defer cancel()
	if err := server.Start(recoveryCtx); err != nil {
		*destination = fmt.Errorf("restore running state after backup: %w", err)
	}
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Montainer"
	}
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_' || character == '.' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	sanitized := strings.Trim(builder.String(), "._-")
	if sanitized == "" {
		return "Montainer"
	}
	return sanitized
}
