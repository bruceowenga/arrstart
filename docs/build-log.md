## Build log

Newest first, three lines per session.

## 2026-11-23 WP0.1 scaffold
Result: repo initialized on main with README, MIT LICENSE, go.mod, PRD, BUILD-PLAN; scaffold branch chore/scaffold created with .gitignore, .gitattributes, .editorconfig, SECURITY.md, CODE_OF_CONDUCT, Makefile, CLAUDE.md, PR/issue templates, pre-commit config, ADRs 0009-0012.
Open: next is WP0.3 Spike S1 (Servarr API keys).
Next: WP0.3.

## 2026-11-23 WP0.2 CI skeleton
Result: CI pipeline green on main — lint (golangci-lint, shellcheck, markdownlint), unit (go test -race, conditional on empty module), pr-title (inline Conventional Commits check). Temporarily added internal/placeholder/placeholder.go so golangci-lint has files to analyze; will be removed at WP1.x.
Open: next is WP0.3 Spike S1.
Next: WP0.3.
