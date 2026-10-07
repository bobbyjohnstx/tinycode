package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenAuth_ValidToken(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := TokenAuth("test-secret-token")(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected inner handler to be called with valid token")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestTokenAuth_MissingHeader(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	handler := TokenAuth("test-secret-token")(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("inner handler should not be called without Authorization header")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("error = %q, want %q", body["error"], "unauthorized")
	}
}

func TestTokenAuth_WrongToken(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	handler := TokenAuth("correct-token")(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("inner handler should not be called with wrong token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestTokenAuth_OptionsRequestBypassesAuth(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler := TokenAuth("test-secret-token")(inner)

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	// No Authorization header set
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler should be called for OPTIONS without auth")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}

func TestTokenAuth_EmptyTokenDisablesAuth(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := TokenAuth("")(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Authorization header set
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler should be called when token is empty (auth disabled)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestTokenAuth_QueryParamRedirectsToStripToken(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	token := "test-secret-token"
	encoded := base64.StdEncoding.EncodeToString([]byte("user:" + token))
	handler := TokenAuth(token)(inner)

	req := httptest.NewRequest(http.MethodGet, "/dashboard?auth_token="+encoded+"&view=main", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("inner handler should not be called — middleware should redirect")
	}
	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
	}

	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected Location header on redirect")
	}
	if loc != "/dashboard?view=main" {
		t.Errorf("Location = %q, want %q", loc, "/dashboard?view=main")
	}

	// Cookie should still be set on the redirect response.
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == authCookieName {
			found = true
			if c.Value != encoded {
				t.Errorf("cookie value = %q, want %q", c.Value, encoded)
			}
		}
	}
	if !found {
		t.Error("expected auth cookie to be set on redirect response")
	}
}

func TestTokenAuth_QueryParamNoOtherParams(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called — middleware should redirect")
	})

	token := "test-secret-token"
	encoded := base64.StdEncoding.EncodeToString([]byte("user:" + token))
	handler := TokenAuth(token)(inner)

	req := httptest.NewRequest(http.MethodGet, "/app?auth_token="+encoded, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if loc != "/app" {
		t.Errorf("Location = %q, want %q", loc, "/app")
	}
}

func TestTokenAuth_HTMLUnauthorizedForBrowser(t *testing.T) {
	handler := TokenAuthMode("secret", AuthModeAPI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner should not run")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "tinycode web") {
		t.Fatalf("expected tinycode web hint in HTML, got %q", body)
	}
	if !strings.Contains(body, "API") {
		t.Fatalf("expected API-only hint in serve mode HTML, got %q", body)
	}
}

func TestTokenAuth_JSONUnauthorizedForAPI(t *testing.T) {
	handler := TokenAuthMode("secret", AuthModeWeb)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/session", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatal("API client should not receive HTML")
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "unauthorized" {
		t.Fatalf("error = %q", body["error"])
	}
}

func TestTokenAuth_CookieSameSiteLax(t *testing.T) {
	token := "test-secret-token"
	encoded := base64.StdEncoding.EncodeToString([]byte("user:" + token))
	handler := TokenAuth(token)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/?auth_token="+encoded, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName {
			if c.SameSite != http.SameSiteLaxMode {
				t.Fatalf("SameSite = %v, want Lax", c.SameSite)
			}
			return
		}
	}
	t.Fatal("auth cookie not set")
}

func TestIsAuthenticated(t *testing.T) {
	token := "sekrit"
	encoded := base64.StdEncoding.EncodeToString([]byte("u:" + token))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if IsAuthenticated(req, token) {
		t.Fatal("expected unauthenticated")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if !IsAuthenticated(req, token) {
		t.Fatal("expected bearer auth")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/?auth_token="+encoded, nil)
	if !IsAuthenticated(req2, token) {
		t.Fatal("expected query auth")
	}
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.AddCookie(&http.Cookie{Name: authCookieName, Value: encoded})
	if !IsAuthenticated(req3, token) {
		t.Fatal("expected cookie auth")
	}
	if !IsAuthenticated(req3, "") {
		t.Fatal("empty server token should treat as auth disabled (authenticated)")
	}
}
