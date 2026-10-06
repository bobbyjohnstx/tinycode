package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestMerge_ExperimentalConfig(t *testing.T) {
	dst := &Info{}
	src := &Info{
		Experimental: &ExperimentalConfig{
			DoomLoopThreshold: 5,
			AutoContinue:      2,
		},
	}
	result := Merge(dst, src)
	if result.Experimental == nil {
		t.Fatal("expected experimental config to be set")
	}
	if result.Experimental.DoomLoopThreshold != 5 {
		t.Errorf("expected doom_loop_threshold 5, got %d", result.Experimental.DoomLoopThreshold)
	}
	if result.Experimental.AutoContinue != 2 {
		t.Errorf("expected auto_continue 2, got %d", result.Experimental.AutoContinue)
	}
}

func TestMerge_TemperatureTopPMaxTokens(t *testing.T) {
	temp := 0.5
	topP := 0.8
	maxTok := 2048
	dst := &Info{}
	src := &Info{
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTok,
	}
	result := Merge(dst, src)
	if result.Temperature == nil || *result.Temperature != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", result.Temperature)
	}
	if result.TopP == nil || *result.TopP != 0.8 {
		t.Errorf("expected top_p 0.8, got %v", result.TopP)
	}
	if result.MaxTokens == nil || *result.MaxTokens != 2048 {
		t.Errorf("expected max_tokens 2048, got %v", result.MaxTokens)
	}
}

func TestMerge_SkillsConfig(t *testing.T) {
	dst := &Info{}
	src := &Info{
		Skills: &SkillsConfig{
			Paths: []string{"/path/to/skills"},
			URLs:  []string{"https://example.com/skills"},
		},
	}
	result := Merge(dst, src)
	if result.Skills == nil {
		t.Fatal("expected skills config")
	}
	if len(result.Skills.Paths) != 1 || result.Skills.Paths[0] != "/path/to/skills" {
		t.Errorf("unexpected skills paths: %v", result.Skills.Paths)
	}
}

func TestMerge_AttachmentConfig(t *testing.T) {
	maxSize := 1024
	dst := &Info{}
	src := &Info{
		Attachment: &AttachmentConfig{
			MaxSize: &maxSize,
			Image: &ImageConfig{
				MaxWidth: 800,
				Format:   "webp",
			},
		},
	}
	result := Merge(dst, src)
	if result.Attachment == nil {
		t.Fatal("expected attachment config")
	}
	if *result.Attachment.MaxSize != 1024 {
		t.Errorf("expected max_size 1024, got %d", *result.Attachment.MaxSize)
	}
	if result.Attachment.Image == nil || result.Attachment.Image.MaxWidth != 800 {
		t.Error("expected image config with max_width 800")
	}
}

func TestMerge_CommandAndReference(t *testing.T) {
	dst := &Info{
		Command: map[string]string{"build": "make build"},
	}
	src := &Info{
		Command:   map[string]string{"test": "go test ./..."},
		Reference: map[string]string{"docs": "https://docs.example.com"},
	}
	result := Merge(dst, src)
	if result.Command["build"] != "make build" {
		t.Error("expected 'build' command preserved")
	}
	if result.Command["test"] != "go test ./..." {
		t.Error("expected 'test' command added")
	}
	if result.Reference["docs"] != "https://docs.example.com" {
		t.Error("expected 'docs' reference added")
	}
}

func TestMerge_Watcher(t *testing.T) {
	dst := &Info{Watcher: []string{"*.go"}}
	src := &Info{Watcher: []string{"*.js", "*.go"}}
	result := Merge(dst, src)
	if len(result.Watcher) != 2 {
		t.Errorf("expected 2 deduplicated watchers, got %d: %v", len(result.Watcher), result.Watcher)
	}
}

func TestParseConfig_ExperimentalConfig(t *testing.T) {
	input := `{
		"experimental": {
			"doom_loop_threshold": 5,
			"auto_continue": 3
		},
		"temperature": 0.7
	}`
	info, err := ParseConfig(input, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if info.Experimental == nil {
		t.Fatal("expected experimental config")
	}
	if info.Experimental.DoomLoopThreshold != 5 {
		t.Errorf("expected doom_loop_threshold 5, got %d", info.Experimental.DoomLoopThreshold)
	}
	if info.Temperature == nil || *info.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", info.Temperature)
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

func TestMCPConfig_UnmarshalJSON_CommandAndArgs(t *testing.T) {
	var m MCPConfig
	err := json.Unmarshal([]byte(`{
		"command": "npx",
		"args": ["-y", "@my/mcp-server"]
	}`), &m)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Command != "npx" {
		t.Errorf("Command = %q, want npx", m.Command)
	}
	wantArgs := []string{"-y", "@my/mcp-server"}
	if !reflect.DeepEqual(m.Args, wantArgs) {
		t.Errorf("Args = %#v, want %#v", m.Args, wantArgs)
	}
}

func TestMCPConfig_UnmarshalJSON_CommandArray(t *testing.T) {
	var m MCPConfig
	err := json.Unmarshal([]byte(`{
		"command": ["npx", "-y", "@my/mcp-server"]
	}`), &m)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Command != "npx" {
		t.Errorf("Command = %q, want npx", m.Command)
	}
	wantArgs := []string{"-y", "@my/mcp-server"}
	if !reflect.DeepEqual(m.Args, wantArgs) {
		t.Errorf("Args = %#v, want %#v", m.Args, wantArgs)
	}
}

func TestMCPConfig_UnmarshalJSON_ExplicitArgsWinsOverCommandArray(t *testing.T) {
	var m MCPConfig
	err := json.Unmarshal([]byte(`{
		"command": ["npx", "ignored"],
		"args": ["-y", "@explicit"]
	}`), &m)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Command != "npx" {
		t.Errorf("Command = %q, want npx", m.Command)
	}
	wantArgs := []string{"-y", "@explicit"}
	if !reflect.DeepEqual(m.Args, wantArgs) {
		t.Errorf("Args = %#v, want %#v", m.Args, wantArgs)
	}
}

func TestMCPConfig_MarshalJSON_IncludesCommandAndArgs(t *testing.T) {
	m := MCPConfig{
		Command: "npx",
		Args:    []string{"-y", "@my/mcp-server"},
		URL:     "http://example.com",
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if got["command"] != "npx" {
		t.Errorf("command = %v, want npx", got["command"])
	}
	args, ok := got["args"].([]any)
	if !ok || len(args) != 2 {
		t.Fatalf("args = %#v, want 2 elements", got["args"])
	}
}

func TestLoad_PreferredGlobalOnly_JsoncWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_CONFIG_DIR", dir)

	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"model":"from-config-json"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tinycode.json"), []byte(`{"model":"from-tinycode-json"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tinycode.jsonc"), []byte(`{"model":"from-jsonc"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if info.Model != "from-jsonc" {
		t.Errorf("model = %q, want from-jsonc (jsonc exclusively)", info.Model)
	}
}

func TestLoad_ProjectDotTinycode(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TINYCODE_CONFIG_DIR", t.TempDir()) // isolate global

	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(sub, ".tinycode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tinycode.json"), []byte(`{"model":"root","shell":"/bin/root"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".tinycode", "tinycode.json"), []byte(`{"model":"dot-inner"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Load(sub)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if info.Model != "dot-inner" {
		t.Errorf("model = %q, want dot-inner", info.Model)
	}
	if info.Shell != "/bin/root" {
		t.Errorf("shell = %q, want /bin/root preserved from outer", info.Shell)
	}
}

func TestLoad_GlobalParseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "tinycode.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected parse error for preferred global file")
	}
}

func TestMerge_ServerPortKeepsHost(t *testing.T) {
	port := 4096
	port2 := 8080
	dst := &Info{Server: &ServerConfig{Host: "127.0.0.1", Port: &port}}
	src := &Info{Server: &ServerConfig{Port: &port2}}
	result := Merge(dst, src)
	if result.Server == nil {
		t.Fatal("expected server config")
	}
	if result.Server.Host != "127.0.0.1" {
		t.Errorf("host = %q, want 127.0.0.1", result.Server.Host)
	}
	if result.Server.Port == nil || *result.Server.Port != 8080 {
		t.Errorf("port = %v, want 8080", result.Server.Port)
	}
}
