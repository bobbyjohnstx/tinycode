package plugin

import (
	"encoding/json"
	"testing"
)

func TestPluginSpec_UnmarshalJSON_String(t *testing.T) {
	var spec PluginSpec
	if err := json.Unmarshal([]byte(`"notify"`), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Name != "notify" {
		t.Errorf("expected name 'notify', got %q", spec.Name)
	}
	if spec.Options != nil {
		t.Errorf("expected nil options for string spec, got %v", spec.Options)
	}
}

func TestPluginSpec_UnmarshalJSON_Tuple(t *testing.T) {
	var spec PluginSpec
	if err := json.Unmarshal([]byte(`["code-review", {"auto": true}]`), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Name != "code-review" {
		t.Errorf("expected name 'code-review', got %q", spec.Name)
	}
	if spec.Options == nil {
		t.Fatal("expected options to be set")
	}
	if v, ok := spec.Options["auto"]; !ok || v != true {
		t.Errorf("expected options.auto=true, got %v", spec.Options)
	}
}

func TestPluginSpec_UnmarshalJSON_TupleNameOnly(t *testing.T) {
	var spec PluginSpec
	if err := json.Unmarshal([]byte(`["notify"]`), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Name != "notify" {
		t.Errorf("expected name 'notify', got %q", spec.Name)
	}
	if spec.Options != nil {
		t.Errorf("expected nil options, got %v", spec.Options)
	}
}

func TestPluginSpec_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var spec PluginSpec
	if err := json.Unmarshal([]byte(`{}`), &spec); err == nil {
		t.Fatal("expected error for object input")
	}
}

func TestPluginSpec_UnmarshalJSON_EmptyTuple(t *testing.T) {
	var spec PluginSpec
	if err := json.Unmarshal([]byte(`[]`), &spec); err == nil {
		t.Fatal("expected error for empty tuple")
	}
}

func TestPluginSpec_UnmarshalJSON_TupleTooLong(t *testing.T) {
	var spec PluginSpec
	if err := json.Unmarshal([]byte(`["a", {}, "extra"]`), &spec); err == nil {
		t.Fatal("expected error for tuple with 3 elements")
	}
}

func TestParsePluginConfig(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`"notify"`),
		json.RawMessage(`["code-review", {"auto": true}]`),
	}
	specs, err := ParsePluginConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Name != "notify" {
		t.Errorf("specs[0].Name: expected 'notify', got %q", specs[0].Name)
	}
	if specs[1].Name != "code-review" {
		t.Errorf("specs[1].Name: expected 'code-review', got %q", specs[1].Name)
	}
}

func TestParsePluginConfig_InvalidEntry(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`"ok"`),
		json.RawMessage(`123`),
	}
	_, err := ParsePluginConfig(raw)
	if err == nil {
		t.Fatal("expected error for invalid entry")
	}
}

func TestParsePluginConfig_Empty(t *testing.T) {
	specs, err := ParsePluginConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("expected empty result, got %d specs", len(specs))
	}
}
