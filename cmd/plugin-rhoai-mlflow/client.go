package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
)

// --- mlflow read types ---

type experiment struct {
	ExperimentID   string `json:"experiment_id"`
	Name           string `json:"name"`
	ArtifactLoc    string `json:"artifact_location"`
	LifecycleStage string `json:"lifecycle_stage"`
	LastUpdateTime *int64 `json:"last_update_time,omitempty"`
}

type runInfo struct {
	RunID          string `json:"run_id"`
	ExperimentID   string `json:"experiment_id"`
	Status         string `json:"status"`
	StartTime      int64  `json:"start_time"`
	EndTime        *int64 `json:"end_time,omitempty"`
	ArtifactURI    string `json:"artifact_uri"`
	LifecycleStage string `json:"lifecycle_stage"`
}

type runMetric struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
}

type runParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type mlflowRun struct {
	Info runInfo `json:"info"`
	Data struct {
		Metrics []runMetric `json:"metrics"`
		Params  []runParam  `json:"params"`
	} `json:"data"`
}

type artifact struct {
	Path     string `json:"path"`
	IsDir    bool   `json:"is_dir"`
	FileSize *int64 `json:"file_size,omitempty"`
}

type modelVersion struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	CurrentStage      string `json:"current_stage"`
	Status            string `json:"status"`
	Source            string `json:"source"`
	RunID             string `json:"run_id"`
	CreationTimestamp int64  `json:"creation_timestamp"`
}

type registeredModel struct {
	Name           string         `json:"name"`
	LatestVersions []modelVersion `json:"latest_versions"`
}

type runComparison struct {
	RunID   string             `json:"runId"`
	Metrics map[string]float64 `json:"metrics"`
	Params  map[string]string  `json:"params"`
}

// --- mlflow read client ---

type mlflowReadClient struct {
	api       *redhat.APIClient
	apiPrefix string
}

func newMlflowReadClient(api *redhat.APIClient, apiPrefix string) *mlflowReadClient {
	return &mlflowReadClient{api: api, apiPrefix: apiPrefix}
}

func (c *mlflowReadClient) listExperiments(ctx context.Context) ([]experiment, error) {
	resp, err := c.api.Get(ctx, c.apiPrefix+"/experiments/search", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Experiments []experiment `json:"experiments"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Experiments, nil
}

func (c *mlflowReadClient) listRuns(ctx context.Context, experimentID, filter string) ([]mlflowRun, error) {
	body := map[string]any{
		"experiment_ids": []string{experimentID},
	}
	if filter != "" {
		body["filter"] = filter
	}
	resp, err := c.api.Post(ctx, c.apiPrefix+"/runs/search", body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Runs []mlflowRun `json:"runs"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Runs, nil
}

func (c *mlflowReadClient) compareRuns(ctx context.Context, runIDs []string) ([]runComparison, error) {
	var comparisons []runComparison
	for _, id := range runIDs {
		resp, err := c.api.Get(ctx, c.apiPrefix+"/runs/get", map[string]string{"run_id": id})
		if err != nil {
			return nil, fmt.Errorf("fetching run %s: %w", id, err)
		}
		var result struct {
			Run mlflowRun `json:"run"`
		}
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return nil, fmt.Errorf("parsing run %s: %w", id, err)
		}
		metrics := make(map[string]float64)
		for _, m := range result.Run.Data.Metrics {
			metrics[m.Key] = m.Value
		}
		params := make(map[string]string)
		for _, p := range result.Run.Data.Params {
			params[p.Key] = p.Value
		}
		comparisons = append(comparisons, runComparison{
			RunID:   id,
			Params:  params,
			Metrics: metrics,
		})
	}
	return comparisons, nil
}

func (c *mlflowReadClient) listArtifacts(ctx context.Context, runID, path string) ([]artifact, error) {
	query := map[string]string{"run_id": runID}
	if path != "" {
		query["path"] = path
	}
	resp, err := c.api.Get(ctx, c.apiPrefix+"/artifacts/list", query)
	if err != nil {
		return nil, err
	}
	var result struct {
		Files []artifact `json:"files"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.Files, nil
}

func (c *mlflowReadClient) listRegisteredModels(ctx context.Context) ([]registeredModel, error) {
	resp, err := c.api.Get(ctx, c.apiPrefix+"/registered-models/search", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		RegisteredModels []registeredModel `json:"registered_models"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return result.RegisteredModels, nil
}

func (c *mlflowReadClient) getModelVersion(ctx context.Context, name, version string) (*modelVersion, error) {
	resp, err := c.api.Get(ctx, c.apiPrefix+"/model-versions/get", map[string]string{
		"name":    name,
		"version": version,
	})
	if err != nil {
		return nil, err
	}
	var result struct {
		ModelVersion modelVersion `json:"model_version"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, err
	}
	return &result.ModelVersion, nil
}

func (c *mlflowReadClient) transitionModelStage(ctx context.Context, name, version, stage string) error {
	_, err := c.api.Post(ctx, c.apiPrefix+"/model-versions/transition-stage", map[string]any{
		"name":                      name,
		"version":                   version,
		"stage":                     stage,
		"archive_existing_versions": true,
	})
	return err
}

// --- formatters ---

func formatExperiments(exps []experiment) string {
	if len(exps) == 0 {
		return "No experiments found."
	}
	lines := []string{fmt.Sprintf("Experiments: %d", len(exps)), ""}
	for _, e := range exps {
		lines = append(lines, fmt.Sprintf("  [%s] %s (stage: %s)", e.ExperimentID, e.Name, e.LifecycleStage))
	}
	return strings.Join(lines, "\n")
}

func formatRuns(runs []mlflowRun) string {
	if len(runs) == 0 {
		return "No runs found."
	}
	lines := []string{fmt.Sprintf("Runs: %d", len(runs)), ""}
	for _, r := range runs {
		lines = append(lines, fmt.Sprintf("  [%s] status=%s experiment=%s",
			r.Info.RunID, r.Info.Status, r.Info.ExperimentID))
	}
	return strings.Join(lines, "\n")
}

func formatComparison(comparisons []runComparison) string {
	if len(comparisons) == 0 {
		return "No runs to compare."
	}

	// Collect all metric and param keys.
	metricKeys := map[string]bool{}
	paramKeys := map[string]bool{}
	for _, c := range comparisons {
		for k := range c.Metrics {
			metricKeys[k] = true
		}
		for k := range c.Params {
			paramKeys[k] = true
		}
	}

	lines := []string{fmt.Sprintf("Comparison of %d runs:", len(comparisons)), ""}

	// Header.
	header := "| Key |"
	sep := "| --- |"
	for _, c := range comparisons {
		runID := c.RunID
		short := runID
		if len(short) > 8 {
			short = short[:8]
		}
		header += fmt.Sprintf(" %s |", short)
		sep += " --- |"
	}
	lines = append(lines, header, sep)

	// Params.
	for key := range paramKeys {
		row := fmt.Sprintf("| %s |", key)
		for _, c := range comparisons {
			val := "-"
			if v, ok := c.Params[key]; ok {
				val = v
			}
			row += fmt.Sprintf(" %s |", val)
		}
		lines = append(lines, row)
	}

	// Metrics.
	for key := range metricKeys {
		row := fmt.Sprintf("| %s |", key)
		for _, c := range comparisons {
			val := "-"
			if v, ok := c.Metrics[key]; ok {
				val = fmt.Sprintf("%.4f", v)
			}
			row += fmt.Sprintf(" %s |", val)
		}
		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

func formatArtifacts(artifacts []artifact) string {
	if len(artifacts) == 0 {
		return "No artifacts found."
	}
	lines := []string{fmt.Sprintf("Artifacts: %d", len(artifacts)), ""}
	for _, a := range artifacts {
		kind := "file"
		if a.IsDir {
			kind = "dir"
		}
		size := ""
		if a.FileSize != nil {
			size = fmt.Sprintf(" (%d bytes)", *a.FileSize)
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s%s", kind, a.Path, size))
	}
	return strings.Join(lines, "\n")
}

func formatModels(models []registeredModel) string {
	if len(models) == 0 {
		return "No registered models found."
	}
	lines := []string{fmt.Sprintf("Registered Models: %d", len(models)), ""}
	for _, m := range models {
		lines = append(lines, fmt.Sprintf("  %s (%d versions)", m.Name, len(m.LatestVersions)))
		for _, v := range m.LatestVersions {
			lines = append(lines, fmt.Sprintf("    v%s: stage=%s status=%s run=%s", v.Version, v.CurrentStage, v.Status, v.RunID))
		}
	}
	return strings.Join(lines, "\n")
}

func formatModelVersion(v *modelVersion) string {
	return strings.Join([]string{
		fmt.Sprintf("Model: %s v%s", v.Name, v.Version),
		fmt.Sprintf("Stage: %s | Status: %s", v.CurrentStage, v.Status),
		fmt.Sprintf("Run: %s", v.RunID),
		fmt.Sprintf("Source: %s", v.Source),
	}, "\n")
}
