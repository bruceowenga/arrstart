# Spike S3: Jellyfin headless setup

## Status: verified against pinned container (WP0.9) — headless initial setup not supported via public API

*Verified against `jellyfin/jellyfin:12.4` in a throwaway container (localhost port 18096). Public info endpoint and wizard-completion signal confirmed; **no unauthenticated API path exists to create the initial admin user** in 12.4 — `/Users/New` returns 401 before any user exists, `/Startup/*` wizard endpoints and `/Startup/Device`/`/Startup/Pin` return 404, and no public OpenAPI spec is served.*

**Correction — likely startup flow:** The Jellyfin web setup wizard uses public `/Startup/...` endpoints (server configuration, first-user creation, wizard completion) before requiring auth. Check the running container's own OpenAPI spec (`/swagger.json` or equivalent) rather than replaying browser traffic. Likely flow: call startup/public endpoints → authenticate as the new admin → use that token for libraries and API key creation.

## Goal

Determine how to complete Jellyfin's initial setup headless via API (admin user creation) and add libraries for `/data/media/tv` and `/data/media/movies`, per PRD requirement **A7**:

> Jellyfin: complete the startup wizard via API with an admin user; add libraries for `/data/media/tv` and `/data/media/movies`.

And per build-plan §5.1: "Verify app API behavior (Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr) against the pinned versions running in real containers. Never trust recalled endpoints or field names without a test hitting the real app."

## Method

Searched official Jellyfin docs, api.jellyfin.org (ReDoc — JS-rendered, content not retrievable), GitHub discussions, and third-party API summaries. Ran the pinned image in a throwaway container and probed its public/auth boundaries empirically (including inspecting served `jellyfin-web` chunks for wizard routes).

## Confirmed findings

| Item | Finding | Source |
|------|---------|--------|
| Default port | `8096` (HTTP), `8920` (HTTPS) | jellyfin.org container docs |
| First-run admin user | "Complete the setup wizard to create an administrator account" — **no documented HTTP endpoint for first-run admin creation** | integrations.sh Jellyfin API summary |
| User management UI | Users managed via Dashboard → Users (web UI); no public API doc found for programmatic creation | jellyfin.org user docs |
| API key auth | `X-Emby-Token: <key>` header or `?api_key=<key>` query param | integrations.sh Jellyfin API summary |
| `/Auth/Keys` | `GET` list, `POST` create API keys (requires admin authentication already established) | integrations.sh Jellyfin API summary |
| Config directory | `/config` inside container (Jellyfin appdata dir) | jellyfin.org container docs |
| Image | Official `jellyfin/jellyfin` (LinuxServer Overseerr is deprecated — distinct from Jellyfin) | jellyfin.org |

## Verification results (spikes/verify-compose)

| Check | Result | Detail |
|-------|--------|--------|
| Container startup | ✅ Pass | `jellyfin/jellyfin:12.4` starts on port 8096; config at `/config`; **requires ≥ 2 GiB free at the cache path** (`JELLYFIN_CACHE_DIR`, default `/cache`) — startup aborts with a clear `FTL` log otherwise |
| Public info endpoint `GET /System/Info/Public` | ✅ Pass | 200 without auth; returns `Version`, `StartupWizardCompleted:false` pre-setup — usable to poll wizard state from outside |
| `GET /System/Info/Public` auth model | ✅ Pass | Anonymous access allowed, so setup-completion can be detected before any token exists |
| Initial admin user via `POST /Users/New` (no auth) | ❌ **Fails** | 401 even with `StartupWizardCompleted:false` and no users existing — with and without `IsAdministrator` flag, with and without device-profile header |
| Wizard endpoints `/wizardstart`, `/wizarduser`, `/wizardlibrary`, `/wizardsettings`, `/wizardremoteaccess`, `/wizardfinish` | ⚠️ Client routes only | Exist in `jellyfin-web` as Ember client routes; server returns 404 for direct `/wizard*` requests |
| `/Startup/public`, `/Startup/nodeinfo/public`, `/Startup/Device`, `/Startup/Pin` | ❌ **404** | None of the guessed public startup endpoints exist in 12.4 |
| Public OpenAPI spec (`/swagger.json`, `/swagger/v1/swagger.json`, `/openapi.json`) | ❌ **404** | Not served publicly by 12.4 (Swashbuckle present in binaries but not exposed) |

**Conclusion:** Jellyfin 12.4 has **no public API to create the initial admin user headless**. The web setup wizard must be completed once (manually in the browser, at `http://<host>:8096`, or via automated browser driving `http://localhost:18096`). After the wizard completes, `POST /Auth/Keys` (admin-authenticated) creates API keys and library endpoints become available; poll `GET /System/Info/Public` (`StartupWizardCompleted`) to know when setup is done.

## What's NOT confirmed publicly

| Item | Why it matters | How to verify |
|------|----------------|---------------|
| Endpoint + request body to add media libraries (folders `/data/media/tv`, `/data/media/movies`) | PRD requirement A7 | Inspect Jellyfin API spec for library/folder endpoints, after wizard completion |
| Library folder-type IDs for TV vs Movies libraries via API | Needed to add the right library types | Test authenticated library API against running container |
| API key creation flow `POST /Auth/Keys` + `X-Emby-Token` auth | Needed for wiring engine to call Jellyfin APIs later | Test against container after wizard completion |
| Whether `StartupWizardCompleted` flips to `true` immediately after wizard finish | Drives the poll-and-proceed step in WP3.6 | Verify against running container after manual wizard completion |

## Recommended approach for WP3.6 Jellyfin client

Given the verified results above, the wiring engine cannot create Jellyfin's initial admin user programmatically in 12.4:

1. **One-time manual wizard:** Start Jellyfin, have the user open the setup wizard at `http://<host>:8096` (or drive it via an automated browser), create the admin user, and finish the wizard. The wizard can set library roots during setup; otherwise libraries are added next.
2. **Detect completion:** poll `GET /System/Info/Public` until `StartupWizardCompleted == true` (verified: readable without auth).
3. **Then API-driven:** authenticate as admin (`POST /Users/AuthenticateByName`), create an API key via `POST /Auth/Keys` (`X-Emby-Token` header), then add the TV library (`/data/media/tv`) and Movies library (`/data/media/movies`) through the library API (endpoints/bodies to be confirmed against the running container — see Open below).

If a fully headless flow is later required, re-test after any Jellyfin update that exposes a public first-user endpoint, or automate the wizard UI with a headless browser.

## Open / unverified

| Item | How to verify |
|------|---------------|
| Library creation endpoint + request body for folders (`/data/media/tv`, `/data/media/movies`) | Test authenticated library API against running container after wizard completion |
| Library folder-type IDs for TV vs Movies libraries via API | Same — inspect add-library payload from Dashboard traffic or API spec |
| API key creation flow `POST /Auth/Keys` + `X-Emby-Token` auth | Test against container after wizard completion |
| Whether `StartupWizardCompleted` flips to `true` immediately after wizard finish | Verify against running container after manual wizard completion |

## Decision needed

- Confirm pinned Jellyfin image tag (WP0.9): currently `jellyfin/jellyfin:12.4` in `templates/versions.yaml`.
- Wiring engine flow for Jellyfin = manual wizard once, then API-driven libraries + API key (per recommended approach above). Confirm this is acceptable for WP3.6, or escalate the headless-admin gap as a product decision.

## Sources

- Jellyfin container installation docs: https://jellyfin.org/docs/general/installation/container/
- Jellyfin user management docs: https://jellyfin.org/docs/general/server/users/adding-managing-users/
- Jellyfin HTTP API summary: https://integrations.sh/jellyfin.org/jellyfin-http-api/
- Jellyfin API (ReDoc, JS-rendered): https://api.jellyfin.org/
- Jellyfin API discussion re: user creation: https://github.com/orgs/jellyfin/discussions/12868
