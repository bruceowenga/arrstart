# Spike S3: Jellyfin headless setup

## Goal

Determine how to complete Jellyfin's initial setup headless via API (admin user creation) and add libraries for `/data/media/tv` and `/data/media/movies`, per PRD requirement **A7**:

> Jellyfin: complete the startup wizard via API with an admin user; add libraries for `/data/media/tv` and `/data/media/movies`.

And per build-plan §5.1: "Verify app API behavior (Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr) against the pinned versions running in real containers. Never trust recalled endpoints or field names without a test hitting the real app."

## Method

Searched official Jellyfin docs, api.jellyfin.org (ReDoc — JS-rendered, content not retrievable), GitHub discussions, and third-party API summaries. Did **not** run against real pinned containers yet — verification remains open. Do not trust recalled endpoints; all API fields below must be verified against the pinned Jellyfin image.

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

## What's NOT confirmed publicly

| Item | Why it matters | How to verify |
|------|----------------|---------------|
| Endpoint + request body to create the initial admin user (startup wizard via API) | PRD requires completing setup via API | Launch pinned Jellyfin, capture network traffic from the web setup wizard, or inspect `Jellyfin.Api` source |
| Endpoint + request body to add media libraries (folders `/data/media/tv`, `/data/media/movies`) | PRD requirement A7 | Inspect Jellyfin API spec for library/folder endpoints |
| Whether first-run setup can be bypassed entirely (skip wizard, create admin via config/API) | May avoid UI-wizard scraping | Search Jellyfin source for `StartupWizard` / initial setup logic |
| API key needed before first user exists? | Chicken-and-egg: API key creation requires admin auth | Verify against real container |

## Recommended approach options for WP3.6 Jellyfin client

### Option A: Call the same endpoints the web setup wizard uses
Capture network traffic from Jellyfin's web setup wizard (creating the admin user), replay those requests from the Go wiring client. Most faithful to Jellyfin's own flow.

### Option B: Create admin via direct API endpoint (if one exists)
If `POST /Users` or similar accepts a `Policy`/`IsAdministrator` flag and works unauthenticated on first run, use that directly. Unconfirmed — must verify against real container.

### Option C: Hybrid
Attempt Option B first; if unauthenticated creation fails, scrape/replay wizard traffic (Option A).

## Open / unverified

| Item | How to verify |
|------|---------------|
| Pinned Jellyfin image tag (WP0.9) | Decide before running verification containers |
| Admin user creation endpoint + request body | Capture wizard network traffic; inspect Jellyfin.Api/WebClient source |
| Library creation endpoint + request body for folders | Inspect Jellyfin API spec or web traffic when adding a library via Dashboard |
| Authentication model pre-first-user (can API calls proceed without a token?) | Test against running container |
| Libraries mapping: TV → `/data/media/tv`, Movies → `/data/media/movies` | Verify folder-type IDs and media library types via API |

## Decision needed

- Confirm pinned Jellyfin image tag (WP0.9).
- Decide Approach A vs B/C for the wiring engine implementation.

## Sources

- Jellyfin container installation docs: https://jellyfin.org/docs/general/installation/container/
- Jellyfin user management docs: https://jellyfin.org/docs/general/server/users/adding-managing-users/
- Jellyfin HTTP API summary: https://integrations.sh/jellyfin.org/jellyfin-http-api/
- Jellyfin API (ReDoc, JS-rendered): https://api.jellyfin.org/
- Jellyfin API discussion re: user creation: https://github.com/orgs/jellyfin/discussions/12868
