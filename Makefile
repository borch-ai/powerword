.PHONY: all build test lint fmt clean tidy vuln check-coverage

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

all: lint vuln check-coverage build

build:
	mkdir -p bin
	$(GOBUILD) -o bin/$(BINARY_NAME) ./cmd/powerword
	# Compile plugins if folders exist
	@if [ -d cmd/$(FS_PLUGIN) ]; then $(GOBUILD) -o bin/$(FS_PLUGIN) ./cmd/$(FS_PLUGIN); fi
	@if [ -d cmd/$(GIT_PLUGIN) ]; then $(GOBUILD) -o bin/$(GIT_PLUGIN) ./cmd/$(GIT_PLUGIN); fi
	@if [ -d cmd/$(SHELL_PLUGIN) ]; then $(GOBUILD) -o bin/$(SHELL_PLUGIN) ./cmd/$(SHELL_PLUGIN); fi

test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

check-coverage: test
	@go tool cover -func=coverage.out | go run scripts/check_coverage.go $(MIN_COVERAGE)

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
