package server

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleFileSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respondJSON(w, http.StatusOK, map[string]any{
			"results": []any{},
		})
		return
	}

	type searchResult struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Content string `json:"content"`
	}

	var results []searchResult
	const maxResults = 100

	_ = filepath.Walk(s.config.Directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".tinycode" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 1<<20 {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		rel, _ := filepath.Rel(s.config.Directory, path)
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if !strings.Contains(line, query) {
				continue
			}
			results = append(results, searchResult{
				File:    rel,
				Line:    lineNum,
				Content: line,
			})
			if len(results) >= maxResults {
				break
			}
		}
		return nil
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

func (s *Server) handleFileFind(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		query = r.URL.Query().Get("q")
	}
	if query == "" {
		respondJSON(w, http.StatusOK, []string{})
		return
	}

	dir := requestDirectory(r, s.config.Directory)

	typeFilter := r.URL.Query().Get("type")

	limitStr := r.URL.Query().Get("limit")
	maxFiles := 200
	if limitStr != "" {
		if n := 0; len(limitStr) > 0 {
			for _, c := range limitStr {
				if c >= '0' && c <= '9' {
					n = n*10 + int(c-'0')
				}
			}
			if n > 0 {
				maxFiles = n
			}
		}
	}

	var files []string

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if len(files) >= maxFiles {
			return filepath.SkipAll
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".tinycode" {
				return filepath.SkipDir
			}
			if typeFilter == "directory" {
				if strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
					files = append(files, path)
				}
			}
			return nil
		}

		if typeFilter == "directory" {
			return nil
		}

		name := info.Name()
		matched, _ := filepath.Match(query, name)
		if matched || strings.Contains(name, query) {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, rel)
		}
		return nil
	})

	if files == nil {
		files = []string{}
	}
	respondJSON(w, http.StatusOK, files)
}
