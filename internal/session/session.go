package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/id"
)

type Info struct {
	ID        string     `json:"id"`
	Slug      string     `json:"slug,omitempty"`
	ProjectID string     `json:"projectID"`
	Directory string     `json:"directory"`
	ParentID  string     `json:"parentID,omitempty"`
	Title     string     `json:"title"`
	Agent     string     `json:"agent,omitempty"`
	Model     *ModelRef  `json:"model,omitempty"`
	Version   string     `json:"version"`
	Summary   *Summary   `json:"summary,omitempty"`
	Cost      float64    `json:"cost,omitempty"`
	Tokens    TokenUsage `json:"tokens"`
	Time      TimeInfo   `json:"time"`
	CreatedAt    time.Time `json:"-"`
	UpdatedAt    time.Time `json:"-"`
	TimeArchived int64    `json:"-"`
}

type TimeInfo struct {
	Created  int64 `json:"created"`
	Updated  int64 `json:"updated"`
	Archived int64 `json:"archived,omitempty"`
}

func (i *Info) SyncTime() {
	i.Time = TimeInfo{
		Created:  i.CreatedAt.UnixMilli(),
		Updated:  i.UpdatedAt.UnixMilli(),
		Archived: i.TimeArchived,
	}
}

type ModelRef struct {
	ModelID    string `json:"id"`
	ProviderID string `json:"providerID"`
	Variant    string `json:"variant,omitempty"`
}

// UnmarshalJSON supports both "id" and legacy "modelID" fields.
func (m *ModelRef) UnmarshalJSON(data []byte) error {
	type alias ModelRef
	var raw struct {
		alias
		LegacyID string `json:"modelID"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = ModelRef(raw.alias)
	if m.ModelID == "" {
		m.ModelID = raw.LegacyID
	}
	return nil
}

type SnapshotDiff struct {
	File      string `json:"file,omitempty"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status,omitempty"`
}

type Summary struct {
	Additions int            `json:"additions"`
	Deletions int            `json:"deletions"`
	Files     int            `json:"files"`
	Diffs     []SnapshotDiff `json:"diffs"`
}

type TokenUsage struct {
	Input     int        `json:"input"`
	Output    int        `json:"output"`
	Reasoning int        `json:"reasoning"`
	Cache     CacheUsage `json:"cache"`
}

type CacheUsage struct {
	Read  int `json:"read"`
	Write int `json:"write"`
}

type CreateInput struct {
	ProjectID string
	Directory string
	ParentID  string
	Title     string
	Agent     string
	Model     *ModelRef
}

const (
	defaultVersion     = "1"
	parentTitlePrefix  = "New session - "
	childTitlePrefix   = "Child session - "
)

func DefaultTitle(isChild bool) string {
	prefix := parentTitlePrefix
	if isChild {
		prefix = childTitlePrefix
	}
	return prefix + time.Now().UTC().Format(time.RFC3339Nano)
}

// Store handles session persistence.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(input CreateInput) (*Info, error) {
	sessionID, err := id.Ascending("session")
	if err != nil {
		return nil, fmt.Errorf("generating session ID: %w", err)
	}

	slug := sessionID[4:] // strip "ses_" prefix for slug
	now := time.Now()
	nowMs := now.UnixMilli()

	title := input.Title
	if title == "" {
		title = DefaultTitle(input.ParentID != "")
	}

	var modelJSON []byte
	if input.Model != nil {
		modelJSON, _ = json.Marshal(input.Model)
	}

	var parentID *string
	if input.ParentID != "" {
		parentID = &input.ParentID
	}

	s.ensureProject(input.ProjectID, input.Directory)

	_, err = s.db.Exec(
		`INSERT INTO session (id, project_id, slug, directory, parent_id, title, agent, model, version, cost, tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write, time_created, time_updated)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0, 0, 0, ?, ?)`,
		sessionID, input.ProjectID, slug, input.Directory, parentID,
		title, input.Agent, string(modelJSON), defaultVersion, nowMs, nowMs,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting session: %w", err)
	}

	info := &Info{
		ID:        sessionID,
		Slug:      slug,
		ProjectID: input.ProjectID,
		Directory: input.Directory,
		ParentID:  input.ParentID,
		Title:     title,
		Agent:     input.Agent,
		Model:     input.Model,
		Version:   defaultVersion,
		CreatedAt: now,
		UpdatedAt: now,
	}
	info.SyncTime()
	return info, nil
}

func (s *Store) ensureProject(projectID, directory string) {
	now := time.Now().UnixMilli()
	_, _ = s.db.Exec(
		`INSERT OR IGNORE INTO project (id, worktree, time_created, time_updated)
		 VALUES (?, ?, ?, ?)`,
		projectID, directory, now, now,
	)
}

const sessionSelectCols = `id, project_id, slug, directory, parent_id, title, agent, model, version,
		cost, tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write,
		summary_additions, summary_deletions, summary_files, summary_diffs,
		time_created, time_updated, time_archived`

func (s *Store) Get(sessionID string) (*Info, error) {
	row := s.db.QueryRow(
		`SELECT `+sessionSelectCols+` FROM session WHERE id = ?`, sessionID,
	)
	info, err := scanSession(row)
	if err != nil {
		return nil, fmt.Errorf("session %s: %w", sessionID, err)
	}
	return info, nil
}

func (s *Store) List(projectID string, limit, offset int) ([]Info, error) {
	rows, err := s.db.Query(
		`SELECT `+sessionSelectCols+` FROM session WHERE project_id = ? ORDER BY time_created DESC LIMIT ? OFFSET ?`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var result []Info
	for rows.Next() {
		info, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *info)
	}
	return result, rows.Err()
}

func (s *Store) Children(parentID string) ([]Info, error) {
	rows, err := s.db.Query(
		`SELECT `+sessionSelectCols+` FROM session WHERE parent_id = ? ORDER BY time_created DESC`,
		parentID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing children: %w", err)
	}
	defer rows.Close()

	var result []Info
	for rows.Next() {
		info, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *info)
	}
	return result, rows.Err()
}

func (s *Store) UpdateTitle(sessionID, title string) error {
	_, err := s.db.Exec(
		"UPDATE session SET title = ?, time_updated = ? WHERE id = ?",
		title, time.Now().UnixMilli(), sessionID,
	)
	return err
}

func (s *Store) UpdateCost(sessionID string, cost float64, tokens TokenUsage) error {
	_, err := s.db.Exec(
		`UPDATE session SET cost = ?, tokens_input = ?, tokens_output = ?, tokens_reasoning = ?,
		 tokens_cache_read = ?, tokens_cache_write = ?, time_updated = ? WHERE id = ?`,
		cost, tokens.Input, tokens.Output, tokens.Reasoning,
		tokens.Cache.Read, tokens.Cache.Write, time.Now().UnixMilli(), sessionID,
	)
	return err
}

func (s *Store) UpdateSummary(sessionID string, summary Summary) error {
	var diffsJSON string
	if len(summary.Diffs) > 0 {
		data, _ := json.Marshal(summary.Diffs)
		diffsJSON = string(data)
	}
	_, err := s.db.Exec(
		`UPDATE session SET summary_additions = ?, summary_deletions = ?, summary_files = ?,
		 summary_diffs = ?, time_updated = ? WHERE id = ?`,
		summary.Additions, summary.Deletions, summary.Files,
		diffsJSON, time.Now().UnixMilli(), sessionID,
	)
	return err
}

func (s *Store) Archive(sessionID string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		"UPDATE session SET time_archived = ?, time_updated = ? WHERE id = ?",
		now, now, sessionID,
	)
	return err
}

func (s *Store) Delete(sessionID string) error {
	_, err := s.db.Exec("DELETE FROM session WHERE id = ?", sessionID)
	return err
}

func scanSession(row *sql.Row) (*Info, error) {
	var info Info
	var parentID, agent, modelJSON, summaryDiffs sql.NullString
	var summaryAdd, summaryDel, summaryFiles sql.NullInt64
	var createdMs, updatedMs int64
	var timeArchived sql.NullInt64

	err := row.Scan(
		&info.ID, &info.ProjectID, &info.Slug, &info.Directory,
		&parentID, &info.Title, &agent, &modelJSON, &info.Version,
		&info.Cost, &info.Tokens.Input, &info.Tokens.Output, &info.Tokens.Reasoning,
		&info.Tokens.Cache.Read, &info.Tokens.Cache.Write,
		&summaryAdd, &summaryDel, &summaryFiles, &summaryDiffs,
		&createdMs, &updatedMs, &timeArchived,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("scanning session: %w", err)
	}

	return populateInfo(&info, parentID, agent, modelJSON, summaryAdd, summaryDel, summaryFiles, summaryDiffs, createdMs, updatedMs, timeArchived), nil
}

func scanSessionRows(rows *sql.Rows) (*Info, error) {
	var info Info
	var parentID, agent, modelJSON, summaryDiffs sql.NullString
	var summaryAdd, summaryDel, summaryFiles sql.NullInt64
	var createdMs, updatedMs int64
	var timeArchived sql.NullInt64

	err := rows.Scan(
		&info.ID, &info.ProjectID, &info.Slug, &info.Directory,
		&parentID, &info.Title, &agent, &modelJSON, &info.Version,
		&info.Cost, &info.Tokens.Input, &info.Tokens.Output, &info.Tokens.Reasoning,
		&info.Tokens.Cache.Read, &info.Tokens.Cache.Write,
		&summaryAdd, &summaryDel, &summaryFiles, &summaryDiffs,
		&createdMs, &updatedMs, &timeArchived,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning session row: %w", err)
	}

	return populateInfo(&info, parentID, agent, modelJSON, summaryAdd, summaryDel, summaryFiles, summaryDiffs, createdMs, updatedMs, timeArchived), nil
}

func populateInfo(info *Info, parentID, agent, modelJSON sql.NullString, summaryAdd, summaryDel, summaryFiles sql.NullInt64, summaryDiffs sql.NullString, createdMs, updatedMs int64, timeArchived sql.NullInt64) *Info {
	if parentID.Valid {
		info.ParentID = parentID.String
	}
	if agent.Valid {
		info.Agent = agent.String
	}
	if modelJSON.Valid && modelJSON.String != "" {
		var m ModelRef
		if json.Unmarshal([]byte(modelJSON.String), &m) == nil {
			info.Model = &m
		}
	}
	if summaryAdd.Valid {
		info.Summary = &Summary{
			Additions: int(summaryAdd.Int64),
			Deletions: int(summaryDel.Int64),
			Files:     int(summaryFiles.Int64),
			Diffs:     []SnapshotDiff{},
		}
		if summaryDiffs.Valid && summaryDiffs.String != "" {
			_ = json.Unmarshal([]byte(summaryDiffs.String), &info.Summary.Diffs)
		}
	}
	info.CreatedAt = time.UnixMilli(createdMs)
	info.UpdatedAt = time.UnixMilli(updatedMs)
	if timeArchived.Valid {
		info.TimeArchived = timeArchived.Int64
	}
	info.SyncTime()
	return info
}
