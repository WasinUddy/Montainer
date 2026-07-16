package backup

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var defaultConfigFiles = []string{"server.properties", "allowlist.json", "permissions.json"}

type Paths struct {
	InstanceDir string
	ConfigDir   string
	ConfigFiles []string
}

func stageSnapshot(ctx context.Context, destination string, paths Paths) error {
	if strings.TrimSpace(destination) == "" {
		return fmt.Errorf("snapshot destination is required")
	}
	if strings.TrimSpace(paths.InstanceDir) == "" || strings.TrimSpace(paths.ConfigDir) == "" {
		return fmt.Errorf("instance and config directories are required")
	}
	if len(paths.ConfigFiles) == 0 {
		paths.ConfigFiles = defaultConfigFiles
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}

	if err := copyTree(ctx, filepath.Join(paths.InstanceDir, "worlds"), filepath.Join(destination, "worlds")); err != nil {
		return fmt.Errorf("snapshot worlds: %w", err)
	}
	for _, name := range paths.ConfigFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		if filepath.Base(name) != name || name == "." {
			return fmt.Errorf("invalid config filename %q", name)
		}
		if err := copyRegularFile(ctx, filepath.Join(paths.ConfigDir, name), filepath.Join(destination, name)); err != nil {
			return fmt.Errorf("snapshot config %s: %w", name, err)
		}
	}
	return nil
}

func copyTree(ctx context.Context, source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", source)
	}

	return filepath.WalkDir(source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed in backups: %s", current)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		return copyRegularFile(ctx, current, target)
	})
}

func copyRegularFile(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}

	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, contextReader{ctx: ctx, reader: input})
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return closeErr
}

func createZIP(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create backup archive: %w", err)
	}
	archive := zip.NewWriter(output)

	walkErr := filepath.WalkDir(source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if current == source {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed in backups: %s", current)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if entry.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		input, err := os.Open(current)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, contextReader{ctx: ctx, reader: input})
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr == nil {
		walkErr = ctx.Err()
	}

	zipCloseErr := archive.Close()
	fileCloseErr := output.Close()
	if walkErr != nil {
		_ = os.Remove(destination)
		return walkErr
	}
	if zipCloseErr != nil {
		_ = os.Remove(destination)
		return zipCloseErr
	}
	if fileCloseErr != nil {
		_ = os.Remove(destination)
		return fileCloseErr
	}
	return nil
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
