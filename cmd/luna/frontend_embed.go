package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed frontend
var frontend embed.FS

func getFrontendHandler() (http.Handler, error) {
	sub, err := fs.Sub(frontend, "frontend")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(sub)), nil
}
