package static

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed dist/*
var embeddedFS embed.FS

func Handler(devDir string) http.Handler {
	var fileSystem http.FileSystem
	if devDir != "" {
		if info, err := os.Stat(devDir); err == nil && info.IsDir() {
			fileSystem = http.Dir(devDir)
		}
	}

	if fileSystem == nil {
		sub, err := fs.Sub(embeddedFS, "dist")
		if err != nil {
			sub = embeddedFS
		}
		fileSystem = http.FS(sub)
	}

	fileServer := http.FileServer(fileSystem)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if path == "/" || path == "" {
			serveIndex(w, r, fileSystem)
			return
		}

		ext := filepath.Ext(path)

		if ext != "" {
			if ct, ok := contentTypes[ext]; ok {
				w.Header().Set("Content-Type", ct)
			}
			if strings.HasPrefix(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: non-file paths serve index.html for client-side routing
		if _, err := fileSystem.Open(path); err != nil {
			serveIndex(w, r, fileSystem)
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, fsys http.FileSystem) {
	f, err := fsys.Open("/index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", stat.ModTime(), f.(readSeeker))
}

type readSeeker interface {
	Read([]byte) (int, error)
	Seek(int64, int) (int64, error)
}

var contentTypes = map[string]string{
	".js":    "application/javascript",
	".css":   "text/css",
	".html":  "text/html; charset=utf-8",
	".json":  "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".map":   "application/json",
	".wasm":  "application/wasm",
	".aac":   "audio/aac",
	".webp":  "image/webp",
}
