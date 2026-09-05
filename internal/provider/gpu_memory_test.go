package provider

import (
	"testing"
)

func TestParseSysctlMemsize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:  "16GB Mac",
			input: "17179869184\n",
			want:  17179869184 * 3 / 4,
		},
		{
			name:  "32GB Mac",
			input: "34359738368\n",
			want:  34359738368 * 3 / 4,
		},
		{
			name:  "64GB Mac no trailing newline",
			input: "68719476736",
			want:  68719476736 * 3 / 4,
		},
		{
			name:    "empty output",
			input:   "",
			wantErr: true,
		},
		{
			name:    "non-numeric output",
			input:   "not-a-number\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSysctlMemsize(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseSysctlMemsize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseNvidiaSMIOutput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:  "single GPU 24GB",
			input: "24576\n",
			want:  24576 * 1024 * 1024,
		},
		{
			name:  "two GPUs",
			input: "24576\n24576\n",
			want:  2 * 24576 * 1024 * 1024,
		},
		{
			name:  "GPU with extra whitespace",
			input: "  8192  \n",
			want:  8192 * 1024 * 1024,
		},
		{
			name:  "GPU with MiB suffix",
			input: "24576 MiB\n",
			want:  24576 * 1024 * 1024,
		},
		{
			name:    "empty output",
			input:   "",
			wantErr: true,
		},
		{
			name:    "no numeric values",
			input:   "no gpu found\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNvidiaSMIOutput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseNvidiaSMIOutput(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
