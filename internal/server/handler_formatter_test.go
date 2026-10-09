package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/formatter"
)

func TestHandleFormatter_DisabledReturnsEmptyList(t *testing.T) {
	s := &Server{deps: Dependencies{Config: &config.Info{}}}
	rec := httptest.NewRecorder()
	s.handleFormatter(rec, httptest.NewRequest(http.MethodGet, "/formatter", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []formatter.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	if len(got) != 0 {
		t.Fatalf("status = %#v", got)
	}
}

func TestHandleFormatter_TrueReturnsGofmt(t *testing.T) {
	on := true
	s := &Server{deps: Dependencies{Config: &config.Info{
		Formatter: &config.FormatterConfig{Enabled: &on},
	}}}
	rec := httptest.NewRecorder()
	s.handleFormatter(rec, httptest.NewRequest(http.MethodGet, "/formatter", nil))
	var got []formatter.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body %s: %v", rec.Body.String(), err)
	}
	if len(got) != 1 || got[0].Name != "gofmt" || !got[0].Enabled {
		t.Fatalf("status = %#v", got)
	}
}
