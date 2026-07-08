.PHONY: all build install test test-fast test-integration test-db test-review lint fmt clean tidy vuln check-coverage markdown-lint install-hooks fix-plans build-db-plugin

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
VIRAL_PLUGIN=pw-mcp-viral
TRENDS_PLUGIN=pw-mcp-trends
EPUB_PLUGIN=pw-mcp-epub
PDFCHECK_PLUGIN=pw-mcp-pdfcheck
CRITIC_PLUGIN=pw-mcp-critic
TYPST_PLUGIN=pw-mcp-typst
LINTER_PLUGIN=pw-mcp-linter
CLOUD_PLUGIN=pw-mcp-cloud
COVERAGE_PLUGIN=pw-mcp-coverage
YOUTUBE_PLUGIN=pw-mcp-youtube
PITHOS_PLUGIN=pw-mcp-pithos
# pw-mcp-db requires CGO_ENABLED=1 (DuckDB driver) — deliberate exception; all other plugins use CGO_ENABLED=0.
DB_PLUGIN=pw-mcp-db

# Version parameter (can be overridden via: make build VERSION=v1.2.3)
VERSION?=dev
LDFLAGS=-ldflags "-X github.com/borch-ai/powerword/internal/config.Version=$(VERSION)"

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
	@if [ -d cmd/$(VIRAL_PLUGIN) ]; then $(GOBUILD) -o bin/$(VIRAL_PLUGIN) ./cmd/$(VIRAL_PLUGIN); fi
	@if [ -d cmd/$(TRENDS_PLUGIN) ]; then $(GOBUILD) -o bin/$(TRENDS_PLUGIN) ./cmd/$(TRENDS_PLUGIN); fi
	@if [ -d cmd/$(CRITIC_PLUGIN) ]; then $(GOBUILD) -o bin/$(CRITIC_PLUGIN) ./cmd/$(CRITIC_PLUGIN); fi
	@if [ -d cmd/$(EPUB_PLUGIN) ]; then $(GOBUILD) -o bin/$(EPUB_PLUGIN) ./cmd/$(EPUB_PLUGIN); fi
	@if [ -d cmd/$(PDFCHECK_PLUGIN) ]; then $(GOBUILD) -o bin/$(PDFCHECK_PLUGIN) ./cmd/$(PDFCHECK_PLUGIN); fi
	@if [ -d cmd/$(TYPST_PLUGIN) ]; then $(GOBUILD) -o bin/$(TYPST_PLUGIN) ./cmd/$(TYPST_PLUGIN); fi
	@if [ -d cmd/$(LINTER_PLUGIN) ]; then $(GOBUILD) -o bin/$(LINTER_PLUGIN) ./cmd/$(LINTER_PLUGIN); fi
	@if [ -d cmd/$(CLOUD_PLUGIN) ]; then $(GOBUILD) -o bin/$(CLOUD_PLUGIN) ./cmd/$(CLOUD_PLUGIN); fi
	@if [ -d cmd/$(COVERAGE_PLUGIN) ]; then $(GOBUILD) -o bin/$(COVERAGE_PLUGIN) ./cmd/$(COVERAGE_PLUGIN); fi
	@if [ -d cmd/$(YOUTUBE_PLUGIN) ]; then $(GOBUILD) -o bin/$(YOUTUBE_PLUGIN) ./cmd/$(YOUTUBE_PLUGIN); fi
	@if [ -d cmd/$(PITHOS_PLUGIN) ]; then $(GOBUILD) -o bin/$(PITHOS_PLUGIN) ./cmd/$(PITHOS_PLUGIN); fi
	# pw-mcp-db uses CGO_ENABLED=1 for the DuckDB driver (deliberate exception).
	# Gated by POWERWORD_BUILD_DB=1 to avoid CGO compiler toolchain requirements by default.
	@if [ -d cmd/$(DB_PLUGIN) ] && [ "$(POWERWORD_BUILD_DB)" = "1" ]; then CGO_ENABLED=1 $(GOBUILD) -tags=integration -o bin/$(DB_PLUGIN) ./cmd/$(DB_PLUGIN); fi

install:
	$(GOCMD) install $(LDFLAGS) ./cmd/powerword
	# Install plugins if folders exist
	@if [ -d cmd/$(FS_PLUGIN) ]; then $(GOCMD) install ./cmd/$(FS_PLUGIN); fi
	@if [ -d cmd/$(GIT_PLUGIN) ]; then $(GOCMD) install ./cmd/$(GIT_PLUGIN); fi
	@if [ -d cmd/$(SHELL_PLUGIN) ]; then $(GOCMD) install ./cmd/$(SHELL_PLUGIN); fi
	@if [ -d cmd/$(IMAGEGEN_PLUGIN) ]; then $(GOCMD) install ./cmd/$(IMAGEGEN_PLUGIN); fi
	@if [ -d cmd/$(KDP_MATH_PLUGIN) ]; then $(GOCMD) install ./cmd/$(KDP_MATH_PLUGIN); fi
	@if [ -d cmd/$(SEO_PLUGIN) ]; then $(GOCMD) install ./cmd/$(SEO_PLUGIN); fi
	@if [ -d cmd/$(VIRAL_PLUGIN) ]; then $(GOCMD) install ./cmd/$(VIRAL_PLUGIN); fi
	@if [ -d cmd/$(TRENDS_PLUGIN) ]; then $(GOCMD) install ./cmd/$(TRENDS_PLUGIN); fi
	@if [ -d cmd/$(CRITIC_PLUGIN) ]; then $(GOCMD) install ./cmd/$(CRITIC_PLUGIN); fi
	@if [ -d cmd/$(EPUB_PLUGIN) ]; then $(GOCMD) install ./cmd/$(EPUB_PLUGIN); fi
	@if [ -d cmd/$(PDFCHECK_PLUGIN) ]; then $(GOCMD) install ./cmd/$(PDFCHECK_PLUGIN); fi
	@if [ -d cmd/$(TYPST_PLUGIN) ]; then $(GOCMD) install ./cmd/$(TYPST_PLUGIN); fi
	@if [ -d cmd/$(LINTER_PLUGIN) ]; then $(GOCMD) install ./cmd/$(LINTER_PLUGIN); fi
	@if [ -d cmd/$(CLOUD_PLUGIN) ]; then $(GOCMD) install ./cmd/$(CLOUD_PLUGIN); fi
	@if [ -d cmd/$(COVERAGE_PLUGIN) ]; then $(GOCMD) install ./cmd/$(COVERAGE_PLUGIN); fi
	@if [ -d cmd/$(YOUTUBE_PLUGIN) ]; then $(GOCMD) install ./cmd/$(YOUTUBE_PLUGIN); fi
	@if [ -d cmd/$(PITHOS_PLUGIN) ]; then $(GOCMD) install ./cmd/$(PITHOS_PLUGIN); fi
	# pw-mcp-db uses CGO_ENABLED=1 for the DuckDB driver (deliberate exception).
	# Gated by POWERWORD_BUILD_DB=1 to avoid CGO compiler toolchain requirements by default.
	@if [ -d cmd/$(DB_PLUGIN) ] && [ "$(POWERWORD_BUILD_DB)" = "1" ]; then CGO_ENABLED=1 $(GOCMD) install -tags=integration ./cmd/$(DB_PLUGIN); fi


install-hooks:
	@echo "Installing git hooks..."
	@mkdir -p .git/hooks
	@cp scripts/git-hooks/pre-push .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "Git hooks installed successfully."


test:
	$(GOTEST) -v -race -coverprofile=coverage.out -coverpkg=./internal/...,./pkg/... ./internal/... ./pkg/...

test-fast:
	$(GOTEST) -v ./internal/... ./pkg/...

test-integration:
	CGO_ENABLED=1 $(GOTEST) -v -tags=integration ./...

# Run all database plugin tests (requires CGO and integration tags)
test-db:
	CGO_ENABLED=1 $(GOTEST) -v -race -tags=integration -coverprofile=coverage.out -coverpkg=./internal/...,./pkg/... ./internal/plugins/db/...

check-coverage: test
	$(GOCMD) run ./cmd/powerword check-coverage $(MIN_COVERAGE) coverage.out

markdown-lint:
	$(GOCMD) run ./cmd/powerword lint-plans

fix-plans:
	$(GOCMD) run ./cmd/powerword review --local --fix


lint:
	$(GOCMD) run ./cmd/powerword lint-go

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

# Standalone target to build only pw-mcp-db with CGO enabled.
build-db-plugin:
	mkdir -p bin
	CGO_ENABLED=1 $(GOBUILD) -tags=integration -o bin/$(DB_PLUGIN) ./cmd/$(DB_PLUGIN)
