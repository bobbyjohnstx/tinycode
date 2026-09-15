package mustgather

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractTarGz extracts a .tar.gz archive to destDir. It returns the number
// of files extracted. Paths are sanitized to prevent directory traversal.
func ExtractTarGz(archivePath, destDir string) (int, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return 0, fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	return extractTar(gz, destDir)
}

func extractTar(r io.Reader, destDir string) (int, error) {
	tr := tar.NewReader(r)
	count := 0

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(destDir, header.Name)
		if !isWithinDir(destDir, target) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return count, fmt.Errorf("creating dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return count, fmt.Errorf("creating parent dir: %w", err)
			}
			out, err := os.Create(target)
			if err != nil {
				return count, fmt.Errorf("creating file: %w", err)
			}
			if _, err := io.Copy(out, io.LimitReader(tr, 500<<20)); err != nil {
				out.Close()
				return count, fmt.Errorf("extracting file: %w", err)
			}
			out.Close()
			count++
		}
	}
	return count, nil
}

func isWithinDir(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}
