package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestProviderPathUsable_Scenario(t *testing.T) {
	tests := []struct {
		name               string
		anyLocalConnected  bool
		hasCloudAPIKey     bool
		anyConfigConnected bool
		want               bool
	}{
		{
			name:              "cloud only with openrouter key",
			hasCloudAPIKey:    true,
			anyLocalConnected: false,
			want:              true,
		},
		{
			name:              "local only",
			anyLocalConnected: true,
			want:              true,
		},
		{
			name:               "config provider only",
			anyConfigConnected: true,
			want:               true,
		},
		{
			name:              "no local and no cloud key",
			anyLocalConnected: false,
			hasCloudAPIKey:    false,
			want:              false,
		},
		{
			name:               "all paths available",
			anyLocalConnected:  true,
			hasCloudAPIKey:     true,
			anyConfigConnected: true,
			want:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerPathUsable(tt.anyLocalConnected, tt.hasCloudAPIKey, tt.anyConfigConnected)
			if got != tt.want {
				t.Errorf("providerPathUsable(%v, %v, %v) = %v, want %v",
					tt.anyLocalConnected, tt.hasCloudAPIKey, tt.anyConfigConnected, got, tt.want)
			}
		})
	}
}

func TestPrintProviderNextSteps_MentionsConnect(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	printProviderNextSteps()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "/connect") {
		t.Errorf("expected /connect in next steps, got %q", out)
	}
	if !strings.Contains(out, "tinycode") {
		t.Errorf("expected tinycode launch hint in next steps, got %q", out)
	}
}
