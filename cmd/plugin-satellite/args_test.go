package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

func TestSatelliteRejectsMalformedArgs(t *testing.T) {
	p := newPlugin(options{SatelliteURL: "https://satellite.example.com", Token: "t"})
	var tool plugin.ToolDef
	for _, candidate := range p.Tools {
		if candidate.Name == "satellite_hosts" {
			tool = candidate
			break
		}
	}
	if tool.Execute == nil {
		t.Fatal("satellite_hosts missing")
	}
	_, err := tool.Execute(context.Background(), []byte("{"), plugin.ToolContext{})
	if err == nil || !strings.Contains(err.Error(), "parsing args") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestProbeAuthOnlyOnAPIPort(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := (&satelliteClient{}).probeClient()
	for _, probe := range []struct {
		path string
		port string
	}{
		{"/httpd", "443"},
		{"/proxy", "9090"},
		{"/tomcat", "23443"},
	} {
		if _, _, err := probeOnce(context.Background(), client, srv.URL+probe.path, probe.port, "Bearer secret"); err != nil {
			t.Fatal(err)
		}
	}
	if seen["/httpd"] != "Bearer secret" {
		t.Fatalf("443 auth = %q", seen["/httpd"])
	}
	if seen["/proxy"] != "" || seen["/tomcat"] != "" {
		t.Fatalf("non-API probes sent auth: %#v", seen)
	}
}

func TestSatelliteTLSVerifyDefault(t *testing.T) {
	client := newSatelliteClient("https://satellite.example.com", options{})
	if client.insecureSkipTLS {
		t.Fatal("TLS verification should be on by default")
	}
}
