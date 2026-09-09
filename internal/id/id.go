package id

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const randomSuffixLen = 10

var prefixes = map[string]string{
	"job":        "job",
	"event":      "evt",
	"session":    "ses",
	"message":    "msg",
	"permission": "per",
	"question":   "que",
	"part":       "prt",
	"pty":        "pty",
	"tool":       "tool",
	"workspace":  "wrk",
	"plugin":     "plg",
}

var (
	mu            sync.Mutex
	lastTimestamp int64
	counter       int64
)

var base62 = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

func randomBase62(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = base62[buf[i]%62]
	}
	return string(out)
}

func Ascending(prefix string) (string, error) {
	p, ok := prefixes[prefix]
	if !ok {
		return "", fmt.Errorf("unknown prefix: %s", prefix)
	}
	return create(p, false, 0), nil
}

func Descending(prefix string) (string, error) {
	p, ok := prefixes[prefix]
	if !ok {
		return "", fmt.Errorf("unknown prefix: %s", prefix)
	}
	return create(p, true, 0), nil
}

func AscendingOrValidate(prefix string, given string) (string, error) {
	p, ok := prefixes[prefix]
	if !ok {
		return "", fmt.Errorf("unknown prefix: %s", prefix)
	}
	if given == "" {
		return create(p, false, 0), nil
	}
	if !strings.HasPrefix(given, p+"_") {
		return "", fmt.Errorf("id %s does not start with %s", given, p)
	}
	return given, nil
}

func create(prefix string, descending bool, ts int64) string {
	mu.Lock()
	now := ts
	if now == 0 {
		now = time.Now().UnixMilli()
	}
	if now != lastTimestamp {
		lastTimestamp = now
		counter = 0
	}
	counter++
	c := counter
	mu.Unlock()

	encoded := now*0x1000 + c
	if descending {
		encoded = ^encoded
	}

	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, uint64(encoded))
	hexPart := hex.EncodeToString(timeBytes)

	return prefix + "_" + hexPart + randomBase62(randomSuffixLen)
}

func Timestamp(id string) (time.Time, error) {
	parts := strings.SplitN(id, "_", 2)
	if len(parts) != 2 {
		return time.Time{}, errors.New("invalid id format: missing underscore")
	}
	hexStr := parts[1]
	if len(hexStr) < 16 {
		return time.Time{}, errors.New("invalid id format: hex portion too short")
	}
	hexStr = hexStr[:16]

	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hex in id: %w", err)
	}

	encoded := int64(binary.BigEndian.Uint64(decoded))
	ms := encoded / 0x1000

	return time.UnixMilli(ms), nil
}
