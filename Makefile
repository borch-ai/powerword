.PHONY: all build install test test-review lint fmt clean tidy vuln check-coverage markdown-lint install-hooks

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOFMT=gofmt

# Coverage threshold
MIN_COVERAGE=91.0

# Binary names
BINARY_NAME=powerword
FS_PLUGIN=pw-mcp-fs
GIT_PLUGIN=pw-mcp-git
SHELL_PLUGIN=pw-mcp-shell
IMAGEGEN_PLUGIN=pw-mcp-imagegen
KDP_MATH_PLUGIN=pw-mcp-kdp-math
SEO_PLUGIN=pw-mcp-seo

# Version parameter (can be overridden via: make build VERSION=v1.2.3)
VERSION?=dev
LDFLAGS=-ldflags "-X powerword/internal/config.Version=$(VERSION)"

all: markdown-lint lint vuln check-coverage build

build:
	mkdir -p bin
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/powerword
	# Compile plugins if folders exist
	@if [ -d cmd/$(FS_PLUGIN) ]; then $(GOBUILD) -o bin/$(FS_PLUGIN) ./cmd/$(FS_PLUGIN); fi
	@if [ -d cmd/$(GIT_PLUGIN) ]; then $(GOBUILD) -o bin/$(GIT_PLUGIN) ./cmd/$(GIT_PLUGIN); fi
	@if [ -d cmd/$(SHELL_PLUGIN) ]; then $(GOBUILD) -o bin/$(SHELL_PLUGIN) ./cmd/$(SHELL_PLUGIN); fi
	@if [ -d cmd/$(IMAGEGEN_PLUGIN) ]; then $(GOBUILD) -o bin/$(IMAGEGEN_PLUGIN) ./cmd/$(IMAGEGEN_PLUGIN); fi
	@if [ -d cmd/$(KDP_MATH_PLUGIN) ]; then $(GOBUILD) -o bin/$(KDP_MATH_PLUGIN) ./cmd/$(KDP_MATH_PLUGIN); fi
	@if [ -d cmd/$(SEO_PLUGIN) ]; then $(GOBUILD) -o bin/$(SEO_PLUGIN) ./cmd/$(SEO_PLUGIN); fi

install:
	$(GOCMD) install $(LDFLAGS) ./cmd/powerword
	# Install plugins if folders exist
	@if [ -d cmd/$(FS_PLUGIN) ]; then $(GOCMD) install ./cmd/$(FS_PLUGIN); fi
	@if [ -d cmd/$(GIT_PLUGIN) ]; then $(GOCMD) install ./cmd/$(GIT_PLUGIN); fi
	@if [ -d cmd/$(SHELL_PLUGIN) ]; then $(GOCMD) install ./cmd/$(SHELL_PLUGIN); fi
	@if [ -d cmd/$(IMAGEGEN_PLUGIN) ]; then $(GOCMD) install ./cmd/$(IMAGEGEN_PLUGIN); fi
	@if [ -d cmd/$(KDP_MATH_PLUGIN) ]; then $(GOCMD) install ./cmd/$(KDP_MATH_PLUGIN); fi
	@if [ -d cmd/$(SEO_PLUGIN) ]; then $(GOCMD) install ./cmd/$(SEO_PLUGIN); fi

install-hooks:
	@echo "Installing git hooks..."
	@mkdir -p .git/hooks
	@cp scripts/git-hooks/pre-push .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "Git hooks installed successfully."


test:
	$(GOTEST) -v -race -coverprofile=coverage.out -coverpkg=./internal/... ./internal/...

check-coverage: test
	@go run scripts/check_coverage.go $(MIN_COVERAGE) coverage.out

markdown-lint:
	$(GOCMD) run scripts/lint_markdown/main.go
	$(GOCMD) run scripts/lint_plans/main.go

lint:
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, running basic go vet..."; \
		$(GOCMD) vet ./...; \
	fi

vuln:
	@GOBIN=$$(go env GOBIN); \
	GOPATH=$$(go env GOPATH); \
	if [ -z "$$GOBIN" ]; then GOBIN=$$GOPATH/bin; fi; \
	if [ ! -f "$$GOBIN/govulncheck" ]; then \
		echo "Installing govulncheck..."; \
		$(GOCMD) install golang.org/x/vuln/cmd/govulncheck@latest; \
	fi; \
	$$GOBIN/govulncheck ./...

fmt:
	$(GOFMT) -w -s .

tidy:
	$(GOCMD) mod tidy

clean:
	$(GOCLEAN)
	rm -rf bin/
	rm -f coverage.out
