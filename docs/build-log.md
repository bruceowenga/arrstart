## Build log

Newest first, three lines per session.

## 2026-11-23 WP0.10 ADRs 0001-0007
Result: ADRs 0001–0007 written in docs/adr/ — Docker one-liner bootstrap with temporary wizard container; Go backend with HTMX + Go templates + SSE; native Go wiring clients (no Buildarr); single-root /data layout, one folder asked; Jellyfin only for v1 with media server as a swappable slot; Seerr official image (seerr/seerr:v3.5.2, Docker Hub), compose user: with chown to match, one-time UI setup (/setup) before API wiring; wizard shells out to docker compose. All grounded in S1–S4 spike results.
Open: ADR-0008 (Windows default location) deferred — needs WP0.7 hardlink results from the WP0.11 Windows VM.
Next: WP0.7 spike S5 hardlinks once the Windows VM is ready, or WP0.12 registry/GHCR publishing.

## 2026-11-23 WP0.9 close-out + WP0.6 Spike S4: Seerr setup
Result: S1 Servarr APP__AUTH__APIKEY injection verified on pinned containers (401 without/wrong key, authenticated with correct key); S2 qBittorrent temp password + PBKDF2 format verified but /api/v2/auth/login 401 for every password incl. pre-seeded hashes (upstream #23831) → user sets password once via WebUI; S3 Jellyfin 12.4 runs as node:1000, needs ≥2GiB free at cache path, no public headless admin API (/Users/New 401 pre-first-user, /Startup/* 404, no swagger) → one-time UI wizard at /setup then authenticated settings API; S4 Seerr v3.5.2 runs non-root uid 1000, /app/config must be chowned to compose user: (u=1001 fails otherwise), /api/v1/settings/initialize needs an authenticated session → UI setup once, then authenticated API.
Open: qBittorrent programmatic credential (upstream blocker); Seerr headless login path and Windows behavior; arbitrary-UID chown test (no chown privilege in this env); arm64 pull verification of pinned tags.
Next: ADRs 0001-0008 (WP0.10).

## 2026-11-23 WP0.9: pin exact image tags
Result: templates/versions.yaml created with pinned tags — sonarr 4.0.20.3014-ls6, radarr 7.0.3, prowlarr 3.0.2, qbittorrent 5.3.0_v2.1.2-ls488, jellyfin 12.4, seerr v3.5.2 (Docker Hub; GHCR ghcr.io/seerr-team/seerr publishes no version tags, reconcile at WP0.12). PRD and BUILD-PLAN WP0.9 checkboxes ticked.
Open: verify every tag pulls on amd64 and arm64 at the verification-containers step; reconcile the Seerr registry decision (WP0.12).
Next: throwaway compose in spikes/ with all pinned containers; re-verify S1–S3 against it.

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
