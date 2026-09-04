package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Server) handleFileRead(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path query parameter required")
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "file not found")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"path":    path,
		"content": string(data),
		"size":    len(data),
	})
}

func (s *Server) handleFileList(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	if dir == "" {
		respondError(w, http.StatusBadRequest, "path query parameter required")
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "directory not found")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type fileEntry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
		Size  int64  `json:"size,omitempty"`
	}

	var files []fileEntry
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		fe := fileEntry{
			Name:  name,
			Path:  filepath.Join(dir, name),
			IsDir: entry.IsDir(),
		}

		if !entry.IsDir() {
			info, err := entry.Info()
			if err == nil {
				fe.Size = info.Size()
			}
		}

		files = append(files, fe)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"files": files,
		"count": len(files),
	})
}
