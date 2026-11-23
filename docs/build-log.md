## Build log

Newest first, three lines per session.

## 2026-11-23 WP3.5 Prowlarr wiring client (PRD A5)
Result: internal/wiring/prowlarr native Go client (WaitHealthy/Configure/Verify) on the servarr v1 transport (/api/v1); POST /api/v1/applications schema confirmed against real linuxserver/prowlarr:3.0.2 (:19696): Registers Sonarr ("SonarrSettings") and Radarr ("RadarrSettings") applications so indexers sync, skipping existing names (400 "Should be unique"); no indexers added. prowlarrUrl is required and must be the compose-internal URL the *arr containers use to reach Prowlarr (validated by each target *arr). httptest unit tests pass -race; e2e green against real container (WaitHealthy→Configure→Verify→idempotent re-Configure→Verify).
Open: none for Prowlarr wiring itself.
Next: WP3.6 Jellyfin client (admin user, libraries) per A7.

## 2026-11-23 WP3.4 Sonarr and Radarr wiring clients (PRD A3/A4)
Result: internal/wiring/sonarr + internal/wiring/radarr native Go clients (WaitHealthy/Configure/Verify) sharing internal/wiring/internal/servarr v3 transport; WaitHealthy verified against pinned linuxserver/sonarr:4.0.20.3014-ls6 (:18989) and linuxserver/radarr:7.0.3 (:17878); root-folder add (/data/media/tv, /data/media/movies) verified 201 then GET-returns on real containers; httptest unit tests pass -race including idempotency/wrong-key/verify-failures; e2e run green against real containers. X-Api-Key 401-without/wrong-key path verified in S1.
Open: Servarr v3 POST /api/v3/downloadclient schema NOT confirmed against pinned containers (multiple 400s; missing Implementation/configContract/fields targetType resolution; no OpenAPI spec exists) — download-client registration in Configure is code-present but UNVERIFIED; hardlink/naming settings keys probed, absent → reported unverified, not impossible (CLAUDE.md). Follow-up: resolve download-client schema or re-probe against a second container attempt before marking WP3.4 fully done.
Next: WP3.5 Prowlarr client (app links to Sonarr/Radarr, no indexers per rule).

## 2026-11-23 WP3.3 qBittorrent wiring client (PRD A6)
Result: squash-merged PR #16 as b34f489 — native Go wiring client (WaitHealthy/Configure/Verify) plus SeedConfig pre-seeding the PBKDF2 hash so qBittorrent starts with a known admin credential; unit-tested with httptest fakes, API behavior verified against linuxserver/qbittorrent:5.3.0_v2.1.2-ls488 in spikes S2/S4b (host-port login 204 on matching ports, setPreferences applies save_path/categories). CI lint+unit+pr-title green; branch deleted.
Open: none for qBittorrent.
Next: keep wiring clients one WP at a time (WP3.4 Sonarr/Radarr clients).

## 2026-11-23 WP0.12 publish workflow (GITHUB_TOKEN, no PAT)
Result: added Dockerfile (minimal bootstrap image, alpine:3.20, org.opencontainers.image.source label) and .github/workflows/publish.yml (pushes ghcr.io/<owner>/arrstart on version tags, GITHUB_TOKEN with packages:write, actions pinned by commit SHA). Verified Dockerfile builds locally (12.1MB). Tag v0.0.0-ci.1 pushed; publish workflow built and pushed ghcr.io/bruceowenga/arrstart:0.0.0-ci.1; anonymous `docker pull ghcr.io/bruceowenga/arrstart:0.0.0-ci.1` verified after `docker logout ghcr.io` with empty docker auth — image publicly pullable.
Open: none — WP0.12 publish complete.
Next: qBittorrent known-password/setPreferences/restart-survival tests; WP0.8; WP0.11/WP0.7 when VM ready.

## 2026-11-23 WP0.12 Namespace and registry (complete)
Result: ADR-0010 decided (GHCR only, ghcr.io/bruceowenga/arrstart, pinned tags never latest); internal/registry/registry.go DefaultRegistry is the single Go namespace constant (G5/B1). GHCR publish works with the workflow-scoped GITHUB_TOKEN (packages:write) — no PAT, no personal token. First public push v0.0.0-ci.1 done; anonymous pull verified.
Open: pin Seerr and other tagless images by digest (WP0.12 follow-up).
Next: WP0.8 (HTMX+SSE consolidation already done per BUILD-PLAN), then WP0.11/WP0.7 when the Windows VM is ready.

## 2026-11-23 Spike S2b/S4b follow-up, Jellyfin library spec bodies, CLAUDE.md hardening rule
Result: S2b qBittorrent login from a sibling container on the compose network (http://qbittorrent:8080) → HTTP 204 with default security flags + startup temp password (so host-port 401 is host-specific, not the remap); S3 Jellyfin 12.4 library endpoint bodies confirmed from the served OpenAPI spec (POST /Library/VirtualFolders → AddVirtualFolderDto{LibraryOptions{PathInfos:[{Path:...}]}} for /data/media/tv and /data/media/movies; GET /Libraries/AvailableOptions?libraryContentType=movies|tvshows&isNewLibrary=true) but live auth-gated calls NOT reproducible here — fresh container POST /Startup/User → 404, no known admin token obtainable; S4b Seerr v3.5.2 on the compose network → GET /api/v1/status 200, X-Api-Key authenticates GET, POST /api/v1/settings/initialize → 403 (tested with working Jellyfin admin: `auth/jellyfin` returns `NO_ADMIN_USER` — Seerr has no admin yet; `initialize` needs a `connect.sid` session, 401 without / 403 with API key only), POST /api/v1/auth/jellyfin → body {username,password,hostname,port,useSsl,urlBase,name} reaches Jellyfin (401 INVALID_CREDENTIALS with bad creds = reachability confirmed; earlier INVALID_URL was unreachability). No product image lacks a stable version tag, so nothing to pin by digest yet.
Open: jellyfin admin token not reproducible here (blocks live Jellyfin library calls and Seerr→Jellyfin apiKey exchange); Seerr initialize/settings/sonarr/radarr need connect.sid (headless admin creation blocked — verified `NO_ADMIN_USER` on `auth/jellyfin` with a working Jellyfin admin; no headless login route → admin created once via /setup UI); qBittorrent host-login cause still not isolated.
Next: merge PR #10/#11; WP0.12 (GHCR hello-world, registry-path constant, public-package check).

## 2026-11-23 WP0.10 ADRs 0001-0007
Result: ADRs 0001–0007 written in docs/adr/ — Docker one-liner bootstrap with temporary wizard container; Go backend with HTMX + Go templates + SSE; native Go wiring clients (no Buildarr); single-root /data layout, one folder asked; Jellyfin only for v1 with media server as a swappable slot; Seerr official image (seerr/seerr:v3.5.2, Docker Hub), compose user: with chown to match, Seerr admin created once via /setup UI (headless init blocked in v3.5.2 — verified `NO_ADMIN_USER` on `auth/jellyfin`, `connect.sid` required for `initialize`) before API wiring; wizard shells out to docker compose. All grounded in S1–S4 spike results.
Open: ADR-0008 (Windows default location) deferred — needs WP0.7 hardlink results from the WP0.11 Windows VM.
Next: WP0.7 spike S5 hardlinks once the Windows VM is ready, or WP0.12 registry/GHCR publishing.

## 2026-11-23 WP0.9 close-out + WP0.6 Spike S4: Seerr setup
Result: S1 Servarr `APP__AUTH__APIKEY` injection verified on pinned containers (401 without/wrong key, authenticated with correct key); S2 qBittorrent temp password + PBKDF2 format verified, but /api/v2/auth/login returned 401 `Unauthorized` for every password tried from the host (temp and pre-seeded-hash passwords; cause not isolated — host login works after setting WebUI security flags in qBittorrent.conf, inside-container login via localhost works) → no headless known-credential path confirmed in 5.3.0, user sets password once via WebUI; S3 Jellyfin 12.4 runs as node:1000, needs ≥2GiB free at cache path, `/api-docs/openapi.json` → 200 (OpenAPI 3.0.4), `/Startup/Configuration`/RemoteAccess/Complete → 200/204 anonymous, `POST /Startup/User` → 204 creates the first admin, `POST /Startup/Complete` → 204 flips `StartupWizardCompleted` to `true`, `POST /Users/AuthenticateByName` → 200 with `Authorization: MediaBrowser ...` header + body field `Pw` (not `Password`) returning an admin `AccessToken`, `POST /Auth/Keys` → 200 → fully headless setup works, no web wizard needed; S4 Seerr v3.5.2 runs non-root uid 1000, /app/config must be chowned to compose user: (u=1001 fails otherwise), /api/v1/settings/initialize needs an authenticated session (401 `connect.sid` required; verified with working Jellyfin admin: `auth/jellyfin` → `NO_ADMIN_USER`, headless admin creation blocked → admin created once via /setup UI, then authenticated API).
Open: qBittorrent programmatic credential (cause not isolated — #23831 referenced but not located); Seerr headless login path and Windows behavior; arbitrary-UID chown test (no chown privilege in this env); Jellyfin library endpoint bodies for /data/media/tv and /data/media/movies.
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
