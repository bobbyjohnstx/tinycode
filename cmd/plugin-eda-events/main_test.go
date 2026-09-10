package main

import (
	"regexp"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin(options{})
	if p.ID != "eda-events" {
		t.Errorf("got %q, want %q", p.ID, "eda-events")
	}
}

func TestPluginUnconfigured_NoTools(t *testing.T) {
	p := newPlugin(options{})
	if len(p.Tools) != 0 {
		t.Errorf("unconfigured plugin should have 0 tools, got %d", len(p.Tools))
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name              string
		raw               map[string]any
		edaEndpoint       string
		eventsLen         int
		sensitivePattLen  int
	}{
		{
			name:        "extracts all fields",
			raw:         map[string]any{"edaEndpoint": "http://eda:8080", "events": []any{"a", "b"}, "sensitivePatterns": []any{"^secret"}},
			edaEndpoint: "http://eda:8080",
			eventsLen:   2,
			sensitivePattLen: 1,
		},
		{
			name:        "empty map",
			raw:         map[string]any{},
			edaEndpoint: "",
			eventsLen:   0,
			sensitivePattLen: 0,
		},
		{
			name:        "wrong types ignored",
			raw:         map[string]any{"edaEndpoint": 42, "events": "not-array"},
			edaEndpoint: "",
			eventsLen:   0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions(tt.raw)
			if opts.EdaEndpoint != tt.edaEndpoint {
				t.Errorf("EdaEndpoint = %q, want %q", opts.EdaEndpoint, tt.edaEndpoint)
			}
			if len(opts.Events) != tt.eventsLen {
				t.Errorf("Events len = %d, want %d", len(opts.Events), tt.eventsLen)
			}
			if len(opts.SensitivePatterns) != tt.sensitivePattLen {
				t.Errorf("SensitivePatterns len = %d, want %d", len(opts.SensitivePatterns), tt.sensitivePattLen)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^sha256~`),
		regexp.MustCompile(`^sk-`),
		regexp.MustCompile(`(?i)^Bearer\s+`),
	}

	tests := []struct {
		name string
		data map[string]any
		want map[string]any
	}{
		{
			name: "redacts matching values",
			data: map[string]any{
				"token":   "sha256~abc123",
				"apiKey":  "sk-proj-test",
				"safe":    "hello world",
				"auth":    "Bearer token123",
			},
			want: map[string]any{
				"token":  "[REDACTED]",
				"apiKey": "[REDACTED]",
				"safe":   "hello world",
				"auth":   "[REDACTED]",
			},
		},
		{
			name: "recursively sanitizes nested maps",
			data: map[string]any{
				"outer": map[string]any{
					"secret": "sk-nested-key",
					"ok":     "fine",
				},
			},
			want: map[string]any{
				"outer": map[string]any{
					"secret": "[REDACTED]",
					"ok":     "fine",
				},
			},
		},
		{
			name: "preserves non-string values",
			data: map[string]any{
				"count": 42,
				"flag":  true,
			},
			want: map[string]any{
				"count": 42,
				"flag":  true,
			},
		},
		{
			name: "no patterns returns original data",
			data: map[string]any{"key": "sk-value"},
			want: map[string]any{"key": "sk-value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := patterns
			if tt.name == "no patterns returns original data" {
				p = nil
			}
			got := sanitize(tt.data, p)
			for k, wantV := range tt.want {
				gotV := got[k]
				switch wv := wantV.(type) {
				case string:
					if gv, ok := gotV.(string); !ok || gv != wv {
						t.Errorf("key %q = %v, want %q", k, gotV, wv)
					}
				case map[string]any:
					gm, ok := gotV.(map[string]any)
					if !ok {
						t.Errorf("key %q is not map[string]any", k)
						continue
					}
					for nk, nv := range wv {
						if gs, ok := gm[nk].(string); ok {
							if gs != nv.(string) {
								t.Errorf("nested key %q.%q = %q, want %q", k, nk, gs, nv)
							}
						}
					}
				}
			}
		})
	}
}

func TestDefaultSensitivePatterns(t *testing.T) {
	for _, p := range defaultSensitivePatterns {
		if _, err := regexp.Compile(p); err != nil {
			t.Errorf("invalid default pattern %q: %v", p, err)
		}
	}
}
