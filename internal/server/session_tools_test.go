package server

import (
	"sort"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

func TestExtractAllowedPerms_WildcardDenyOverridesWildcardAllow(t *testing.T) {
	// Simulates an agent like explore: defaultPerms allow-all, then agent denies all,
	// then re-allows specific permissions.
	ruleset := permission.Ruleset{
		{Permission: "*", Pattern: "*", Action: permission.ActionAllow},
		{Permission: "*", Pattern: "*", Action: permission.ActionDeny},
		{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
		{Permission: "grep", Pattern: "*", Action: permission.ActionAllow},
	}

	result := extractAllowedPerms(ruleset)
	sort.Strings(result)

	expected := []string{"grep", "read"}
	if len(result) != len(expected) {
		t.Fatalf("got %v, want %v", result, expected)
	}
	for i, perm := range expected {
		if result[i] != perm {
			t.Errorf("result[%d] = %q, want %q", i, result[i], perm)
		}
	}

	// Wildcard "*" must NOT be in the result.
	for _, p := range result {
		if p == "*" {
			t.Error("wildcard '*' should not be in allowed perms when a later deny overrides it")
		}
	}
}

func TestExtractAllowedPerms_WildcardAllowOnly(t *testing.T) {
	// Default perms with wildcard allow and no deny — everything should pass.
	ruleset := permission.Ruleset{
		{Permission: "*", Pattern: "*", Action: permission.ActionAllow},
	}

	result := extractAllowedPerms(ruleset)

	found := false
	for _, p := range result {
		if p == "*" {
			found = true
			break
		}
	}
	if !found {
		t.Error("wildcard '*' should be in allowed perms when no deny overrides it")
	}
}

func TestExtractAllowedPerms_SpecificAllowAfterWildcardDeny(t *testing.T) {
	// Agent denies everything, then allows only "read".
	ruleset := permission.Ruleset{
		{Permission: "*", Pattern: "*", Action: permission.ActionAllow},
		{Permission: "*", Pattern: "*", Action: permission.ActionDeny},
		{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
	}

	result := extractAllowedPerms(ruleset)

	if len(result) != 1 || result[0] != "read" {
		t.Fatalf("got %v, want [read]", result)
	}
}

func TestExtractAllowedPerms_AskStaysVisible(t *testing.T) {
	ruleset := permission.Ruleset{
		{Permission: "read", Pattern: "*", Action: permission.ActionAllow},
		{Permission: "read", Pattern: ".env*", Action: permission.ActionAsk},
		{Permission: "webfetch", Pattern: "*", Action: permission.ActionAsk},
	}
	result := extractAllowedPerms(ruleset)
	sort.Strings(result)
	expected := []string{"read", "webfetch"}
	if len(result) != len(expected) {
		t.Fatalf("got %v, want %v", result, expected)
	}
	for i, perm := range expected {
		if result[i] != perm {
			t.Errorf("result[%d] = %q, want %q", i, result[i], perm)
		}
	}
}

func TestExtractAllowedPerms_NilRuleset(t *testing.T) {
	result := extractAllowedPerms(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result for nil ruleset, got %v", result)
	}
}
