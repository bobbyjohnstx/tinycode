package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/redhat"
	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

type options struct {
	MlflowURL      string
	ExperimentName string
	APIPrefix      string
}

func parseOptions(raw map[string]any) options {
	var opts options
	if v, ok := raw["mlflowUrl"].(string); ok {
		opts.MlflowURL = v
	}
	if v, ok := raw["experimentName"].(string); ok {
		opts.ExperimentName = v
	}
	if v, ok := raw["apiPrefix"].(string); ok {
		opts.APIPrefix = v
	}
	if opts.APIPrefix == "" {
		opts.APIPrefix = "/api/2.0/mlflow"
	}
	return opts
}

type lastRunInfo struct {
	RunID    string             `json:"runId"`
	Status   string             `json:"status"`
	Metrics  map[string]float64 `json:"metrics"`
	Params   map[string]string  `json:"params"`
	Duration *float64           `json:"duration,omitempty"`
}

type trackerState struct {
	mu             sync.RWMutex
	runID          string
	toolCallCount  int
	startTime      time.Time
	lastRun        *lastRunInfo
	experimentName string
}

func fetchLastRun(ctx context.Context, api *redhat.APIClient, experimentName, apiPrefix string) (*lastRunInfo, error) {
	expResp, err := api.Get(ctx, apiPrefix+"/experiments/get-by-name", map[string]string{
		"experiment_name": experimentName,
	})
	if err != nil {
		return nil, err
	}
	var expResult struct {
		Experiment struct {
			ExperimentID string `json:"experiment_id"`
		} `json:"experiment"`
	}
	if err := json.Unmarshal(expResp.Data, &expResult); err != nil {
		return nil, err
	}

	runsResp, err := api.Post(ctx, apiPrefix+"/runs/search", map[string]any{
		"experiment_ids": []string{expResult.Experiment.ExperimentID},
		"filter":         "status = 'FINISHED'",
		"order_by":       []string{"start_time DESC"},
		"max_results":    1,
	})
	if err != nil {
		return nil, err
	}

	var runsResult struct {
		Runs []struct {
			Info struct {
				RunID     string `json:"run_id"`
				Status    string `json:"status"`
				StartTime int64  `json:"start_time"`
				EndTime   *int64 `json:"end_time,omitempty"`
			} `json:"info"`
			Data struct {
				Metrics []struct {
					Key   string  `json:"key"`
					Value float64 `json:"value"`
				} `json:"metrics"`
				Params []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"params"`
			} `json:"data"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(runsResp.Data, &runsResult); err != nil {
		return nil, err
	}
	if len(runsResult.Runs) == 0 {
		return nil, nil
	}

	run := runsResult.Runs[0]
	metrics := make(map[string]float64)
	for _, m := range run.Data.Metrics {
		metrics[m.Key] = m.Value
	}
	params := make(map[string]string)
	for _, p := range run.Data.Params {
		params[p.Key] = p.Value
	}

	info := &lastRunInfo{
		RunID:   run.Info.RunID,
		Status:  run.Info.Status,
		Metrics: metrics,
		Params:  params,
	}
	if run.Info.EndTime != nil {
		dur := float64(*run.Info.EndTime-run.Info.StartTime) / 1000.0
		info.Duration = &dur
	}
	return info, nil
}

func formatLastSession(info *lastRunInfo) string {
	if info == nil {
		return "No previous session data available."
	}

	parts := []string{
		fmt.Sprintf("run=%s", info.RunID),
		fmt.Sprintf("status=%s", info.Status),
	}

	if info.Duration != nil {
		minutes := int(math.Round(*info.Duration / 60))
		parts = append(parts, fmt.Sprintf("duration=%dm", minutes))
	}

	if len(info.Metrics) > 0 {
		var metricParts []string
		for k, v := range info.Metrics {
			metricParts = append(metricParts, fmt.Sprintf("%s=%g", k, v))
		}
		parts = append(parts, fmt.Sprintf("metrics: %s", strings.Join(metricParts, ", ")))
	}

	if len(info.Params) > 0 {
		var paramParts []string
		for k, v := range info.Params {
			paramParts = append(paramParts, fmt.Sprintf("%s=%s", k, v))
		}
		parts = append(parts, fmt.Sprintf("params: %s", strings.Join(paramParts, ", ")))
	}

	return fmt.Sprintf("<last-session>%s</last-session>", strings.Join(parts, " "))
}

func newPlugin(opts options) plugin.Plugin {
	if opts.MlflowURL == "" {
		return plugin.Plugin{
			ID: "rhoai-experiment-tracker",
			Tools: []plugin.ToolDef{
				{
					Name:        "experiment_last_session",
					Description: "Get information about the last tracked experiment session.",
					Parameters: map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
					Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
						return "Experiment tracker not configured. Set mlflowUrl in plugin options.", nil
					},
				},
			},
		}
	}

	tokenFn := func(_ context.Context) (string, error) { return "", nil }
	api := redhat.NewAPIClient(redhat.APIClientConfig{
		BaseURL: opts.MlflowURL,
		TokenFn: tokenFn,
	})
	mlflow := redhat.NewMlflowClient(api)

	st := &trackerState{}

	return plugin.Plugin{
		ID: "rhoai-experiment-tracker",
		Tools: []plugin.ToolDef{
			{
				Name:        "experiment_last_session",
				Description: "Get information about the last tracked experiment session including metrics and parameters.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
				Execute: func(_ context.Context, _ json.RawMessage, _ plugin.ToolContext) (string, error) {
					st.mu.RLock()
					defer st.mu.RUnlock()
					return formatLastSession(st.lastRun), nil
				},
			},
		},
		Hooks: plugin.HookHandlers{
			SessionStart: func(ctx context.Context, event plugin.SessionStartEvent) error {
				expName := opts.ExperimentName
				if expName == "" {
					expName = filepath.Base(event.Directory)
					if expName == "" {
						expName = "default"
					}
				}

				st.mu.Lock()
				st.experimentName = expName
				st.mu.Unlock()

				// Fetch last run (best-effort).
				if info, err := fetchLastRun(ctx, api, expName, opts.APIPrefix); err == nil {
					st.mu.Lock()
					st.lastRun = info
					st.mu.Unlock()
				}

				// Create experiment if needed, then create run.
				experimentID, err := mlflow.GetExperimentByName(ctx, expName)
				if err != nil || experimentID == "" {
					experimentID, err = mlflow.CreateExperiment(ctx, expName)
					if err != nil {
						slog.Warn("rhoai-experiment-tracker: failed to create experiment", "error", err)
						return nil
					}
				}

				runID, err := mlflow.CreateRun(ctx, experimentID, []redhat.RunTag{
					{Key: "sessionID", Value: event.SessionID},
				})
				if err != nil {
					slog.Warn("rhoai-experiment-tracker: failed to create run", "error", err)
					return nil
				}

				st.mu.Lock()
				st.runID = runID
				st.toolCallCount = 0
				st.startTime = time.Now()
				st.mu.Unlock()

				return nil
			},
			ToolExecAfter: func(ctx context.Context, input plugin.ToolExecAfterInput) (*plugin.ToolExecAfterOutput, error) {
				st.mu.Lock()
				runID := st.runID
				st.toolCallCount++
				step := st.toolCallCount
				st.mu.Unlock()

				if runID == "" {
					return nil, nil
				}

				_ = mlflow.LogMetric(ctx, runID, input.ToolName, 1, &step)
				return nil, nil
			},
			SessionEnd: func(ctx context.Context, _ plugin.SessionEndEvent) error {
				st.mu.Lock()
				runID := st.runID
				toolCallCount := st.toolCallCount
				duration := time.Since(st.startTime).Seconds()
				st.runID = ""
				st.mu.Unlock()

				if runID == "" {
					return nil
				}

				_ = mlflow.LogMetric(ctx, runID, "tool_call_count", float64(toolCallCount), nil)
				_ = mlflow.LogMetric(ctx, runID, "session_duration_seconds", duration, nil)
				_ = mlflow.EndRun(ctx, runID, "FINISHED")
				return nil
			},
			Dispose: func(ctx context.Context) error {
				st.mu.Lock()
				runID := st.runID
				st.runID = ""
				st.mu.Unlock()

				if runID == "" {
					return nil
				}

				_ = mlflow.EndRun(ctx, runID, "KILLED")
				return nil
			},
		},
	}
}

func main() {
	plugin.RunWithOptions(func(params plugin.InitializeParams) (plugin.Plugin, error) {
		opts := parseOptions(params.Options)
		return newPlugin(opts), nil
	})
}
