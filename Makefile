BINARY := tinycode
MODULE := github.com/bobbyjohnstx/tinycode-go
MAIN   := ./cmd/tinycode

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT) \
  -X main.date=$(DATE)

PLATFORMS := \
  linux/amd64 \
  linux/arm64 \
  darwin/amd64 \
  darwin/arm64 \
  windows/amd64

.PHONY: build build-all clean test vet lint check embed-webapp

## build: Build for the current platform
build:
	go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY) $(MAIN)

## build-all: Cross-compile for all supported platforms
build-all:
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		output="dist/$(BINARY)-$${os}-$${arch}$${ext}"; \
		echo "Building $$output ..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$output $(MAIN); \
	done

## package: Create release archives for all platforms
package: build-all
	@mkdir -p dist/release
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		binary="dist/$(BINARY)-$${os}-$${arch}$${ext}"; \
		archiveName="$(BINARY)-$${os}-$${arch}"; \
		mkdir -p "dist/release/$$archiveName"; \
		cp "$$binary" "dist/release/$$archiveName/$(BINARY)$${ext}"; \
		if [ "$$os" = "windows" ]; then \
			(cd dist/release && zip -q "$$archiveName.zip" "$$archiveName/$(BINARY)$${ext}"); \
		else \
			tar -czf "dist/release/$$archiveName.tar.gz" -C dist/release "$$archiveName"; \
		fi; \
		rm -rf "dist/release/$$archiveName"; \
	done

## test: Run all tests
test:
	go test ./... -count=1

## test-verbose: Run all tests with verbose output
test-verbose:
	go test ./... -v -count=1

## vet: Run go vet
vet:
	go vet ./...

## lint: Run go vet (add golangci-lint when configured)
lint: vet

## check: Run vet + tests
check: vet test

## embed-webapp: Build SolidJS web app and embed into Go binary
embed-webapp:
	./script/embed-webapp.sh

## clean: Remove build artifacts
clean:
	rm -rf dist/
	go clean

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
