package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir_EnvOverride(t *testing.T) {
	t.Setenv("TINYCODE_CONFIG_DIR", "/custom/config")
	t.Setenv("XDG_CONFIG_HOME", "/should/be/ignored")

	got := ConfigDir()
	if got != "/custom/config" {
		t.Errorf("expected /custom/config, got %s", got)
	}
}

func TestConfigDir_XDGOverride(t *testing.T) {
	t.Setenv("TINYCODE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")

	got := ConfigDir()
	want := filepath.Join("/xdg/config", "tinycode")
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestConfigDir_DefaultPath(t *testing.T) {
	t.Setenv("TINYCODE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "tinycode")

	got := ConfigDir()
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestDataDir_EnvOverride(t *testing.T) {
	t.Setenv("TINYCODE_DATA_DIR", "/custom/data")
	t.Setenv("XDG_DATA_HOME", "/should/be/ignored")

	got := DataDir()
	if got != "/custom/data" {
		t.Errorf("expected /custom/data, got %s", got)
	}
}

func TestDataDir_XDGOverride(t *testing.T) {
	t.Setenv("TINYCODE_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	got := DataDir()
	want := filepath.Join("/xdg/data", "tinycode")
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestDataDir_DefaultPath(t *testing.T) {
	t.Setenv("TINYCODE_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "share", "tinycode")

	got := DataDir()
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestGlobalConfigFile_ThreeNameFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TINYCODE_CONFIG_DIR", dir)

	// No files exist — should return tinycode.jsonc as default
	got := GlobalConfigFile()
	if filepath.Base(got) != "tinycode.jsonc" {
		t.Errorf("expected default tinycode.jsonc, got %s", got)
	}

	// Create config.json — should find it
	configJSON := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configJSON, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = GlobalConfigFile()
	if filepath.Base(got) != "config.json" {
		t.Errorf("expected config.json, got %s", got)
	}

	// Create tinycode.json — should prefer it over config.json
	tinycodeJSON := filepath.Join(dir, "tinycode.json")
	if err := os.WriteFile(tinycodeJSON, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = GlobalConfigFile()
	if filepath.Base(got) != "tinycode.json" {
		t.Errorf("expected tinycode.json, got %s", got)
	}

	// Create tinycode.jsonc — should prefer it over both
	tinycodeJSONC := filepath.Join(dir, "tinycode.jsonc")
	if err := os.WriteFile(tinycodeJSONC, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = GlobalConfigFile()
	if filepath.Base(got) != "tinycode.jsonc" {
		t.Errorf("expected tinycode.jsonc, got %s", got)
	}
}

func TestProjectConfigFiles_WalksUpTree(t *testing.T) {
	// Create: root/sub/deep
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	deep := filepath.Join(sub, "deep")
	os.MkdirAll(deep, 0o755)

	// Place config at root and deep levels
	rootCfg := filepath.Join(root, "tinycode.json")
	deepCfg := filepath.Join(deep, "tinycode.json")
	os.WriteFile(rootCfg, []byte("{}"), 0o644)
	os.WriteFile(deepCfg, []byte("{}"), 0o644)

	files := ProjectConfigFiles("tinycode", deep)

	if len(files) < 2 {
		t.Fatalf("expected at least 2 config files, got %d: %v", len(files), files)
	}

	// First should be outermost (root), last should be innermost (deep)
	if files[0] != rootCfg {
		t.Errorf("expected first file to be root config %s, got %s", rootCfg, files[0])
	}
	if files[len(files)-1] != deepCfg {
		t.Errorf("expected last file to be deep config %s, got %s", deepCfg, files[len(files)-1])
	}
}

func TestProjectDotDirs_WalksUpTree(t *testing.T) {
	// Create: root/.tinycode and root/sub/.tinycode
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	os.MkdirAll(filepath.Join(root, ".tinycode"), 0o755)
	os.MkdirAll(filepath.Join(sub, ".tinycode"), 0o755)

	dirs := ProjectDotDirs(sub)

	if len(dirs) < 2 {
		t.Fatalf("expected at least 2 dot dirs, got %d: %v", len(dirs), dirs)
	}

	// First should be outermost (root), last should be innermost (sub)
	if dirs[0] != filepath.Join(root, ".tinycode") {
		t.Errorf("expected first dir to be root .tinycode, got %s", dirs[0])
	}
	if dirs[len(dirs)-1] != filepath.Join(sub, ".tinycode") {
		t.Errorf("expected last dir to be sub .tinycode, got %s", dirs[len(dirs)-1])
	}
}
