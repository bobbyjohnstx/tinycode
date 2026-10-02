package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/pkg/mustgather"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "ingress-inspect" {
		t.Errorf("got %q, want %q", p.ID, "ingress-inspect")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	wantNames := []string{
		"ingress_controllers", "ingress_backends", "ingress_route_check",
		"ingress_config", "ingress_health",
	}
	if len(p.Tools) != len(wantNames) {
		t.Fatalf("got %d tools, want %d", len(p.Tools), len(wantNames))
	}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Execute == nil {
			t.Errorf("tool[%d] %q Execute is nil", i, want)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin()
	for _, tool := range p.Tools {
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("%s: params type = %v, want %q", tool.Name, params["type"], "object")
		}
	}
}

func TestParseHAProxyConfig(t *testing.T) {
	cfg := parseHAProxyConfig(testHAProxyConfig)

	if cfg.Global.MaxConn != "50000" {
		t.Errorf("global maxconn = %q, want %q", cfg.Global.MaxConn, "50000")
	}

	if cfg.Defaults.Mode != "http" {
		t.Errorf("defaults mode = %q, want %q", cfg.Defaults.Mode, "http")
	}
	if cfg.Defaults.Timeouts["client"] != "30s" {
		t.Errorf("defaults timeout client = %q, want %q", cfg.Defaults.Timeouts["client"], "30s")
	}
	if cfg.Defaults.Timeouts["server"] != "30s" {
		t.Errorf("defaults timeout server = %q, want %q", cfg.Defaults.Timeouts["server"], "30s")
	}

	if len(cfg.Frontends) != 2 {
		t.Fatalf("got %d frontends, want 2", len(cfg.Frontends))
	}
	if cfg.Frontends[0].Name != "public" {
		t.Errorf("frontend[0].Name = %q", cfg.Frontends[0].Name)
	}
	if len(cfg.Frontends[0].Binds) != 1 {
		t.Errorf("frontend[0].Binds = %d, want 1", len(cfg.Frontends[0].Binds))
	}
	if cfg.Frontends[1].Name != "public_ssl" {
		t.Errorf("frontend[1].Name = %q", cfg.Frontends[1].Name)
	}
	if !strings.Contains(cfg.Frontends[1].Binds[0], "ssl") {
		t.Errorf("frontend[1] should have SSL bind, got %q", cfg.Frontends[1].Binds[0])
	}

	if len(cfg.Backends) != 3 {
		t.Fatalf("got %d backends, want 3", len(cfg.Backends))
	}

	be0 := cfg.Backends[0]
	if be0.Name != "be_http:default:myapp" {
		t.Errorf("backend[0].Name = %q", be0.Name)
	}
	if be0.Mode != "http" {
		t.Errorf("backend[0].Mode = %q", be0.Mode)
	}
	if be0.Balance != "roundrobin" {
		t.Errorf("backend[0].Balance = %q", be0.Balance)
	}
	if len(be0.Servers) != 2 {
		t.Errorf("backend[0].Servers = %d, want 2", len(be0.Servers))
	}
	if be0.Servers[0].Name != "pod1" {
		t.Errorf("server[0].Name = %q", be0.Servers[0].Name)
	}
	if be0.Servers[0].Address != "10.128.0.5:8080" {
		t.Errorf("server[0].Address = %q", be0.Servers[0].Address)
	}

	be1 := cfg.Backends[1]
	if be1.Mode != "tcp" {
		t.Errorf("backend[1].Mode = %q", be1.Mode)
	}
	if be1.Balance != "source" {
		t.Errorf("backend[1].Balance = %q", be1.Balance)
	}

	be2 := cfg.Backends[2]
	if len(be2.Servers) != 0 {
		t.Errorf("backend[2].Servers = %d, want 0", len(be2.Servers))
	}
}

func TestIsHighTimeout(t *testing.T) {
	tests := []struct {
		val  string
		high bool
	}{
		{"30s", false},
		{"5m", false},
		{"6m", true},
		{"300000", false},
		{"300001", true},
		{"10m", true},
		{"", false},
	}
	for _, tc := range tests {
		got := isHighTimeout(tc.val)
		if got != tc.high {
			t.Errorf("isHighTimeout(%q) = %v, want %v", tc.val, got, tc.high)
		}
	}
}

// setupTestMustGather creates a minimal must-gather for ingress inspection tests.
func setupTestMustGather(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mkFile(t, dir, "cluster-scoped-resources/operator.openshift.io/ingresscontrollers/default.yaml", ingressControllerYAML)
	mkFile(t, dir, "namespaces/openshift-ingress/pods/router-default-abc123/haproxy.config", testHAProxyConfig)
	mkFile(t, dir, "namespaces/openshift-ingress/pods/router-default-abc123/router/router/logs/current.log", routerLogData)
	mkFile(t, dir, "namespaces/default/route.openshift.io/routes.yaml", defaultRoutesYAML)
	mkFile(t, dir, "namespaces/openshift-console/route.openshift.io/routes.yaml", consoleRoutesYAML)
	mkFile(t, dir, "namespaces/openshift-ingress-operator/core/.keep", "")

	return dir
}

func mkFile(t *testing.T, base, rel, content string) {
	t.Helper()
	full := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output does not contain %q\noutput:\n%s", needle, truncate(haystack, 500))
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func TestToolIngressControllers(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolIngressControllers(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "IngressControllers: 1")
	assertContains(t, out, "default")
	assertContains(t, out, "apps.example.com")
	assertContains(t, out, "LoadBalancerService")
	assertContains(t, out, "my-custom-cert")
}

func TestToolIngressBackends(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all backends", func(t *testing.T) {
		out, err := toolIngressBackends(root, "")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "Backends: 3")
		assertContains(t, out, "be_http:default:myapp")
		assertContains(t, out, "be_tcp:default:passthrough-app")
	})

	t.Run("filter", func(t *testing.T) {
		out, err := toolIngressBackends(root, "myapp")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "be_http:default:myapp")
		if strings.Contains(out, "be_tcp:default:passthrough-app") {
			t.Error("filter should have excluded passthrough backend")
		}
	})
}

func TestToolIngressRouteCheck(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all namespaces", func(t *testing.T) {
		out, err := toolIngressRouteCheck(root, "")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "Route-Backend Cross-Reference")
		assertContains(t, out, "Stale backends")
		assertContains(t, out, "be_http:stale:old-app")
		assertContains(t, out, "Missing backends")
		assertContains(t, out, "openshift-console/console")
		assertContains(t, out, "balance=source on passthrough")
	})

	t.Run("filter namespace", func(t *testing.T) {
		out, err := toolIngressRouteCheck(root, "default")
		if err != nil {
			t.Fatal(err)
		}
		assertContains(t, out, "Route-Backend Cross-Reference")
		assertContains(t, out, "Routes: 2")
	})
}

func TestToolIngressConfig(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolIngressConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Global:")
	assertContains(t, out, "maxconn: 50000")
	assertContains(t, out, "Defaults:")
	assertContains(t, out, "mode: http")
	assertContains(t, out, "client: 30s")
	assertContains(t, out, "Frontends: 2")
	assertContains(t, out, "Backends: 3 total")
}

func TestToolIngressHealth(t *testing.T) {
	dir := setupTestMustGather(t)
	root, err := mustgather.Use(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := toolIngressHealth(root)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Ingress Health Summary")
	assertContains(t, out, "IngressControllers: 1")
	assertContains(t, out, "HAProxy configs: 1")
	assertContains(t, out, "Issues found:")
}

// Test fixtures

const ingressControllerYAML = `apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  replicas: 2
  endpointPublishingStrategy:
    type: LoadBalancerService
  defaultCertificate:
    name: my-custom-cert
status:
  domain: apps.example.com
  availableReplicas: 2
  conditions:
    - type: Available
      status: "True"
    - type: Degraded
      status: "False"
`

const testHAProxyConfig = `global
  maxconn 50000
  log /dev/log local0

defaults
  mode http
  timeout client 30s
  timeout server 30s
  timeout connect 5s
  maxconn 20000

frontend public
  bind *:80
  mode http

frontend public_ssl
  bind *:443 ssl crt /etc/pki/tls/certs/default.pem
  mode tcp

backend be_http:default:myapp
  mode http
  balance roundrobin
  server pod1 10.128.0.5:8080 check inter 5000ms
  server pod2 10.128.0.6:8080 check inter 5000ms

backend be_tcp:default:passthrough-app
  mode tcp
  balance source
  server pod3 10.128.0.7:8443 check

backend be_http:stale:old-app
  mode http
  balance roundrobin
`

const routerLogData = `I0115 10:30:45.123456       1 router.go:100] router started
I0115 10:30:46.234567       1 router.go:200] loading routes
I0115 10:30:47.345678       1 router.go:300] ready to accept connections
`

const defaultRoutesYAML = `apiVersion: route.openshift.io/v1
kind: RouteList
items:
  - metadata:
      name: myapp
      namespace: default
    spec:
      host: myapp.apps.example.com
      to:
        kind: Service
        name: myapp
  - metadata:
      name: passthrough-app
      namespace: default
    spec:
      host: passthrough.apps.example.com
      tls:
        termination: passthrough
      to:
        kind: Service
        name: passthrough-app
`

const consoleRoutesYAML = `apiVersion: route.openshift.io/v1
kind: RouteList
items:
  - metadata:
      name: console
      namespace: openshift-console
    spec:
      host: console.apps.example.com
      tls:
        termination: reencrypt
      to:
        kind: Service
        name: console
`
