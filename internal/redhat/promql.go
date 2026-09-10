package redhat

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

type PromQLVector struct {
	Metric map[string]string `json:"metric"`
	Value  [2]any            `json:"value"`
}

type PromQLMatrix struct {
	Metric map[string]string `json:"metric"`
	Values [][2]any          `json:"values"`
}

type PromQLResult struct {
	ResultType string `json:"resultType"`
	Result     any    `json:"result"`
}

type Alert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	State        string            `json:"state"`
	ActiveAt     string            `json:"activeAt"`
	Value        string            `json:"value"`
	Fingerprint  string            `json:"fingerprint"`
}

type AlertMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

type PromQLClientConfig struct {
	BaseURL         string
	TokenFn         func(ctx context.Context) (string, error)
	AlertManagerURL string
}

type PromQLClient struct {
	prometheus   *APIClient
	alertManager *APIClient
}

func NewPromQLClient(cfg PromQLClientConfig) *PromQLClient {
	prometheus := NewAPIClient(APIClientConfig{
		BaseURL: cfg.BaseURL,
		TokenFn: cfg.TokenFn,
	})

	alertManager := prometheus
	if cfg.AlertManagerURL != "" {
		alertManager = NewAPIClient(APIClientConfig{
			BaseURL: cfg.AlertManagerURL,
			TokenFn: cfg.TokenFn,
		})
	}

	return &PromQLClient{
		prometheus:   prometheus,
		alertManager: alertManager,
	}
}

func (c *PromQLClient) InstantQuery(ctx context.Context, query string, queryTime string) (*PromQLResult, error) {
	params := map[string]string{"query": query}
	if queryTime != "" {
		params["time"] = queryTime
	}

	resp, err := c.prometheus.Get(ctx, "/api/v1/query", params)
	if err != nil {
		return nil, err
	}

	var promResp struct {
		Status    string       `json:"status"`
		Data      PromQLResult `json:"data"`
		ErrorType string       `json:"errorType,omitempty"`
		Error     string       `json:"error,omitempty"`
	}
	if _, err := DecodeResponse[any](resp); err != nil {
		return nil, err
	}
	if err := decodeRaw(resp.Data, &promResp); err != nil {
		return nil, err
	}
	if promResp.Status == "error" {
		return nil, fmt.Errorf("prometheus query error (%s): %s", promResp.ErrorType, promResp.Error)
	}
	return &promResp.Data, nil
}

func (c *PromQLClient) RangeQuery(ctx context.Context, query, start, end, step string) (*PromQLResult, error) {
	params := map[string]string{
		"query": query,
		"start": start,
		"end":   end,
		"step":  step,
	}

	resp, err := c.prometheus.Get(ctx, "/api/v1/query_range", params)
	if err != nil {
		return nil, err
	}

	var promResp struct {
		Status    string       `json:"status"`
		Data      PromQLResult `json:"data"`
		ErrorType string       `json:"errorType,omitempty"`
		Error     string       `json:"error,omitempty"`
	}
	if err := decodeRaw(resp.Data, &promResp); err != nil {
		return nil, err
	}
	if promResp.Status == "error" {
		return nil, fmt.Errorf("prometheus query error (%s): %s", promResp.ErrorType, promResp.Error)
	}
	return &promResp.Data, nil
}

func (c *PromQLClient) Alerts(ctx context.Context, active, silenced *bool) ([]Alert, error) {
	params := map[string]string{}
	if active != nil {
		params["active"] = strconv.FormatBool(*active)
	}
	if silenced != nil {
		params["silenced"] = strconv.FormatBool(*silenced)
	}

	resp, err := c.alertManager.Get(ctx, "/api/v2/alerts", params)
	if err != nil {
		return nil, err
	}

	var alerts []Alert
	if err := decodeRaw(resp.Data, &alerts); err != nil {
		return nil, err
	}
	return alerts, nil
}

func (c *PromQLClient) SilenceAlert(ctx context.Context, matchers []AlertMatcher, duration, createdBy, comment string) (string, error) {
	dur, err := ParseDuration(duration)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	endsAt := now.Add(dur)

	body := map[string]any{
		"matchers":  matchers,
		"startsAt":  now.Format(time.RFC3339),
		"endsAt":    endsAt.Format(time.RFC3339),
		"createdBy": createdBy,
		"comment":   comment,
	}

	resp, err := c.alertManager.Post(ctx, "/api/v2/silences", body)
	if err != nil {
		return "", err
	}

	var silenceResp struct {
		SilenceID string `json:"silenceID"`
	}
	if err := decodeRaw(resp.Data, &silenceResp); err != nil {
		return "", err
	}
	return silenceResp.SilenceID, nil
}

var durationRe = regexp.MustCompile(`^(\d+)([smhdw])$`)

func ParseDuration(s string) (time.Duration, error) {
	m := durationRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid duration format: %s (use e.g. \"30s\", \"5m\", \"1h\", \"7d\", \"1w\")", s)
	}

	value, _ := strconv.Atoi(m[1])
	multipliers := map[string]time.Duration{
		"s": time.Second,
		"m": time.Minute,
		"h": time.Hour,
		"d": 24 * time.Hour,
		"w": 7 * 24 * time.Hour,
	}

	return time.Duration(value) * multipliers[m[2]], nil
}

func decodeRaw(data []byte, target any) error {
	return decodeJSON(data, target)
}

func decodeJSON(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decoding JSON: %w", err)
	}
	return nil
}
