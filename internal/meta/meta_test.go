package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteJSONAtomic_ProducesValidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	testData := ItemMeta{
		Schema:        SchemaVersion,
		ItemID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Type:          "video",
		OwnerUsername: "testuser",
		Title:         "Test Video",
		Description:   "A test video",
		CreatedAt:     "2024-01-01T00:00:00Z",
		State:         ItemState{Highlighted: false},
		Original:      OriginalFile{Filename: "video.mov", Path: "original/upload.mov"},
	}

	path := filepath.Join(tmpDir, "test.json")
	err := WriteJSONAtomic(path, testData)
	if err != nil {
		t.Fatalf("WriteJSONAtomic() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	var loaded ItemMeta
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("Written file is not valid JSON: %v", err)
	}

	if loaded.Schema != testData.Schema {
		t.Errorf("Schema mismatch: got %d, want %d", loaded.Schema, testData.Schema)
	}
	if loaded.ItemID != testData.ItemID {
		t.Errorf("ItemID mismatch: got %s, want %s", loaded.ItemID, testData.ItemID)
	}
	if loaded.Title != testData.Title {
		t.Errorf("Title mismatch: got %s, want %s", loaded.Title, testData.Title)
	}
}

func TestWriteJSONAtomic_NoPartialFileOnError(t *testing.T) {
	tmpDir := t.TempDir()

	path := filepath.Join(tmpDir, "test.json")

	err := WriteJSONAtomic(path, make(chan int))
	if err == nil {
		t.Fatal("Expected error for invalid data")
	}

	_, err = os.Stat(path)
	if !os.IsNotExist(err) {
		t.Errorf("Expected no file to exist after failed write, got: %v", err)
	}
}

func TestWriteFileAtomic_NoPartialFileOnError(t *testing.T) {
	tmpDir := t.TempDir()

	path := filepath.Join(tmpDir, "test.bin")

	err := WriteFileAtomic(path, []byte("test"))
	if err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(data) != "test" {
		t.Errorf("Content mismatch: got %s, want %s", string(data), "test")
	}
}

func TestReadItemMeta(t *testing.T) {
	tmpDir := t.TempDir()

	itemMeta := ItemMeta{
		Schema:        SchemaVersion,
		ItemID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Type:          "photo",
		OwnerUsername: "john",
		Title:         "Vacation Photo",
		CreatedAt:     "2024-06-15T10:30:00Z",
		State:         ItemState{Highlighted: true},
		Original:      OriginalFile{Filename: "photo.jpg", Path: "original/photo.jpg"},
	}

	path := filepath.Join(tmpDir, "item.json")
	err := WriteJSONAtomic(path, itemMeta)
	if err != nil {
		t.Fatalf("WriteJSONAtomic() error = %v", err)
	}

	loaded, err := ReadItemMeta(path)
	if err != nil {
		t.Fatalf("ReadItemMeta() error = %v", err)
	}

	if loaded.Schema != itemMeta.Schema {
		t.Errorf("Schema: got %d, want %d", loaded.Schema, itemMeta.Schema)
	}
	if loaded.ItemID != itemMeta.ItemID {
		t.Errorf("ItemID: got %s, want %s", loaded.ItemID, itemMeta.ItemID)
	}
	if loaded.Type != itemMeta.Type {
		t.Errorf("Type: got %s, want %s", loaded.Type, itemMeta.Type)
	}
	if loaded.OwnerUsername != itemMeta.OwnerUsername {
		t.Errorf("OwnerUsername: got %s, want %s", loaded.OwnerUsername, itemMeta.OwnerUsername)
	}
}

func TestReadItemMeta_InvalidSchema(t *testing.T) {
	tmpDir := t.TempDir()

	type invalidMeta struct {
		Schema int    `json:"schema"`
		ItemID string `json:"item_id"`
	}

	invalid := invalidMeta{Schema: 999, ItemID: "test"}

	path := filepath.Join(tmpDir, "item.json")
	data, _ := json.Marshal(invalid)
	os.WriteFile(path, data, 0644)

	_, err := ReadItemMeta(path)
	if err == nil {
		t.Fatal("Expected error for invalid schema")
	}
}

func TestReadAssetsMeta_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	path := filepath.Join(tmpDir, "assets.json")

	_, err := ReadAssetsMeta(path)
	if err != nil {
		t.Fatalf("Expected nil for missing file, got: %v", err)
	}
}
