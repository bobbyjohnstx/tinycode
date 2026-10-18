BINARY := tinycode
MODULE := github.com/bobbyjohnstx/tinycode
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

PLUGIN_PLATFORMS := \
  linux/amd64 \
  linux/arm64 \
  darwin/amd64 \
  darwin/arm64

.PHONY: build build-all build-plugins build-full clean test vet lint fmt check build-webapp embed-webapp package package-plugins image image-full

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
## (Local/optional fat archives — release CI uses thin CLI + package-plugins.)
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

## package-plugins: Build platform bundles + per-plugin tarballs (4 platforms; no Windows)
## Bundle: tinycode-plugins-{os}-{arch}.tar.gz; per-plugin: tinycode-plugin-{name}-{os}-{arch}.tar.gz
## Archive members use registry names (strip leading plugin- from cmd/plugin-<name>).
package-plugins:
	@mkdir -p dist/release
	@for platform in $(PLUGIN_PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		stageDir="dist/release/plugins-stage-$${os}-$${arch}"; \
		rm -rf "$$stageDir"; \
		mkdir -p "$$stageDir"; \
		for dir in $(PLUGIN_DIRS); do \
			cmdName=$$(basename $$dir); \
			regName=$${cmdName#plugin-}; \
			echo "Building plugin $$regName ($$os/$$arch) ..."; \
			GOOS=$$os GOARCH=$$arch go build -ldflags "-s -w" -o "$$stageDir/$$regName" ./$$dir; \
		done; \
		tar -czf "dist/release/tinycode-plugins-$${os}-$${arch}.tar.gz" -C "$$stageDir" .; \
		echo "Created dist/release/tinycode-plugins-$${os}-$${arch}.tar.gz"; \
		for bin in "$$stageDir"/*; do \
			[ -f "$$bin" ] || continue; \
			regName=$$(basename "$$bin"); \
			tar -czf "dist/release/tinycode-plugin-$${regName}-$${os}-$${arch}.tar.gz" -C "$$stageDir" "$$regName"; \
			echo "Created dist/release/tinycode-plugin-$${regName}-$${os}-$${arch}.tar.gz"; \
		done; \
		rm -rf "$$stageDir"; \
	done

## test: Run all tests
test:
	go test ./... -count=1

## test-race: Run all tests with the race detector
test-race:
	go test ./... -count=1 -race -timeout 300s

## test-verbose: Run all tests with verbose output
test-verbose:
	go test ./... -v -count=1

## vet: Run go vet
vet:
	go vet ./...

## staticcheck: Run staticcheck v0.8.1 (2026.2.1)
staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...

## fmt: Format Go source code
fmt:
	gofmt -w .

## lint: Run go vet and staticcheck
lint: vet staticcheck

## check: Run vet, staticcheck, and tests
check: lint test

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
