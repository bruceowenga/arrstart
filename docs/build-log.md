## Build log

Newest first, three lines per session.

## 2026-11-23 WP0.5 Spike S3: Jellyfin headless setup
Result: docs/spikes/03-jellyfin-headless-setup.md — default ports 8096/8920, /config volume, X-Emby-Token auth, /Auth/Keys requires admin auth. Admin user creation endpoint via API and library creation endpoint unverified — recommend capturing web setup wizard traffic.
Open: next is WP0.6 Spike S4 (Seerr setup and config ownership).
Next: WP0.6.

## 2026-11-23 WP0.4 Spike S2: qBittorrent credentials
Result: docs/spikes/02-qbittorrent-credentials.md — fixed admin user, temp random password in logs, no credential env vars, PBKDF2 hash in qBittorrent.conf; recommends Option A (pre-seed hash). Container path and hash format unverified against pinned image.
Open: next is WP0.5 Spike S3 (Jellyfin headless setup).
Next: WP0.5.

## 2026-11-23 WP0.1 scaffold
Result: repo initialized on main with README, MIT LICENSE, go.mod, PRD, BUILD-PLAN; scaffold branch chore/scaffold created with .gitignore, .gitattributes, .editorconfig, SECURITY.md, CODE_OF_CONDUCT, Makefile, CLAUDE.md, PR/issue templates, pre-commit config, ADRs 0009-0012.
Open: next is WP0.3 Spike S1 (Servarr API keys).
Next: WP0.3.

## 2026-11-23 WP0.2 CI skeleton
Result: CI pipeline green on main — lint (golangci-lint, shellcheck, markdownlint), unit (go test -race, conditional on empty module), pr-title (inline Conventional Commits check). Temporarily added internal/placeholder/placeholder.go so golangci-lint has files to analyze; will be removed at WP1.x.
Open: next is WP0.3 Spike S1.
Next: WP0.3.
