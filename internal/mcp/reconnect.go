package mcp

import (
	"math"
	"math/rand"
	"time"
)

const (
	reconnectBaseDelay   = 1 * time.Second
	reconnectMaxDelay    = 30 * time.Second
	reconnectJitter      = 0.25
	maxReconnectAttempts = 10
)

// backoffDelay returns an exponential backoff duration with jitter.
// Starts at 1s, doubles each attempt, caps at 30s, with 25% jitter.
func backoffDelay(attempt int) time.Duration {
	base := float64(reconnectBaseDelay) * math.Pow(2, float64(attempt))
	if base > float64(reconnectMaxDelay) {
		base = float64(reconnectMaxDelay)
	}
	jitter := base * reconnectJitter * (2*rand.Float64() - 1)
	return time.Duration(base + jitter)
}
