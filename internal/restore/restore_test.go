package restore

import (
	"os"
	"path/filepath"
	"testing"

	"luna/internal/snapshot"
)

func TestRestore_RefuseNonEmpty(t *testing.T) {
	mediaRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(mediaRoot, "items"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	_, err := Restore("/nonexistent/snapshot.tar.gz", RestoreOptions{
		Force:     false,
		MediaRoot: mediaRoot,
	})

	if err == nil {
		t.Error("Expected error for non-empty target without --force")
	}
}

func TestRestore_EmptyTarget(t *testing.T) {
	mediaRoot := t.TempDir()
	snapshotPath := filepath.Join(mediaRoot, "test_snapshot.tar.gz")

	origMediaRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(origMediaRoot, "db"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(origMediaRoot, "items"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(origMediaRoot, "avatars"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	sqlitePath := filepath.Join(origMediaRoot, "db", "app.sqlite")
	if err := os.WriteFile(sqlitePath, []byte("test db"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := snapshot.Snapshot(origMediaRoot, sqlitePath, snapshotPath, snapshot.SnapshotOptions{
		IncludeDB:      true,
		IncludeMeta:    true,
		IncludeAvatars: true,
	})
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	restoreMediaRoot := t.TempDir()
	result, err := Restore(snapshotPath, RestoreOptions{
		Force:     true,
		MediaRoot: restoreMediaRoot,
	})

	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if result.DBFilesRestored == 0 {
		t.Error("Expected DB files to be restored")
	}

	if _, err := os.Stat(filepath.Join(restoreMediaRoot, "db", "app.sqlite")); os.IsNotExist(err) {
		t.Error("Expected DB file to exist after restore")
	}
}

func TestRestore_SkipsExistingFiles(t *testing.T) {
	mediaRoot := t.TempDir()
	snapshotPath := filepath.Join(mediaRoot, "test_snapshot.tar.gz")

	origMediaRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(origMediaRoot, "db"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(origMediaRoot, "items"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	sqlitePath := filepath.Join(origMediaRoot, "db", "app.sqlite")
	if err := os.WriteFile(sqlitePath, []byte("original db"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := snapshot.Snapshot(origMediaRoot, sqlitePath, snapshotPath, snapshot.SnapshotOptions{
		IncludeDB:   true,
		IncludeMeta: true,
	})
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	restoreMediaRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(restoreMediaRoot, "db"), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(restoreMediaRoot, "db", "app.sqlite"), []byte("existing db"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result, err := Restore(snapshotPath, RestoreOptions{
		Force:     true,
		MediaRoot: restoreMediaRoot,
	})

	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if len(result.Warnings) == 0 {
		t.Error("Expected warning about existing file")
	}
}
