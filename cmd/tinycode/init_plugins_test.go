package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestRunInit_WritesOnlyInstalledPlugins(t *testing.T) {
	selected := []string{"ok-a", "fail-b", "ok-c"}
	resolve := func(name string) (string, error) {
		return "", errors.New("not installed")
	}
	install := func(name string) error {
		if name == "fail-b" {
			return errors.New("boom")
		}
		return nil
	}
	ok, failed := ensurePluginsInstalled(selected, resolve, install)
	if len(ok) != 2 || ok[0] != "ok-a" || ok[1] != "ok-c" {
		t.Fatalf("ok=%v, want [ok-a ok-c]", ok)
	}
	if len(failed) != 1 || failed[0] != "fail-b" {
		t.Fatalf("failed=%v, want [fail-b]", failed)
	}

	result := map[string]any{"plugins": ok}
	out, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"plugins":["ok-a","ok-c"]}` {
		t.Fatalf("config JSON = %s", out)
	}
}

func TestRunInit_AllFailWritesEmptyPlugins(t *testing.T) {
	selected := []string{"a", "b"}
	resolve := func(string) (string, error) { return "", errors.New("missing") }
	install := func(string) error { return errors.New("nope") }
	ok, failed := ensurePluginsInstalled(selected, resolve, install)
	if len(ok) != 0 {
		t.Fatalf("ok=%v, want empty", ok)
	}
	if len(failed) != 2 {
		t.Fatalf("failed=%v, want 2", failed)
	}
	if ok == nil {
		ok = []string{}
	}
	result := map[string]any{"plugins": ok}
	out, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"plugins":[]}` {
		t.Fatalf("config JSON = %s", out)
	}
}

func TestRunInit_SkipsDownloadWhenResolved(t *testing.T) {
	selected := []string{"already"}
	var installs int
	resolve := func(name string) (string, error) {
		if name == "already" {
			return "/bin/already", nil
		}
		return "", fmt.Errorf("missing")
	}
	install := func(name string) error {
		installs++
		return nil
	}
	ok, failed := ensurePluginsInstalled(selected, resolve, install)
	if installs != 0 {
		t.Fatalf("install called %d times, want 0", installs)
	}
	if len(failed) != 0 || len(ok) != 1 || ok[0] != "already" {
		t.Fatalf("ok=%v failed=%v", ok, failed)
	}
}
