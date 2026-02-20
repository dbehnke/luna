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

var extContentTypes = map[string]string{
	".m3u8": "application/vnd.apple.mpegurl",
	".ts":   "video/MP2T",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".webp": "image/webp",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
}

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

	path = strings.TrimPrefix(path, "/")
	path = strings.TrimPrefix(path, "media/")
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	var baseDir string
	if strings.HasPrefix(path, "avatars/") {
		baseDir = h.avatarsDir
		path = strings.TrimPrefix(path, "avatars/")
	} else if strings.HasPrefix(path, "items/") {
		baseDir = h.itemsDir
		path = strings.TrimPrefix(path, "items/")
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

	ext := strings.ToLower(filepath.Ext(absFilePath))
	if contentType, ok := extContentTypes[ext]; ok {
		w.Header().Set("Content-Type", contentType)
		if ext == ".m3u8" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
	}

	http.ServeFile(w, r, absFilePath)
}
