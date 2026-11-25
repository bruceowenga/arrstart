.PHONY: check fmt build e2e docs-check clean test lint

GO ?= go

# Default target: run lint plus unit tests.
check: lint test

# Format Go, shell, and PowerShell sources.
fmt:
	$(GO) fmt ./...
	@if command -v shellcheck >/dev/null 2>&1; then shellcheck install.sh; fi
	@if command -v pwsh >/dev/null 2>&1; then pwsh -Command "Invoke-ScriptAnalyzer -Path install.ps1"; fi

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build the arrstart binary with ldflags (version, commit, date).
build:
	$(GO) build -ldflags="-X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.date=$(DATE)'" -o bin/arrstart ./cmd/arrstart

# Build the OCI image (plain docker build; no --platform list).
image:
	docker build -t ghcr.io/bruceowenga/arrstart:edge .

# Headless end-to-end run against real containers (Linux).
e2e:
	$(GO) test ./... -tags=e2e -run TestE2E

# Link check on docs.
docs-check:
	@if command -v markdown-link-check >/dev/null 2>&1; then markdown-link-check 'docs/*.md'; else echo "markdown-link-check not installed; skipping"; fi

# Unit tests with race detector.
test:
	$(GO) test ./... -race

# Hard-lint: required tools (golangci-lint, shellcheck, markdownlint) must be
# present — CI never skips a lint step. ALLOW_SKIP_LINT=1 is an explicit local
# escape hatch: it prints a loud warning and continues without missing tools
# instead of failing.
LINT_TOOLS := golangci-lint shellcheck markdownlint
lint: lint-tools lint-run

lint-tools:
	@missing=''; \
	for t in $(LINT_TOOLS); do \
		command -v $$t >/dev/null 2>&1 || missing="$$missing $$t"; \
	done; \
	if [ -n "$$missing" ]; then \
		if [ "$$ALLOW_SKIP_LINT" = "1" ]; then \
			echo ""; \
			echo "######################################################################"; \
			echo "WARNING: ALLOW_SKIP_LINT=1 — skipping missing lint tools:$$missing"; \
			echo "CI hard-fails on these. Install to lint locally:"; \
			echo "  golangci-lint: https://golangci-lint.run/welcome/install/"; \
			echo "  shellcheck:    sudo apt-get install -y shellcheck"; \
			echo "  markdownlint:  npm install -g markdownlint-cli"; \
			echo "######################################################################"; \
			echo ""; \
			exit 0; \
		fi; \
		echo ""; \
		echo "FATAL: required lint tool(s) missing:$$missing"; \
		echo "make check will not pass without them (CI hard-fails on missing tools)."; \
		echo "Install:"; \
		echo "  golangci-lint: https://golangci-lint.run/welcome/install/"; \
		echo "  shellcheck:    sudo apt-get install -y shellcheck"; \
		echo "  markdownlint:  npm install -g markdownlint-cli"; \
		echo "Or run locally with: ALLOW_SKIP_LINT=1 make check"; \
		echo ""; \
		exit 1; \
	fi

lint-run:
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; fi
	@if command -v shellcheck >/dev/null 2>&1; then shellcheck install.sh; fi
	@if command -v markdownlint >/dev/null 2>&1; then markdownlint 'docs/**/*.md'; fi

clean:
	rm -rf /bin/ /dist/