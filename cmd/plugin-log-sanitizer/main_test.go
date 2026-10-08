package main

import (
	"testing"
)

func TestSensitivePatternsNonEmpty(t *testing.T) {
	if len(sensitivePatterns) == 0 {
		t.Fatal("sensitivePatterns must not be empty")
	}
}

func TestAPIKeysRedacted(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"sk- prefix key",
			"key is sk-proj-abc123def456ghi789jkl012mno",
			"key is [REDACTED]",
		},
		{
			"api_key assignment",
			"api_key=supersecretvalue123",
			"[REDACTED]",
		},
		{
			"apikey header",
			"apikey: my-secret-api-key-value",
			"[REDACTED]",
		},
		{
			"AWS access key ID",
			"found AKIAIOSFODNN7EXAMPLE in config",
			"found [REDACTED] in config",
		},
		{
			"GitHub PAT",
			"token ghp_ABCDEFghijklmnopqrstuvwxyz012345678901",
			"token [REDACTED]",
		},
		{
			"GitHub OAuth token",
			"auth: gho_ABCDEFghijklmnopqrstuvwxyz012345678901",
			"auth: [REDACTED]",
		},
		{
			"GitLab PAT",
			"PRIVATE_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx",
			"PRIVATE_TOKEN=[REDACTED]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(tc.input)
			if got != tc.want {
				t.Errorf("sanitize(%q)\n got: %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTokensRedacted(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"Bearer token",
			"Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature",
			"Authorization: [REDACTED]",
		},
		{
			"token assignment",
			"token=abc123def456ghi789",
			"[REDACTED]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(tc.input)
			if got != tc.want {
				t.Errorf("sanitize(%q)\n got: %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPasswordsRedacted(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"password equals",
			"password=hunter2",
			"[REDACTED]",
		},
		{
			"passwd colon",
			"passwd: s3cretP@ss!",
			"[REDACTED]",
		},
		{
			"PASS variable",
			"PASS=myDatabasePass",
			"[REDACTED]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(tc.input)
			if got != tc.want {
				t.Errorf("sanitize(%q)\n got: %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPrivateKeysRedacted(t *testing.T) {
	rsaKey := `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy0AHB7MhgHcTz6sE2I2yPB
aFDrBz9vFqU4zK7G4lxfwAxMOcfOL+bRGJEMfC3XPZK
-----END RSA PRIVATE KEY-----`

	got := sanitize("key: " + rsaKey + " done")
	if got != "key: [REDACTED] done" {
		t.Errorf("RSA private key not redacted, got: %q", got)
	}

	ecKey := `-----BEGIN EC PRIVATE KEY-----
MHQCAQEEIBkg4LVWM9nuwNSk3yByxZpYRTBnVJk5GkMkGDLBMfBaoAcGBSuBBAAi
-----END EC PRIVATE KEY-----`

	got = sanitize(ecKey)
	if got != "[REDACTED]" {
		t.Errorf("EC private key not redacted, got: %q", got)
	}

	genericKey := `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC7
-----END PRIVATE KEY-----`

	got = sanitize(genericKey)
	if got != "[REDACTED]" {
		t.Errorf("generic private key not redacted, got: %q", got)
	}
}

func TestConnectionStringsRedacted(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"postgres connection string",
			"postgres://admin:s3cret@db.example.com:5432/mydb",
			"postgres[REDACTED]db.example.com:5432/mydb",
		},
		{
			"redis connection string",
			"redis://user:pass123@redis.local:6379",
			"redis[REDACTED]redis.local:6379",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(tc.input)
			if got != tc.want {
				t.Errorf("sanitize(%q)\n got: %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAWSSecretKeyRedacted(t *testing.T) {
	input := "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	got := sanitize(input)
	if got != "[REDACTED]" {
		t.Errorf("AWS secret key not redacted, got: %q", got)
	}
}

func TestNonSensitivePassthrough(t *testing.T) {
	safe := []string{
		"hello world",
		"go build ./...",
		"git status",
		"ls -la /tmp",
		"total 42\ndrwxr-xr-x  5 user staff 160 Sep  1 12:00 .",
		"PASS\nok  \tgithub.com/example/pkg\t0.042s",
		"the password policy requires 8 characters",
		"API documentation is available at /docs",
	}

	for _, text := range safe {
		t.Run(text[:min(len(text), 30)], func(t *testing.T) {
			got := sanitize(text)
			if got != text {
				t.Errorf("non-sensitive text was modified\n input: %q\n   got: %q", text, got)
			}
		})
	}
}

func TestEdgeCases(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		got := sanitize("")
		if got != "" {
			t.Errorf("empty string modified, got: %q", got)
		}
	})

	t.Run("already redacted", func(t *testing.T) {
		input := "password was [REDACTED]"
		got := sanitize(input)
		if got != input {
			t.Errorf("already-redacted text modified, got: %q", got)
		}
	})

	t.Run("multiple secrets in one string", func(t *testing.T) {
		input := "api_key=secret123 and password=hunter2"
		got := sanitize(input)
		if got == input {
			t.Errorf("expected redaction but got original: %q", got)
		}
		// Both should be redacted
		if got != "[REDACTED] and [REDACTED]" {
			t.Errorf("multiple secrets not fully redacted, got: %q", got)
		}
	})

	t.Run("short sk- prefix not matched", func(t *testing.T) {
		input := "sk-short"
		got := sanitize(input)
		if got != input {
			t.Errorf("short sk- prefix should not match, got: %q", got)
		}
	})
}

func TestAdditionalSecretsRedacted(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"openshift token",
			"token sha256~abcDEF_123-xyz",
			"token [REDACTED]",
		},
		{
			"github fine grained pat",
			"auth github_pat_11AAAAAAA0123456789",
			"auth [REDACTED]",
		},
		{
			"bare jwt",
			"cookie eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIn0.signaturepart",
			"cookie [REDACTED]",
		},
		{
			"quoted password with space",
			`password="hello world"`,
			"[REDACTED]",
		},
		{
			"openssh private key",
			"-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----",
			"[REDACTED]",
		},
		{
			"encrypted private key",
			"-----BEGIN ENCRYPTED PRIVATE KEY-----\nabc\n-----END ENCRYPTED PRIVATE KEY-----",
			"[REDACTED]",
		},
		{
			"token count unchanged",
			"token count: 3",
			"token count: 3",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(tc.input)
			if got != tc.want {
				t.Errorf("sanitize(%q)\n got: %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
