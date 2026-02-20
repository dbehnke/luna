package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerServesIndexForClientRoute(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":         {Data: []byte("<html>SPA</html>")},
		"assets/app.js":      {Data: []byte("console.log('ok')")},
		"assets/style.css":   {Data: []byte("body{}")},
		"favicon.ico":        {Data: []byte("ico")},
		"site.webmanifest":   {Data: []byte("{}")},
		"android-chrome.png": {Data: []byte("png")},
	}

	h := newFrontendHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/library", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for client route, got %d", w.Code)
	}
	if body := w.Body.String(); body != "<html>SPA</html>" {
		t.Fatalf("expected index html body, got %q", body)
	}
}

func TestSPAHandlerServesStaticAsset(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    {Data: []byte("<html>SPA</html>")},
		"assets/app.js": {Data: []byte("console.log('ok')")},
	}

	h := newFrontendHandler(fsys)
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for asset, got %d", w.Code)
	}
	if body := w.Body.String(); body != "console.log('ok')" {
		t.Fatalf("expected asset body, got %q", body)
	}
}

func TestSPAHandlerReturns404ForMissingAssetWithExtension(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>SPA</html>")},
	}

	h := newFrontendHandler(fs.FS(fsys))
	req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset, got %d", w.Code)
	}
}
