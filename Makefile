BINARY_NAME=sekha-cluster-tool
PKG=github.com/Duara-Cortex/sekha-cluster-tool/cmd/sekha-cluster-tool
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.0.0-dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

ENV_FILE?=.env
ifneq (,$(wildcard $(ENV_FILE)))
    -include $(ENV_FILE)
endif

# Cluster layer addresses configured by the machine building it.
# Defaults are dynamically loaded from $(ENV_FILE) if present, or blank if unconfigured.
SENSORY_URL?=$(CLUSTER_SENSORY_URL)
WORKING_URL?=$(CLUSTER_WORKING_URL)
KNOWLEDGE_URL?=$(CLUSTER_KNOWLEDGE_URL)

# Dynamic resolution from $(ENV_FILE) if created or updated during the build recipe
SENSORY_URL_RESOLVED=$(if $(SENSORY_URL),$(SENSORY_URL),$(shell [ -f "$(ENV_FILE)" ] && grep -E '^CLUSTER_SENSORY_URL=' "$(ENV_FILE)" | head -n 1 | cut -d= -f2- | tr -d '\r"' | tr -d "'"))
WORKING_URL_RESOLVED=$(if $(WORKING_URL),$(WORKING_URL),$(shell [ -f "$(ENV_FILE)" ] && grep -E '^CLUSTER_WORKING_URL=' "$(ENV_FILE)" | head -n 1 | cut -d= -f2- | tr -d '\r"' | tr -d "'"))
KNOWLEDGE_URL_RESOLVED=$(if $(KNOWLEDGE_URL),$(KNOWLEDGE_URL),$(shell [ -f "$(ENV_FILE)" ] && grep -E '^CLUSTER_KNOWLEDGE_URL=' "$(ENV_FILE)" | head -n 1 | cut -d= -f2- | tr -d '\r"' | tr -d "'"))

CONFIG_PKG=github.com/Duara-Cortex/sekha-cluster-tool/internal/config
LDFLAGS=-s -w -X 'main.Version=$(VERSION)' \
        -X '$(CONFIG_PKG).BuildSensoryURL=$(SENSORY_URL_RESOLVED)' \
        -X '$(CONFIG_PKG).BuildWorkingURL=$(WORKING_URL_RESOLVED)' \
        -X '$(CONFIG_PKG).BuildKnowledgeURL=$(KNOWLEDGE_URL_RESOLVED)'

INSTALL_DIR?=$(HOME)/.local/bin

.PHONY: all build install test cross-compile package clean help env

all: test build

build:
	@if [ ! -f .env ] && [ -z "$(SENSORY_URL)" ]; then \
		echo "No .env file found. Setting up environment variables..."; \
		./scripts/setup-env.sh .env; \
	fi
	@mkdir -p bin
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/sekha-cluster-tool
	@echo "Build complete: bin/$(BINARY_NAME) ($(VERSION))"

env:
	@./scripts/setup-env.sh .env

install: build
	@mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"
	@if [ -f "$(ENV_FILE)" ]; then \
		sensory=$$(grep -E '^CLUSTER_SENSORY_URL=' "$(ENV_FILE)" 2>/dev/null | cut -d= -f2- | tr -d '\r"' | tr -d "'"); \
		working=$$(grep -E '^CLUSTER_WORKING_URL=' "$(ENV_FILE)" 2>/dev/null | cut -d= -f2- | tr -d '\r"' | tr -d "'"); \
		knowledge=$$(grep -E '^CLUSTER_KNOWLEDGE_URL=' "$(ENV_FILE)" 2>/dev/null | cut -d= -f2- | tr -d '\r"' | tr -d "'"); \
		if [ -n "$$sensory" ] && [ -n "$$working" ] && [ -n "$$knowledge" ] && \
		   ! echo "$$sensory" | grep -q "<" && ! echo "$$working" | grep -q "<" && ! echo "$$knowledge" | grep -q "<"; then \
			cp "$(ENV_FILE)" "$(INSTALL_DIR)/.env"; \
			echo "✓ Valid $(ENV_FILE) verified and copied to $(INSTALL_DIR)/.env"; \
		else \
			echo "⚠️  $(ENV_FILE) is present but incomplete (missing endpoints or placeholders)."; \
			echo "   Run './scripts/setup-env.sh' or 'make env' to complete configuration."; \
		fi \
	else \
		echo "ℹ️  No $(ENV_FILE) found in repository."; \
		echo "   Run './scripts/setup-env.sh' or 'make env' to configure your environment."; \
	fi

test:
	go test -v -race -timeout 30s ./...

cross-compile:
	@mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)_$(VERSION)_darwin_arm64 ./cmd/sekha-cluster-tool
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)_$(VERSION)_darwin_amd64 ./cmd/sekha-cluster-tool
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)_$(VERSION)_linux_arm64 ./cmd/sekha-cluster-tool
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)_$(VERSION)_linux_amd64 ./cmd/sekha-cluster-tool
	@echo "Cross-compilation complete in dist/"

package: cross-compile
	@cd dist && \
	for bin in $(BINARY_NAME)_$(VERSION)_*; do \
		tar -czf "$${bin}.tar.gz" "$${bin}"; \
	done && \
	if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum *.tar.gz > checksums.txt; \
	else \
		shasum -a 256 *.tar.gz > checksums.txt; \
	fi
	@echo "Packaging and checksums generated in dist/"

clean:
	rm -rf bin dist

help:
	@echo "Available targets:"
	@echo "  build         - Compile $(BINARY_NAME) (prompts for .env if missing)"
	@echo "  env           - Interactively prompt and configure the local .env file"
	@echo "  install       - Compile and copy $(BINARY_NAME) to $(INSTALL_DIR)"
	@echo "  test          - Run unit and mock integration tests"
	@echo "  cross-compile - Cross-compile for macOS (arm64/amd64) and Linux (arm64/amd64)"
	@echo "  package       - Create release tarballs and SHA-256 checksums"
	@echo "  clean         - Remove compiled binaries and artifacts"
