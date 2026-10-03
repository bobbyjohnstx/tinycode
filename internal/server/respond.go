package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func requestDirectory(r *http.Request, fallback string) string {
	dir := fallback
	if q := r.URL.Query().Get("directory"); q != "" {
		dir = q
	} else if hdr := r.Header.Get("x-tinycode-directory"); hdr != "" {
		if decoded, err := url.PathUnescape(hdr); err == nil {
			dir = decoded
		} else {
			dir = hdr
		}
	}

	return validateDirectoryPath(dir, fallback)
}

// validateDirectoryPath ensures dir is under root. If not, it returns root.
func validateDirectoryPath(dir, root string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return root
	}
	// Clean both paths and ensure the directory is under root.
	cleanRoot := filepath.Clean(absRoot) + string(filepath.Separator)
	cleanDir := filepath.Clean(absDir)

	// Allow exact match or subdirectory.
	if cleanDir == filepath.Clean(absRoot) || strings.HasPrefix(cleanDir, cleanRoot) {
		return cleanDir
	}
	return root
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	defer r.Body.Close()
	err := json.NewDecoder(r.Body).Decode(v)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
