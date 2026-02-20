package main

import (
	"path/filepath"
	"testing"

	"luna/internal/config"
	"luna/internal/meta"
	"luna/internal/storage"
)

func TestRebuildDB_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	mediaRoot := t.TempDir()

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	err := storage.EnsureRootLayout(mediaRoot)
	if err != nil {
		t.Fatalf("EnsureRootLayout() error = %v", err)
	}

	err = storage.EnsureItemDirs(mediaRoot, itemID)
	if err != nil {
		t.Fatalf("EnsureItemDirs() error = %v", err)
	}

	personaName := "SuperKid"
	itemMeta := &meta.ItemMeta{
		Schema:             meta.SchemaVersion,
		ItemID:             itemID,
		Type:               "video",
		OwnerUsername:      "testuser",
		PersonaDisplayName: &personaName,
		Title:              "Test Video",
		Description:        "A test video for rebuild",
		CreatedAt:          "2024-01-15T10:30:00Z",
		State: meta.ItemState{
			Highlighted: true,
		},
		Original: meta.OriginalFile{
			Filename: "video.mov",
			Path:     "original/upload.mov",
		},
	}

	err = meta.WriteItemMetaAtomic(mediaRoot, itemID, itemMeta)
	if err != nil {
		t.Fatalf("WriteItemMetaAtomic() error = %v", err)
	}

	assetsMeta := &meta.AssetsMeta{
		Schema:     meta.SchemaVersion,
		Assets:     []meta.Asset{},
		Thumbnails: []meta.Thumbnail{},
		Photos:     []meta.Photo{},
	}

	err = meta.WriteAssetsMetaAtomic(mediaRoot, itemID, assetsMeta)
	if err != nil {
		t.Fatalf("WriteAssetsMetaAtomic() error = %v", err)
	}

	cfg := &config.Config{
		MediaRoot:  mediaRoot,
		SQLitePath: filepath.Join(mediaRoot, "db", "test.sqlite"),
	}

	if err := storage.EnsureRootLayout(cfg.MediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout() error = %v", err)
	}

	loadedMeta, err := meta.ReadItemMetaByID(mediaRoot, itemID)
	if err != nil {
		t.Fatalf("ReadItemMetaByID() error = %v", err)
	}

	if loadedMeta.ItemID != itemID {
		t.Errorf("ItemID mismatch: got %s, want %s", loadedMeta.ItemID, itemID)
	}

	if loadedMeta.OwnerUsername != "testuser" {
		t.Errorf("OwnerUsername mismatch: got %s, want testuser", loadedMeta.OwnerUsername)
	}

	if loadedMeta.PersonaDisplayName == nil || *loadedMeta.PersonaDisplayName != "SuperKid" {
		t.Errorf("PersonaDisplayName mismatch")
	}

	if !loadedMeta.State.Highlighted {
		t.Error("Expected highlighted to be true")
	}

	loadedAssets, err := meta.ReadAssetsMetaByID(mediaRoot, itemID)
	if err != nil {
		t.Fatalf("ReadAssetsMetaByID() error = %v", err)
	}

	if loadedAssets.Schema != meta.SchemaVersion {
		t.Errorf("Assets schema mismatch: got %d, want %d", loadedAssets.Schema, meta.SchemaVersion)
	}
}
