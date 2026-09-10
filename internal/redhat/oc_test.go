package redhat

import (
	"encoding/json"
	"testing"
)

func TestOcError_Error(t *testing.T) {
	err := &OcError{Msg: "oc get pods failed with exit code 1", ExitCode: 1, Stderr: "error: forbidden"}
	if err.Error() != "oc get pods failed with exit code 1" {
		t.Errorf("Error() = %q, want %q", err.Error(), "oc get pods failed with exit code 1")
	}
}

func TestOcError_Fields(t *testing.T) {
	tests := []struct {
		name     string
		err      OcError
		wantMsg  string
		wantCode int
		wantErr  string
	}{
		{
			name:     "exit code 1",
			err:      OcError{Msg: "oc whoami failed with exit code 1", ExitCode: 1, Stderr: "not logged in"},
			wantMsg:  "oc whoami failed with exit code 1",
			wantCode: 1,
			wantErr:  "not logged in",
		},
		{
			name:     "exit code 127",
			err:      OcError{Msg: "oc version failed with exit code 127", ExitCode: 127, Stderr: "command not found"},
			wantMsg:  "oc version failed with exit code 127",
			wantCode: 127,
			wantErr:  "command not found",
		},
		{
			name:     "empty stderr",
			err:      OcError{Msg: "oc apply failed", ExitCode: 2, Stderr: ""},
			wantMsg:  "oc apply failed",
			wantCode: 2,
			wantErr:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Msg != tt.wantMsg {
				t.Errorf("Msg = %q, want %q", tt.err.Msg, tt.wantMsg)
			}
			if tt.err.ExitCode != tt.wantCode {
				t.Errorf("ExitCode = %d, want %d", tt.err.ExitCode, tt.wantCode)
			}
			if tt.err.Stderr != tt.wantErr {
				t.Errorf("Stderr = %q, want %q", tt.err.Stderr, tt.wantErr)
			}
		})
	}
}

func TestOcError_ImplementsError(t *testing.T) {
	var err error = &OcError{Msg: "test"}
	if err.Error() != "test" {
		t.Errorf("error interface Error() = %q, want test", err.Error())
	}
}

func TestOcGetOptions_Fields(t *testing.T) {
	opts := OcGetOptions{
		Namespace:     "my-ns",
		Selector:      "app=web",
		FieldSelector: "status.phase=Running",
	}
	if opts.Namespace != "my-ns" {
		t.Errorf("Namespace = %q, want my-ns", opts.Namespace)
	}
	if opts.Selector != "app=web" {
		t.Errorf("Selector = %q, want app=web", opts.Selector)
	}
	if opts.FieldSelector != "status.phase=Running" {
		t.Errorf("FieldSelector = %q, want status.phase=Running", opts.FieldSelector)
	}
}

func TestOcGetOptions_ZeroValue(t *testing.T) {
	opts := OcGetOptions{}
	if opts.Namespace != "" || opts.Selector != "" || opts.FieldSelector != "" {
		t.Error("zero-value OcGetOptions should have empty fields")
	}
}

func TestOcLogOptions_Fields(t *testing.T) {
	opts := OcLogOptions{
		Container: "sidecar",
		Tail:      100,
		Since:     "1h",
	}
	if opts.Container != "sidecar" {
		t.Errorf("Container = %q, want sidecar", opts.Container)
	}
	if opts.Tail != 100 {
		t.Errorf("Tail = %d, want 100", opts.Tail)
	}
	if opts.Since != "1h" {
		t.Errorf("Since = %q, want 1h", opts.Since)
	}
}

func TestOcVersionInfo_JSONUnmarshal(t *testing.T) {
	data := `{
		"clientVersion": {"major":"4","minor":"14","gitVersion":"v4.14.0"},
		"serverVersion": {"major":"1","minor":"27"},
		"openshiftVersion": "4.14.0"
	}`
	var info OcVersionInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if info.ClientVersion["major"] != "4" {
		t.Errorf("ClientVersion.major = %q, want 4", info.ClientVersion["major"])
	}
	if info.ClientVersion["gitVersion"] != "v4.14.0" {
		t.Errorf("ClientVersion.gitVersion = %q, want v4.14.0", info.ClientVersion["gitVersion"])
	}
	if info.ServerVersion["major"] != "1" {
		t.Errorf("ServerVersion.major = %q, want 1", info.ServerVersion["major"])
	}
	if info.OpenshiftVersion != "4.14.0" {
		t.Errorf("OpenshiftVersion = %q, want 4.14.0", info.OpenshiftVersion)
	}
}

func TestOcVersionInfo_ClientOnly(t *testing.T) {
	data := `{"clientVersion": {"major":"4","minor":"14"}}`
	var info OcVersionInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if info.ServerVersion != nil {
		t.Error("expected nil ServerVersion when not provided")
	}
	if info.OpenshiftVersion != "" {
		t.Errorf("expected empty OpenshiftVersion, got %q", info.OpenshiftVersion)
	}
}

func TestOcVersionInfo_JSONMarshalRoundTrip(t *testing.T) {
	info := OcVersionInfo{
		ClientVersion:    map[string]string{"major": "4", "minor": "14"},
		ServerVersion:    map[string]string{"major": "1", "minor": "27"},
		OpenshiftVersion: "4.14.0",
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded OcVersionInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.ClientVersion["major"] != "4" {
		t.Errorf("roundtrip: ClientVersion.major = %q, want 4", decoded.ClientVersion["major"])
	}
	if decoded.OpenshiftVersion != "4.14.0" {
		t.Errorf("roundtrip: OpenshiftVersion = %q, want 4.14.0", decoded.OpenshiftVersion)
	}
}

func TestNewOcClient_ReturnsNonNil(t *testing.T) {
	c := NewOcClient()
	if c == nil {
		t.Fatal("NewOcClient() returned nil")
	}
}
