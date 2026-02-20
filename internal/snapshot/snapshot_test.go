package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"luna/internal/meta"
	"luna/internal/storage"
)

func TestSnapshot_DefaultIncludes(t *testing.T) {
	mediaRoot := t.TempDir()
	sqlitePath := filepath.Join(mediaRoot, "db", "test.sqlite")
	snapshotPath := filepath.Join(mediaRoot, "backup.tar.gz")

	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout() error = %v", err)
	}

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if err := storage.EnsureItemDirs(mediaRoot, itemID); err != nil {
		t.Fatalf("EnsureItemDirs() error = %v", err)
	}

	itemMeta := &meta.ItemMeta{
		Schema:        meta.SchemaVersion,
		ItemID:        itemID,
		Type:          "video",
		OwnerUsername: "testuser",
		Title:         "Test Video",
		CreatedAt:     "2024-01-15T10:30:00Z",
		Original: meta.OriginalFile{
			Filename: "video.mov",
			Path:     "original/upload.mov",
		},
	}

	if err := meta.WriteItemMetaAtomic(mediaRoot, itemID, itemMeta); err != nil {
		t.Fatalf("WriteItemMetaAtomic() error = %v", err)
	}

	assetsMeta := &meta.AssetsMeta{
		Schema:     meta.SchemaVersion,
		Assets:     []meta.Asset{},
		Thumbnails: []meta.Thumbnail{},
		Photos:     []meta.Photo{},
	}

	if err := meta.WriteAssetsMetaAtomic(mediaRoot, itemID, assetsMeta); err != nil {
		t.Fatalf("WriteAssetsMetaAtomic() error = %v", err)
	}

	opts := SnapshotOptions{
		IncludeDB:      true,
		IncludeMeta:    true,
		IncludeAvatars: true,
	}

	result, err := Snapshot(mediaRoot, sqlitePath, snapshotPath, opts)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		t.Error("Expected snapshot file to exist")
	}

	if result.Manifest.Counts.MetaFiles == 0 {
		t.Error("Expected meta files to be included")
	}

	manifest, err := ReadManifest(snapshotPath)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}

	if manifest.Schema != SchemaVersion {
		t.Errorf("Expected schema %d, got %d", SchemaVersion, manifest.Schema)
	}

	if manifest.Counts.MetaFiles != 2 {
		t.Errorf("Expected 2 meta files, got %d", manifest.Counts.MetaFiles)
	}
}

func TestSnapshot_WithMedia(t *testing.T) {
	mediaRoot := t.TempDir()
	sqlitePath := filepath.Join(mediaRoot, "db", "test.sqlite")
	snapshotPath := filepath.Join(mediaRoot, "backup.tar.gz")

	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout() error = %v", err)
	}

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if err := storage.EnsureItemDirs(mediaRoot, itemID); err != nil {
		t.Fatalf("EnsureItemDirs() error = %v", err)
	}

	originalPath := filepath.Join(mediaRoot, "items", itemID, "original", "upload.mov")
	if err := os.WriteFile(originalPath, []byte("test video content"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	derivedPath := filepath.Join(mediaRoot, "items", itemID, "derived", "master.mp4")
	if err := os.WriteFile(derivedPath, []byte("test derived content"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	opts := SnapshotOptions{
		IncludeMedia: true,
	}

	result, err := Snapshot(mediaRoot, sqlitePath, snapshotPath, opts)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	if result.Manifest.Counts.Derived == 0 {
		t.Error("Expected derived files to be included")
	}

	if result.Manifest.Counts.Originals == 0 {
		t.Error("Expected original files to be included")
	}
}

func TestReadManifest_NotFound(t *testing.T) {
	_, err := ReadManifest("/nonexistent/path.tar.gz")
	if err == nil {
		t.Error("Expected error for non-existent snapshot")
	}
}
