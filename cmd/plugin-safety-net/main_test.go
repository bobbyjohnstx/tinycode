package main

import (
	"testing"
)

func TestCheckPermission_AllowReturnsAllowedTrue(t *testing.T) {
	out := checkPermission("bash", "ls -la")
	if out == nil {
		t.Fatal("expected non-nil PermissionOutput on allow")
	}
	if !out.Allowed {
		t.Fatalf("expected Allowed=true, got %+v", out)
	}
}

func TestCheckPermission_Deny(t *testing.T) {
	out := checkPermission("bash", "rm -rf /")
	if out == nil || out.Allowed {
		t.Fatalf("expected deny, got %+v", out)
	}
}

func TestCheckPermission_IgnoresNonShellTools(t *testing.T) {
	out := checkPermission("read", "rm -rf /")
	if out == nil || !out.Allowed {
		t.Fatalf("non-shell tool should be allowed, got %+v", out)
	}
	out = checkPermission("shell", "rm -rf /")
	if out == nil || out.Allowed {
		t.Fatalf("shell permission should deny, got %+v", out)
	}
}

func TestDangerousPatternsNonEmpty(t *testing.T) {
	if len(dangerousPatterns) == 0 {
		t.Fatal("dangerousPatterns must not be empty")
	}
}

func TestDangerousCommandsBlocked(t *testing.T) {
	blocked := []struct {
		name string
		cmd  string
	}{
		{"rm -rf /", "rm -rf /"},
		{"rm -rf / with path", "rm -rf /etc"},
		{"chmod 777", "chmod 777 /tmp/file"},
		{"mkfs", "mkfs.ext4 /dev/sda1"},
		{"dd if=", "dd if=/dev/zero of=/dev/sda"},
		{"shutdown", "shutdown -h now"},
		{"reboot", "reboot"},
		{"fork bomb", ":(){ :|:& };:"},
		{"rm long recursive", "rm --recursive --force /"},
		{"rm long recursive absolute", "rm --recursive /home"},
		{"rm long recursive nested", "rm --recursive --force /var/log"},
		{"chmod recursive 777", "chmod -R 777 /tmp"},
		{"find delete", "find /var/tmp -name '*.log' -delete"},
		{"git clean force", "git clean -fdx"},
		{"git clean long force", "git clean --force -d"},
	}

	for _, tc := range blocked {
		t.Run(tc.name, func(t *testing.T) {
			if m := matchDangerous(tc.cmd); m == nil {
				t.Errorf("expected %q to be blocked, but it was allowed", tc.cmd)
			}
		})
	}
}

func TestSafeCommandsAllowed(t *testing.T) {
	safe := []struct {
		name string
		cmd  string
	}{
		{"ls", "ls -la"},
		{"git status", "git status"},
		{"go build", "go build ./..."},
		{"cat", "cat /etc/hosts"},
		{"echo", "echo hello"},
		{"mkdir", "mkdir -p /tmp/test"},
		{"rm single file", "rm /tmp/file.txt"},
		{"rm recursive relative", "rm --recursive ./rel"},
		{"git clean dry run", "git clean -n"},
		{"find print", "find . -name '*.go' -print"},
	}

	for _, tc := range safe {
		t.Run(tc.name, func(t *testing.T) {
			if m := matchDangerous(tc.cmd); m != nil {
				t.Errorf("expected %q to be allowed, but it matched %s", tc.cmd, m.String())
			}
		})
	}
}
