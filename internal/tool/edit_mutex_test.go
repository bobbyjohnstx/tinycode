package tool

import (
	"sync"
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

	// The new mutex is independently usable.
	mu2.Lock()
	mu2.Unlock()
}

func TestFileMutexCleanupSkipsLockedEntries(t *testing.T) {
	// Reset the map for a controlled test.
	fileMutexes.mu.Lock()
	fileMutexes.m = make(map[string]*sync.Mutex)
	fileMutexes.mu.Unlock()

	// Fill past the cleanup threshold.
	for i := 0; i < fileMutexCleanupThreshold+10; i++ {
		getFileMutex("/tmp/cleanup_" + string(rune('a'+i%26)) + "_" + itoa(i))
	}

	// Lock one entry so cleanup must skip it.
	held := getFileMutex("/tmp/cleanup_held")
	held.Lock()
	defer held.Unlock()

	// Trigger cleanup by requesting a new entry beyond the threshold.
	getFileMutex("/tmp/cleanup_trigger")

	// The held entry must still be in the map.
	fileMutexes.mu.Lock()
	_, ok := fileMutexes.m["/tmp/cleanup_held"]
	fileMutexes.mu.Unlock()

	if !ok {
		t.Error("cleanup removed a locked mutex entry")
	}
}

// itoa is a minimal int-to-string to avoid importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
