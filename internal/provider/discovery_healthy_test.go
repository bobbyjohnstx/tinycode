package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestShouldPoll_HealthyWaitsThenElapses(t *testing.T) {
	b := bus.New()
	defer b.Close()
	d := NewDiscovery(NewRegistry(), b)
	base := time.Unix(1_700_000_000, 0)
	d.now = func() time.Time { return base }

	d.handleDiscoverySuccess("ollama")
	if d.shouldPoll("ollama") {
		t.Fatal("healthy provider should wait for healthyPollInterval")
	}

	d.now = func() time.Time { return base.Add(healthyPollInterval) }
	if !d.shouldPoll("ollama") {
		t.Fatal("healthy provider should be eligible once the interval elapses")
	}
}

func TestShouldPoll_FailureClearsHealthyImmediately(t *testing.T) {
	b := bus.New()
	defer b.Close()
	d := NewDiscovery(NewRegistry(), b)
	base := time.Unix(1_700_000_000, 0)
	d.now = func() time.Time { return base }

	d.handleDiscoverySuccess("ollama")
	d.handleDiscoveryFailure("ollama", "Ollama", errProbe)

	if !d.shouldPoll("ollama") {
		t.Fatal("a single failure should return the provider to fast polling")
	}
}

var errProbe = probeError("down")

type probeError string

func (e probeError) Error() string { return string(e) }

func TestRescan_BypassesHealthySchedule(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(ollamaTagsResponse{
			Models: []ollamaModel{{Name: "llama3:latest"}},
		})
	}))
	defer srv.Close()

	b := bus.New()
	defer b.Close()
	d := NewDiscovery(NewRegistry(), b)
	d.ollamaURL = srv.URL

	d.Rescan(t.Context())
	if hits.Load() != 1 {
		t.Fatalf("first rescan hits = %d, want 1", hits.Load())
	}
	if d.shouldPoll("ollama") {
		t.Fatal("successful rescan should mark the provider healthy")
	}

	d.Rescan(t.Context())
	if hits.Load() != 2 {
		t.Fatalf("forced rescan hits = %d, want 2", hits.Load())
	}
}
