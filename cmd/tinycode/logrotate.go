package main

import (
	"fmt"
	"os"
	"sync"
)

const (
	logMaxBytes = 5 << 20
	logMaxFiles = 3
)

// rotatingFile appends to path and renames it once the next write would pass max.
// Older files are path.1 through path.keep. The oldest is removed.
type rotatingFile struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
	max  int64
	keep int
}

func openRotatingLog(path string, max int64, keep int) (*rotatingFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &rotatingFile{path: path, f: f, size: info.Size(), max: max, keep: keep}, nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.max > 0 && r.size+int64(len(p)) > r.max {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

func (r *rotatingFile) rotate() error {
	if r.f != nil {
		if err := r.f.Close(); err != nil {
			return err
		}
		r.f = nil
	}
	if r.keep > 0 {
		_ = os.Remove(fmt.Sprintf("%s.%d", r.path, r.keep))
		for i := r.keep - 1; i >= 1; i-- {
			_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
		}
		_ = os.Rename(r.path, r.path+".1")
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	r.f = f
	r.size = 0
	return nil
}
