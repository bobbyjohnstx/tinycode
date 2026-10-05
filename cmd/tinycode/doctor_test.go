package main

import "testing"

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
