package redhat

import "testing"

func TestStripHTML_Basic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text passes through",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "removes tags",
			input:    "<p>Hello <b>world</b></p>",
			expected: "Hello world",
		},
		{
			name:     "removes script tags and content",
			input:    `<p>Hello</p><script>alert("xss")</script><p>World</p>`,
			expected: "Hello World",
		},
		{
			name:     "removes style tags and content",
			input:    `<style>body{color:red}</style><p>Content</p>`,
			expected: "Content",
		},
		{
			name:     "decodes HTML entities",
			input:    `&amp; &lt; &gt; &quot; &#39; &nbsp;`,
			expected: `& < > " '`,
		},
		{
			name:     "collapses whitespace",
			input:    "<div>  hello   <span>  world  </span>  </div>",
			expected: "hello world",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripHTML(tt.input)
			if result != tt.expected {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
