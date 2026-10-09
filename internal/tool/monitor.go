package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const (
	defaultMonitorTimeout = 300 * time.Second  // 5 minutes
	maxMonitorTimeout     = 1800 * time.Second // 30 minutes
	maxMonitors           = 5
	ringBufferSize        = 100
)

// ringBuffer holds the last N lines.
type ringBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{
		lines: make([]string, 0, max),
		max:   max,
	}
}

func (rb *ringBuffer) Add(line string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if len(rb.lines) >= rb.max {
		copy(rb.lines, rb.lines[1:])
		rb.lines = rb.lines[:rb.max-1]
	}
	rb.lines = append(rb.lines, line)
}

// Drain returns all buffered lines joined by newline and clears the buffer.
func (rb *ringBuffer) Drain() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if len(rb.lines) == 0 {
		return ""
	}
	result := strings.Join(rb.lines, "\n")
	rb.lines = rb.lines[:0]
	return result
}

// Len returns the current number of buffered lines.
func (rb *ringBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return len(rb.lines)
}

type monitorProcess struct {
	id          string
	description string
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	output      *ringBuffer
	started     time.Time
	deadline    time.Time
	done        chan struct{}
}

// MonitorManager manages background monitor processes.
type MonitorManager struct {
	mu       sync.Mutex
	monitors map[string]*monitorProcess
	seq      int
	maxCount int
}

// NewMonitorManager creates a new MonitorManager.
func NewMonitorManager() *MonitorManager {
	return &MonitorManager{
		monitors: make(map[string]*monitorProcess),
		maxCount: maxMonitors,
	}
}

// Start launches a background command and returns the monitor ID.
func (mm *MonitorManager) Start(ctx context.Context, command, description, dir string, timeout time.Duration) (string, error) {
	mm.mu.Lock()

	// Prune finished monitors.
	for id, m := range mm.monitors {
		select {
		case <-m.done:
			delete(mm.monitors, id)
		default:
		}
	}

	if len(mm.monitors) >= mm.maxCount {
		mm.mu.Unlock()
		return "", fmt.Errorf("maximum concurrent monitors (%d) reached", mm.maxCount)
	}

	mm.seq++
	monitorID := fmt.Sprintf("mon_%d", mm.seq)

	monCtx, cancel := context.WithTimeout(ctx, timeout)
	cmd := exec.CommandContext(monCtx, "sh", "-c", command)
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Dir = dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		mm.mu.Unlock()
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		mm.mu.Unlock()
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	now := time.Now()
	mon := &monitorProcess{
		id:          monitorID,
		description: description,
		cmd:         cmd,
		cancel:      cancel,
		output:      newRingBuffer(ringBufferSize),
		started:     now,
		deadline:    now.Add(timeout),
		done:        make(chan struct{}),
	}
	mm.monitors[monitorID] = mon
	mm.mu.Unlock()

	if err := cmd.Start(); err != nil {
		cancel()
		mm.mu.Lock()
		delete(mm.monitors, monitorID)
		mm.mu.Unlock()
		close(mon.done)
		return "", fmt.Errorf("start command: %w", err)
	}

	// Scan stdout and stderr into the ring buffer.
	var wg sync.WaitGroup
	scanInto := func(r io.Reader, prefix string) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if prefix != "" {
				line = prefix + line
			}
			mon.output.Add(line)
		}
	}
	wg.Add(2)
	safego.Go(func() { scanInto(stdout, "") })
	safego.Go(func() { scanInto(stderr, "STDERR: ") })

	safego.Go(func() {
		_ = cmd.Wait()
		wg.Wait()
		close(mon.done)
		slog.Info("monitor finished", "id", monitorID, "description", description)
	})

	slog.Info("monitor started", "id", monitorID, "command", command, "dir", dir, "timeout", timeout)
	return monitorID, nil
}

// Stop cancels a running monitor.
func (mm *MonitorManager) Stop(id string) error {
	mm.mu.Lock()
	mon, ok := mm.monitors[id]
	mm.mu.Unlock()

	if !ok {
		return fmt.Errorf("monitor %s not found", id)
	}

	mon.cancel()
	<-mon.done

	mm.mu.Lock()
	delete(mm.monitors, id)
	mm.mu.Unlock()

	return nil
}

// MonitorInfo describes a running monitor for listing.
type MonitorInfo struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Started     string `json:"started"`
	Deadline    string `json:"deadline"`
	BufferedLen int    `json:"buffered_lines"`
	Running     bool   `json:"running"`
}

// List returns info about all monitors.
func (mm *MonitorManager) List() []MonitorInfo {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	result := make([]MonitorInfo, 0, len(mm.monitors))
	for _, mon := range mm.monitors {
		running := true
		select {
		case <-mon.done:
			running = false
		default:
		}
		result = append(result, MonitorInfo{
			ID:          mon.id,
			Description: mon.description,
			Started:     mon.started.Format(time.RFC3339),
			Deadline:    mon.deadline.Format(time.RFC3339),
			BufferedLen: mon.output.Len(),
			Running:     running,
		})
	}
	return result
}

// DrainAll returns buffered output from all monitors that have accumulated
// output since the last drain. Returns empty string if nothing to report.
func (mm *MonitorManager) DrainAll() string {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	var parts []string
	for _, mon := range mm.monitors {
		output := mon.output.Drain()
		if output == "" {
			continue
		}
		desc := mon.description
		if desc == "" {
			desc = mon.id
		}
		running := true
		select {
		case <-mon.done:
			running = false
		default:
		}
		status := "running"
		if !running {
			status = "exited"
		}
		parts = append(parts, fmt.Sprintf("[Monitor %s (%s) — %s]\n%s", mon.id, desc, status, output))
	}

	// Clean up finished monitors after draining.
	for id, mon := range mm.monitors {
		select {
		case <-mon.done:
			delete(mm.monitors, id)
		default:
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// Shutdown cancels all running monitors.
func (mm *MonitorManager) Shutdown() {
	mm.mu.Lock()
	monitors := make([]*monitorProcess, 0, len(mm.monitors))
	for _, mon := range mm.monitors {
		monitors = append(monitors, mon)
	}
	mm.mu.Unlock()

	for _, mon := range monitors {
		mon.cancel()
		<-mon.done
	}

	mm.mu.Lock()
	mm.monitors = make(map[string]*monitorProcess)
	mm.mu.Unlock()
}

type monitorArgs struct {
	Command     string `json:"command"`
	TimeoutMS   *int   `json:"timeout_ms,omitempty"`
	Description string `json:"description,omitempty"`
	Action      string `json:"action,omitempty"`
	MonitorID   string `json:"monitor_id,omitempty"`
}

// MonitorTool returns the tool definition for the monitor tool.
func MonitorTool() *Def {
	return &Def{
		ID:          "monitor",
		Description: "Run a command in the background and watch its output. Output is buffered and delivered at the next turn boundary. Use action='list' to see running monitors or action='stop' with monitor_id to stop one.",
		Permission:  "shell",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to run in the background",
				},
				"timeout_ms": map[string]any{
					"type":        "integer",
					"description": "How long to run in milliseconds (default 300000 = 5 min, max 1800000 = 30 min)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Label for this monitor (shown in listings)",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action to perform: 'start' (default), 'stop', or 'list'",
					"enum":        []string{"start", "stop", "list"},
				},
				"monitor_id": map[string]any{
					"type":        "string",
					"description": "Monitor ID for stop operations",
				},
			},
		},
		Execute: executeMonitor,
	}
}

func executeMonitor(ctx context.Context, tc *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args monitorArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if tc.MonitorManager == nil {
		return &ExecuteResult{Output: "Monitor manager not available", IsError: true}, nil
	}

	action := args.Action
	if action == "" {
		if args.MonitorID != "" && args.Command == "" {
			action = "stop"
		} else {
			action = "start"
		}
	}

	switch action {
	case "list":
		monitors := tc.MonitorManager.List()
		result, _ := json.Marshal(monitors)
		return &ExecuteResult{Output: string(result)}, nil

	case "stop":
		if args.MonitorID == "" {
			return &ExecuteResult{Output: "monitor_id is required for stop action", IsError: true}, nil
		}
		if err := tc.MonitorManager.Stop(args.MonitorID); err != nil {
			return &ExecuteResult{Output: fmt.Sprintf("Failed to stop monitor: %v", err), IsError: true}, nil
		}
		return &ExecuteResult{Output: fmt.Sprintf("Monitor %s stopped", args.MonitorID)}, nil

	case "start":
		if args.Command == "" {
			return &ExecuteResult{Output: "command is required to start a monitor", IsError: true}, nil
		}

		if blocked := gateSecretAccess(ctx, tc, args.Command); blocked != nil {
			return blocked, nil
		}

		if IsDestructive(args.Command) {
			if tc.Perms != nil {
				askErr := tc.Perms.Ask(ctx, permission.AskInput{
					SessionID:  tc.SessionID,
					Permission: "destructive-shell",
					Patterns:   []string{args.Command},
					Metadata:   map[string]any{"command": args.Command},
					Ruleset:    tc.Ruleset,
				})
				if askErr != nil {
					return &ExecuteResult{Output: askErr.Error(), IsError: true}, nil
				}
			} else {
				return &ExecuteResult{
					Output:  fmt.Sprintf("Potentially destructive command detected: %s\nUse with caution.", args.Command),
					IsError: true,
				}, nil
			}
		}

		timeout := defaultMonitorTimeout
		if args.TimeoutMS != nil {
			t := time.Duration(*args.TimeoutMS) * time.Millisecond
			if t > maxMonitorTimeout {
				t = maxMonitorTimeout
			}
			if t > 0 {
				timeout = t
			}
		}

		desc := args.Description
		if desc == "" {
			desc = args.Command
			if len(desc) > 60 {
				desc = desc[:60] + "..."
			}
		}

		// Detach from the tool-call deadline so the monitor can outlive Execute,
		// while still inheriting any context values from ctx.
		monCtx := context.WithoutCancel(ctx)
		monID, err := tc.MonitorManager.Start(monCtx, args.Command, desc, tc.Directory, timeout)
		if err != nil {
			return &ExecuteResult{Output: fmt.Sprintf("Failed to start monitor: %v", err), IsError: true}, nil
		}

		return &ExecuteResult{Output: fmt.Sprintf("Monitor started: %s (ID: %s)\nOutput will be delivered at the next turn boundary.", desc, monID)}, nil

	default:
		return &ExecuteResult{Output: fmt.Sprintf("Unknown action: %s", action), IsError: true}, nil
	}
}
