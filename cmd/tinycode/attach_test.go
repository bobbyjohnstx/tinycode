package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tui/api"
)

func TestResolveAttachToken_Precedence(t *testing.T) {
	t.Setenv("TINYCODE_AUTH_TOKEN", "from-auth")
	t.Setenv("TINYCODE_SERVER_PASSWORD", "legacy")
	if got := resolveAttachToken("flag"); got != "flag" {
		t.Fatalf("flag = %q", got)
	}
	if got := resolveAttachToken(""); got != "from-auth" {
		t.Fatalf("auth token = %q", got)
	}
	t.Setenv("TINYCODE_AUTH_TOKEN", "")
	if got := resolveAttachToken(""); got != "legacy" {
		t.Fatalf("legacy = %q", got)
	}
}

func TestResolveRemoteResume_ByIDAndContinue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/session/ses_exact":
			_ = json.NewEncoder(w).Encode(session.Info{ID: "ses_exact", Title: "Exact"})
		case r.Method == http.MethodGet && r.URL.Path == "/session":
			_ = json.NewEncoder(w).Encode([]session.Info{
				{ID: "ses_new", Title: "Newest", Slug: "newest"},
				{ID: "ses_old", Title: "Oldest", Slug: "oldest"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)

	client := api.New(ts.URL, "/tmp", "")

	id, err := resolveRemoteResume(client, false, "ses_exact")
	if err != nil || id != "ses_exact" {
		t.Fatalf("by id: id=%q err=%v", id, err)
	}

	id, err = resolveRemoteResume(client, false, "newest")
	if err != nil || id != "ses_new" {
		t.Fatalf("by slug: id=%q err=%v", id, err)
	}

	id, err = resolveRemoteResume(client, true, "")
	if err != nil || id != "ses_new" {
		t.Fatalf("continue: id=%q err=%v", id, err)
	}

	id, err = resolveRemoteResume(client, false, "")
	if err != nil || id != "" {
		t.Fatalf("none: id=%q err=%v", id, err)
	}
}

func TestResolveRemoteResume_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode([]session.Info{})
	}))
	t.Cleanup(ts.Close)

	client := api.New(ts.URL, "/tmp", "")
	_, err := resolveRemoteResume(client, false, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}
