package logging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultLogFileMaxBytes = 100 * 1024 * 1024
	defaultLogFileBackups  = 5
)

type FileSinkOptions struct {
	MaxBytes   int64
	MaxBackups int
}

// FileSink appends plain log bodies to a local file, preserving compatibility
// with the existing instance.log format while bounding disk usage.
type FileSink struct {
	mu         sync.Mutex
	file       *os.File
	path       string
	size       int64
	maxBytes   int64
	maxBackups int
	closed     bool
}

// NewFileSink opens path in append mode and creates its parent directory.
func NewFileSink(path string) (*FileSink, error) {
	return NewFileSinkWithOptions(path, FileSinkOptions{})
}

func NewFileSinkWithOptions(path string, options FileSinkOptions) (*FileSink, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("log file path must not be blank")
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = defaultLogFileMaxBytes
	}
	if options.MaxBackups <= 0 {
		options.MaxBackups = defaultLogFileBackups
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	file, size, err := openLogFile(path)
	if err != nil {
		return nil, err
	}
	return &FileSink{
		file:       file,
		path:       path,
		size:       size,
		maxBytes:   options.MaxBytes,
		maxBackups: options.MaxBackups,
	}, nil
}

func (s *FileSink) Write(_ context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return os.ErrClosed
	}
	if s.file == nil {
		return fmt.Errorf("log file is unavailable")
	}
	line := strings.TrimRight(record.Body, "\r\n") + "\n"
	if s.size > 0 && s.size+int64(len(line)) > s.maxBytes {
		if err := s.rotateLocked(); err != nil {
			return err
		}
	}
	written, err := s.file.WriteString(line)
	s.size += int64(written)
	return err
}

func (s *FileSink) rotateLocked() (returnErr error) {
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("sync log before rotation: %w", err)
	}
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("close log before rotation: %w", err)
	}
	s.file = nil

	// Always reopen the active path, including after a partial rotation, so a
	// filesystem error does not leave the sink in a nil-file state.
	defer func() {
		file, size, err := openLogFile(s.path)
		if err != nil {
			returnErr = errors.Join(returnErr, err)
			return
		}
		s.file = file
		s.size = size
	}()

	last := rotatedLogPath(s.path, s.maxBackups)
	if err := os.Remove(last); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove oldest rotated log: %w", err)
	}
	for index := s.maxBackups - 1; index >= 1; index-- {
		if err := os.Rename(rotatedLogPath(s.path, index), rotatedLogPath(s.path, index+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("shift rotated log %d: %w", index, err)
		}
	}
	if err := os.Rename(s.path, rotatedLogPath(s.path, 1)); err != nil {
		return fmt.Errorf("rotate active log: %w", err)
	}
	return nil
}

func openLogFile(path string) (*os.File, int64, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, 0, fmt.Errorf("open log file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, fmt.Errorf("inspect log file: %w", err)
	}
	return file, info.Size(), nil
}

func rotatedLogPath(path string, index int) string {
	return path + "." + strconv.Itoa(index)
}

func (s *FileSink) ForceFlush(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if s.file == nil {
		return fmt.Errorf("log file is unavailable")
	}
	return s.file.Sync()
}

func (s *FileSink) Shutdown(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.file == nil {
		return fmt.Errorf("log file is unavailable")
	}
	if err := s.file.Sync(); err != nil {
		_ = s.file.Close()
		return err
	}
	return s.file.Close()
}
