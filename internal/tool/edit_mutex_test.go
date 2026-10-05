package tool

import (
	"testing"
)

func TestFileMutexConcurrentFiles(t *testing.T) {
	// Two goroutines can hold file mutexes for different files simultaneously.
	mu1 := getFileMutex("/tmp/file1.txt")
	mu2 := getFileMutex("/tmp/file2.txt")

	mu1.Lock()
	defer mu1.Unlock()

	// mu2 for a different file must be independently lockable.
	locked := make(chan struct{})
	go func() {
		mu2.Lock()
		close(locked)
		mu2.Unlock()
	}()

	<-locked // would deadlock if mu1 and mu2 were the same mutex
}

func TestFileMutexSameFile(t *testing.T) {
	// Two calls with the same path return the same mutex.
	mu1 := getFileMutex("/tmp/same.txt")
	mu2 := getFileMutex("/tmp/same.txt")

	if mu1 != mu2 {
		t.Error("getFileMutex returned different mutexes for the same path")
	}
}

func TestClearFileMutexesDoesNotBreakHeldLocks(t *testing.T) {
	// A mutex obtained before ClearFileMutexes still provides mutual
	// exclusion for its holder — the clear only affects future lookups.
	mu := getFileMutex("/tmp/held.txt")
	mu.Lock()

	ClearFileMutexes()

	// After clear, a new lookup returns a different mutex object.
	mu2 := getFileMutex("/tmp/held.txt")
	if mu == mu2 {
		t.Error("ClearFileMutexes did not replace the map")
	}

	// The original mutex is still held and can be unlocked without panic.
	mu.Unlock()
}

