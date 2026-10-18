package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginReleaseTag_DevRequiresEnv(t *testing.T) {
	for _, emb := range []string{"dev", ""} {
		_, err := PluginReleaseTag(emb, "")
		if err == nil {
			t.Fatalf("embedded %q without env: expected error", emb)
		}
		if !strings.Contains(err.Error(), "TINYCODE_PLUGIN_RELEASE") {
			t.Fatalf("embedded %q: expected TINYCODE_PLUGIN_RELEASE hint, got %v", emb, err)
		}
	}
	tag, err := PluginReleaseTag("dev", "v1.2.3")
	if err != nil {
		t.Fatalf("dev with override: %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("got %q, want v1.2.3", tag)
	}
}

func TestPluginReleaseTag_ReleaseIgnoresEnv(t *testing.T) {
	tag, err := PluginReleaseTag("v1.2.3", "v9.9.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("got %q, want v1.2.3 (override ignored)", tag)
	}
	tag, err = PluginReleaseTag("1.2.3", "v9.9.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("got %q, want v1.2.3", tag)
	}
}

func TestInstallFromRelease_HappyPath(t *testing.T) {
	ClearReleaseCache()
	destDir := t.TempDir()
	cacheDir := t.TempDir()

	archive, sum := mustPluginArchive(t, map[string][]byte{
		"safety-net": []byte("#!/bin/sh\necho ok\n"),
		"other":      []byte("other"),
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums-sha256.txt"):
			fmt.Fprintf(w, "%s  tinycode-plugins-linux-amd64.tar.gz\n", sum)
		case strings.HasSuffix(r.URL.Path, "/tinycode-plugins-linux-amd64.tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := InstallFromRelease("safety-net", InstallOptions{
		EmbeddedVersion: "v1.0.0",
		GOOS:            "linux",
		GOARCH:          "amd64",
		BaseURL:         srv.URL,
		Repo:            "owner/repo",
		DestDir:         destDir,
		CacheDir:        cacheDir,
		HTTPClient:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("InstallFromRelease: %v", err)
	}
	dest := filepath.Join(destDir, "safety-net")
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("dest missing: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("dest not executable")
	}
}

func TestInstallFromRelease_BadChecksum(t *testing.T) {
	ClearReleaseCache()
	destDir := t.TempDir()
	cacheDir := t.TempDir()

	archive, _ := mustPluginArchive(t, map[string][]byte{"safety-net": []byte("bin")})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums-sha256.txt"):
			fmt.Fprintf(w, "%s  tinycode-plugins-linux-amd64.tar.gz\n", strings.Repeat("0", 64))
		case strings.HasSuffix(r.URL.Path, "/tinycode-plugins-linux-amd64.tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := InstallFromRelease("safety-net", InstallOptions{
		EmbeddedVersion: "v1.0.0",
		GOOS:            "linux",
		GOARCH:          "amd64",
		BaseURL:         srv.URL,
		Repo:            "owner/repo",
		DestDir:         destDir,
		CacheDir:        cacheDir,
		HTTPClient:      srv.Client(),
	})
	if err == nil {
		t.Fatal("expected checksum error")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum in error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "safety-net")); !os.IsNotExist(err) {
		t.Fatal("dest should be absent after bad checksum")
	}
}

func TestInstallFromRelease_MissingMember(t *testing.T) {
	ClearReleaseCache()
	destDir := t.TempDir()
	cacheDir := t.TempDir()

	archive, sum := mustPluginArchive(t, map[string][]byte{"other": []byte("bin")})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums-sha256.txt"):
			fmt.Fprintf(w, "%s  tinycode-plugins-linux-amd64.tar.gz\n", sum)
		case strings.HasSuffix(r.URL.Path, "/tinycode-plugins-linux-amd64.tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := InstallFromRelease("safety-net", InstallOptions{
		EmbeddedVersion: "v1.0.0",
		GOOS:            "linux",
		GOARCH:          "amd64",
		BaseURL:         srv.URL,
		Repo:            "owner/repo",
		DestDir:         destDir,
		CacheDir:        cacheDir,
		HTTPClient:      srv.Client(),
	})
	if err == nil {
		t.Fatal("expected missing member error")
	}
	if !strings.Contains(err.Error(), "not found in archive") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "safety-net")); !os.IsNotExist(err) {
		t.Fatal("dest should be absent")
	}
}

func TestInstallFromRelease_MirrorEnv(t *testing.T) {
	ClearReleaseCache()
	destDir := t.TempDir()
	cacheDir := t.TempDir()

	var seenURLs []string
	archive, sum := mustPluginArchive(t, map[string][]byte{"safety-net": []byte("bin")})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenURLs = append(seenURLs, r.URL.String())
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums-sha256.txt"):
			fmt.Fprintf(w, "%s  tinycode-plugins-linux-amd64.tar.gz\n", sum)
		case strings.HasSuffix(r.URL.Path, "/tinycode-plugins-linux-amd64.tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("TINYCODE_BASE_URL", srv.URL)
	t.Setenv("TINYCODE_REPO", "mirror/tinycode")

	err := InstallFromRelease("safety-net", InstallOptions{
		EmbeddedVersion: "v1.0.0",
		GOOS:            "linux",
		GOARCH:          "amd64",
		DestDir:         destDir,
		CacheDir:        cacheDir,
		HTTPClient:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("InstallFromRelease: %v", err)
	}
	joined := strings.Join(seenURLs, "\n")
	if !strings.Contains(joined, "/mirror/tinycode/releases/download/v1.0.0/") {
		t.Fatalf("expected mirror repo in URLs, got:\n%s", joined)
	}
}

func TestInstallFromRelease_ExtractOnlyRequested(t *testing.T) {
	ClearReleaseCache()
	destDir := t.TempDir()
	cacheDir := t.TempDir()

	archive, sum := mustPluginArchive(t, map[string][]byte{
		"safety-net": []byte("a"),
		"telemetry":  []byte("b"),
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums-sha256.txt"):
			fmt.Fprintf(w, "%s  tinycode-plugins-linux-amd64.tar.gz\n", sum)
		case strings.HasSuffix(r.URL.Path, "/tinycode-plugins-linux-amd64.tar.gz"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := InstallFromRelease("safety-net", InstallOptions{
		EmbeddedVersion: "v1.0.0",
		GOOS:            "linux",
		GOARCH:          "amd64",
		BaseURL:         srv.URL,
		Repo:            "owner/repo",
		DestDir:         destDir,
		CacheDir:        cacheDir,
		HTTPClient:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("InstallFromRelease: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "safety-net")); err != nil {
		t.Fatalf("safety-net missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "telemetry")); !os.IsNotExist(err) {
		t.Fatal("telemetry should not be extracted")
	}
}

func mustPluginArchive(t *testing.T, files map[string][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:])
}
