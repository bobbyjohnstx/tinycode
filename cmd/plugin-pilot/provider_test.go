package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDoRequest_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := doRequest(ctx, providerConfig{baseURL: srv.URL}, http.MethodGet, "/", nil)
	if err == nil {
		t.Fatal("expected cancelled context to fail the request")
	}
}

func TestDoRequest_BodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("a"), maxProviderBody+8))
	}))
	defer srv.Close()

	_, _, err := doRequest(context.Background(), providerConfig{baseURL: srv.URL}, http.MethodGet, "/", nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected body limit error, got %v", err)
	}
}
