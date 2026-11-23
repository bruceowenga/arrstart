# Spike S4: Seerr setup via API and config ownership

## Status: verified against pinned container (Linux) — headless init requires the web UI

*Verified against `seerr/seerr:v3.5.2` (Docker Hub) in a throwaway container on localhost port 18051 with `/app/config` bind-mounted. Linux behavior confirmed: the image runs non-root (uid 1000 `node`) and config ownership must match the compose `user:` (PRD G4). Windows behavior not tested — needs the WP0.11 Windows VM. **Headless initial setup via API is not supported in v3.5.2:** initialization requires an authenticated `connect.sid` session and nothing anonymous issues one.*

## Goal

Determine how Seerr is set up headless via API (initial admin, connections to Jellyfin/Sonarr/Radarr), how the non-root `user:` and `/app/config` ownership should be handled on Linux and Windows, per PRD requirement **A8**:

> Seerr: connect to Jellyfin, Sonarr, Radarr; set default quality profile and root folders.

And PRD §9/#G4: "Seerr does not use the LSIO PUID/PGID convention: set compose `user:` and chown its config folder to match (change the user or the owner, not both)."

## Method

Ran the pinned image against localhost, inspected the running user and `/app/config` ownership, probed the API surface (`/api/v1/status`, settings endpoints, auth routes), inspected `settings.json`, fetched the `/setup` Next.js page and its JS chunks to find the initialization endpoint, and attempted headless initialization with and without a session cookie.

## Confirmed findings

| Item | Value | Detail |
|------|-------|--------|
| Image / tag | `seerr/seerr:v3.5.2` | Docker Hub (ghcr.io/seerr-team/seerr publishes no version tags — see `templates/versions.yaml` note; reconcile at WP0.12) |
| Container port | `5055` | Mapped to localhost-only host port in verification |
| Default user | `uid=1000(node) gid=1000(node)` | Non-root by default ✅ |
| Config directory | `/app/config` | Owned by node:node inside container; `settings.json`, `db/`, `logs/` created there |
| Host dir ownership when bind-mounted | matches container uid/gid | With default user (1000:1000) on a uid-1000 host, files are writable; written files show host owner 1000 ✅ |
| `user:` mismatch behavior | ❌ fails | Container started as `1001:1001` against a dir owned `1000:1000` exited 1 — could not write /app/config. chown (or matching `user:`) is mandatory. |
| Health endpoint | `GET /api/v1/status` | 200 without auth; returns `version`, `updateAvailable`, `commitsBehind`, `restartRequired`. Use for `WaitHealthy`. |
| Initialized state | `settings.json` → `public.initialized` | `False` before setup; not exposed by `/api/v1/status` in v3.5.2 |
| Initialization endpoint | `POST /api/v1/settings/initialize` | Used by the `/setup` wizard page; **requires** a `connect.sid` session cookie — 401 without any cookie, 403 with a dummy cookie (no permission). |
| No anonymous init API | — | `/api/v1/auth/login`, `/api/v1/auth/register`, `/api/v1/users`, `/api/v1/admin/*` are 404; `/api/auth/*` (NextAuth paths) 307-redirect to `/setup` while uninitialized. |
| Media server defaults | `settings.json` jellyfin block | Pre-filled `port: 8096`, empty `serverId`/`apiKey`/`urlBase`/`name`/libraries — ready to be pointed at the Jellyfin container once wizard completes. |
| Sonarr/Radarr blocks | `settings.json` `sonarr: []`, `radarr: []` | Empty arrays — connections added post-setup via authenticated settings API. |
| `main.apiKey` | present from first start | Base64 API key pre-generated; `main.localLogin: true`. |

## What's NOT confirmed publicly / on this host

| Item | Why it matters | How to verify |
|------|----------------|---------------|
| Initial admin user creation | v3.5.2 appears to create no usable local-login path via API; the `/setup` UI creates the admin | Verify whether `POST /api/v1/settings/initialize` (with a real session created how?) creates a log-in-able admin, or confirm the UI-only path by completing `/setup` in a browser once |
| Headless login after init | No `/api/v1/auth/login` route in v3.5.2 | Explore `/api/auth/*` NextAuth callbacks (credentials provider) after initialization |
| `user:` with non-1000 UID on Linux | PRD G4 requires chown match; unable to test here (no chown privilege as non-root user) | Run as `PUID:PGID` ≠ 1000:1000 against a chowned dir; confirm start + write succeeded |
| Windows behavior | Compose `user:` not used on Windows; config dir created by Docker Desktop | Run pinned image on Windows, inspect process user and /app/config ownership (WP0.11 VM) |
| Connection payloads | `POST /api/v1/settings/jellyfin`, `/sonarr`, `/radarr` exist (found in chunks) but require authenticated session | Record bodies from a real session after UI setup; encode as test fixtures |

## Recommendation for WP3.7 Seerr client

1. **Linux compose:** set `user: "${PUID}:${PGID}"` (default `1000:1000`) on the Seerr service — do NOT add PUID/PGID env vars (Seerr ignores them). The wizard **chowns `<root>/config/seerr` to PUID:PGID** before first start so the container can write `settings.json`/`db/`. If the dir already matches, no-op (idempotent).
2. **Windows:** no `user:`; config dir is created by Docker Desktop — verify ownership works on the WP0.11 VM before sealing this.
3. **First-run setup:** complete once via the web UI at `http://<host>:<port>/setup` (user creates the admin). The wiring engine cannot replace this step in v3.5.2 — document it in the wizard's "what to do next" or as a one-time setup screen with a link to Seerr's URL.
4. **Post-setup connections:** with an authenticated session, call `POST /api/v1/settings/jellyfin` (Jellyfin URL + API key from Jellyfin's `/Auth/Keys`), `POST /api/v1/settings/sonarr` and `/api/v1/settings/radarr` (URL + API key) — record exact bodies against the real running container before WP3.7 implementation; store as fixtures.
5. **Health:** poll `GET /api/v1/status` (200 = up) for `WaitHealthy`; `restartRequired`/`updateAvailable` are informational.

## Open / unverified

- Exact request bodies for Seerr→Jellyfin/Sonarr/Radarr connection settings (needs authenticated session; do after UI init)
- Whether any programmatic local-login path exists after initialization
- Windows run-as-user / config-ownership behavior (WP0.11)

## Decision needed

- Confirm the one-time-web-UI setup step is acceptable for PRD A8/A7-equivalent Seerr flow, or escalate headless Seerr init as a product decision (it blocks fully unattended installs in v3.5.2).
- Pinned Seerr tag `v3.5.2` already recorded in `templates/versions.yaml`; registry (`seerr/seerr` Docker Hub vs `ghcr.io/seerr-team/seerr`) reconciled at WP0.12.

## Sources

- Seerr docs: https://docs.seerr.tv/
- Seerr image: https://hub.docker.com/r/seerr/seerr/tags (v3.5.2)
- ghcr.io/seerr-team/seerr has no version tags (seerr-team/seerr#736, seerr-team/seerr#2264)
