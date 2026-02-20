package mediahttp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"luna/internal/storage"
)

func TestMediaHandler_BlocksNonGETHEAD(t *testing.T) {
	mediaRoot := t.TempDir()
	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout failed: %v", err)
	}

	handler := New(mediaRoot)

	methods := []string{"POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/media/test.mp4", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
			}
		})
	}
}

func TestMediaHandler_AllowsGET(t *testing.T) {
	mediaRoot := t.TempDir()
	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout failed: %v", err)
	}

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if err := storage.EnsureItemDirs(mediaRoot, itemID); err != nil {
		t.Fatalf("EnsureItemDirs failed: %v", err)
	}

	testContent := []byte("test video content")
	testPath := filepath.Join(mediaRoot, "items", itemID, "original", "video.mp4")
	if err := os.WriteFile(testPath, testContent, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	handler := New(mediaRoot)

	req := httptest.NewRequest(http.MethodGet, "/media/"+itemID+"/original/video.mp4", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if string(body) != string(testContent) {
		t.Error("Content mismatch")
	}
}

func TestMediaHandler_BlocksTraversal(t *testing.T) {
	mediaRoot := t.TempDir()
	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout failed: %v", err)
	}

	handler := New(mediaRoot)

	attackPaths := []string{
		"/media/../../../etc/passwd",
		"/media/..%2F..%2F..%2Fetc%2Fpasswd",
		"/media/01ARZ3NDEKTSV4RRFFQ69G5FAV/../../../db/app.sqlite",
	}

	for _, path := range attackPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
				t.Errorf("Expected status %d or %d for path %s, got %d", http.StatusForbidden, http.StatusNotFound, path, w.Code)
			}
		})
	}
}

func TestMediaHandler_BlocksDirectoryListing(t *testing.T) {
	mediaRoot := t.TempDir()
	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout failed: %v", err)
	}

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if err := storage.EnsureItemDirs(mediaRoot, itemID); err != nil {
		t.Fatalf("EnsureItemDirs failed: %v", err)
	}

	handler := New(mediaRoot)

	req := httptest.NewRequest(http.MethodGet, "/media/"+itemID, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d for directory, got %d", http.StatusNotFound, w.Code)
	}
}

func TestMediaHandler_BlocksNonexistent(t *testing.T) {
	mediaRoot := t.TempDir()
	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout failed: %v", err)
	}

	handler := New(mediaRoot)

	req := httptest.NewRequest(http.MethodGet, "/media/nonexistent/file.mp4", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestMediaHandler_SupportsHEAD(t *testing.T) {
	mediaRoot := t.TempDir()
	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout failed: %v", err)
	}

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if err := storage.EnsureItemDirs(mediaRoot, itemID); err != nil {
		t.Fatalf("EnsureItemDirs failed: %v", err)
	}

	testContent := []byte("test content")
	testPath := filepath.Join(mediaRoot, "items", itemID, "original", "test.mp4")
	if err := os.WriteFile(testPath, testContent, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	handler := New(mediaRoot)

	req := httptest.NewRequest(http.MethodHead, "/media/"+itemID+"/original/test.mp4", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMediaHandler_ServesAvatarPaths(t *testing.T) {
	mediaRoot := t.TempDir()
	if err := storage.EnsureRootLayout(mediaRoot); err != nil {
		t.Fatalf("EnsureRootLayout failed: %v", err)
	}

	avatarPath := filepath.Join(mediaRoot, "avatars", "users", "1", "personas", "abc", "avatar.jpg")
	if err := os.MkdirAll(filepath.Dir(avatarPath), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	testContent := []byte("avatar bytes")
	if err := os.WriteFile(avatarPath, testContent, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	handler := New(mediaRoot)

	// Path style used by ServeMux with http.StripPrefix("/media/", ...)
	req := httptest.NewRequest(http.MethodGet, "/avatars/users/1/personas/abc/avatar.jpg", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if string(body) != string(testContent) {
		t.Fatal("avatar content mismatch")
	}
}
