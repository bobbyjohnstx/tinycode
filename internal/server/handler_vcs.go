package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/vcs"
)

func (s *Server) handleVCSInfo(w http.ResponseWriter, r *http.Request) {
	info, err := vcs.GitInfo(s.config.Directory)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"type":   "git",
			"branch": "",
			"remote": "",
		})
		return
	}
	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleVCSStatus(w http.ResponseWriter, r *http.Request) {
	status, err := vcs.GitStatus(s.config.Directory)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"clean":   true,
			"changes": []any{},
		})
		return
	}
	respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleVCSDiff(w http.ResponseWriter, r *http.Request) {
	diff, err := vcs.GitDiff(s.config.Directory)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"diff": "",
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"diff": diff,
	})
}

func (s *Server) handleVCSDiffRaw(w http.ResponseWriter, r *http.Request) {
	diff, err := vcs.GitDiff(s.config.Directory)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(diff))
}

func (s *Server) handleFileStatus(w http.ResponseWriter, r *http.Request) {
	status, err := vcs.GitStatus(s.config.Directory)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"clean":   true,
			"changes": []any{},
		})
		return
	}
	respondJSON(w, http.StatusOK, status)
}
