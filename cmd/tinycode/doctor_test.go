package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func TestOllamaURL(t *testing.T) {
	const defaultURL = "http://127.0.0.1:11434"

	tests := []struct {
		name     string
		tinycode string
		ollama   string
		want     string
	}{
		{name: "default when unset", want: defaultURL},
		{name: "OLLAMA_HOST fallback", ollama: "http://ollama:11434", want: "http://ollama:11434"},
		{name: "TINYCODE_OLLAMA_HOST preferred", tinycode: "http://tiny:11434", want: "http://tiny:11434"},
		{
			name:     "both set prefers TINYCODE_OLLAMA_HOST",
			tinycode: "http://tiny:11434",
			ollama:   "http://ollama:11434",
			want:     "http://tiny:11434",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TINYCODE_OLLAMA_HOST", tt.tinycode)
			t.Setenv("OLLAMA_HOST", tt.ollama)
			if got := ollamaURL(); got != tt.want {
				t.Errorf("ollamaURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLMStudioURL(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "default", want: "http://127.0.0.1:1234"},
		{name: "override", env: "http://lm:1234", want: "http://lm:1234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TINYCODE_LMSTUDIO_HOST", tt.env)
			if got := lmStudioURL(); got != tt.want {
				t.Errorf("lmStudioURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	if !checkWritable(dir) {
		t.Fatal("expected writable temp dir")
	}
	missing := filepath.Join(dir, "nested", "more")
	if !checkWritable(missing) {
		t.Fatal("expected checkWritable to create nested dirs")
	}
}

func TestCountConfigPlugins(t *testing.T) {
	if got := countConfigPlugins(&config.Info{}); got != 0 {
		t.Fatalf("empty = %d", got)
	}
	cfg := &config.Info{
		Plugins: []json.RawMessage{
			json.RawMessage(`"notify"`),
			json.RawMessage(`["code-review", {"auto": true}]`),
		},
	}
	if got := countConfigPlugins(cfg); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
	bad := &config.Info{Plugins: []json.RawMessage{json.RawMessage(`not-json`)}}
	if got := countConfigPlugins(bad); got != 0 {
		t.Fatalf("invalid plugins = %d, want 0", got)
	}
}

func TestProviderPathUsable_Scenario(t *testing.T) {
	tests := []struct {
		name               string
		anyLocalConnected  bool
		hasCloudAPIKey     bool
		anyConfigConnected bool
		want               bool
	}{
		{
			name:              "cloud only with openrouter key",
			hasCloudAPIKey:    true,
			anyLocalConnected: false,
			want:              true,
		},
		{
			name:              "local only",
			anyLocalConnected: true,
			want:              true,
		},
		{
			name:               "config provider only",
			anyConfigConnected: true,
			want:               true,
		},
		{
			name:              "no local and no cloud key",
			anyLocalConnected: false,
			hasCloudAPIKey:    false,
			want:              false,
		},
		{
			name:               "all paths available",
			anyLocalConnected:  true,
			hasCloudAPIKey:     true,
			anyConfigConnected: true,
			want:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerPathUsable(tt.anyLocalConnected, tt.hasCloudAPIKey, tt.anyConfigConnected)
			if got != tt.want {
				t.Errorf("providerPathUsable(%v, %v, %v) = %v, want %v",
					tt.anyLocalConnected, tt.hasCloudAPIKey, tt.anyConfigConnected, got, tt.want)
			}
		})
	}
}

func TestPrintProviderNextSteps_MentionsConnect(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	printProviderNextSteps()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "/connect") {
		t.Errorf("expected /connect in next steps, got %q", out)
	}
	if !strings.Contains(out, "tinycode") {
		t.Errorf("expected tinycode launch hint in next steps, got %q", out)
	}
}

func stubPATHBinaries(t *testing.T, names ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			path += ".exe"
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestCheckIDEs_DetectsStubsOnPATH(t *testing.T) {
	stubPATHBinaries(t, "code", "cursor", "nvim", "idea")
	cap := captureStdout(t)
	checkIDEs()
	out := cap.read()

	for _, want := range []string{
		"VS Code detected",
		"Cursor detected",
		"Neovim detected",
		"JetBrains detected",
		"docs/acp-integration.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got %q", want, out)
		}
	}
	if strings.Contains(out, "No IDE detected") {
		t.Errorf("did not expect standalone hint when IDEs are on PATH, got %q", out)
	}
	if strings.Contains(out, "install:") {
		t.Errorf("IDE check must not print install actions, got %q", out)
	}
}

func TestCheckIDEs_NoIDEOnPATH(t *testing.T) {
	stubPATHBinaries(t) // empty PATH dir — no IDE binaries
	cap := captureStdout(t)
	checkIDEs()
	out := cap.read()

	if !strings.Contains(out, "No IDE detected") {
		t.Errorf("expected standalone hint when no IDEs on PATH, got %q", out)
	}
	if !strings.Contains(out, "standalone") {
		t.Errorf("expected standalone wording, got %q", out)
	}
	for _, name := range []string{"VS Code detected", "Cursor detected", "Neovim detected", "JetBrains detected"} {
		if strings.Contains(out, name) {
			t.Errorf("did not expect %q when PATH has no IDEs, got %q", name, out)
		}
	}
}

func TestCheckIDEs_PartialDetection(t *testing.T) {
	stubPATHBinaries(t, "nvim")
	cap := captureStdout(t)
	checkIDEs()
	out := cap.read()

	if !strings.Contains(out, "Neovim detected") {
		t.Errorf("expected Neovim detected, got %q", out)
	}
	if !strings.Contains(out, "docs/acp-integration.md") {
		t.Errorf("expected ACP docs pointer, got %q", out)
	}
	if strings.Contains(out, "VS Code detected") {
		t.Errorf("did not expect VS Code when only nvim stubbed, got %q", out)
	}
	if strings.Contains(out, "No IDE detected") {
		t.Errorf("did not expect standalone hint when nvim is present, got %q", out)
	}
}

func TestCheckClipboard_LinuxMissingToolsWarns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("clipboard doctor check is linux-only")
	}
	stubPATHBinaries(t) // empty PATH — no clipboard helpers
	cap := captureStdout(t)
	checkClipboard()
	out := cap.read()

	if !strings.Contains(out, checkWarn) {
		t.Errorf("expected warning marker, got %q", out)
	}
	if !strings.Contains(out, "Clipboard:") {
		t.Errorf("expected Clipboard line, got %q", out)
	}
	if !strings.Contains(out, "error toast") {
		t.Errorf("expected error toast mention, got %q", out)
	}
	if strings.Contains(out, checkFail) {
		t.Errorf("clipboard-only missing must not fail doctor, got %q", out)
	}
	if strings.Contains(out, checkPass) {
		t.Errorf("did not expect pass when no helpers, got %q", out)
	}
}

func TestCheckClipboard_LinuxFindsWlCopy(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("clipboard doctor check is linux-only")
	}
	stubPATHBinaries(t, "wl-copy")
	cap := captureStdout(t)
	checkClipboard()
	out := cap.read()

	if !strings.Contains(out, checkPass) {
		t.Errorf("expected pass when wl-copy present, got %q", out)
	}
	if !strings.Contains(out, "wl-copy found") {
		t.Errorf("expected wl-copy found, got %q", out)
	}
	if strings.Contains(out, checkWarn) {
		t.Errorf("did not expect warn when helper present, got %q", out)
	}
}

func TestCheckClipboard_NonLinuxSkips(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-linux skip path")
	}
	cap := captureStdout(t)
	checkClipboard()
	out := cap.read()
	if out != "" {
		t.Errorf("expected no clipboard output on %s, got %q", runtime.GOOS, out)
	}
}

func TestParseJavaMajorVersion(t *testing.T) {
	tests := []struct {
		name    string
		stderr  string
		want    int
		wantErr bool
	}{
		{name: "OpenJDK21", stderr: `openjdk version "21.0.2" 2024-01-16`, want: 21},
		{name: "Temurin17", stderr: `openjdk version "17.0.1" 2021-10-19`, want: 17},
		{name: "Legacy18", stderr: `java version "1.8.0_392"`, want: 8},
		{name: "Empty", stderr: "", wantErr: true},
		{name: "Garbage", stderr: "not a version string", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseJavaMajorVersion(tt.stderr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got major %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseJavaMajorVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestJavaProjectMarkersPresent(t *testing.T) {
	dir := t.TempDir()
	if javaProjectMarkersPresent(dir) {
		t.Fatal("expected no markers")
	}
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !javaProjectMarkersPresent(dir) {
		t.Fatal("expected pom.xml to gate Java checks")
	}
}

func TestCheckJavaDevTools_SkipsWithoutMarkers(t *testing.T) {
	dir := t.TempDir()
	stubPATHBinaries(t) // empty PATH
	cap := captureStdout(t)
	checkJavaDevTools(dir)
	out := cap.read()
	if out != "" {
		t.Errorf("expected no Java doctor output without markers, got %q", out)
	}
}

func TestCheckJavaDevTools_WarnsNeverFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	// Missing jdtls and java — warn only; checkJavaDevTools has no failed pointer.
	stubPATHBinaries(t)
	cap := captureStdout(t)
	checkJavaDevTools(dir)
	out := cap.read()
	if !strings.Contains(out, checkWarn) {
		t.Errorf("expected warn markers, got %q", out)
	}
	if strings.Contains(out, checkFail) {
		t.Errorf("Java/jdtls warnings must never use checkFail, got %q", out)
	}
	if !strings.Contains(out, "jdtls") {
		t.Errorf("expected jdtls warn, got %q", out)
	}
}

func TestCheckJavaDevTools_PassWithJava21AndJDTLS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	writeStubBin(t, binDir, "jdtls", "#!/bin/sh\n")
	writeStubBin(t, binDir, "java", "#!/bin/sh\necho 'openjdk version \"21.0.2\" 2024-01-16' >&2\n")
	t.Setenv("PATH", binDir)

	cap := captureStdout(t)
	checkJavaDevTools(dir)
	out := cap.read()
	if !strings.Contains(out, checkPass) || !strings.Contains(out, "jdtls") {
		t.Errorf("expected jdtls pass, got %q", out)
	}
	if !strings.Contains(out, "java 21") {
		t.Errorf("expected java 21 pass, got %q", out)
	}
	if strings.Contains(out, checkFail) {
		t.Errorf("must not fail doctor, got %q", out)
	}
}

func TestCheckDevTools_MarkerGatedAndAlwaysOn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubPATHBinaries(t, "gopls", "bash-language-server")
	cap := captureStdout(t)
	checkDevTools(dir)
	out := cap.read()
	if !strings.Contains(out, "gopls") {
		t.Errorf("expected gopls check with go.mod, got %q", out)
	}
	if !strings.Contains(out, "bash-language-server") {
		t.Errorf("expected always-on bash-language-server check, got %q", out)
	}
	if strings.Contains(out, "typescript-language-server") {
		t.Errorf("did not expect TS tooling without package.json/tsconfig, got %q", out)
	}
}

func writeStubBin(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
