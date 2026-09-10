package redhat

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type OcError struct {
	Msg      string
	ExitCode int
	Stderr   string
}

func (e *OcError) Error() string {
	return e.Msg
}

type OcGetOptions struct {
	Namespace     string
	Selector      string
	FieldSelector string
}

type OcLogOptions struct {
	Container string
	Tail      int
	Since     string
}

type OcVersionInfo struct {
	ClientVersion    map[string]string `json:"clientVersion"`
	ServerVersion    map[string]string `json:"serverVersion,omitempty"`
	OpenshiftVersion string            `json:"openshiftVersion,omitempty"`
}

type OcClient struct{}

func NewOcClient() *OcClient {
	return &OcClient{}
}

func (c *OcClient) exec(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "oc", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return "", &OcError{
			Msg:      fmt.Sprintf("oc %s failed with exit code %d", strings.Join(args, " "), exitCode),
			ExitCode: exitCode,
			Stderr:   string(out),
		}
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *OcClient) IsAvailable(ctx context.Context) bool {
	_, err := exec.LookPath("oc")
	if err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, "oc", "version", "--client")
	return cmd.Run() == nil
}

func (c *OcClient) IsLoggedIn(ctx context.Context) bool {
	_, err := c.exec(ctx, "whoami")
	return err == nil
}

func (c *OcClient) Get(ctx context.Context, resource string, opts *OcGetOptions) (json.RawMessage, error) {
	args := []string{"get", resource, "-o", "json"}
	if opts != nil {
		if opts.Namespace != "" {
			args = append(args, "--namespace", opts.Namespace)
		}
		if opts.Selector != "" {
			args = append(args, "--selector", opts.Selector)
		}
		if opts.FieldSelector != "" {
			args = append(args, "--field-selector", opts.FieldSelector)
		}
	}
	out, err := c.exec(ctx, args...)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

func (c *OcClient) Apply(ctx context.Context, manifest string) (string, error) {
	cmd := exec.CommandContext(ctx, "oc", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return "", &OcError{
			Msg:      fmt.Sprintf("oc apply failed with exit code %d", exitCode),
			ExitCode: exitCode,
			Stderr:   string(out),
		}
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *OcClient) Logs(ctx context.Context, pod string, opts *OcLogOptions) (string, error) {
	args := []string{"logs", pod}
	if opts != nil {
		if opts.Container != "" {
			args = append(args, "--container", opts.Container)
		}
		if opts.Tail > 0 {
			args = append(args, "--tail", fmt.Sprintf("%d", opts.Tail))
		}
		if opts.Since != "" {
			args = append(args, "--since", opts.Since)
		}
	}
	return c.exec(ctx, args...)
}

func (c *OcClient) Describe(ctx context.Context, resource, name string, namespace string) (string, error) {
	args := []string{"describe", resource, name}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	return c.exec(ctx, args...)
}

func (c *OcClient) Whoami(ctx context.Context) (string, error) {
	return c.exec(ctx, "whoami")
}

func (c *OcClient) Token(ctx context.Context) (string, error) {
	return c.exec(ctx, "whoami", "-t")
}

func (c *OcClient) Version(ctx context.Context) (*OcVersionInfo, error) {
	out, err := c.exec(ctx, "version", "-o", "json")
	if err != nil {
		return nil, err
	}
	var info OcVersionInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return nil, fmt.Errorf("parsing version output: %w", err)
	}
	return &info, nil
}

func (c *OcClient) Raw(ctx context.Context, args ...string) (string, error) {
	return c.exec(ctx, args...)
}
