package safego

import (
	"log/slog"
	"runtime/debug"
)

// LogPanic logs a recovered panic with a stack trace. If logger is nil,
// slog.Default() is used. Extra attrs are included before panic/stack.
func LogPanic(logger *slog.Logger, msg string, r any, args ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	args = append(args, "panic", r, "stack", string(debug.Stack()))
	logger.Error(msg, args...)
}

func Go(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				LogPanic(nil, "recovered panic in goroutine", r)
			}
		}()
		fn()
	}()
}
