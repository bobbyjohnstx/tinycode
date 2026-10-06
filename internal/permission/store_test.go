package permission

import (
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/storage"
)

func TestSQLiteRuleStore_SaveAndLoad(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	now := int64(1700000000000)
	if _, err := db.Exec(
		`INSERT INTO project (id, worktree, sandboxes, time_created, time_updated) VALUES (?, ?, ?, ?, ?)`,
		"prj_rules", "/tmp", "[]", now, now,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	store := NewSQLiteRuleStore(db.DB)
	rules := Ruleset{
		{Permission: "bash", Pattern: "ls *", Action: ActionAllow},
		{Permission: "edit", Pattern: "*.go", Action: ActionAllow},
	}
	if err := store.SaveRules("prj_rules", rules); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}

	loaded, err := store.LoadRules("prj_rules")
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(loaded))
	}
	if loaded[0].Permission != "bash" || loaded[0].Pattern != "ls *" || loaded[0].Action != ActionAllow {
		t.Errorf("unexpected first rule: %+v", loaded[0])
	}

	// Missing project returns empty, not error.
	empty, err := store.LoadRules("missing")
	if err != nil {
		t.Fatalf("LoadRules missing: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty ruleset, got %#v", empty)
	}

	// Upsert replaces data.
	updated := Ruleset{{Permission: "read", Pattern: "*", Action: ActionAllow}}
	if err := store.SaveRules("prj_rules", updated); err != nil {
		t.Fatalf("SaveRules upsert: %v", err)
	}
	loaded, err = store.LoadRules("prj_rules")
	if err != nil {
		t.Fatalf("LoadRules after upsert: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Permission != "read" {
		t.Fatalf("expected upserted rule, got %#v", loaded)
	}
}

func TestService_SetStore_LoadsFromSQLite(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	now := int64(1700000000000)
	if _, err := db.Exec(
		`INSERT INTO project (id, worktree, sandboxes, time_created, time_updated) VALUES (?, ?, ?, ?, ?)`,
		"prj_svc", "/tmp", "[]", now, now,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	store := NewSQLiteRuleStore(db.DB)
	if err := store.SaveRules("prj_svc", Ruleset{
		{Permission: "bash", Pattern: "echo *", Action: ActionAllow},
	}); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}

	b := bus.New()
	defer b.Close()
	svc := NewService(b)
	svc.SetStore(store, "prj_svc")

	// Ask with matching pattern should allow from loaded rules without UI.
	err = svc.Ask(t.Context(), AskInput{
		SessionID:  "ses_1",
		Permission: "bash",
		Patterns:   []string{"echo hi"},
	})
	if err != nil {
		t.Fatalf("Ask should allow via persisted rules: %v", err)
	}
}
