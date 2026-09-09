package main

import (
	"net/url"
	"regexp"
	"strings"
)

// ParsedRemote holds the host, owner, and repo extracted from a git remote URL.
type ParsedRemote struct {
	Host  string
	Owner string
	Repo  string
}

var (
	// ssh://git@{host}/{path}[.git]
	sshProtoRe = regexp.MustCompile(`^ssh://[^@]+@([^/]+)/(.+?)(?:\.git)?$`)
	// git@{host}:{path}[.git]
	sshRe = regexp.MustCompile(`^[^@]+@([^:]+):(.+?)(?:\.git)?$`)
)

// parseGitRemoteURL parses a git remote URL into host, owner, and repo.
// Handles https://, git@, and ssh:// formats.
// For paths with 3+ segments (e.g. GitLab nested groups), the last segment
// is the repo and the rest form the owner.
func parseGitRemoteURL(rawURL string) *ParsedRemote {
	var host, pathPart string

	if m := sshProtoRe.FindStringSubmatch(rawURL); m != nil {
		host = m[1]
		pathPart = m[2]
	} else if m := sshRe.FindStringSubmatch(rawURL); m != nil {
		host = m[1]
		pathPart = m[2]
	} else {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Hostname() == "" {
			return nil
		}
		host = parsed.Hostname()
		pathPart = strings.TrimPrefix(parsed.Path, "/")
		pathPart = strings.TrimSuffix(pathPart, ".git")
	}

	if host == "" || pathPart == "" {
		return nil
	}

	segments := strings.Split(pathPart, "/")
	if len(segments) < 2 {
		return nil
	}

	repo := segments[len(segments)-1]
	owner := strings.Join(segments[:len(segments)-1], "/")

	return &ParsedRemote{Host: host, Owner: owner, Repo: repo}
}

var knownHosts = map[string]string{
	"github.com": "github",
	"gitlab.com": "gitlab",
}

// detectProvider determines the provider name from a remote URL with an
// optional environment override. Priority: envOverride > URL host > default (gitea).
func detectProvider(remoteURL string, envOverride string) string {
	switch envOverride {
	case "github", "gitlab", "gitea":
		return envOverride
	}

	if remoteURL == "" {
		return "gitea"
	}

	parsed := parseGitRemoteURL(remoteURL)
	if parsed == nil {
		return "gitea"
	}

	if name, ok := knownHosts[parsed.Host]; ok {
		return name
	}
	return "gitea"
}
