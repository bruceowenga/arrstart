# ADR-0012: Scaffold conventions (WP0.1)

## Context

The first WP creates the repo skeleton: license, code of conduct, `.gitignore`, `Makefile`, pre-commit config, issue/PR templates, `CLAUDE.md`, and the empty directory layout. Several choices here are cheap to reverse now and expensive later (line endings, secret leakage, branch protection).

## Decision

- Code of conduct: Contributor Covenant (current version); enforcement contact via project email or GitHub private reporting, never a personal address
- `.gitignore`: standard Go ignores plus OS noise (`.DS_Store`, `Thumbs.db`, `*.swp`, `.idea/`, `.vscode/`), plus `.env`/`.env.*` with `.env.example` kept, `/dist/`, `/bin/`, `go.work`, and local test media folders
- `.gitattributes`: `*.sh`, `*.go`, `*.yaml`, `testdata/**` forced to LF line endings; `install.ps1` kept ASCII-only so Windows PowerShell 5.1 never mangles it
- `.editorconfig` added for editor consistency
- `SECURITY.md` added with a private vulnerability reporting path (per build-plan §9)
- README in the first commit, with a one-paragraph legal framing: media management software, no indexers, no links to content sources, user responsible for what they download

## Process fix: no direct commits to main after the initial commit

1. Initial commit (direct to `main`): README + framing, LICENSE (MIT), PRD, BUILD-PLAN. No auto-generated files.
2. WP0.1 scaffold lands as a PR from branch `chore/scaffold` — the workflow is exercised from the second commit.
3. Branch protection (`require status checks`) is added after WP0.2 creates CI jobs.

## Consequences

- Every change after the first two files goes through a PR with CI gating
- `.env` leaks are structurally prevented at the git layer
- CRLF/LF cross-platform breakage on scripts and golden files is structurally prevented
