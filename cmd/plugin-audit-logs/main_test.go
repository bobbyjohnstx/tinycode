package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeFixture creates a temporary must-gather directory with audit log files
// and returns the root path. The caller should defer os.RemoveAll(root).
func makeFixture(t *testing.T, events []auditEvent) string {
	t.Helper()
	root := t.TempDir()

	dir := filepath.Join(root, "audit_logs", "kube-apiserver")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var fixtureEvents = []auditEvent{
	{
		Verb: "get",
		User: auditUser{Username: "system:admin"},
		ObjectRef: &auditObjRef{
			Resource: "pods", Namespace: "default", Name: "web-1",
		},
		ResponseStatus:              &auditStatus{Code: 200},
		RequestReceivedTimestamp:     "2024-01-15T10:00:00Z",
		RequestURI:                  "/api/v1/namespaces/default/pods/web-1",
	},
	{
		Verb: "list",
		User: auditUser{Username: "system:admin"},
		ObjectRef: &auditObjRef{
			Resource: "pods", Namespace: "kube-system",
		},
		ResponseStatus:              &auditStatus{Code: 200},
		RequestReceivedTimestamp:     "2024-01-15T10:05:00Z",
		RequestURI:                  "/api/v1/namespaces/kube-system/pods",
	},
	{
		Verb: "create",
		User: auditUser{Username: "developer"},
		ObjectRef: &auditObjRef{
			Resource: "deployments", Namespace: "myapp", APIGroup: "apps",
		},
		ResponseStatus:              &auditStatus{Code: 201},
		RequestReceivedTimestamp:     "2024-01-15T10:10:00Z",
		RequestURI:                  "/apis/apps/v1/namespaces/myapp/deployments",
	},
	{
		Verb: "delete",
		User: auditUser{Username: "developer"},
		ObjectRef: &auditObjRef{
			Resource: "pods", Namespace: "myapp", Name: "old-pod",
		},
		ResponseStatus:              &auditStatus{Code: 200},
		RequestReceivedTimestamp:     "2024-01-15T10:15:00Z",
		RequestURI:                  "/api/v1/namespaces/myapp/pods/old-pod",
	},
	{
		Verb: "get",
		User: auditUser{Username: "system:serviceaccount:monitoring:prometheus"},
		ObjectRef: &auditObjRef{
			Resource: "nodes",
		},
		ResponseStatus:              &auditStatus{Code: 200},
		RequestReceivedTimestamp:     "2024-01-15T11:00:00Z",
		RequestURI:                  "/api/v1/nodes",
	},
	{
		Verb: "get",
		User: auditUser{Username: "hacker"},
		ObjectRef: &auditObjRef{
			Resource: "secrets", Namespace: "kube-system",
		},
		ResponseStatus:              &auditStatus{Code: 403},
		RequestReceivedTimestamp:     "2024-01-15T11:05:00Z",
		RequestURI:                  "/api/v1/namespaces/kube-system/secrets",
	},
	{
		Verb: "create",
		User: auditUser{Username: "hacker"},
		ObjectRef: &auditObjRef{
			Resource:  "clusterrolebindings",
			APIGroup:  "rbac.authorization.k8s.io",
		},
		ResponseStatus:              &auditStatus{Code: 403},
		RequestReceivedTimestamp:     "2024-01-15T11:06:00Z",
		RequestURI:                  "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings",
	},
}

func TestAuditTop_ByUser(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditTop(root, "user", 10, &filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "system:admin") {
		t.Error("expected system:admin in output")
	}
	if !strings.Contains(out, "developer") {
		t.Error("expected developer in output")
	}
	if !strings.Contains(out, "7 total events") {
		t.Errorf("expected 7 total events, got:\n%s", out)
	}
}

func TestAuditTop_ByVerb(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditTop(root, "verb", 10, &filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "get") {
		t.Error("expected 'get' in output")
	}
}

func TestAuditTop_WithFilter(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditTop(root, "user", 10, &filter{Verb: "get"})
	if err != nil {
		t.Fatal(err)
	}
	// Only get events: system:admin(1), prometheus(1), hacker(1)
	if !strings.Contains(out, "3 total events") {
		t.Errorf("expected 3 total events with verb=get filter, got:\n%s", out)
	}
}

func TestAuditSearch_ByUser(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditSearch(root, 50, &filter{User: "hacker"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Matching events: 2") {
		t.Errorf("expected 2 matching events for user=hacker, got:\n%s", out)
	}
	if !strings.Contains(out, "403") {
		t.Error("expected status 403 in output")
	}
}

func TestAuditSearch_MaxLimit(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditSearch(root, 2, &filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "limit 2 reached") {
		t.Errorf("expected limit message, got:\n%s", out)
	}
}

func TestAuditSearch_NoMatches(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditSearch(root, 50, &filter{User: "nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No matching") {
		t.Errorf("expected no matches message, got:\n%s", out)
	}
}

func TestAuditTimeline_ByHour(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditTimeline(root, "hour", &filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2024-01-15 10:00") {
		t.Errorf("expected hour bucket 10:00, got:\n%s", out)
	}
	if !strings.Contains(out, "2024-01-15 11:00") {
		t.Errorf("expected hour bucket 11:00, got:\n%s", out)
	}
}

func TestAuditTimeline_ByMinute(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditTimeline(root, "minute", &filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2024-01-15 10:00") {
		t.Errorf("expected minute bucket 10:00, got:\n%s", out)
	}
	if !strings.Contains(out, "2024-01-15 10:10") {
		t.Errorf("expected minute bucket 10:10, got:\n%s", out)
	}
}

func TestAuditAnomalies(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditAnomalies(root)
	if err != nil {
		t.Fatal(err)
	}
	// Should detect auth failures
	if !strings.Contains(out, "Auth Failures") {
		t.Error("expected auth failures section")
	}
	if !strings.Contains(out, "hacker") {
		t.Error("expected hacker in auth failures")
	}
	// Should detect deletions
	if !strings.Contains(out, "Deletions") {
		t.Error("expected deletions section")
	}
	// Should detect privilege escalation (RBAC clusterrolebinding create)
	if !strings.Contains(out, "Privilege Escalation") {
		t.Error("expected privilege escalation section")
	}
	// Should detect service account activity
	if !strings.Contains(out, "Service Account") {
		t.Error("expected service account section")
	}
}

func TestAuditHealth(t *testing.T) {
	root := makeFixture(t, fixtureEvents)
	out, err := toolAuditHealth(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Health Summary") {
		t.Error("expected health summary header")
	}
	if !strings.Contains(out, "[OK") || !strings.Contains(out, "Overall:") {
		t.Errorf("expected OK rating and overall, got:\n%s", out)
	}
	if !strings.Contains(out, "Auth Failures") {
		t.Error("expected auth failures check")
	}
	if !strings.Contains(out, "Event Volume") {
		t.Error("expected event volume check")
	}
}

func TestAuditHealth_HighAuthFailures(t *testing.T) {
	// Create a fixture with > 10% auth failures to trigger CRITICAL
	events := make([]auditEvent, 0, 20)
	for i := 0; i < 15; i++ {
		events = append(events, auditEvent{
			Verb: "get",
			User: auditUser{Username: "baduser"},
			ObjectRef: &auditObjRef{
				Resource: "secrets", Namespace: "default",
			},
			ResponseStatus:          &auditStatus{Code: 403},
			RequestReceivedTimestamp: "2024-01-15T10:00:00Z",
			RequestURI:              "/api/v1/namespaces/default/secrets",
		})
	}
	for i := 0; i < 5; i++ {
		events = append(events, auditEvent{
			Verb: "get",
			User: auditUser{Username: "gooduser"},
			ObjectRef: &auditObjRef{
				Resource: "pods", Namespace: "default",
			},
			ResponseStatus:          &auditStatus{Code: 200},
			RequestReceivedTimestamp: "2024-01-15T10:00:00Z",
			RequestURI:              "/api/v1/namespaces/default/pods",
		})
	}

	root := makeFixture(t, events)
	out, err := toolAuditHealth(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CRITICAL") {
		t.Errorf("expected CRITICAL rating with 75%% auth failures, got:\n%s", out)
	}
}

func TestFilter_Matches(t *testing.T) {
	ev := &auditEvent{
		Verb: "create",
		User: auditUser{Username: "developer"},
		ObjectRef: &auditObjRef{
			Resource: "pods", Namespace: "myapp",
		},
		ResponseStatus: &auditStatus{Code: 201},
	}

	tests := []struct {
		name    string
		f       filter
		matches bool
	}{
		{"empty filter matches all", filter{}, true},
		{"verb match", filter{Verb: "create"}, true},
		{"verb mismatch", filter{Verb: "delete"}, false},
		{"user match (substring)", filter{User: "dev"}, true},
		{"user mismatch", filter{User: "admin"}, false},
		{"resource match", filter{Resource: "pods"}, true},
		{"resource mismatch", filter{Resource: "secrets"}, false},
		{"namespace match", filter{Namespace: "myapp"}, true},
		{"namespace mismatch", filter{Namespace: "default"}, false},
		{"status code match", filter{StatusCode: 201}, true},
		{"status code mismatch", filter{StatusCode: 200}, false},
		{"combined match", filter{Verb: "create", Namespace: "myapp"}, true},
		{"combined partial mismatch", filter{Verb: "create", Namespace: "wrong"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.matches(ev)
			if got != tt.matches {
				t.Errorf("filter %+v: got %v, want %v", tt.f, got, tt.matches)
			}
		})
	}
}

func TestFindAuditFiles_MissingDir(t *testing.T) {
	root := t.TempDir()
	_, err := findAuditFiles(root)
	if err == nil {
		t.Error("expected error for missing audit dirs")
	}
}

func TestStreamEvents_HostnamePrefix(t *testing.T) {
	// Some audit logs prefix lines with a hostname
	root := t.TempDir()
	dir := filepath.Join(root, "audit_logs", "kube-apiserver")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `master-0 {"verb":"get","user":{"username":"admin"},"requestReceivedTimestamp":"2024-01-15T10:00:00Z","requestURI":"/api/v1/pods"}
`
	if err := os.WriteFile(filepath.Join(dir, "audit.log"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var count int
	err := streamEvents(filepath.Join(dir, "audit.log"), func(ev *auditEvent) error {
		count++
		if ev.Verb != "get" {
			t.Errorf("expected verb=get, got %s", ev.Verb)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}
}

func TestIsPrivilegeEscalation(t *testing.T) {
	tests := []struct {
		name   string
		ev     auditEvent
		expect bool
	}{
		{
			name: "RBAC clusterrole create",
			ev: auditEvent{
				Verb: "create",
				ObjectRef: &auditObjRef{
					Resource: "clusterroles",
					APIGroup: "rbac.authorization.k8s.io",
				},
				RequestURI: "/apis/rbac.authorization.k8s.io/v1/clusterroles",
			},
			expect: true,
		},
		{
			name: "regular pod get",
			ev: auditEvent{
				Verb: "get",
				ObjectRef: &auditObjRef{
					Resource: "pods",
				},
				RequestURI: "/api/v1/pods",
			},
			expect: false,
		},
		{
			name: "impersonation",
			ev: auditEvent{
				Verb:       "create",
				ObjectRef:  &auditObjRef{Resource: "pods"},
				RequestURI: "/api/v1/pods?impersonate-user=admin",
			},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPrivilegeEscalation(&tt.ev)
			if got != tt.expect {
				t.Errorf("got %v, want %v", got, tt.expect)
			}
		})
	}
}
