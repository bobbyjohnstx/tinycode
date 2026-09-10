package redhat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewPromQLClient_SharedAlertManager(t *testing.T) {
	client := NewPromQLClient(PromQLClientConfig{
		BaseURL: "http://prom:9090",
	})
	if client.prometheus != client.alertManager {
		t.Error("expected prometheus and alertManager to share the same APIClient when AlertManagerURL is empty")
	}
}

func TestNewPromQLClient_SeparateAlertManager(t *testing.T) {
	client := NewPromQLClient(PromQLClientConfig{
		BaseURL:         "http://prom:9090",
		AlertManagerURL: "http://alertmanager:9093",
	})
	if client.prometheus == client.alertManager {
		t.Error("expected prometheus and alertManager to be separate APIClients when AlertManagerURL is set")
	}
}

func TestInstantQuery_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result":     []any{},
			},
		})
	}))
	defer srv.Close()

	client := NewPromQLClient(PromQLClientConfig{BaseURL: srv.URL})
	result, err := client.InstantQuery(context.Background(), "up", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/query" {
		t.Errorf("path = %q, want /api/v1/query", gotPath)
	}
	if gotQuery != "up" {
		t.Errorf("query param = %q, want %q", gotQuery, "up")
	}
	if result.ResultType != "vector" {
		t.Errorf("resultType = %q, want %q", result.ResultType, "vector")
	}
}

func TestInstantQuery_WithTime(t *testing.T) {
	var gotTime string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTime = r.URL.Query().Get("time")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result":     []any{},
			},
		})
	}))
	defer srv.Close()

	client := NewPromQLClient(PromQLClientConfig{BaseURL: srv.URL})
	_, err := client.InstantQuery(context.Background(), "up", "2024-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTime != "2024-01-01T00:00:00Z" {
		t.Errorf("time param = %q, want %q", gotTime, "2024-01-01T00:00:00Z")
	}
}

func TestInstantQuery_PrometheusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":    "error",
			"errorType": "bad_data",
			"error":     "parse error at char 5",
		})
	}))
	defer srv.Close()

	client := NewPromQLClient(PromQLClientConfig{BaseURL: srv.URL})
	_, err := client.InstantQuery(context.Background(), "bad{", "")
	if err == nil {
		t.Fatal("expected error for prometheus error response")
	}
	want := "prometheus query error (bad_data): parse error at char 5"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestRangeQuery_Success(t *testing.T) {
	var gotPath string
	var gotParams map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotParams = map[string]string{
			"query": r.URL.Query().Get("query"),
			"start": r.URL.Query().Get("start"),
			"end":   r.URL.Query().Get("end"),
			"step":  r.URL.Query().Get("step"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "matrix",
				"result":     []any{},
			},
		})
	}))
	defer srv.Close()

	client := NewPromQLClient(PromQLClientConfig{BaseURL: srv.URL})
	result, err := client.RangeQuery(context.Background(), "rate(http_requests_total[5m])", "2024-01-01T00:00:00Z", "2024-01-01T01:00:00Z", "15s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/query_range" {
		t.Errorf("path = %q, want /api/v1/query_range", gotPath)
	}
	if gotParams["query"] != "rate(http_requests_total[5m])" {
		t.Errorf("query = %q, want %q", gotParams["query"], "rate(http_requests_total[5m])")
	}
	if gotParams["start"] != "2024-01-01T00:00:00Z" {
		t.Errorf("start = %q, want %q", gotParams["start"], "2024-01-01T00:00:00Z")
	}
	if gotParams["end"] != "2024-01-01T01:00:00Z" {
		t.Errorf("end = %q, want %q", gotParams["end"], "2024-01-01T01:00:00Z")
	}
	if gotParams["step"] != "15s" {
		t.Errorf("step = %q, want %q", gotParams["step"], "15s")
	}
	if result.ResultType != "matrix" {
		t.Errorf("resultType = %q, want %q", result.ResultType, "matrix")
	}
}

func TestRangeQuery_PrometheusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":    "error",
			"errorType": "execution",
			"error":     "query timed out",
		})
	}))
	defer srv.Close()

	client := NewPromQLClient(PromQLClientConfig{BaseURL: srv.URL})
	_, err := client.RangeQuery(context.Background(), "up", "start", "end", "15s")
	if err == nil {
		t.Fatal("expected error for prometheus error response")
	}
	want := "prometheus query error (execution): query timed out"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestAlerts_UsesAlertManager(t *testing.T) {
	promCalled := false
	promSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer promSrv.Close()

	var gotPath string
	amSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Alert{
			{
				Labels:      map[string]string{"alertname": "HighCPU"},
				State:       "firing",
				Fingerprint: "abc123",
			},
		})
	}))
	defer amSrv.Close()

	client := NewPromQLClient(PromQLClientConfig{
		BaseURL:         promSrv.URL,
		AlertManagerURL: amSrv.URL,
	})

	active := true
	alerts, err := client.Alerts(context.Background(), &active, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promCalled {
		t.Error("Alerts should use alertManager, not prometheus")
	}
	if gotPath != "/api/v2/alerts" {
		t.Errorf("path = %q, want /api/v2/alerts", gotPath)
	}
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(alerts))
	}
	if alerts[0].Labels["alertname"] != "HighCPU" {
		t.Errorf("alertname = %q, want %q", alerts[0].Labels["alertname"], "HighCPU")
	}
	if alerts[0].State != "firing" {
		t.Errorf("state = %q, want %q", alerts[0].State, "firing")
	}
}

func TestAlerts_WithBoolParams(t *testing.T) {
	var gotActive, gotSilenced string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActive = r.URL.Query().Get("active")
		gotSilenced = r.URL.Query().Get("silenced")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Alert{})
	}))
	defer srv.Close()

	client := NewPromQLClient(PromQLClientConfig{BaseURL: srv.URL})

	active := true
	silenced := false
	_, err := client.Alerts(context.Background(), &active, &silenced)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotActive != "true" {
		t.Errorf("active param = %q, want %q", gotActive, "true")
	}
	if gotSilenced != "false" {
		t.Errorf("silenced param = %q, want %q", gotSilenced, "false")
	}
}

func TestAlerts_NilParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Alert{})
	}))
	defer srv.Close()

	client := NewPromQLClient(PromQLClientConfig{BaseURL: srv.URL})
	_, err := client.Alerts(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "" {
		t.Errorf("expected no query params, got %q", gotQuery)
	}
}

func TestSilenceAlert_Success(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"silenceID": "silence-42"})
	}))
	defer srv.Close()

	client := NewPromQLClient(PromQLClientConfig{
		AlertManagerURL: srv.URL,
		BaseURL:         srv.URL,
	})

	matchers := []AlertMatcher{
		{Name: "alertname", Value: "HighCPU", IsRegex: false, IsEqual: true},
	}
	id, err := client.SilenceAlert(context.Background(), matchers, "1h", "admin", "maintenance window")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v2/silences" {
		t.Errorf("path = %q, want /api/v2/silences", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if id != "silence-42" {
		t.Errorf("silenceID = %q, want %q", id, "silence-42")
	}
	if gotBody["createdBy"] != "admin" {
		t.Errorf("createdBy = %v, want %q", gotBody["createdBy"], "admin")
	}
	if gotBody["comment"] != "maintenance window" {
		t.Errorf("comment = %v, want %q", gotBody["comment"], "maintenance window")
	}
	if gotBody["startsAt"] == nil || gotBody["startsAt"] == "" {
		t.Error("expected startsAt to be set")
	}
	if gotBody["endsAt"] == nil || gotBody["endsAt"] == "" {
		t.Error("expected endsAt to be set")
	}
}

func TestSilenceAlert_InvalidDuration(t *testing.T) {
	client := NewPromQLClient(PromQLClientConfig{BaseURL: "http://unused"})
	_, err := client.SilenceAlert(context.Background(), nil, "invalid", "admin", "test")
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}
