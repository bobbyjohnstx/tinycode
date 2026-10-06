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

func TestPoll_DormantProviderReconnects(t *testing.T) {
	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n <= int32(maxConsecutiveFailures) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(ollamaTagsResponse{
			Models: []ollamaModel{{Name: "llama3:latest"}},
		})
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	removed := b.Subscribe("provider.removed")
	defer removed.Unsubscribe()
	reconnected := b.Subscribe("provider.reconnected")
	defer reconnected.Unsubscribe()

	d := NewDiscovery(reg, b)

	// Fail until dormant.
	for i := 0; i < maxConsecutiveFailures; i++ {
		d.poll(t.Context(), srv.URL, "", "")
	}

	if reg.Has("ollama") {
		t.Fatal("expected ollama removed after consecutive failures")
	}
	if !d.isDormant("ollama") {
		t.Fatal("expected ollama to be marked dormant")
	}
	if !d.shouldPoll("ollama") {
		t.Fatal("dormant providers must remain eligible for polling")
	}

	select {
	case <-removed.C:
	case <-time.After(time.Second):
		t.Fatal("expected provider.removed")
	}

	// Next poll succeeds → re-register + reconnect.
	d.poll(t.Context(), srv.URL, "", "")

	if !reg.Has("ollama") {
		t.Fatal("expected ollama re-registered after successful poll")
	}
	if d.isDormant("ollama") {
		t.Fatal("expected dormant cleared on success")
	}

	select {
	case evt := <-reconnected.C:
		props := evt.Properties.(map[string]any)
		if props["providerID"] != "ollama" {
			t.Errorf("providerID = %v, want ollama", props["providerID"])
		}
	case <-time.After(time.Second):
		t.Fatal("expected provider.reconnected")
	}
}
