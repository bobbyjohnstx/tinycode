package permission

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SQLiteRuleStore persists approved permission rules in the permission table.
type SQLiteRuleStore struct {
	db *sql.DB
}

// NewSQLiteRuleStore returns a RuleStore backed by the given database.
func NewSQLiteRuleStore(db *sql.DB) *SQLiteRuleStore {
	return &SQLiteRuleStore{db: db}
}

func (s *SQLiteRuleStore) SaveRules(projectID string, rules Ruleset) error {
	if s == nil || s.db == nil {
		return errors.New("permission store: nil database")
	}
	data, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("marshaling permission rules: %w", err)
	}
	now := time.Now().UnixMilli()
	_, err = s.db.Exec(
		`INSERT INTO permission (project_id, time_created, time_updated, data)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(project_id) DO UPDATE SET
		   time_updated = excluded.time_updated,
		   data = excluded.data`,
		projectID, now, now, string(data),
	)
	if err != nil {
		return fmt.Errorf("saving permission rules: %w", err)
	}
	return nil
}

func (s *SQLiteRuleStore) LoadRules(projectID string) (Ruleset, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("permission store: nil database")
	}
	var data string
	err := s.db.QueryRow(
		`SELECT data FROM permission WHERE project_id = ?`, projectID,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading permission rules: %w", err)
	}
	var rules Ruleset
	if err := json.Unmarshal([]byte(data), &rules); err != nil {
		return nil, fmt.Errorf("unmarshaling permission rules: %w", err)
	}
	return rules, nil
}
