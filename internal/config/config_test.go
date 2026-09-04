package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSONC_Comments(t *testing.T) {
	input := `{
  // line comment
  "key": "value", /* block comment */
  "num": 42
}`
	json, err := ParseJSONC(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info := &Info{}
	if _, err := ParseConfig(json, nil); err != nil {
		t.Fatalf("failed to parse clean JSON: %v, json: %s", err, json)
	}
	_ = info
}

func TestParseJSONC_TrailingCommas(t *testing.T) {
	input := `{"a": 1, "b": 2,}`
	json, err := ParseJSONC(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if json != `{"a": 1, "b": 2}` {
		t.Errorf("expected trailing comma stripped, got: %s", json)
	}
}

func TestParseJSONC_TrailingCommaInArray(t *testing.T) {
	input := `{"arr": [1, 2, 3,]}`
	json, err := ParseJSONC(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if json != `{"arr": [1, 2, 3]}` {
		t.Errorf("expected trailing comma stripped, got: %s", json)
	}
}

func TestParseJSONC_StringsPreserved(t *testing.T) {
	input := `{"key": "value // not a comment"}`
	json, err := ParseJSONC(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if json != input {
		t.Errorf("expected string preserved, got: %s", json)
	}
}

func TestSubstituteEnvVars(t *testing.T) {
	env := map[string]string{
		"MY_VAR": "hello",
	}
	result := SubstituteEnvVars(`{"url": "{env:MY_VAR}/api"}`, env)
	if result != `{"url": "hello/api"}` {
		t.Errorf("unexpected substitution result: %s", result)
	}
}

func TestSubstituteEnvVars_MissingVar(t *testing.T) {
	result := SubstituteEnvVars(`{"url": "{env:NONEXISTENT_VAR_XYZ}"}`, nil)
	if result != `{"url": ""}` {
		t.Errorf("expected empty substitution for missing var, got: %s", result)
	}
}

func TestSubstituteEnvVars_ProcessEnv(t *testing.T) {
	t.Setenv("TEST_CONFIG_VAR_ABC", "from_env")
	result := SubstituteEnvVars(`{"val": "{env:TEST_CONFIG_VAR_ABC}"}`, nil)
	if result != `{"val": "from_env"}` {
		t.Errorf("expected process env lookup, got: %s", result)
	}
}

func TestParseConfig_ForwardCompatibility(t *testing.T) {
	input := `{
		"shell": "/bin/zsh",
		"unknown_field_xyz": true,
		"another_unknown": {"nested": "value"},
		"model": "ollama/qwen3:8b"
	}`
	info, err := ParseConfig(input, nil)
	if err != nil {
		t.Fatalf("forward compatibility broken — unknown fields should be ignored: %v", err)
	}
	if info.Shell != "/bin/zsh" {
		t.Errorf("expected /bin/zsh, got %s", info.Shell)
	}
	if info.Model != "ollama/qwen3:8b" {
		t.Errorf("expected ollama/qwen3:8b, got %s", info.Model)
	}
}

func TestLoadFile_NonExistent(t *testing.T) {
	info, err := LoadFile("/nonexistent/path/config.json", nil)
	if err != nil {
		t.Fatalf("expected empty Info for missing file, got error: %v", err)
	}
	if info.Shell != "" {
		t.Errorf("expected empty shell, got %s", info.Shell)
	}
}

func TestLoadFile_ValidFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"shell": "/bin/bash", "model": "test/model"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := LoadFile(configPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Shell != "/bin/bash" {
		t.Errorf("expected /bin/bash, got %s", info.Shell)
	}
	if info.Model != "test/model" {
		t.Errorf("expected test/model, got %s", info.Model)
	}
}

func TestMerge(t *testing.T) {
	dst := &Info{
		Shell:    "/bin/zsh",
		Model:    "provider/model-a",
		Provider: map[string]ProviderConfig{"ollama": {NPM: "old"}},
	}
	src := &Info{
		Model:    "provider/model-b",
		Username: "bob",
		Provider: map[string]ProviderConfig{"vllm": {NPM: "new"}},
	}

	result := Merge(dst, src)

	if result.Shell != "/bin/zsh" {
		t.Errorf("expected shell preserved, got %s", result.Shell)
	}
	if result.Model != "provider/model-b" {
		t.Errorf("expected model overridden, got %s", result.Model)
	}
	if result.Username != "bob" {
		t.Errorf("expected username set, got %s", result.Username)
	}
	if _, ok := result.Provider["ollama"]; !ok {
		t.Error("expected ollama provider preserved")
	}
	if _, ok := result.Provider["vllm"]; !ok {
		t.Error("expected vllm provider added")
	}
}

func TestMerge_NilInputs(t *testing.T) {
	info := &Info{Shell: "/bin/zsh"}
	if Merge(info, nil) != info {
		t.Error("Merge with nil src should return dst")
	}
	if Merge(nil, info) != info {
		t.Error("Merge with nil dst should return src")
	}
}

func TestMerge_PermissionsConcatenate(t *testing.T) {
	dst := &Info{
		Permission: &PermissionConfig{
			Allow: []string{"Bash(go test *)"},
		},
	}
	src := &Info{
		Permission: &PermissionConfig{
			Allow: []string{"Bash(go build *)", "Bash(go test *)"},
		},
	}
	result := Merge(dst, src)
	if len(result.Permission.Allow) != 2 {
		t.Errorf("expected 2 deduplicated allow rules, got %d: %v", len(result.Permission.Allow), result.Permission.Allow)
	}
}
