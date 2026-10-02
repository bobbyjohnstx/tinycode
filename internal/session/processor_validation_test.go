package session

import (
	"context"
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
)

func TestCheckExternalDirectory_NilPerms_AllowsEverything(t *testing.T) {
	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Directory: "/project",
			Perms:     nil, // no permission service
		},
	}

	call := ToolCallPart("call-1", "read", `{"file_path":"/etc/passwd"}`)
	result := p.checkExternalDirectory(context.Background(), call)
	if result != "" {
		t.Errorf("expected empty string (allowed) when Perms is nil, got %q", result)
	}
}

func TestCheckExternalDirectory_WithPerms_DeniesExternalPath(t *testing.T) {
	b := bus.New()
	defer b.Close()

	perms := permission.NewService(b)

	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Directory: "/project",
			Perms:     perms,
			Ruleset: permission.Ruleset{
				{Permission: "external_directory", Pattern: "*", Action: permission.ActionDeny},
			},
		},
	}

	call := ToolCallPart("call-1", "read", `{"file_path":"/etc/passwd"}`)
	result := p.checkExternalDirectory(context.Background(), call)
	if result == "" {
		t.Error("expected denial message for external path when Perms is set, got empty string")
	}
}

func TestCheckExternalDirectory_WithPerms_AllowsInternalPath(t *testing.T) {
	b := bus.New()
	defer b.Close()

	perms := permission.NewService(b)

	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Directory: "/project",
			Perms:     perms,
			Ruleset: permission.Ruleset{
				{Permission: "external_directory", Pattern: "*", Action: permission.ActionDeny},
			},
		},
	}

	call := ToolCallPart("call-1", "read", `{"file_path":"/project/src/main.go"}`)
	result := p.checkExternalDirectory(context.Background(), call)
	if result != "" {
		t.Errorf("expected empty string for internal path, got %q", result)
	}
}

func TestCheckExternalDirectory_EmptyDirectory_AllowsEverything(t *testing.T) {
	b := bus.New()
	defer b.Close()

	perms := permission.NewService(b)

	p := &Processor{
		config: ProcessorConfig{
			SessionID: "ses-test",
			Directory: "", // no directory configured
			Perms:     perms,
		},
	}

	call := ToolCallPart("call-1", "read", `{"file_path":"/etc/passwd"}`)
	result := p.checkExternalDirectory(context.Background(), call)
	if result != "" {
		t.Errorf("expected empty string when Directory is empty, got %q", result)
	}
}
