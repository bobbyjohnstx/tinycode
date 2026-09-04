package id

import (
	"strings"
	"testing"
	"time"
)

func TestAscending_ValidPrefix(t *testing.T) {
	id, err := Ascending("session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(id, "ses_") {
		t.Errorf("expected prefix ses_, got %s", id)
	}
}

func TestAscending_InvalidPrefix(t *testing.T) {
	_, err := Ascending("bogus")
	if err == nil {
		t.Fatal("expected error for unknown prefix")
	}
}

func TestDescending_ValidPrefix(t *testing.T) {
	id, err := Descending("event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(id, "evt_") {
		t.Errorf("expected prefix evt_, got %s", id)
	}
}

func TestAscendingOrValidate_GeneratesNew(t *testing.T) {
	id, err := AscendingOrValidate("session", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(id, "ses_") {
		t.Errorf("expected prefix ses_, got %s", id)
	}
}

func TestAscendingOrValidate_AcceptsValid(t *testing.T) {
	id, err := AscendingOrValidate("session", "ses_abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ses_abc123" {
		t.Errorf("expected ses_abc123, got %s", id)
	}
}

func TestAscendingOrValidate_RejectsWrongPrefix(t *testing.T) {
	_, err := AscendingOrValidate("session", "msg_abc123")
	if err == nil {
		t.Fatal("expected error for wrong prefix")
	}
}

func TestMonotonicOrdering(t *testing.T) {
	var ids []string
	for i := 0; i < 100; i++ {
		id, err := Ascending("session")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ids = append(ids, id)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("IDs not monotonically ascending at index %d: %s <= %s", i, ids[i], ids[i-1])
		}
	}
}

func TestTimestamp_RoundTrip(t *testing.T) {
	before := time.Now().Truncate(time.Millisecond)
	id, err := Ascending("message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now().Truncate(time.Millisecond).Add(time.Millisecond)

	ts, err := Timestamp(id)
	if err != nil {
		t.Fatalf("timestamp extraction failed: %v", err)
	}

	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not in expected range [%v, %v]", ts, before, after)
	}
}

func TestTimestamp_InvalidFormat(t *testing.T) {
	_, err := Timestamp("nounderscore")
	if err == nil {
		t.Fatal("expected error for missing underscore")
	}

	_, err = Timestamp("x_abc")
	if err == nil {
		t.Fatal("expected error for short hex portion")
	}
}

func TestAllPrefixes(t *testing.T) {
	for name, prefix := range prefixes {
		id, err := Ascending(name)
		if err != nil {
			t.Errorf("prefix %s (%s): unexpected error: %v", name, prefix, err)
			continue
		}
		if !strings.HasPrefix(id, prefix+"_") {
			t.Errorf("prefix %s: expected %s_, got %s", name, prefix, id)
		}
	}
}

func TestIDLength(t *testing.T) {
	id, _ := Ascending("session")
	// ses_ + 16 hex + 10 random = 4 + 16 + 10 = 30
	if len(id) != 30 {
		t.Errorf("expected length 30, got %d for id %s", len(id), id)
	}
}

func TestUniqueIDs(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		id, _ := Ascending("session")
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate ID at iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}
