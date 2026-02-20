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
	storage.EnsureRootLayout(mediaRoot)

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
	storage.EnsureRootLayout(mediaRoot)

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	storage.EnsureItemDirs(mediaRoot, itemID)

	testContent := []byte("test video content")
	testPath := filepath.Join(mediaRoot, "items", itemID, "original", "video.mp4")
	os.WriteFile(testPath, testContent, 0644)

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
	storage.EnsureRootLayout(mediaRoot)

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
	storage.EnsureRootLayout(mediaRoot)

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	storage.EnsureItemDirs(mediaRoot, itemID)

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
	storage.EnsureRootLayout(mediaRoot)

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
	storage.EnsureRootLayout(mediaRoot)

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	storage.EnsureItemDirs(mediaRoot, itemID)

	testContent := []byte("test content")
	testPath := filepath.Join(mediaRoot, "items", itemID, "original", "test.mp4")
	os.WriteFile(testPath, testContent, 0644)

	handler := New(mediaRoot)

	req := httptest.NewRequest(http.MethodHead, "/media/"+itemID+"/original/test.mp4", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}
