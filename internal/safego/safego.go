package safego

import (
	"log/slog"
	"runtime/debug"
)

func Go(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in goroutine", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}
