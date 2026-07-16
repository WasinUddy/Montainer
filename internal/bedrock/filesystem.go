package bedrock

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var defaultConfigFiles = []string{"server.properties", "allowlist.json", "permissions.json"}

// FilesystemConfig describes the existing Montainer volume layout.
type FilesystemConfig struct {
	InstanceDir      string
	ConfigDir        string
	ResourcePacksDir string
	ConfigFiles      []string
}

// FilesystemLifecycle synchronizes persisted config files and resource packs
// around a process lifetime.
type FilesystemLifecycle struct {
	cfg FilesystemConfig
}

func NewFilesystemLifecycle(cfg FilesystemConfig) (*FilesystemLifecycle, error) {
	if strings.TrimSpace(cfg.InstanceDir) == "" || strings.TrimSpace(cfg.ConfigDir) == "" {
		return nil, fmt.Errorf("instance and config directories must not be blank")
	}
	if len(cfg.ConfigFiles) == 0 {
		cfg.ConfigFiles = append([]string(nil), defaultConfigFiles...)
	}
	for _, name := range cfg.ConfigFiles {
		if name == "" || name != filepath.Base(name) || name == "." {
			return nil, fmt.Errorf("invalid config filename %q", name)
		}
	}
	return &FilesystemLifecycle{cfg: cfg}, nil
}

// BeforeStart makes the persistent config authoritative when it exists. On a
// first run, defaults shipped with Bedrock are copied out to the config volume.
func (l *FilesystemLifecycle) BeforeStart(ctx context.Context) error {
	if err := os.MkdirAll(l.cfg.InstanceDir, 0o755); err != nil {
		return fmt.Errorf("create instance directory: %w", err)
	}
	if err := os.MkdirAll(l.cfg.ConfigDir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	for _, name := range l.cfg.ConfigFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		instancePath := filepath.Join(l.cfg.InstanceDir, name)
		persistentPath := filepath.Join(l.cfg.ConfigDir, name)
		if _, err := os.Stat(persistentPath); err == nil {
			if err := copyFileAtomic(ctx, instancePath, persistentPath); err != nil {
				return fmt.Errorf("restore config %s: %w", name, err)
			}
		} else if os.IsNotExist(err) {
			if err := copyFileAtomic(ctx, persistentPath, instancePath); err != nil {
				return fmt.Errorf("persist initial config %s: %w", name, err)
			}
		} else {
			return fmt.Errorf("inspect persisted config %s: %w", name, err)
		}
	}
	if strings.TrimSpace(l.cfg.ResourcePacksDir) != "" {
		if err := copyTreeWithoutClobber(ctx, filepath.Join(l.cfg.InstanceDir, "resource_packs"), l.cfg.ResourcePacksDir); err != nil {
			return fmt.Errorf("sync resource packs: %w", err)
		}
	}
	return nil
}

// AfterStop persists configs only after the supervisor has reaped the child.
func (l *FilesystemLifecycle) AfterStop(ctx context.Context) error {
	if err := os.MkdirAll(l.cfg.ConfigDir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	for _, name := range l.cfg.ConfigFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyFileAtomic(
			ctx,
			filepath.Join(l.cfg.ConfigDir, name),
			filepath.Join(l.cfg.InstanceDir, name),
		); err != nil {
			return fmt.Errorf("persist config %s: %w", name, err)
		}
	}
	return nil
}

// copyFileAtomic copies source to destination through a temporary file in the
// destination directory and renames it into place.
func copyFileAtomic(ctx context.Context, destination, source string) (returnErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".montainer-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if returnErr != nil {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := io.Copy(temporary, contextReader{ctx: ctx, reader: sourceFile}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return err
	}
	return nil
}

func copyTreeWithoutClobber(ctx context.Context, destinationRoot, sourceRoot string) error {
	if _, err := os.Stat(sourceRoot); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(sourceRoot, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(sourceRoot, sourcePath)
		if err != nil {
			return err
		}
		destinationPath := filepath.Join(destinationRoot, relative)
		if relative == "." || entry.IsDir() {
			return os.MkdirAll(destinationPath, 0o755)
		}
		if _, err := os.Lstat(destinationPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(sourcePath)
			if err != nil {
				return err
			}
			return os.Symlink(target, destinationPath)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return copyFileAtomic(ctx, destinationPath, sourcePath)
	})
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
