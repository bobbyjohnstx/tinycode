# Multi-stage build for tinycode-go
# Build:        podman build -t tinycode .
# With plugins: podman build --build-arg BUILD_PLUGINS=1 -t tinycode .
# Run (TUI):    podman run --rm -it tinycode
# Run (web):    podman run --rm -p 4096:4096 tinycode web --hostname 0.0.0.0

# --- Stage 1: Build ---
FROM docker.io/library/golang:1.27-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_PLUGINS=0

RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /out/tinycode ./cmd/tinycode

RUN mkdir -p /out/plugins && \
    if [ "${BUILD_PLUGINS}" = "1" ]; then \
    for dir in cmd/plugin-*/; do \
        name=$(basename "$dir"); \
        echo "Building plugin ${name} ..." && \
        CGO_ENABLED=0 go build -ldflags "-s -w" -o "/out/plugins/${name#plugin-}" "./$dir"; \
    done; \
    fi

# --- Stage 2: Runtime ---
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Core tools
RUN microdnf install -y \
    git-core \
    tar \
    gzip \
    findutils \
    procps-ng \
    jq \
    --nodocs && microdnf clean all

# OpenShift CLI (oc + kubectl)
ARG OCP_VERSION=stable
ARG TARGETARCH=amd64
RUN OC_SUFFIX="" && \
    if [ "${TARGETARCH}" = "arm64" ]; then OC_SUFFIX="-arm64"; fi && \
    curl -sL "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/${OCP_VERSION}/openshift-client-linux${OC_SUFFIX}.tar.gz" \
    | tar xz -C /usr/local/bin oc kubectl && \
    chmod +x /usr/local/bin/oc /usr/local/bin/kubectl

# tinycode binary
COPY --from=builder /out/tinycode /usr/local/bin/tinycode

# Plugins (empty dir if BUILD_PLUGINS=0)
COPY --from=builder /out/plugins/ /opt/tinycode/plugins/

# Entrypoint
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# XDG directories
ENV XDG_DATA_HOME=/home/tinycode/.local/share \
    XDG_CONFIG_HOME=/home/tinycode/.config \
    XDG_STATE_HOME=/home/tinycode/.local/state \
    XDG_CACHE_HOME=/home/tinycode/.cache \
    HOME=/home/tinycode \
    SHELL=/bin/sh

# Create non-root user (GID 0 for OpenShift arbitrary UID support)
RUN useradd -u 1001 -r -g 0 -m -d /home/tinycode -s /bin/sh tinycode && \
    mkdir -p \
      /home/tinycode/.config/tinycode/plugins \
      /home/tinycode/.local/share/tinycode \
      /home/tinycode/.local/state/tinycode \
      /home/tinycode/.cache/tinycode \
      /projects && \
    chown -R 1001:0 /home/tinycode /projects && \
    chmod -R g=u /home/tinycode /projects

EXPOSE 4096

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD curl -sf http://localhost:4096/global/health || exit 1

USER 1001
WORKDIR /projects

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
