FROM registry.access.redhat.com/ubi9/ubi-minimal AS base

RUN microdnf install -y ripgrep && microdnf clean all

ARG TARGETARCH
FROM base AS build-amd64
COPY dist/tinycode-linux-amd64 /usr/local/bin/tinycode

FROM base AS build-arm64
COPY dist/tinycode-linux-arm64 /usr/local/bin/tinycode

FROM build-${TARGETARCH}
RUN chmod +x /usr/local/bin/tinycode && tinycode --version
ENTRYPOINT ["tinycode"]
