package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinItem_RejectsTraversal(t *testing.T) {
	root := t.TempDir()

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	tests := []struct {
		name    string
		relPath string
		wantErr error
	}{
		{
			name:    "single dot dot",
			relPath: "..",
			wantErr: ErrPathTraversal,
		},
		{
			name:    "path with dot dot",
			relPath: "../etc/passwd",
			wantErr: ErrPathTraversal,
		},
		{
			name:    "dot dot in middle",
			relPath: "derived/../../meta/item.json",
			wantErr: ErrPathTraversal,
		},
		{
			name:    "encoded dot dot",
			relPath: "..%2F..%2Fetc%2Fpasswd",
			wantErr: ErrPathTraversal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeJoinItem(root, itemID, tt.relPath)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("SafeJoinItem() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("SafeJoinItem() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestSafeJoinItem_RejectsAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	tests := []struct {
		name    string
		relPath string
		wantErr error
	}{
		{
			name:    "absolute unix",
			relPath: "/etc/passwd",
			wantErr: ErrInvalidPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeJoinItem(root, itemID, tt.relPath)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("SafeJoinItem() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("SafeJoinItem() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestSafeJoinItem_AcceptsValidPaths(t *testing.T) {
	root := t.TempDir()
	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	tests := []struct {
		name    string
		relPath string
	}{
		{
			name:    "simple filename",
			relPath: "video.mp4",
		},
		{
			name:    "nested path",
			relPath: "derived/master.mp4",
		},
		{
			name:    "thumbs path",
			relPath: "thumbs/t_0001.webp",
		},
		{
			name:    "meta file",
			relPath: "meta/item.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoinItem(root, itemID, tt.relPath)
			if err != nil {
				t.Errorf("SafeJoinItem() unexpected error = %v", err)
				return
			}

			expected := filepath.Join(root, "items", itemID, tt.relPath)
			if got != expected {
				t.Errorf("SafeJoinItem() = %v, want %v", got, expected)
			}
		})
	}
}

func TestEnsureRootLayout(t *testing.T) {
	root := t.TempDir()

	err := EnsureRootLayout(root)
	if err != nil {
		t.Fatalf("EnsureRootLayout() error = %v", err)
	}

	expectedDirs := []string{
		"items",
		"tmp/uploads",
		"tmp/jobs",
		"db",
	}

	for _, dir := range expectedDirs {
		path := filepath.Join(root, dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Directory %s not created: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("Path %s is not a directory", dir)
		}
	}
}

func TestEnsureItemDirs(t *testing.T) {
	root := t.TempDir()
	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"

	err := EnsureItemDirs(root, itemID)
	if err != nil {
		t.Fatalf("EnsureItemDirs() error = %v", err)
	}

	expectedDirs := []string{
		"items/" + itemID + "/original",
		"items/" + itemID + "/derived",
		"items/" + itemID + "/thumbs",
		"items/" + itemID + "/photos",
		"items/" + itemID + "/meta",
	}

	for _, dir := range expectedDirs {
		path := filepath.Join(root, dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Directory %s not created: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("Path %s is not a directory", dir)
		}
	}
}
