package redhat

import (
	"context"
	"fmt"
	"time"
)

type RunTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type MlflowClient struct {
	api *APIClient
}

func NewMlflowClient(api *APIClient) *MlflowClient {
	return &MlflowClient{api: api}
}

func (c *MlflowClient) CreateExperiment(ctx context.Context, name string) (string, error) {
	resp, err := c.api.Post(ctx, "/api/2.0/mlflow/experiments/create", map[string]string{"name": name})
	if err != nil {
		return "", fmt.Errorf("creating experiment: %w", err)
	}
	var result struct {
		ExperimentID string `json:"experiment_id"`
	}
	if err := decodeRaw(resp.Data, &result); err != nil {
		return "", err
	}
	return result.ExperimentID, nil
}

func (c *MlflowClient) GetExperimentByName(ctx context.Context, name string) (string, error) {
	resp, err := c.api.Get(ctx, "/api/2.0/mlflow/experiments/get-by-name", map[string]string{"experiment_name": name})
	if err != nil {
		return "", err
	}
	var result struct {
		Experiment struct {
			ExperimentID string `json:"experiment_id"`
		} `json:"experiment"`
	}
	if err := decodeRaw(resp.Data, &result); err != nil {
		return "", err
	}
	return result.Experiment.ExperimentID, nil
}

func (c *MlflowClient) CreateRun(ctx context.Context, experimentID string, tags []RunTag) (string, error) {
	body := map[string]any{
		"experiment_id": experimentID,
	}
	if len(tags) > 0 {
		body["tags"] = tags
	}
	resp, err := c.api.Post(ctx, "/api/2.0/mlflow/runs/create", body)
	if err != nil {
		return "", fmt.Errorf("creating run: %w", err)
	}
	var result struct {
		Run struct {
			Info struct {
				RunID string `json:"run_id"`
			} `json:"info"`
		} `json:"run"`
	}
	if err := decodeRaw(resp.Data, &result); err != nil {
		return "", err
	}
	return result.Run.Info.RunID, nil
}

func (c *MlflowClient) LogMetric(ctx context.Context, runID, key string, value float64, step *int) error {
	body := map[string]any{
		"run_id": runID,
		"key":    key,
		"value":  value,
	}
	if step != nil {
		body["step"] = *step
	}
	_, err := c.api.Post(ctx, "/api/2.0/mlflow/runs/log-metric", body)
	return err
}

func (c *MlflowClient) LogParam(ctx context.Context, runID, key, value string) error {
	_, err := c.api.Post(ctx, "/api/2.0/mlflow/runs/log-param", map[string]string{
		"run_id": runID,
		"key":    key,
		"value":  value,
	})
	return err
}

func (c *MlflowClient) EndRun(ctx context.Context, runID, status string) error {
	_, err := c.api.Post(ctx, "/api/2.0/mlflow/runs/update", map[string]any{
		"run_id":   runID,
		"status":   status,
		"end_time": time.Now().UnixMilli(),
	})
	return err
}
