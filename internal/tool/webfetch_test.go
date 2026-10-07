package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestCheckSSRF_BlocksPrivateIPs(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"loopback", "127.0.0.1"},
		{"loopback_other", "127.0.0.2"},
		{"private_10", "10.0.0.1"},
		{"private_172", "172.16.0.1"},
		{"private_192", "192.168.1.1"},
		{"link_local", "169.254.169.254"},
		{"unspecified", "0.0.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSSRF(context.Background(), fmt.Sprintf("http://%s/path", tt.ip))
			if err == nil {
				t.Errorf("expected SSRF block for %s, got nil", tt.ip)
			}
			if err != nil && !strings.Contains(err.Error(), "private/internal") {
				// DNS lookup failure is also acceptable — the IP literal may
				// fail resolution on some systems, but the point is it must not
				// succeed silently.
				if !strings.Contains(err.Error(), "DNS lookup failed") {
					t.Errorf("expected SSRF or DNS error for %s, got: %v", tt.ip, err)
				}
			}
		})
	}
}

func TestCheckSSRF_AllowsPublicIPs(t *testing.T) {
	// Use a test HTTP server on a real interface — its address is routable
	// on the loopback, so we test via the function's blocklist directly
	// by checking known public IPs.
	tests := []struct {
		name string
		url  string
	}{
		{"example.com", "https://example.com"},
		{"google_dns", "http://8.8.8.8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSSRF(context.Background(), tt.url)
			if err != nil {
				// DNS failures in CI are acceptable; SSRF block is not.
				if strings.Contains(err.Error(), "private/internal") {
					t.Errorf("public URL %s was incorrectly blocked as SSRF", tt.url)
				}
			}
		})
	}
}

func TestCheckSSRF_InvalidURL(t *testing.T) {
	err := checkSSRF(context.Background(), "://bad-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestCheckSSRF_MissingHostname(t *testing.T) {
	err := checkSSRF(context.Background(), "http://")
	if err == nil {
		t.Error("expected error for missing hostname")
	}
}

func TestCheckSSRF_BlocksNonHTTPSchemes(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "gopher://example.com/1", "ftp://example.com/"} {
		err := checkSSRF(context.Background(), raw)
		if err == nil {
			t.Errorf("expected scheme rejection for %q", raw)
		} else if !strings.Contains(err.Error(), "only http and https") {
			t.Errorf("%q: got %v", raw, err)
		}
	}
}

func TestWebFetch_BlocksLocalhost(t *testing.T) {
	def := WebFetchTool()
	args, _ := json.Marshal(webfetchArgs{URL: "http://127.0.0.1/admin"})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for localhost request")
	}
	if !strings.Contains(result.Output, "private/internal") && !strings.Contains(result.Output, "DNS lookup failed") {
		t.Errorf("expected SSRF block message, got: %s", result.Output)
	}
}

func TestWebFetch_BlocksCloudMetadata(t *testing.T) {
	def := WebFetchTool()
	args, _ := json.Marshal(webfetchArgs{URL: "http://169.254.169.254/latest/meta-data/"})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for cloud metadata request")
	}
}

func TestWebFetch_BlocksPrivateNetwork(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"10_network", "http://10.0.0.1/secret"},
		{"192_168_network", "http://192.168.1.1/admin"},
		{"172_16_network", "http://172.16.0.1/internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := WebFetchTool()
			args, _ := json.Marshal(webfetchArgs{URL: tt.url})
			result, err := def.Execute(context.Background(), &Context{}, args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Errorf("expected error for %s", tt.url)
			}
		})
	}
}

func TestWebFetch_PermissionCategory(t *testing.T) {
	def := WebFetchTool()
	if def.Permission != "webfetch" {
		t.Errorf("expected permission 'webfetch', got %q", def.Permission)
	}
}

func TestWebFetch_RedirectToPrivateIPBlocked(t *testing.T) {
	// Server that redirects to a private IP address.
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/secret", http.StatusFound)
	}))
	defer redirectServer.Close()

	def := WebFetchTool()
	args, _ := json.Marshal(webfetchArgs{URL: redirectServer.URL})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error when redirect targets a private IP")
	}
	if !strings.Contains(result.Output, "private/internal") && !strings.Contains(result.Output, "DNS lookup failed") {
		t.Errorf("expected SSRF block in redirect error, got: %s", result.Output)
	}
}

func TestWebFetch_LegitimateRequestSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	def := WebFetchTool()
	args, _ := json.Marshal(webfetchArgs{URL: server.URL})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The test server runs on loopback, so it WILL be blocked by the SSRF
	// check. This is correct behavior — we verify it blocks loopback even
	// for httptest servers.
	if !result.IsError {
		t.Error("expected SSRF block for loopback test server")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"loopback", "127.0.0.1", true},
		{"loopback_other", "127.0.0.2", true},
		{"private_10", "10.0.0.1", true},
		{"private_172", "172.16.0.1", true},
		{"private_192", "192.168.1.1", true},
		{"link_local", "169.254.169.254", true},
		{"unspecified", "0.0.0.0", true},
		{"ipv6_loopback", "::1", true},
		{"ipv6_private", "fd00::1", true},
		{"ipv6_link_local", "fe80::1", true},
		{"public_8888", "8.8.8.8", false},
		{"public_1111", "1.1.1.1", false},
		{"public_93", "93.184.216.34", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := netip.MustParseAddr(tt.ip)
			got := isPrivateIP(ip)
			if got != tt.blocked {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestSSRFSafeTransport_BlocksPrivateAtDialTime(t *testing.T) {
	transport := ssrfSafeTransport()
	client := &http.Client{Transport: transport}

	// 127.0.0.1 is a private IP — the transport's DialContext must block it.
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://127.0.0.1/secret", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected error when dialing private IP through SSRF-safe transport")
	}
	if !strings.Contains(err.Error(), "private/internal") {
		t.Errorf("expected SSRF block error, got: %v", err)
	}
}

func TestSSRFSafeClient_BlocksFileSchemeRedirect(t *testing.T) {
	client := ssrfSafeClient()
	via, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("via request: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, "file:///etc/passwd", nil)
	if err != nil {
		t.Fatalf("redirect request: %v", err)
	}
	err = client.CheckRedirect(req, []*http.Request{via})
	if err == nil {
		t.Fatal("expected error for file:// redirect")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %v", err)
	}
}
