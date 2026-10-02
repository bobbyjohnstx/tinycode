package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode/internal/project"
)

func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.DB.Query(
		`SELECT id, worktree, vcs, time_created, time_updated, time_initialized, sandboxes FROM project ORDER BY time_created`,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var projects []*project.Info
	for rows.Next() {
		var p project.Info
		var vcsVal, sandboxesJSON sql.NullString
		var timeInit sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Worktree, &vcsVal, &p.Time.Created, &p.Time.Updated, &timeInit, &sandboxesJSON); err != nil {
			continue
		}
		if vcsVal.Valid {
			p.VCS = vcsVal.String
		}
		if timeInit.Valid {
			p.Time.Initialized = timeInit.Int64
		}
		p.Sandboxes = []string{}
		if sandboxesJSON.Valid && sandboxesJSON.String != "" {
			_ = json.Unmarshal([]byte(sandboxesJSON.String), &p.Sandboxes)
		}
		projects = append(projects, &p)
	}
	if projects == nil {
		projects = []*project.Info{}
	}
	respondJSON(w, http.StatusOK, projects)
}

func (s *Server) handleProjectCurrent(w http.ResponseWriter, r *http.Request) {
	p := project.FromDirectory(s.config.Directory)
	respondJSON(w, http.StatusOK, p)
}

func (s *Server) handlePathGet(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "tinycode")
	stateDir := filepath.Join(home, ".local", "share", "tinycode")

	respondJSON(w, http.StatusOK, map[string]any{
		"home":      home,
		"state":     stateDir,
		"config":    configDir,
		"worktree":  s.config.Directory,
		"directory": s.config.Directory,
	})
}
