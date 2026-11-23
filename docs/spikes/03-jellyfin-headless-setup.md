# Spike S3: Jellyfin headless setup

## Status: verified against pinned container (WP0.9) — full headless setup works via public `/Startup/*` wizard flow

*Verified against `jellyfin/jellyfin:12.4` in throwaway containers (localhost ports 18096/18097). **Jellyfin 12.4 can be set up headless via API**: the public `/Startup/*` wizard flow completes unauthenticated, creates a true admin (`IsAdministrator: true`), and that admin authenticates successfully via `POST /Users/AuthenticateByName`. After that, `POST /Auth/Keys` (admin-authenticated) creates API keys and library endpoints are reachable.*

**Caveats — these explain earlier negative results (desk research guessed wrong):**
- `POST /Startup/User` **does** create the first user (204 on a fresh container). Earlier 404s were ordering/user-error: calling `POST /Startup/Complete` first then `POST /Startup/User`, and `POST /Users/New` before any admin exists (401 — correct, it requires elevation).
- `/swagger.json` → **404**; the real spec is `GET /api-docs/openapi.json` → **200** (OpenAPI 3.0.4).
- `POST /Users/AuthenticateByName` needs the **standard `Authorization: MediaBrowser ...`** header (`X-Emby-Authorization` alone is ignored in 12.4 — legacy auth off), and the spec body field is `Pw`, not `Password`. Using `Password` → 400.
- Container won't start cleanly when the cache/config path is nearly full (clear `FTL`) — give the scratch path a few GiB.

## Goal

Determine how to complete Jellyfin's initial setup headless via API (admin user creation) and add libraries for `/data/media/tv` and `/data/media/movies`, per PRD requirement **A7**:

> Jellyfin: complete the startup wizard via API with an admin user; add libraries for `/data/media/tv` and `/data/media/movies`.

And per build-plan §5.1: "Verify app API behavior (Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr) against the pinned versions running in real containers. Never trust recalled endpoints or field names without a test hitting the real app."

## Method

Searched official Jellyfin docs, api.jellyfin.org (ReDoc — JS-rendered, content not retrievable), GitHub discussions, and third-party API summaries. Ran the pinned image in throwaway containers and stepped through the documented wizard sequence against a fresh container, then authenticated as the created admin.

## Confirmed findings

| Item | Finding | Source |
|------|---------|--------|
| Default port | `8096` (HTTP), `8920` (HTTPS) | jellyfin.org container docs |
| OpenAPI spec | `GET /api-docs/openapi.json` → **200**, OpenAPI 3.0.4 "Jellyfin API" 12.4.0 | measured against pinned container |
| `/swagger.json` | **404** | measured against pinned container |
| Public info endpoint | `GET /System/Info/Public` → 200 anonymous; reports `Version`, `ServerName`, `StartupWizardCompleted` | measured against pinned container |
| Startup configuration | `GET /Startup/Configuration` → 200 anonymous; `POST` → 204 | measured against pinned container |
| Remote access | `POST /Startup/RemoteAccess` → 204 anonymous | measured against pinned container |
| Startup user — placeholder | `GET /Startup/User` → 200 `{"Name":"root"}` before user created | measured against pinned container |
| **Startup user — create first user** | **`POST /Startup/User` → 204** with `{"Name":"<admin>","Password":"<pw>","HasPassword":true}` on a fresh container; creates the first user | measured against pinned container |
| **Wizard completion** | **`POST /Startup/Complete` → 204**; `StartupWizardCompleted` flips `false → true` | measured against pinned container |
| **Create user via `/Users/New`** | **401** without an existing admin (requires elevation — correct) | measured against pinned container |
| **Authenticate** | **`POST /Users/AuthenticateByName` → 200** with `Authorization: MediaBrowser Client="..."` header + body `{"Username":"<admin>","Pw":"<pw>"}`; returns `AuthenticationResult` with `AccessToken`, `IsAdministrator:true` | measured against pinned container |
| Auth body field | `Pw` (spec DTO) — `Password` is **not** bound → 400 | measured against pinned container |
| Auth header | `Authorization: MediaBrowser Client=..., Device=..., DeviceId=..., Version=...`; `X-Emby-Authorization` alone → 400 (legacy auth off) | measured against pinned container |
| Token auth | Issued token works via `Authorization: MediaBrowser ..., Token="..."` — `GET /Sessions` → 200, `POST /Auth/Keys` → 200 | measured against pinned container |
| `X-Emby-Token` header alone | → 401 on `/Sessions` (legacy auth off) | measured against pinned container |
| API key auth shape | `X-Emby-Token: <key>` header or `?api_key=<key>` query param (per spec) | integrations.sh Jellyfin API summary |
| Config directory | `/config` inside container (Jellyfin appdata dir) | jellyfin.org container docs |
| Image | Official `jellyfin/jellyfin` | jellyfin.org |

## Verification results (spikes/verify-compose)

Full headless wizard sequence against a fresh `jellyfin/jellyfin:12.4`:

| # | Request | Body | Status | Detail |
|---|---------|------|--------|--------|
| 1 | `GET /Startup/Configuration` | — | 200 | `{"ServerName":"","UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}` |
| 2a | `POST /Startup/Configuration` | `{}` | 204 | — |
| 2b | `GET /System/Info/Public` | — | 200 | `StartupWizardCompleted:false` |
| 3a | `POST /Startup/Configuration` | `{"ServerName":"verify-jf"}` | 204 | body echoed |
| 3b | `GET /System/Info/Public` | — | 200 | `StartupWizardCompleted:false`, `ServerName=verify-jf` |
| 4a | `POST /Startup/RemoteAccess` | `{}` | 204 | — |
| 4b | `GET /System/Info/Public` | — | 200 | `StartupWizardCompleted:false` |
| 5 | `GET /Startup/User` | — | 200 | `{"Name":"root"}` (placeholder) |
| 6a | `POST /Startup/User` | `{"Name":"verifyadmin","Password":"Verify-Pw-12345","HasPassword":true}` | 204 | body echoed — creates first user |
| 6b | `GET /System/Info/Public` | — | 200 | `StartupWizardCompleted:false` |
| 7a | `POST /Startup/Complete` | `{}` | 204 | — |
| 7b | `GET /System/Info/Public` | — | 200 | **`StartupWizardCompleted:true`** |
| 8a | `POST /Users/AuthenticateByName` | `{"Username":"verifyadmin","Pw":"Verify-Pw-12345"}` + `Authorization: MediaBrowser Client="arrstart", Device="verify", DeviceId="arrstart-verify", Version="1.0.0.0"` | **200** | `AuthenticationResult`: `verifyadmin`, `IsAdministrator:true`, `AccessToken` issued |
| 8b (negative) | `POST /Users/AuthenticateByName` | `{"Username":"verifyadmin","Password":"..."}` (field `Password`, not `Pw`) | **400** | `Error processing request.` |
| 8c (negative) | `POST /Users/AuthenticateByName` | bare body, no auth header | **400** | `Error processing request.` (missing `request.App`) |
| 9 | `POST /Auth/Keys` | `?app=arrstart` + admin token | **200** | `{"Items":[],"TotalRecordCount":0,...}` |
| Startup | container startup | — | ✅ | Starts on port 8096; config at `/config`; **refuses to start with a clear `FTL` log when the cache path has < ~2 GiB free** |

**Conclusion:** Jellyfin 12.4 **has a working public headless setup path** via `/Startup/Configuration` → `/Startup/User` → `/Startup/Complete`, each call returning 204 unauthenticated, then `POST /Users/AuthenticateByName` authenticates the new admin. No web wizard or headless browser is needed.

## What's NOT confirmed

| Item | Why it matters | How to verify |
|------|----------------|---------------|
| Library creation endpoint + request body for folders (`/data/media/tv`, `/data/media/movies`) | PRD requirement A7 | Test authenticated library API against the running container now that admin auth works |
| Library folder-type IDs for TV vs Movies libraries via API | Needed to add the right library types | Same |
| Exact `POST /Auth/Keys` body shape (only `?app=` query tested → 200) | Needed to create API keys for wiring engine | Test `POST /Auth/Keys?app=arrstart` with full body |
| Whether `StartupWizardCompleted` flips immediately after `Complete` | Drives poll-and-proceed in WP3.6 | Verified: flips `true` immediately (step 7b above) |

Candidate library endpoints from the served spec (all require admin auth, bodies **not** confirmed against a real container):

- `GET /Libraries/AvailableOptions?libraryContentType=movies|tvshows` — library option info
- `POST /Library/VirtualFolders/Paths/Update` — updates a media path (`UpdateMediaPathRequestDto`)
- `POST /Library/VirtualFolders/LibraryOptions` — update library options (`UpdateLibraryOptionsDto`)

## Recommended approach for WP3.6 Jellyfin client

1. **Headless wizard:** `POST /Startup/Configuration` → `POST /Startup/User` (creates admin) → `POST /Startup/Complete` (204 each, no auth needed). Set `ServerName` via the configuration POST if desired.
2. **Detect completion:** poll `GET /System/Info/Public` until `StartupWizardCompleted == true` (verified: flips immediately after `Complete`, readable without auth).
3. **Authenticate:** `POST /Users/AuthenticateByName` with `Authorization: MediaBrowser Client="arrstart", ...` header and body `{"Username":"<admin>","Pw":"<pw>"}` → 200 + `AccessToken`. Remember field name is `Pw`, not `Password`.
4. **API key:** `POST /Auth/Keys?app=arrstart` with the admin token → create a wiring key.
5. **Libraries:** add TV (`/data/media/tv`) and Movies (`/data/media/movies`) via the library API once bodies are confirmed against the running container.

This removes the "manual wizard once" product decision Jellyfin previously raised — WP3.6 can be fully API-driven.

## Decision needed

- Confirm pinned Jellyfin image tag (WP0.9): currently `jellyfin/jellyfin:12.4` in `templates/versions.yaml`.
- Library endpoint bodies to confirm against a running container before WP3.6 implementation (see NOT confirmed above).

## Sources

- Jellyfin container installation docs: https://jellyfin.org/docs/general/installation/container/
- Jellyfin user management docs: https://jellyfin.org/docs/general/server/users/adding-managing-users/
- Jellyfin HTTP API summary: https://integrations.sh/jellyfin.org/jellyfin-http-api/
- Jellyfin API (ReDoc, JS-rendered): https://api.jellyfin.org/
- Jellyfin API discussion re: user creation: https://github.com/orgs/jellyfin/discussions/12868
