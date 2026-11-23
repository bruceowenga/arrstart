# Spike S3: Jellyfin headless setup

## Status: verified against pinned container (WP0.9) — no reliable public headless path to create the initial admin user

*Verified against `jellyfin/jellyfin:12.4` in throwaway containers (localhost ports 18096/18097). Public info and startup-wizard endpoints respond; **no API path tested produced a login-able initial admin user**. `POST /Users/New` returns 401 without an existing admin; `POST /Startup/Complete` finishes the wizard without creating any user; `POST /Startup/User` returned 404 on one running container and 204 without a login-able user on another. One-time web wizard required to create the first admin.

**Correction — startup flow:** Jellyfin 12.4 serves a real OpenAPI spec at `GET /api-docs/openapi.json` (HTTP 200, OpenAPI 3.0.4). The wizard endpoints exist (`/Startup/Configuration`, `/Startup/RemoteAccess`, `/Startup/Complete`) and respond anonymously, but none creates the first user. Earlier desk research guessed `/swagger.json` — that returns 404 — and assumed `/Startup/*` wizard endpoints were all 404; that was wrong for `/Startup/Configuration`, `/Startup/RemoteAccess`, and `/Startup/Complete`, which return 200/204.

## Goal

Determine how to complete Jellyfin's initial setup headless via API (admin user creation) and add libraries for `/data/media/tv` and `/data/media/movies`, per PRD requirement **A7**:

> Jellyfin: complete the startup wizard via API with an admin user; add libraries for `/data/media/tv` and `/data/media/movies`.

And per build-plan §5.1: "Verify app API behavior (Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr) against the pinned versions running in real containers. Never trust recalled endpoints or field names without a test hitting the real app."

## Method

Searched official Jellyfin docs, api.jellyfin.org (ReDoc — JS-rendered, content not retrievable), GitHub discussions, and third-party API summaries. Ran the pinned image in throwaway containers and probed its public/auth boundaries empirically against the served OpenAPI spec.

## Confirmed findings

| Item | Finding | Source |
|------|---------|--------|
| Default port | `8096` (HTTP), `8920` (HTTPS) | jellyfin.org container docs |
| OpenAPI spec | `GET /api-docs/openapi.json` → **200**, OpenAPI 3.0.4 for Jellyfin 12.4.0 | measured against pinned container |
| `/swagger.json` | 404 | measured against pinned container |
| Public info endpoint | `GET /System/Info/Public` → 200 anonymous; reports `StartupWizardCompleted` and `Version` | measured against pinned container |
| Startup configuration | `GET /Startup/Configuration` → 200 anonymous (`ServerName`, `UICulture`, `MetadataCountryCode`, `PreferredMetadataLanguage`); `POST` → 204 | measured against pinned container |
| Remote access | `POST /Startup/RemoteAccess` → 204 | measured against pinned container |
| Wizard completion | `POST /Startup/Complete` → 204; sets `StartupWizardCompleted:true` but creates **no user** | measured against pinned container |
| Startup user | `POST /Startup/User` → **404** on one container; **204** with no login-able user on another — inconsistent with the OpenAPI spec which lists this POST; not a reliable first-user path | measured against pinned containers |
| First-user placeholder | `GET /Startup/User` → 200 `{"Name":"root"}` pre-first-user | measured against pinned container |
| Create user | `POST /Users/New` → **401** every time (with/without body, with/without device-profile headers, before and after wizard completion) | measured against pinned container |
| Authenticate | `POST /Users/AuthenticateByName` → real endpoint; 400 for unknown/bad credentials; never produced a token in this session (no login-able user exists yet) | measured against pinned container |
| API key auth shape | `X-Emby-Token: <key>` header or `?api_key=<key>` query param (per spec, unverified end-to-end — no admin token obtained) | integrations.sh Jellyfin API summary |
| `/Auth/Keys` | `GET` list, `POST ?app=` create — requires admin authentication (never reached without a wizard-created admin) | integrations.sh Jellyfin API summary |
| Config directory | `/config` inside container (Jellyfin appdata dir) | jellyfin.org container docs |
| Image | Official `jellyfin/jellyfin` | jellyfin.org |

## Verification results (spikes/verify-compose)

| Check | Result | Detail |
|-------|--------|--------|
| Container startup | ✅ Pass | `jellyfin/jellyfin:12.4` starts on port 8096; config at `/config`; earlier run aborted with a clear `FTL` log when the cache path had < 2 GiB free |
| `GET /System/Info/Public` | ✅ Pass | 200 without auth; `StartupWizardCompleted:false` pre-setup — usable to poll wizard state from outside |
| `GET /api-docs/openapi.json` | ✅ Pass | 200; full OpenAPI 3.0.4 spec — the authoritative source for endpoint shapes (field names still must be confirmed against the running container) |
| `GET /swagger.json` | ❌ 404 | Not served |
| `POST /Startup/Configuration` | ✅ Pass | 204 anonymous |
| `POST /Startup/RemoteAccess` | ✅ Pass | 204 anonymous |
| `POST /Startup/Complete` | ✅ Pass | 204 anonymous; `StartupWizardCompleted` becomes `true`, but no admin user exists afterwards |
| `POST /Startup/User` | ⚠️ Inconsistent | 404 on one running container; 204 on another but `root/p` still authenticates 400 — no login-able user created either way |
| `POST /Users/New` (no auth) | ❌ Fails | 401 even with `StartupWizardCompleted:false`, no users existing, `IsAdministrator` flag, or device-profile header |
| `POST /Users/AuthenticateByName` | ❌ No token | Real endpoint (400 on bad/unknown credentials); nothing to authenticate against until the wizard creates a user |

**Conclusion:** Jellyfin 12.4 has **no verified public headless path to create a login-able initial admin user**. The web setup wizard must be completed once (browser at `http://<host>:8096`, or an automated browser driving `http://localhost:18096`) to create the admin. After the wizard: poll `GET /System/Info/Public` until `StartupWizardCompleted == true`, authenticate via `POST /Users/AuthenticateByName`, create an API key via `POST /Auth/Keys` (auth header per spec), then add libraries via the library API (endpoint shapes below must be confirmed against the running container post-wizard).

Candidate library endpoints from the served spec (all require admin auth, bodies **not** confirmed against a real container):

- `GET /Libraries/AvailableOptions?libraryContentType=movies|tvshows` — library option info
- `POST /Library/VirtualFolders/Paths/Update` — updates a media path (`UpdateMediaPathRequestDto`)
- `POST /Library/VirtualFolders/LibraryOptions` — update library options (`UpdateLibraryOptionsDto`)

## What's NOT confirmed

| Item | Why it matters | How to verify |
|------|----------------|---------------|
| Library creation endpoint + request body for folders (`/data/media/tv`, `/data/media/movies`) | PRD requirement A7 | Complete the wizard once, then test authenticated library API |
| API key creation flow `POST /Auth/Keys` + `X-Emby-Token` auth end-to-end | Needed for wiring engine to call Jellyfin APIs | Test against container after wizard completion |
| Whether `StartupWizardCompleted` flips to `true` immediately after the wizard finishes | Drives the poll-and-proceed step in WP3.6 | Verify against running container after manual wizard completion |
| Auth token delivery shape (`AuthenticationResult`) from `POST /Users/AuthenticateByName` | Needed to call authenticated APIs | Test after wizard-created admin exists |

## Recommended approach for WP3.6 Jellyfin client

1. **One-time manual wizard:** Start Jellyfin, have the user open the setup wizard at `http://<host>:8096` (or drive it via an automated browser), create the admin user, and finish the wizard. The wizard can set library roots during setup; otherwise libraries are added next.
2. **Detect completion:** poll `GET /System/Info/Public` until `StartupWizardCompleted == true` (verified: readable without auth).
3. **Then API-driven:** authenticate as admin (`POST /Users/AuthenticateByName`), create an API key via `POST /Auth/Keys` (`X-Emby-Token` header per spec — to be confirmed end-to-end), then add the TV library (`/data/media/tv`) and Movies library (`/data/media/movies`).

If a fully headless flow is later required, automate the wizard UI with a headless browser, or re-test after a Jellyfin update that exposes a public first-user endpoint.

## Decision needed

- Confirm pinned Jellyfin image tag (WP0.9): currently `jellyfin/jellyfin:12.4` in `templates/versions.yaml`.
- Wiring engine flow for Jellyfin = manual wizard once, then API-driven libraries + API key (per recommended approach above). Confirm this is acceptable for WP3.6, or escalate the headless-admin gap as a product decision.

## Sources

- Jellyfin container installation docs: https://jellyfin.org/docs/general/installation/container/
- Jellyfin user management docs: https://jellyfin.org/docs/general/server/users/adding-managing-users/
- Jellyfin HTTP API summary: https://integrations.sh/jellyfin.org/jellyfin-http-api/
- Jellyfin API (ReDoc, JS-rendered): https://api.jellyfin.org/
- Jellyfin API discussion re: user creation: https://github.com/orgs/jellyfin/discussions/12868
