package bedrock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemLifecyclePreservesVolumeContract(t *testing.T) {
	root := t.TempDir()
	instanceDir := filepath.Join(root, "instance")
	configDir := filepath.Join(root, "configs")
	packsDir := filepath.Join(root, "mounted-packs")
	mustWriteFile(t, filepath.Join(instanceDir, "server.properties"), "shipped-default")
	mustWriteFile(t, filepath.Join(instanceDir, "allowlist.json"), "shipped-allowlist")
	mustWriteFile(t, filepath.Join(configDir, "allowlist.json"), "persisted-allowlist")
	mustWriteFile(t, filepath.Join(packsDir, "new", "manifest.json"), "new-pack")
	mustWriteFile(t, filepath.Join(packsDir, "existing", "manifest.json"), "mounted-version")
	mustWriteFile(t, filepath.Join(instanceDir, "resource_packs", "existing", "manifest.json"), "instance-version")

	lifecycle, err := NewFilesystemLifecycle(FilesystemConfig{
		InstanceDir:      instanceDir,
		ConfigDir:        configDir,
		ResourcePacksDir: packsDir,
		ConfigFiles:      []string{"server.properties", "allowlist.json"},
	})
	if err != nil {
		t.Fatalf("NewFilesystemLifecycle() error = %v", err)
	}
	if err := lifecycle.BeforeStart(context.Background()); err != nil {
		t.Fatalf("BeforeStart() error = %v", err)
	}
	assertFile(t, filepath.Join(configDir, "server.properties"), "shipped-default")
	assertFile(t, filepath.Join(instanceDir, "allowlist.json"), "persisted-allowlist")
	assertFile(t, filepath.Join(instanceDir, "resource_packs", "new", "manifest.json"), "new-pack")
	assertFile(t, filepath.Join(instanceDir, "resource_packs", "existing", "manifest.json"), "instance-version")

	mustWriteFile(t, filepath.Join(instanceDir, "server.properties"), "changed-by-server")
	mustWriteFile(t, filepath.Join(instanceDir, "allowlist.json"), "changed-allowlist")
	if err := lifecycle.AfterStop(context.Background()); err != nil {
		t.Fatalf("AfterStop() error = %v", err)
	}
	assertFile(t, filepath.Join(configDir, "server.properties"), "changed-by-server")
	assertFile(t, filepath.Join(configDir, "allowlist.json"), "changed-allowlist")
}

func TestCopyFileAtomicHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	mustWriteFile(t, source, "content")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := copyFileAtomic(ctx, destination, source)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("copyFileAtomic() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination exists after canceled copy: %v", err)
	}
}

func TestFilesystemLifecycleRejectsNestedConfigName(t *testing.T) {
	_, err := NewFilesystemLifecycle(FilesystemConfig{
		InstanceDir: "instance",
		ConfigDir:   "configs",
		ConfigFiles: []string{"../secret"},
	})
	if err == nil {
		t.Fatal("NewFilesystemLifecycle() succeeded with traversal filename")
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if string(contents) != want {
		t.Fatalf("%s = %q, want %q", path, contents, want)
	}
}
