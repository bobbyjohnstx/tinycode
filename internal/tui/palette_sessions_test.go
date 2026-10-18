package tui

import (
	"testing"
	"time"
)

func TestTimeAgo_Table(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		unix  int64
		want  string
	}{
		{"zero", 0, ""},
		{"just now", now.Unix(), "just now"},
		{"30 seconds", now.Add(-30 * time.Second).Unix(), "just now"},
		{"1 minute", now.Add(-1 * time.Minute).Unix(), "1m ago"},
		{"5 minutes", now.Add(-5 * time.Minute).Unix(), "5m ago"},
		{"1 hour", now.Add(-1 * time.Hour).Unix(), "1h ago"},
		{"3 hours", now.Add(-3 * time.Hour).Unix(), "3h ago"},
		{"1 day", now.Add(-24 * time.Hour).Unix(), "1d ago"},
		{"7 days", now.Add(-7 * 24 * time.Hour).Unix(), "7d ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeAgo(tt.unix)
			if got != tt.want {
				t.Errorf("timeAgo(%d) = %q, want %q", tt.unix, got, tt.want)
			}
		})
	}
}
