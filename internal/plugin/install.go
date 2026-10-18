package plugin

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

const (
	defaultPluginBaseURL = "https://github.com"
	defaultPluginRepo    = "bobbyjohnstx/tinycode"
	maxPluginArchiveSize = 512 << 20 // 512 MiB
	pluginHTTPTimeout    = 90 * time.Second
)

var (
	releaseSemverRE   = regexp.MustCompile(`^(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$`)
	gitDescribeNoiseRE = regexp.MustCompile(`-\d+-g[0-9a-f]+`)

	releaseCacheMu sync.Mutex
	// releaseCache keys: tag|goos|goarch → verified local archive path for this process.
	releaseCache = map[string]string{}
)

// PluginReleaseTag selects the release tag used for plugin bundle downloads (4B/A13).
// Release semver embedded versions always win and ignore envOverride.
// Non-release embedded values (dev, empty, ci, git-describe) require envOverride.
func PluginReleaseTag(embedded, envOverride string) (string, error) {
	emb := strings.TrimSpace(embedded)
	ov := strings.TrimSpace(envOverride)

	if tag, ok := normalizeReleaseTag(emb); ok {
		return tag, nil
	}
	if ov == "" {
		return "", fmt.Errorf("cannot determine plugin release tag from version %q; set TINYCODE_PLUGIN_RELEASE=vX.Y.Z", emb)
	}
	if tag, ok := normalizeReleaseTag(ov); ok {
		return tag, nil
	}
	if strings.HasPrefix(ov, "v") {
		return ov, nil
	}
	return "v" + ov, nil
}

func normalizeReleaseTag(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" || v == "ci" {
		return "", false
	}
	s := strings.TrimPrefix(v, "v")
	if gitDescribeNoiseRE.MatchString(s) {
		return "", false
	}
	m := releaseSemverRE.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return "v" + m[1], true
}

// PluginsArchiveURL builds the download URL for the platform plugins tarball (7B/A6).
func PluginsArchiveURL(base, repo, tag, goos, goarch string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	filename := fmt.Sprintf("tinycode-plugins-%s-%s.tar.gz", goos, goarch)
	return fmt.Sprintf("%s/%s/releases/download/%s/%s", base, repo, tag, filename)
}

func pluginsChecksumsURL(base, repo, tag string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	return fmt.Sprintf("%s/%s/releases/download/%s/checksums-sha256.txt", base, repo, tag)
}

// VerifySHA256 checks that path's SHA-256 matches the line for filename in checksumsBody.
func VerifySHA256(path, checksumsBody, filename string) error {
	want, err := lookupChecksum(checksumsBody, filename)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash archive: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", filename, got, want)
	}
	return nil
}

func lookupChecksum(body, filename string) (string, error) {
	base := filepath.Base(filename)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := filepath.Base(fields[len(fields)-1])
		if name == base {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum entry not found for %s", base)
}

// InstallOptions controls InstallFromRelease behavior (tests inject HTTP client / dirs).
type InstallOptions struct {
	EmbeddedVersion string
	GOOS            string
	GOARCH          string
	BaseURL         string
	Repo            string
	ReleaseOverride string
	DestDir         string // default: ConfigDir()/plugins
	CacheDir        string // default: DataDir()/cache
	HTTPClient      *http.Client
}

// InstallFromRelease downloads the platform plugins tarball, verifies SHA-256,
// and extracts only name into the config plugins directory.
func InstallFromRelease(name string, opts InstallOptions) error {
	if name == "" {
		return fmt.Errorf("plugin name is required")
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return fmt.Errorf("unsupported GOARCH %q for plugin bundles (amd64/arm64 only)", goarch)
	}
	switch goos {
	case "linux", "darwin":
	default:
		return fmt.Errorf("unsupported GOOS %q for plugin bundles (linux/darwin only)", goos)
	}

	base := opts.BaseURL
	if base == "" {
		base = os.Getenv("TINYCODE_BASE_URL")
	}
	if base == "" {
		base = defaultPluginBaseURL
	}
	repo := opts.Repo
	if repo == "" {
		repo = os.Getenv("TINYCODE_REPO")
	}
	if repo == "" {
		repo = defaultPluginRepo
	}
	override := opts.ReleaseOverride
	if override == "" {
		override = os.Getenv("TINYCODE_PLUGIN_RELEASE")
	}
	tag, err := PluginReleaseTag(opts.EmbeddedVersion, override)
	if err != nil {
		return err
	}

	destDir := opts.DestDir
	if destDir == "" {
		destDir = filepath.Join(config.ConfigDir(), "plugins")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}
	destPath := filepath.Join(destDir, name)

	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(config.DataDir(), "cache")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: pluginHTTPTimeout}
	}

	archiveName := fmt.Sprintf("tinycode-plugins-%s-%s.tar.gz", goos, goarch)
	cacheKey := tag + "|" + goos + "|" + goarch

	archivePath, err := cachedOrDownloadArchive(client, cacheKey, cacheDir, archiveName, base, repo, tag)
	if err != nil {
		return err
	}

	if err := extractPluginMember(archivePath, name, destPath); err != nil {
		_ = os.Remove(destPath)
		return err
	}
	return nil
}

func cachedOrDownloadArchive(client *http.Client, cacheKey, cacheDir, archiveName, base, repo, tag string) (string, error) {
	releaseCacheMu.Lock()
	defer releaseCacheMu.Unlock()

	if p, ok := releaseCache[cacheKey]; ok {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		delete(releaseCache, cacheKey)
	}

	checksumsURL := pluginsChecksumsURL(base, repo, tag)
	checksumsBody, err := httpGetLimited(client, checksumsURL)
	if err != nil {
		return "", fmt.Errorf("download checksums: %w", err)
	}

	parts := strings.Split(cacheKey, "|")
	archiveURL := PluginsArchiveURL(base, repo, tag, parts[1], parts[2])

	tmp, err := os.CreateTemp(cacheDir, "plugins-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create temp archive: %w", err)
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := httpDownloadTo(client, archiveURL, tmp); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	if err := VerifySHA256(tmpPath, string(checksumsBody), archiveName); err != nil {
		return "", err
	}

	finalPath := filepath.Join(cacheDir, archiveName+"."+tag)
	_ = os.Remove(finalPath)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		// Cross-device: fall back to keeping temp path in cache map.
		releaseCache[cacheKey] = tmpPath
		ok = true
		return tmpPath, nil
	}
	releaseCache[cacheKey] = finalPath
	ok = true
	return finalPath, nil
}

func httpGetLimited(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("network: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("404 not found: %s", url)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	limited := io.LimitReader(resp.Body, maxPluginArchiveSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("network: %w", err)
	}
	if int64(len(data)) > maxPluginArchiveSize {
		return nil, fmt.Errorf("response exceeds %d byte limit", maxPluginArchiveSize)
	}
	return data, nil
}

func httpDownloadTo(client *http.Client, url string, w io.Writer) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("network: download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("404 not found: %s", url)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, url)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, maxPluginArchiveSize+1))
	if err != nil {
		return fmt.Errorf("network: %w", err)
	}
	if n > maxPluginArchiveSize {
		return fmt.Errorf("archive exceeds %d byte limit", maxPluginArchiveSize)
	}
	return nil
}

func extractPluginMember(archivePath, name, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var found bool
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != 0 {
			continue
		}
		base := filepath.Base(hdr.Name)
		// Accept flat `<name>` or nested `plugins/<name>`.
		clean := filepath.ToSlash(hdr.Name)
		if base != name && clean != "plugins/"+name && !strings.HasSuffix(clean, "/plugins/"+name) {
			continue
		}
		found = true
		if err := atomicWriteExecutable(destPath, tr); err != nil {
			return err
		}
		break
	}
	if !found {
		return fmt.Errorf("plugin %q not found in archive", name)
	}
	return nil
}

func atomicWriteExecutable(destPath string, r io.Reader) error {
	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp dest: %w", err)
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	limited := io.LimitReader(r, maxPluginArchiveSize+1)
	n, err := io.Copy(tmp, limited)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("extract member: %w", err)
	}
	if n > maxPluginArchiveSize {
		tmp.Close()
		return fmt.Errorf("extracted member exceeds %d byte limit", maxPluginArchiveSize)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("install rename: %w", err)
	}
	ok = true
	return nil
}

// ClearReleaseCache clears the process-local download cache (tests only).
func ClearReleaseCache() {
	releaseCacheMu.Lock()
	defer releaseCacheMu.Unlock()
	releaseCache = map[string]string{}
}
