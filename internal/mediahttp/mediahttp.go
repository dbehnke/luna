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
	mediaRoot  string
	itemsDir   string
	avatarsDir string
}

func New(mediaRoot string) *MediaHandler {
	return &MediaHandler{
		mediaRoot:  mediaRoot,
		itemsDir:   filepath.Join(mediaRoot, "items"),
		avatarsDir: filepath.Join(mediaRoot, "avatars"),
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

	var baseDir string
	if strings.HasPrefix(path, "avatars/") {
		baseDir = h.avatarsDir
	} else {
		baseDir = h.itemsDir
	}

	filePath := filepath.Join(baseDir, path)

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !strings.HasPrefix(absFilePath, absBaseDir+string(filepath.Separator)) && absFilePath != absBaseDir {
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
