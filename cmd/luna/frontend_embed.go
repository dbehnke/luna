package main

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed frontend
var frontend embed.FS

func getFrontendHandler() (http.Handler, error) {
	sub, err := fs.Sub(frontend, "frontend")
	if err != nil {
		return nil, err
	}
	return newFrontendHandler(sub), nil
}

type spaHandler struct {
	fsys       fs.FS
	fileServer http.Handler
}

func newFrontendHandler(fsys fs.FS) http.Handler {
	return &spaHandler{
		fsys:       fsys,
		fileServer: http.FileServer(http.FS(fsys)),
	}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Let static files through directly.
	cleanPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	trimmed := strings.TrimPrefix(cleanPath, "/")
	if trimmed == "" {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	if strings.Contains(path.Base(cleanPath), ".") {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	// For client-side routes, always serve index.html.
	if indexBytes, err := fs.ReadFile(h.fsys, "index.html"); err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(indexBytes)
		}
		return
	}

	h.fileServer.ServeHTTP(w, r)
}
