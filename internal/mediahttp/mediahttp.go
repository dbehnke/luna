package mediahttp

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidMethod   = errors.New("method not allowed")
	ErrPathTraversal   = errors.New("path traversal attempt detected")
	ErrPathEscapesRoot = errors.New("path escapes media root")
)

type MediaHandler struct {
	mediaRoot string
	itemsDir  string
}

func New(mediaRoot string) *MediaHandler {
	return &MediaHandler{
		mediaRoot: mediaRoot,
		itemsDir:  filepath.Join(mediaRoot, "items"),
	}
}

func (h *MediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path

	if path == "" || path == "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	path = strings.TrimPrefix(path, "/media/")
	if path == "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	filePath := filepath.Join(h.itemsDir, path)

	absItemsDir, err := filepath.Abs(h.itemsDir)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !strings.HasPrefix(absFilePath, absItemsDir+string(filepath.Separator)) && absFilePath != absItemsDir {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(absFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if info.IsDir() {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, absFilePath)
}
