## Build log

Newest first, three lines per session.

## 2026-11-25 Docs-only: re-baseline plan around headless apply engine (PR #27)

Result: BUILD-PLAN Phase 1 re-ordered: WP1.1a apply engine (headless, NEW), WP1.3 templates, WP1.4 generator, WP1.5 layout, WP1.11 CI headless apply (Gate 1, NEW), WP1.2 hardware, WP1.1 wizard skeleton, WP1.7 UI screens, WP1.8 deploy/SSE progress, WP1.6 install.sh, WP1.10 finish/helper wrapper; standalone WP1.9 removed (content folded into WP1.1a, W9 kept). Gate 1 = `arrstart apply --answers answers.yaml` headless on clean Ubuntu, healthy + wired containers, verified by CI every PR; Gate 3 = section 5.2 scenario headless with mock indexer, zero manual steps. PRD: Goal 3 reworded (CI verification planned at Gate 1 + Gate 3, neither green yet); W9 headless mode (§10.2); B10 Windows edition/WSL2/virtualization check (§10.1). NEW docs/adr/0013-apply-engine-headless-plus-ui.md marked Status: Proposed — not yet decided; PRD §17 and BUILD-PLAN §8 list it unticked/proposed. Docs/adr/0013 reapplied as proposed; BUILD-PLAN §8 ADR-0010/0011/0012 ticked, ADR-0008 deferred, WP0.10 ticked with caveat. No code changes; all "verified" claims name scripts/CI steps only. `arrstack` → `arrstart` everywhere (helper names now `arrstart.sh`/`arrstart.ps1`).
Open: none (docs-only).
Next: commit (`docs: re-baseline plan around a headless walking skeleton`, Refs: G3, W9, B10, W8, A10; WP1.x planning) and open PR #27 with a file list in the description.

## 2026-11-24 Task 2 CI hygiene (PR #26)
Result: lint job now hard-fails if golangci-lint/shellcheck/markdownlint is missing (actions pinned by SHA; gofmt/go vet/go build added); `make check` FATALs on missing tools with `ALLOW_SKIP_LINT=1` escape hatch; `.golangci.yml` v1 enables gofmt linter; pr-title reruns on title edits.
Next: none, merged

## 2026-11-24 WP3.8 Mock indexer and Gate 3 e2e test (PRD A5)
Result: tests/mock-indexer Go mock Torznab indexer (caps + tv/movie search returning only obviously-fake results; provider "mock-indexer", fake titles/IDs — no real indexer names or content, ADR-0009), httptest unit tests pass -race; Dockerfile (golang:1.24-alpine builder → alpine:3.20 non-root runtime) and throwaway docker-compose.yml (Prowlarr + mock indexer on a shared network). Additive AddIndexer(ctx, Indexer) on internal/wiring/prowlarr.Client calling Prowlarr POST /api/v1/indexer; Torznab field names probed against real linuxserver/prowlarr:3.0.2 (:19696) before trusting. Full wizard-driven Gate 3 e2e NOT verified yet — wizard/generator (WP1.x) not built; POST /api/v1/indexer schema probed live against real linuxserver/prowlarr:3.0.2 (:19696): appProfileId > 0, priority 1-50, baseUrl field, dial failures observed ("Name does not resolve", "Connection refused"); HTTP 201 registration needs a reachable mock endpoint which this session's shell blocked standing up, so end-to-end registration is NOT verified and nothing was registered or left running.
Open: wizard-driven end-to-end Gate 3 (stack up via wizard, Seerr request → Radarr grab → qBittorrent) blocked on WP1.x wizard/headless mode (WP1.9); rewire idempotency (WP3.9).
Next: WP3.9 rewire command and idempotency tests (PRD A10, C6).

## 2026-11-24 WP3.2 Credentials and keys (PRD A1, A9)
Result: internal/credentials package — AppSpec/Defaults for sonarr/radarr/prowlarr (SONARR__AUTH__APIKEY etc.), GenerateKey (32 bytes → 64-char hex via crypto/rand), Generate(specs)/GenerateDefaultKeys(), plus LoadDotEnv/WriteDotEnv with dedup against existing entries, sorted KEY=VALUE output, 0o600 perms, write-if-changed so idempotent re-runs change nothing. Unit tests (defaults, uniqueness, key format, missing-file load, parse/dedup, create/perms/preserve) pass -race; gofmt/go vet clean. No live-container verification needed — no app API calls; keys are generated locally and written to .env for later injection.
Open: none — WP3.2 complete; WP3.1–3.7 all ticked in BUILD-PLAN.
Next: WP3.8 Mock indexer and Gate 3 e2e test (BUILD-PLAN §5.2, tests/mock-indexer/).

## 2026-11-24 WP3.7 Seerr wiring client (PRD A8)
Result: internal/wiring/seerr native Go client (WaitHealthy/Configure/Verify) over Seerr's /api/v1 with X-Api-Key auth via servarr transport; POST /api/v1/settings/{jellyfin,sonarr,radarr} applies connections (readOnly Jellyfin "name" omitted, idempotent upsert), GET settings re-checked by Verify; errSeerrNotInitialized sentinel with /setup + `arrstart rewire` guidance when not initialized; unconfigured (empty URL) connections skipped; trailing slash trimmed in New. Verified live against real seerr/seerr:v3.5.2 (:29055): GET /api/v1/status 200 no auth (WaitHealthy OK), X-Api-Key authenticates settings endpoints, settings writes return 403 while initialized=false → headless wiring blocked until the admin user is created once through the web UI at /setup (documented product gap per ADR-0006, surfaced by the client rather than hidden). httptest unit tests pass -race (wizard flow, idempotency implied by upsert, mismatch verify failure, not-initialized paths); golangci-lint clean; gofmt/go vet clean.
Open: none — WP3.7 complete; WP3.x wiring clients all done.
Next: WP3.1 Wiring framework (PRD A2, A10, A11).

## 2026-11-24 WP3.1 Wiring framework (PRD A2, A10, A11)
Result: internal/wiring/wiring.go extended with Step, StepResult, Summary, AppStep types; new internal/wiring/runner.go Runner executing WaitHealthy→Configure→Verify per app with shared backoff, isolating failures so one app's malfunction does not abort the others (A11) and reporting a per-step summary; runs idempotent (A10); new internal/wiring/internal/retry package with Backoff.Do for the health-endpoint wait (A2). httptest unit tests pass -race (retry success/retry/cancel/max-cap, step strings, summary error joining, runner healthy run, failure isolation); gofmt/go vet clean; make check green; make e2e green.
Open: none for WP3.1 framework itself; WP3.2 Credentials and keys (PRD A1, A9) is the next Phase-3 item.
Next: WP3.2 Credentials and keys (PRD A1, A9).

## 2026-11-24 WP3.6 Jellyfin wiring client (PRD A7)
Result: internal/wiring/jellyfin native Go client (WaitHealthy/Configure/Verify) over Jellyfin's MediaBrowser auth scheme; headless wizard → admin user → API key → libraries verified live against real jellyfin/jellyfin:12.4 (:18889): POST /Startup/Configuration, GET /Startup/User placeholder → POST /Startup/User (body Name/Password/HasPassword) → POST /Startup/Complete → authenticated POST /Users/AuthenticateByName (body Pw; Authorization MediaBrowser Client/Device/DeviceId/Version + Token) → POST /Auth/Keys?app=arrstart → POST /Library/VirtualFolders?name=...&collectionType=... (body LibraryOptions.PathInfos); library paths /data/media/tv, /data/media/movies pre-created on disk in the container before library creation (Jellyfin validates existence). Idempotent re-Configure skips existing user/key/library paths; fresh-client Verify re-authenticates and re-checks wizard + library paths. httptest unit tests pass -race (wizard flow, idempotency, custom libraries, missing-library verify failure, bad-credentials verify failure); golangci-lint clean; gofmt/go vet clean.
Open: none for Jellyfin wiring itself.
Next: WP3.7 Seerr client (PRD A8).

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
