package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
)

// --- redactSecret tests ---

func TestRedactSecret_EmptySecretReturnsOriginalText(t *testing.T) {
	text := "some output with token sha256~abc123"
	got := redactSecret(text, "")
	if got != text {
		t.Errorf("expected original text unchanged, got %q", got)
	}
}

func TestRedactSecret_ReplacesTokenInText(t *testing.T) {
	got := redactSecret("Login with token sha256~secret123 succeeded", "sha256~secret123")
	if strings.Contains(got, "sha256~secret123") {
		t.Errorf("secret not redacted: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("expected [REDACTED] placeholder, got %q", got)
	}
}

func TestRedactSecret_ReplacesMultipleOccurrences(t *testing.T) {
	text := "token=MYSECRET and again MYSECRET in the output"
	got := redactSecret(text, "MYSECRET")
	count := strings.Count(got, "[REDACTED]")
	if count != 2 {
		t.Errorf("expected 2 redactions, got %d in: %q", count, got)
	}
	if strings.Contains(got, "MYSECRET") {
		t.Errorf("secret not fully redacted: %q", got)
	}
}

func TestRedactSecret_NoMatchReturnsSameText(t *testing.T) {
	text := "no secrets here"
	got := redactSecret(text, "notpresent")
	if got != text {
		t.Errorf("expected text unchanged, got %q", got)
	}
}

// --- queryFiringAlerts tests ---

func TestQueryFiringAlerts_ParsesCriticalWarningAndInfoAlerts(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "oc")
	// The fake oc returns alert JSON when exec'd with the right args
	body := `#!/bin/sh
if echo "$*" | grep -q "alertmanager"; then
  cat <<'ALERTS'
[
  {"labels":{"alertname":"HighCPU","namespace":"openshift-monitoring","severity":"critical"}},
  {"labels":{"alertname":"DiskFull","namespace":"openshift-storage","severity":"warning"}},
  {"labels":{"alertname":"NetworkSlow","namespace":"","severity":"warning"}},
  {"labels":{"alertname":"InfoAlert","namespace":"kube-system","severity":"info"}},
  {"labels":{"alertname":"InfoAlert2","namespace":"","severity":"info"}}
]
ALERTS
  exit 0
fi
exit 1
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oc := redhat.NewOcClient()
	summary := queryFiringAlerts(context.Background(), oc)
	if summary == nil {
		t.Fatal("expected non-nil alert summary")
	}
	if len(summary.Critical) != 1 {
		t.Errorf("expected 1 critical alert, got %d", len(summary.Critical))
	}
	if summary.Critical[0].Name != "HighCPU" {
		t.Errorf("expected critical alert 'HighCPU', got %q", summary.Critical[0].Name)
	}
	if len(summary.Warning) != 2 {
		t.Errorf("expected 2 warning alerts, got %d", len(summary.Warning))
	}
	if summary.Info != 2 {
		t.Errorf("expected 2 info alerts, got %d", summary.Info)
	}
}

func TestQueryFiringAlerts_ReturnsNilWhenOcFails(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "oc")
	body := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oc := redhat.NewOcClient()
	summary := queryFiringAlerts(context.Background(), oc)
	if summary != nil {
		t.Errorf("expected nil when oc fails, got %+v", summary)
	}
}

func TestQueryFiringAlerts_ReturnsNilForNoAlerts(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "oc")
	body := "#!/bin/sh\necho '[]'\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oc := redhat.NewOcClient()
	summary := queryFiringAlerts(context.Background(), oc)
	if summary != nil {
		t.Errorf("expected nil for empty alerts, got %+v", summary)
	}
}

func TestQueryFiringAlerts_HandlesAlertWithEmptyAlertname(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "oc")
	body := `#!/bin/sh
echo '[{"labels":{"alertname":"","namespace":"ns1","severity":"critical"}}]'
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oc := redhat.NewOcClient()
	summary := queryFiringAlerts(context.Background(), oc)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if len(summary.Critical) != 1 {
		t.Fatalf("expected 1 critical alert, got %d", len(summary.Critical))
	}
	if summary.Critical[0].Name != "Unknown" {
		t.Errorf("expected 'Unknown' for empty alertname, got %q", summary.Critical[0].Name)
	}
}

// --- queryClusterContext tests ---

func TestQueryClusterContext_ReturnsNilWhenOcNotAvailable(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "oc")
	// version --client fails = not available
	body := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oc := redhat.NewOcClient()
	cc := queryClusterContext(context.Background(), oc)
	if cc != nil {
		t.Errorf("expected nil when oc not available, got %+v", cc)
	}
}

func TestQueryClusterContext_ReturnsNilWhenNotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "oc")
	// version --client succeeds (available), whoami fails (not logged in)
	body := `#!/bin/sh
if [ "$1" = "version" ] && [ "$2" = "--client" ]; then
  exit 0
fi
if [ "$1" = "whoami" ]; then
  exit 1
fi
exit 1
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oc := redhat.NewOcClient()
	cc := queryClusterContext(context.Background(), oc)
	if cc != nil {
		t.Errorf("expected nil when not logged in, got %+v", cc)
	}
}

func TestQueryClusterContext_ParsesClusterInfoFromOcOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "oc")
	body := `#!/bin/sh
case "$1" in
  version)
    if [ "$2" = "--client" ]; then
      exit 0
    fi
    # version -o json
    cat <<'JSON'
{"clientVersion":{"major":"4"},"openshiftVersion":"4.14.5"}
JSON
    exit 0
    ;;
  whoami)
    echo "admin"
    exit 0
    ;;
  get)
    if [ "$2" = "nodes" ]; then
      cat <<'JSON'
{"items":[
  {"metadata":{"labels":{"node-role.kubernetes.io/control-plane":"","node-role.kubernetes.io/worker":""}}},
  {"metadata":{"labels":{"node-role.kubernetes.io/control-plane":""}}},
  {"metadata":{"labels":{"node-role.kubernetes.io/worker":""}}}
]}
JSON
      exit 0
    fi
    if [ "$2" = "csv" ]; then
      cat <<'JSON'
{"items":[
  {"metadata":{"name":"prometheus-operator.v0.65"},"spec":{"displayName":"Prometheus Operator"}},
  {"metadata":{"name":"elasticsearch-operator.v5.8"},"spec":{"displayName":"OpenShift Elasticsearch Operator"}}
]}
JSON
      exit 0
    fi
    exit 1
    ;;
  config)
    echo "default/api-mycluster-example-com:6443/admin"
    exit 0
    ;;
  -n)
    # alertmanager query
    echo '[]'
    exit 0
    ;;
esac
exit 1
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oc := redhat.NewOcClient()
	cc := queryClusterContext(context.Background(), oc)
	if cc == nil {
		t.Fatal("expected non-nil cluster context")
	}
	if cc.Version != "4.14.5" {
		t.Errorf("expected version '4.14.5', got %q", cc.Version)
	}
	if cc.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", cc.Namespace)
	}
	if !strings.Contains(cc.Cluster, "api-mycluster-example-com") {
		t.Errorf("expected cluster containing 'api-mycluster-example-com', got %q", cc.Cluster)
	}
	// 3 nodes: 2 control-plane, 2 worker (one node has both roles)
	if !strings.Contains(cc.Nodes, "3") {
		t.Errorf("expected 3 total nodes, got %q", cc.Nodes)
	}
	if len(cc.Operators) != 2 {
		t.Errorf("expected 2 operators, got %d: %v", len(cc.Operators), cc.Operators)
	}
}

// --- parseOptions additional coverage ---

func TestParseOptions_ExtractsAllFields(t *testing.T) {
	raw := map[string]any{
		"consoleOfflineToken":  "tok-123",
		"clientId":             "client-abc",
		"apiUrl":               "https://api.example.com",
		"clusterId":            "cluster-xyz",
		"insecureSkipTlsVerify": true,
	}
	opts := parseOptions(raw)
	if opts.ConsoleOfflineToken != "tok-123" {
		t.Errorf("ConsoleOfflineToken = %q", opts.ConsoleOfflineToken)
	}
	if opts.ClientID != "client-abc" {
		t.Errorf("ClientID = %q", opts.ClientID)
	}
	if opts.APIURL != "https://api.example.com" {
		t.Errorf("APIURL = %q", opts.APIURL)
	}
	if opts.ClusterID != "cluster-xyz" {
		t.Errorf("ClusterID = %q", opts.ClusterID)
	}
	if !opts.InsecureSkipTLS {
		t.Error("InsecureSkipTLS should be true")
	}
}

func TestParseOptions_IgnoresWrongTypes(t *testing.T) {
	raw := map[string]any{
		"consoleOfflineToken":  42,
		"clientId":             true,
		"apiUrl":               []string{"nope"},
		"clusterId":            nil,
		"insecureSkipTlsVerify": "yes",
	}
	opts := parseOptions(raw)
	if opts.ConsoleOfflineToken != "" {
		t.Errorf("ConsoleOfflineToken should be empty for wrong type, got %q", opts.ConsoleOfflineToken)
	}
	if opts.ClientID != "" {
		t.Errorf("ClientID should be empty for wrong type, got %q", opts.ClientID)
	}
	if opts.APIURL != "" {
		t.Errorf("APIURL should be empty for wrong type, got %q", opts.APIURL)
	}
	if opts.ClusterID != "" {
		t.Errorf("ClusterID should be empty for wrong type, got %q", opts.ClusterID)
	}
	if opts.InsecureSkipTLS {
		t.Error("InsecureSkipTLS should be false for wrong type")
	}
}
