## Build log

Newest first, three lines per session.

## 2026-11-23 WP0.9 close-out + WP0.6 Spike S4: Seerr setup
Result: S1 Servarr `APP__AUTH__APIKEY` injection verified on pinned containers (401 without/wrong key, authenticated with correct key); S2 qBittorrent temp password + PBKDF2 format verified, but /api/v2/auth/login returned 401 `Unauthorized` for every password tried from the host (temp and pre-seeded-hash passwords; cause not isolated — host login works after setting WebUI security flags in qBittorrent.conf, inside-container login via localhost works) → no headless known-credential path confirmed in 5.3.0, user sets password once via WebUI; S3 Jellyfin 12.4 runs as node:1000, needs ≥2GiB free at cache path, `/api-docs/openapi.json` → 200 (OpenAPI 3.0.4), `/Startup/Configuration`/RemoteAccess/Complete → 200/204 anonymous, `POST /Startup/User` → 204 creates the first admin, `POST /Startup/Complete` → 204 flips `StartupWizardCompleted` to `true`, `POST /Users/AuthenticateByName` → 200 with `Authorization: MediaBrowser ...` header + body field `Pw` (not `Password`) returning an admin `AccessToken`, `POST /Auth/Keys` → 200 → fully headless setup works, no web wizard needed; S4 Seerr v3.5.2 runs non-root uid 1000, /app/config must be chowned to compose user: (u=1001 fails otherwise), /api/v1/settings/initialize needs an authenticated session (401 `connect.sid` required) → UI setup once, then authenticated API.
Open: qBittorrent programmatic credential (cause not isolated — #23831 referenced but not located); Seerr headless login path and Windows behavior; arbitrary-UID chown test (no chown privilege in this env); Jellyfin library endpoint bodies for /data/media/tv and /data/media/movies.
Next: tick WP0.3-WP0.6, then ADRs 0001-0008 (WP0.10) or WP0.7 hardlinks once the Windows VM is ready.

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
