package main

import (
	"strings"
	"time"
)

// auditEvent is a minimal struct for the fields we need from Kubernetes audit
// events. We intentionally avoid parsing the full event to keep memory low.
type auditEvent struct {
	Verb      string       `json:"verb"`
	User      auditUser    `json:"user"`
	ObjectRef *auditObjRef `json:"objectRef"`
	RequestURI string      `json:"requestURI"`

	ResponseStatus *auditStatus `json:"responseStatus"`

	RequestReceivedTimestamp string `json:"requestReceivedTimestamp"`
	StageTimestamp           string `json:"stageTimestamp"`

	UserAgent string `json:"userAgent"`
}

type auditUser struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
}

type auditObjRef struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	APIGroup   string `json:"apiGroup"`
	APIVersion string `json:"apiVersion"`
}

type auditStatus struct {
	Code int `json:"code"`
}

// filter holds optional predicates for stream-filtering audit events.
type filter struct {
	Verb       string
	User       string
	Resource   string
	Namespace  string
	StatusCode int
}

// matches returns true if the event passes all non-empty filter fields.
func (f *filter) matches(ev *auditEvent) bool {
	if f.Verb != "" && !strings.EqualFold(ev.Verb, f.Verb) {
		return false
	}
	if f.User != "" && !strings.Contains(strings.ToLower(ev.User.Username), strings.ToLower(f.User)) {
		return false
	}
	if f.Resource != "" {
		res := ""
		if ev.ObjectRef != nil {
			res = ev.ObjectRef.Resource
		}
		if !strings.EqualFold(res, f.Resource) {
			return false
		}
	}
	if f.Namespace != "" {
		ns := ""
		if ev.ObjectRef != nil {
			ns = ev.ObjectRef.Namespace
		}
		if !strings.EqualFold(ns, f.Namespace) {
			return false
		}
	}
	if f.StatusCode != 0 {
		code := 0
		if ev.ResponseStatus != nil {
			code = ev.ResponseStatus.Code
		}
		if code != f.StatusCode {
			return false
		}
	}
	return true
}

// eventResource returns the resource string from an audit event, or "(unknown)".
func eventResource(ev *auditEvent) string {
	if ev.ObjectRef != nil && ev.ObjectRef.Resource != "" {
		r := ev.ObjectRef.Resource
		if ev.ObjectRef.APIGroup != "" {
			r += "." + ev.ObjectRef.APIGroup
		}
		return r
	}
	return "(unknown)"
}

// eventNamespace returns the namespace from an audit event, or "(cluster-scoped)".
func eventNamespace(ev *auditEvent) string {
	if ev.ObjectRef != nil && ev.ObjectRef.Namespace != "" {
		return ev.ObjectRef.Namespace
	}
	return "(cluster-scoped)"
}

// parseTimestamp parses the Kubernetes audit timestamp format.
func parseTimestamp(s string) (time.Time, error) {
	// Try RFC3339Nano first (most common), then RFC3339.
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	return t, err
}

// truncateToHour truncates a time to the start of its hour.
func truncateToHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

// truncateToMinute truncates a time to the start of its minute.
func truncateToMinute(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
}

// isPrivilegeEscalation checks if the event is a privilege-escalation-related action.
func isPrivilegeEscalation(ev *auditEvent) bool {
	if ev.ObjectRef == nil {
		return false
	}
	r := ev.ObjectRef.Resource
	g := ev.ObjectRef.APIGroup

	// RBAC mutations
	if g == "rbac.authorization.k8s.io" &&
		(r == "clusterroles" || r == "clusterrolebindings" || r == "roles" || r == "rolebindings") &&
		(ev.Verb == "create" || ev.Verb == "update" || ev.Verb == "patch") {
		return true
	}

	// Impersonation (user/group headers in URI)
	if strings.Contains(ev.RequestURI, "impersonate") {
		return true
	}

	// ServiceAccount token requests
	if r == "serviceaccounts" && strings.Contains(ev.RequestURI, "/token") {
		return true
	}

	return false
}

// isServiceAccount returns true if the username looks like a service account.
func isServiceAccount(username string) bool {
	return strings.HasPrefix(username, "system:serviceaccount:")
}
