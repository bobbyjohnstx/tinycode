package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
