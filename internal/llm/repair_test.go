package llm

import "testing"

func TestRepairToolCallJSON(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   *string
		nilOut bool
	}{
		{
			name:  "valid JSON passthrough",
			input: `{"key": "val"}`,
			want:  strPtr(`{"key": "val"}`),
		},
		{
			name:  "markdown fence with json tag",
			input: "```json\n{\"key\": \"val\"}\n```",
			want:  strPtr(`{"key": "val"}`),
		},
		{
			name:  "markdown fence without language",
			input: "```\n{\"key\": \"val\"}\n```",
			want:  strPtr(`{"key": "val"}`),
		},
		{
			name:  "trailing comma in object",
			input: `{"a": 1, "b": 2,}`,
			want:  strPtr(`{"a": 1, "b": 2}`),
		},
		{
			name:  "trailing comma in array",
			input: `[1, 2, 3,]`,
			want:  strPtr(`[1, 2, 3]`),
		},
		{
			name:  "leading and trailing whitespace",
			input: "  \n{\"key\": \"val\"}\n  ",
			want:  strPtr(`{"key": "val"}`),
		},
		{
			name:  "nested trailing commas",
			input: `{"a": [1, 2,], "b": {"c": 3,},}`,
			want:  strPtr(`{"a": [1, 2], "b": {"c": 3}}`),
		},
		{
			name:   "completely invalid JSON",
			input:  `{not json at all`,
			nilOut: true,
		},
		{
			name:   "empty string",
			input:  "",
			nilOut: true,
		},
		{
			name:  "empty object",
			input: `{}`,
			want:  strPtr(`{}`),
		},
		{
			name:  "fence with extra whitespace",
			input: "```json  \n{\"a\": 1}\n```  ",
			want:  strPtr(`{"a": 1}`),
		},
		{
			name:  "fence and trailing comma combined",
			input: "```json\n{\"a\": 1, \"b\": 2,}\n```",
			want:  strPtr(`{"a": 1, "b": 2}`),
		},
		{
			name:  "case insensitive fence",
			input: "```JSON\n{\"a\": 1}\n```",
			want:  strPtr(`{"a": 1}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RepairToolCallJSON(tt.input)
			if tt.nilOut {
				if got != nil {
					t.Errorf("RepairToolCallJSON(%q) = %q, want nil", tt.input, *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("RepairToolCallJSON(%q) = nil, want %q", tt.input, *tt.want)
			}
			if *got != *tt.want {
				t.Errorf("RepairToolCallJSON(%q) = %q, want %q", tt.input, *got, *tt.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
