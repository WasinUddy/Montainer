package backup

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestStageSnapshotAndCreateZIPPreserveBackupLayout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	instanceDir := filepath.Join(root, "instance")
	configDir := filepath.Join(root, "configs")
	worldsDir := filepath.Join(instanceDir, "worlds", "Bedrock level")
	mustWriteFile(t, filepath.Join(worldsDir, "level.dat"), "world")
	for _, name := range defaultConfigFiles {
		mustWriteFile(t, filepath.Join(configDir, name), name)
	}

	stage := filepath.Join(root, "stage")
	if err := stageSnapshot(context.Background(), stage, Paths{InstanceDir: instanceDir, ConfigDir: configDir}); err != nil {
		t.Fatalf("stageSnapshot() error = %v", err)
	}
	archivePath := filepath.Join(root, "backup.zip")
	if err := createZIP(context.Background(), stage, archivePath); err != nil {
		t.Fatalf("createZIP() error = %v", err)
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var names []string
	for _, file := range reader.File {
		if !file.FileInfo().IsDir() {
			names = append(names, file.Name)
		}
	}
	sort.Strings(names)
	want := []string{
		"allowlist.json",
		"permissions.json",
		"server.properties",
		"worlds/Bedrock level/level.dat",
	}
	if len(names) != len(want) {
		t.Fatalf("archive files = %v, want %v", names, want)
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("archive files = %v, want %v", names, want)
		}
	}
}

func TestArchiveOperationsHonorCanceledContext(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	mustWriteFile(t, filepath.Join(source, "large.dat"), "content")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := copyRegularFile(ctx, filepath.Join(source, "large.dat"), filepath.Join(root, "copy.dat")); !errors.Is(err, context.Canceled) {
		t.Fatalf("copyRegularFile() error = %v, want context.Canceled", err)
	}
	if err := createZIP(ctx, source, filepath.Join(root, "backup.zip")); !errors.Is(err, context.Canceled) {
		t.Fatalf("createZIP() error = %v, want context.Canceled", err)
	}
}

func TestStageSnapshotRejectsWorldSymlink(t *testing.T) {
	root := t.TempDir()
	instanceDir := filepath.Join(root, "instance")
	configDir := filepath.Join(root, "configs")
	mustWriteFile(t, filepath.Join(root, "secret"), "do not include")
	if err := os.MkdirAll(filepath.Join(instanceDir, "worlds"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "secret"), filepath.Join(instanceDir, "worlds", "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, name := range defaultConfigFiles {
		mustWriteFile(t, filepath.Join(configDir, name), name)
	}

	err := stageSnapshot(context.Background(), filepath.Join(root, "stage"), Paths{InstanceDir: instanceDir, ConfigDir: configDir})
	if err == nil {
		t.Fatal("stageSnapshot() unexpectedly accepted a symlink")
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o640); err != nil {
		t.Fatal(err)
	}
}
