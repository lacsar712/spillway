package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed static/*
var staticFS embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}

// DirHandler serves from disk when running `go run` from a checkout so
// UI tweaks do not require a rebuild. Falls back to embed.
func DirHandler(dir string) http.Handler {
	if dir == "" {
		dir = "web"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Handler()
	}
	st, err := os.Stat(filepath.Join(abs, "index.html"))
	if err != nil || st.IsDir() {
		return Handler()
	}
	return http.FileServer(http.Dir(abs))
}
