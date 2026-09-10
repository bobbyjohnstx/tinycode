package frontmatter

import "testing"

func TestParse_ValidFrontmatter(t *testing.T) {
	content := "---\nname: test-skill\ndescription: A test skill\n---\nBody content here."
	fm, body := Parse(content)

	if fm == nil {
		t.Fatal("expected non-nil frontmatter map")
	}
	if fm["name"] != "test-skill" {
		t.Errorf("name = %q, want %q", fm["name"], "test-skill")
	}
	if fm["description"] != "A test skill" {
		t.Errorf("description = %q, want %q", fm["description"], "A test skill")
	}
	if len(fm) != 2 {
		t.Errorf("len(fm) = %d, want 2", len(fm))
	}
	if body != "\nBody content here." {
		t.Errorf("body = %q, want %q", body, "\nBody content here.")
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	content := "Just plain text without frontmatter."
	fm, body := Parse(content)

	if fm != nil {
		t.Errorf("expected nil frontmatter, got %v", fm)
	}
	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
}

func TestParse_UnterminatedFrontmatter(t *testing.T) {
	content := "---\nname: test\nno closing delimiter"
	fm, body := Parse(content)

	if fm != nil {
		t.Errorf("expected nil frontmatter, got %v", fm)
	}
	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
}

func TestParse_EmptyFrontmatter(t *testing.T) {
	content := "---\n\n---\nbody"
	fm, body := Parse(content)

	if fm == nil {
		t.Fatal("expected non-nil frontmatter map")
	}
	if len(fm) != 0 {
		t.Errorf("len(fm) = %d, want 0", len(fm))
	}
	if body != "\nbody" {
		t.Errorf("body = %q, want %q", body, "\nbody")
	}
}

func TestParse_WhitespaceAroundKeysAndValues(t *testing.T) {
	content := "---\n  name  :  my-skill  \n  version  :  1.0  \n---\n"
	fm, _ := Parse(content)

	if fm == nil {
		t.Fatal("expected non-nil frontmatter map")
	}
	if fm["name"] != "my-skill" {
		t.Errorf("name = %q, want %q", fm["name"], "my-skill")
	}
	if fm["version"] != "1.0" {
		t.Errorf("version = %q, want %q", fm["version"], "1.0")
	}
}

func TestParse_ColonInValue(t *testing.T) {
	content := "---\nurl: http://example.com:8080/path\n---\n"
	fm, _ := Parse(content)

	if fm == nil {
		t.Fatal("expected non-nil frontmatter map")
	}
	if fm["url"] != "http://example.com:8080/path" {
		t.Errorf("url = %q, want %q", fm["url"], "http://example.com:8080/path")
	}
}

func TestParse_EmptyBodyAfterFrontmatter(t *testing.T) {
	content := "---\nkey: value\n---\n"
	fm, body := Parse(content)

	if fm == nil {
		t.Fatal("expected non-nil frontmatter map")
	}
	if fm["key"] != "value" {
		t.Errorf("key = %q, want %q", fm["key"], "value")
	}
	if body != "\n" {
		t.Errorf("body = %q, want %q", body, "\n")
	}
}

func TestParse_LinesWithoutColonsSkipped(t *testing.T) {
	content := "---\nname: test\ninvalid-line\ndescription: hello\n---\nbody"
	fm, _ := Parse(content)

	if fm == nil {
		t.Fatal("expected non-nil frontmatter map")
	}
	if len(fm) != 2 {
		t.Errorf("len(fm) = %d, want 2 (invalid line should be skipped)", len(fm))
	}
	if fm["name"] != "test" {
		t.Errorf("name = %q, want %q", fm["name"], "test")
	}
	if fm["description"] != "hello" {
		t.Errorf("description = %q, want %q", fm["description"], "hello")
	}
}

func TestParse_EmptyLinesInFrontmatterSkipped(t *testing.T) {
	content := "---\nname: test\n\n\ndescription: hello\n---\nbody"
	fm, _ := Parse(content)

	if fm == nil {
		t.Fatal("expected non-nil frontmatter map")
	}
	if len(fm) != 2 {
		t.Errorf("len(fm) = %d, want 2", len(fm))
	}
}
