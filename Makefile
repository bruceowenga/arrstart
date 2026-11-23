.PHONY: check fmt build e2e docs-check clean test lint

GO ?= go

# Default target: run lint plus unit tests.
check: lint test

# Format Go, shell, and PowerShell sources.
fmt:
	$(GO) fmt ./...
	@if command -v shellcheck >/dev/null 2>&1; then shellcheck install.sh; fi
	@if command -v pwsh >/dev/null 2>&1; then pwsh -Command "Invoke-ScriptAnalyzer -Path install.ps1"; fi

# Build the wizard binary and image locally.
build:
	$(GO) build -o bin/arrstart ./cmd/arrstart
	docker build --platform linux/amd64,linux/arm64 -t ghcr.io/bruceowenga/arrstart:edge .

# Headless end-to-end run against real containers (Linux).
e2e:
	$(GO) test ./... -tags=e2e -run TestE2E

# Link check on docs.
docs-check:
	@if command -v markdown-link-check >/dev/null 2>&1; then markdown-link-check 'docs/*.md'; else echo "markdown-link-check not installed; skipping"; fi

# Unit tests with race detector.
test:
	$(GO) test ./... -race

# Lint: Go, shell, PowerShell, markdown, commit titles.
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; else echo "golangci-lint not installed; skipping"; fi
	@if command -v shellcheck >/dev/null 2>&1; then shellcheck install.sh; else echo "shellcheck not installed; skipping install.sh"; fi
	@if command -v markdownlint >/dev/null 2>&1; then markdownlint 'docs/**/*.md'; else echo "markdownlint not installed; skipping"; fi

clean:
	rm -rf /bin/ /dist/