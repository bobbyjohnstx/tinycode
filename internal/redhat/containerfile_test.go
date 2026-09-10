package redhat

import (
	"testing"
)

func TestParseContainerfile_SingleStage(t *testing.T) {
	content := `FROM golang:1.21 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/app ./cmd/app
EXPOSE 8080/tcp
USER nonroot
`
	parsed := ParseContainerfile(content)

	if len(parsed.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(parsed.Stages))
	}

	from := parsed.Stages[0].From
	if from.Image != "golang" {
		t.Errorf("expected image 'golang', got %q", from.Image)
	}
	if from.Tag != "1.21" {
		t.Errorf("expected tag '1.21', got %q", from.Tag)
	}
	if from.Alias != "builder" {
		t.Errorf("expected alias 'builder', got %q", from.Alias)
	}

	instrs := parsed.Stages[0].Instructions
	if len(instrs) != 8 {
		t.Fatalf("expected 8 instructions, got %d", len(instrs))
	}
}

func TestParseContainerfile_MultiStage(t *testing.T) {
	content := `FROM golang:1.21 AS builder
RUN go build -o /app

FROM scratch
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
`
	parsed := ParseContainerfile(content)

	if len(parsed.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(parsed.Stages))
	}

	if parsed.Stages[1].From.Image != "scratch" {
		t.Errorf("expected second stage image 'scratch', got %q", parsed.Stages[1].From.Image)
	}

	copyInstr := parsed.Stages[1].Instructions[1]
	if copyInstr.Type != InstrCopy {
		t.Errorf("expected COPY instruction, got %s", copyInstr.Type)
	}
	if copyInstr.CopyFrom != "builder" {
		t.Errorf("expected --from=builder, got %q", copyInstr.CopyFrom)
	}
}

func TestParseContainerfile_GlobalArgs(t *testing.T) {
	content := `ARG BASE_IMAGE=golang:1.21
FROM ${BASE_IMAGE}
RUN echo hello
`
	parsed := ParseContainerfile(content)

	if len(parsed.GlobalArgs) != 1 {
		t.Fatalf("expected 1 global arg, got %d", len(parsed.GlobalArgs))
	}
	if parsed.GlobalArgs[0].ArgName != "BASE_IMAGE" {
		t.Errorf("expected arg name 'BASE_IMAGE', got %q", parsed.GlobalArgs[0].ArgName)
	}
	if parsed.GlobalArgs[0].DefaultValue != "golang:1.21" {
		t.Errorf("expected default 'golang:1.21', got %q", parsed.GlobalArgs[0].DefaultValue)
	}
}

func TestParseContainerfile_DigestRef(t *testing.T) {
	content := `FROM ubuntu@sha256:abc123
RUN apt-get update
`
	parsed := ParseContainerfile(content)
	from := parsed.Stages[0].From
	if from.Image != "ubuntu" {
		t.Errorf("expected image 'ubuntu', got %q", from.Image)
	}
	if from.Digest != "sha256:abc123" {
		t.Errorf("expected digest 'sha256:abc123', got %q", from.Digest)
	}
}

func TestParseContainerfile_LineContinuation(t *testing.T) {
	content := `FROM alpine
RUN apk add --no-cache \
    curl \
    git
`
	parsed := ParseContainerfile(content)
	run := parsed.Stages[0].Instructions[1]
	if run.Type != InstrRun {
		t.Fatalf("expected RUN instruction, got %s", run.Type)
	}
	if run.Command != "apk add --no-cache curl git" {
		t.Errorf("unexpected merged command: %q", run.Command)
	}
}

func TestParseContainerfile_EnvKeyValue(t *testing.T) {
	content := `FROM alpine
ENV APP_HOME="/app" PORT=8080
`
	parsed := ParseContainerfile(content)
	envs := parsed.Stages[0].Instructions[1:]
	if len(envs) != 2 {
		t.Fatalf("expected 2 ENV instructions, got %d", len(envs))
	}
	if envs[0].Key != "APP_HOME" || envs[0].EnvValue != "/app" {
		t.Errorf("unexpected ENV: key=%q value=%q", envs[0].Key, envs[0].EnvValue)
	}
	if envs[1].Key != "PORT" || envs[1].EnvValue != "8080" {
		t.Errorf("unexpected ENV: key=%q value=%q", envs[1].Key, envs[1].EnvValue)
	}
}

func TestExtractDependencies_Pip(t *testing.T) {
	content := `FROM python:3.11
RUN pip install flask==2.3.0 requests>=2.28.0 numpy
`
	parsed := ParseContainerfile(content)
	deps := ExtractDependencies(parsed)

	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d", len(deps))
	}

	if deps[0].Name != "flask" || deps[0].Version != "2.3.0" || deps[0].Source != DepPip {
		t.Errorf("unexpected dep: %+v", deps[0])
	}
	if deps[1].Name != "requests" || deps[1].Version != ">=2.28.0" {
		t.Errorf("unexpected dep: %+v", deps[1])
	}
	if deps[2].Name != "numpy" || deps[2].Version != "" {
		t.Errorf("unexpected dep: %+v", deps[2])
	}
}

func TestExtractDependencies_Dnf(t *testing.T) {
	content := `FROM ubi9
RUN dnf install -y httpd mod_ssl && dnf clean all
`
	parsed := ParseContainerfile(content)
	deps := ExtractDependencies(parsed)

	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
	if deps[0].Name != "httpd" || deps[0].Source != DepDnf {
		t.Errorf("unexpected dep: %+v", deps[0])
	}
}

func TestExtractDependencies_Go(t *testing.T) {
	content := `FROM golang:1.21
RUN go get github.com/gin-gonic/gin@v1.9.1
`
	parsed := ParseContainerfile(content)
	deps := ExtractDependencies(parsed)

	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "github.com/gin-gonic/gin" || deps[0].Version != "v1.9.1" || deps[0].Source != DepGo {
		t.Errorf("unexpected dep: %+v", deps[0])
	}
}
