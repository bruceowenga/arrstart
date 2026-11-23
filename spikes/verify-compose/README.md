# Spike: verify pinned containers against S1–S3

**Status: complete.** Throwaway verification harness for WP0.9. Brought up the pinned app images (exactly as pinned in `templates/versions.yaml`) on localhost-only ports and re-ran the doc-only findings from:

- `docs/spikes/01-servarr-api-keys.md` (S1: Servarr `APP__AUTH__APIKEY` env-var injection) — **verified ✅**
- `docs/spikes/02-qbittorrent-credentials.md` (S2: qBittorrent temp password + known credential) — **verified, upstream blocker ⚠️**
- `docs/spikes/03-jellyfin-headless-setup.md` (S3: Jellyfin headless setup) — **verified, full headless setup via public `/Startup/*` wizard flow ✅**

## Run

```
cd spikes/verify-compose
mkdir -p scratch/sonarr scratch/radarr scratch/prowlarr scratch/qbittorrent scratch/jellyfin
docker compose up -d
./verify.sh
docker compose down -v
```

`compose.option-a.yaml` is a single-service experiment (qBittorrent pre-seeded `Password_PBKDF2` hash on port 18081) — run separately:

```
docker compose -f compose.option-a.yaml up -d
```

Scratch volumes are gitignored (`spikes/verify-compose/scratch/`).

## Results summary

| Spike | Image | Port | Result |
|-------|-------|------|--------|
| S1 — Servarr API key injection | sonarr 4.0.20.3014-ls6 / radarr 7.0.3 / prowlarr 3.0.2 | 18989 / 17878 / 19696 | ✅ All three enforce the injected `APP__AUTH__APIKEY`: 401 without/with wrong key, authenticated with correct key; `AUTH__ENABLED=true AUTH__REQUIRED=true AUTH__METHOD=external` accepted |
| S2 — qBittorrent credentials | qbittorrent 5.3.0_v2.1.2-ls488 | 18080 | Temp password logged on startup ✅; config path `/config/qBittorrent/qBittorrent.conf` ✅; `WebUI\Password_PBKDF2` under `[WebUI]` verified ✅ (100k SHA-512, 16-byte salt, `@ByteArray(...)`) — **but** `/api/v2/auth/login` returns HTTP 401 `Unauthorized` for every password tried from the host (temp password, pre-seeded-hash password, with/without `Host`/`Referer`/`Origin` headers); login works from inside the container via `localhost:8080` and from the host after setting `LocalHostAuth=true`, `CSRFProtection=false`, `HostHeaderValidation=false` in `qBittorrent.conf` (all three flags changed together; individual attribution not isolated). Upstream issue #23831 referenced in earlier notes but **not located** — treat any regression claim as unconfirmed. No headless known-credential path confirmed in 5.3.0 (`/api/v2/app/webApiKey` → 404, endpoint does not exist); user sets the password once via WebUI using the logged temp password. |
| S3 — Jellyfin headless setup | jellyfin 12.4 | 18096 | Starts only when cache path has ≥ 2 GiB free (clear `FTL` log otherwise) ✅; `GET /System/Info/Public` works anonymously and reports `StartupWizardCompleted` ✅; `GET /api-docs/openapi.json` → 200 (full OpenAPI 3.0.4 spec) ✅; `/swagger.json` → 404. **Full headless setup works via the public `/Startup/*` wizard flow** ✅ — `POST /Startup/Configuration` → 204, `POST /Startup/User` → 204 (creates first admin), `POST /Startup/Complete` → 204 (flips `StartupWizardCompleted` to `true`); `POST /Users/New` → 401 without admin (correct, requires elevation); authenticate via `POST /Users/AuthenticateByName` → 200 with `Authorization: MediaBrowser Client="..."` header and body field `Pw` (not `Password`), returning an `AccessToken` for an `IsAdministrator:true` user; `POST /Auth/Keys` → 200 with that token. No web wizard needed. |

## Scope

Verify against REAL containers, then record results back into the S1–S3 spike docs (done — replace "doc-only, unverified" with verified results; failures documented as upstream blockers) and into `docs/build-log.md`. Anything that fails stays marked unverified and goes into the open-items list for the wiring engine (WP3).
