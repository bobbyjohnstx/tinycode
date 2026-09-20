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

PLUGIN_DIRS := $(wildcard cmd/plugin-*)
PLUGIN_NAMES := $(notdir $(PLUGIN_DIRS))

.PHONY: build build-all build-plugins build-full clean test vet lint check build-webapp embed-webapp package image image-full

## build: Build tinycode for the current platform
build:
	go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY) $(MAIN)

## build-plugins: Build all plugins for the current platform
build-plugins:
	@for dir in $(PLUGIN_DIRS); do \
		name=$$(basename $$dir); \
		echo "Building plugin $$name ..."; \
		go build -ldflags "-s -w" -o dist/plugins/$$name ./$$dir; \
	done

## build-full: Build tinycode and all plugins for the current platform
build-full: build build-plugins

## build-all: Cross-compile tinycode for all supported platforms
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

## build-all-plugins: Cross-compile all plugins for all supported platforms
build-all-plugins:
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		for dir in $(PLUGIN_DIRS); do \
			name=$$(basename $$dir); \
			output="dist/plugins/$${name}-$${os}-$${arch}$${ext}"; \
			echo "Building plugin $$output ..."; \
			GOOS=$$os GOARCH=$$arch go build -ldflags "-s -w" -o $$output ./$$dir; \
		done; \
	done

## build-all-full: Cross-compile tinycode and all plugins for all platforms
build-all-full: build-all build-all-plugins

## package: Create release archives with tinycode + all plugins for all platforms
package: build-all-full
	@mkdir -p dist/release
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		archiveName="$(BINARY)-$${os}-$${arch}"; \
		stageDir="dist/release/$$archiveName"; \
		mkdir -p "$$stageDir/plugins"; \
		cp "dist/$(BINARY)-$${os}-$${arch}$${ext}" "$$stageDir/$(BINARY)$${ext}"; \
		for dir in $(PLUGIN_DIRS); do \
			name=$$(basename $$dir); \
			pluginBin="dist/plugins/$${name}-$${os}-$${arch}$${ext}"; \
			if [ -f "$$pluginBin" ]; then \
				cp "$$pluginBin" "$$stageDir/plugins/$${name}$${ext}"; \
			fi; \
		done; \
		if [ "$$os" = "windows" ]; then \
			(cd dist/release && zip -qr "$$archiveName.zip" "$$archiveName"); \
		else \
			tar -czf "dist/release/$$archiveName.tar.gz" -C dist/release "$$archiveName"; \
		fi; \
		rm -rf "$$stageDir"; \
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

## build-webapp: Build SolidJS web app with esbuild (output in packages/app/dist/)
build-webapp:
	node script/build-webapp.mjs

## embed-webapp: Build SolidJS web app and embed into Go binary
embed-webapp:
	./script/embed-webapp.sh

IMAGE_NAME ?= tinycode
IMAGE_TAG  ?= $(VERSION)

## image: Build container image (tinycode only)
image:
	podman build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		-f Containerfile .

## image-full: Build container image with all plugins
image-full:
	podman build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_PLUGINS=1 \
		-t $(IMAGE_NAME):$(IMAGE_TAG)-full \
		-t $(IMAGE_NAME):latest-full \
		-f Containerfile .

## clean: Remove build artifacts
clean:
	rm -rf dist/
	go clean

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
