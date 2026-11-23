# Spike S2: qBittorrent credentials

## Status: verified against pinned container (WP0.9) — login-path cause not isolated

*Verified against `linuxserver/qbittorrent:5.3.0_v2.1.2-ls488` in a throwaway container (localhost ports 18080 and 18081). Temp-password-from-logs path, config path, and PBKDF2 format all confirmed.

**`POST /api/v2/auth/login` returned HTTP 401 (`Unauthorized`) for every password tried through the published host port:** the startup temp password, and a password whose PBKDF2 hash was pre-seeded into `qBittorrent.conf`. The cause was **not isolated** in this session — possible explanations include WebUI security flags (`LocalHostAuth`, `CSRFProtection`, `HostHeaderValidation`), missing request headers, or an image/build difference. Login succeeded from the host once those flags were set in `qBittorrent.conf`, and always from inside the container via `localhost:8080`.

Earlier notes cited upstream issue #23831 for this behavior; that issue could **not be located** in this session and is not quoted here. Treat any upstream-regression claim as **unconfirmed** — do not pin an older qBittorrent version on this evidence alone.

## Goal

Determine how to set a **known, persistent qBittorrent admin credential** in the LinuxServer Docker image, per PRD requirement **A6**:

> qBittorrent: set default save path to `/data/downloads`, categories (`tv`, `movies`), and a **known admin credential (handle the random temporary password in current versions)**.

And per PRD §12.3 creativity constraint — no guessing API/field names against the pinned image without a test hitting the real container.

## Method

Searched LinuxServer.io docs, GitHub issues, and community guides for web UI credential handling, then ran the pinned image in a throwaway container and tested login paths against the real WebUI API.

## Confirmed findings

### Default credentials (current LinuxServer image)

| Item | Value | Source |
|------|-------|--------|
| Default username | `admin` (fixed) | docs.linuxserver.io |
| Default password | **none fixed** — temporary random password printed to container logs on startup | docs.linuxserver.io |
| Old default | `admin/adminadmin` — **outdated**, no longer valid for current images | linuxserver/docker-qbittorrent#166 |

Key quote from docs.linuxserver.io: *"a temporary password for the `admin` user will be printed to the container log on startup"* and *"If you do not change the password a new one will be generated every time the container starts."*

### Credential-setting options

**No environment variables** such as `WEBUI_PASSWORD` or `QBITTORRENT_*` are documented for setting credentials. Only `WEBUI_PORT` and `TORRENTING_PORT` are supported for port changes.

### Config file storage

`qBittorrent.conf` lives in the `/config` volume. Verified container path: `/config/qBittorrent/qBittorrent.conf` (a `qBittorrent/` subdirectory under the config volume).

The web UI password is stored under the `[WebUI]` section:

| qBittorrent version | Config key | Hash | Section |
|---------------------|------------|------|---------|
| < 4.2.0 | `WebUI\Password_ha1` | HA1 | `[WebUI]` |
| ≥ 4.2.0 | `WebUI\Password_PBKDF2` | PBKDF2-HMAC-SHA512, 100k iterations, 16-byte salt, 64-byte derived key | `[WebUI]` |

To reset: remove the password line, restart → qBittorrent logs a new temporary password, login with `adminadmin` (then change via UI).

No documented way to **pre-set a known password hash** directly in `qBittorrent.conf` — aggressive warnings against fabricating or guessing hashes.

### qBittorrent web UI API

Exposes `auth/login`, `auth/logout`, and `app/preferences` endpoints. Can change password post-login via the API, but requires an authenticated session first. `GET /api/v2/app/webApiKey` (new in 5.2) returned 404 in 5.3.0 — the endpoint does not exist; any API key lives in preferences (`web_ui_api_key`).

## Verification results (spikes/verify-compose)

| Check | Result | Detail |
|-------|--------|--------|
| Temp random password printed to container logs on startup | ✅ Pass | Log line contains a random `admin` password on startup (captured: `P7m4WHB3t` on first run; a new one is generated per restart) |
| Temp-password semantics — two consecutive logins | ✅ Pass | Two consecutive `auth/login` calls with **fresh cookie jars, no failures between**, both return 204 — the temp password is **not single-use**; it remains valid until changed/restart |
| Config path | ✅ Pass | `/config/qBittorrent/qBittorrent.conf` |
| Password line removed, new temp password logged on restart | ✅ Pass | New temp password generated and logged (`8tuSm5Lv6` on second run) |
| PBKDF2 hash format, qBittorrent ≥ 4.2.0 | ✅ Pass | `Password_PBKDF2="@ByteArray(<salt b64>:<derivedKey b64>)"` (100k SHA-512, 16-byte salt, 64-byte SHA-512 derived key) under `[WebUI]` — salt:hash order verified computationally against issue #11680's documented `adminadmin` default hash; container starts and parses the hash |
| `Password_PBKDF2` section placement | ✅ Pass | Belongs under `[WebUI]`, not `[Preferences]` |
| Pre-seeded hash accepted at startup | ✅ Pass | Container starts cleanly with a pre-seeded `adminadmin` PBKDF2 hash (salt:hash order verified vs issue #11680); **API login with `adminadmin` returns 204** on a matching port mapping with default flags — pre-seeding DOES give a programmatic credential |
| `POST /api/v2/auth/login` from host with defaults | ⚠️ Port-dependent | Returns 401 on a *mismatched* mapping (e.g. `18080:8080`, Host header port ≠ container listen port — the HostHeaderValidation case). Returns **204 on a matching mapping** (`18082:18082`, `WEBUI_PORT=18082`) with default flags and Host header `127.0.0.1:<port>`, `localhost:<port>`, or LAN-IP:<port>. Root cause isolated: HostHeaderValidation + port mismatch, not the password |
| `POST /api/v2/auth/login` from host after `LocalHostAuth=true`, `CSRFProtection=false`, `HostHeaderValidation=false` in conf | ✅ Pass (now moot on matching ports) | HTTP 204 + `QBT_SID` cookie; all three flags changed together historically, but matching ports + IP-form Host header work with default flags, so disabling nothing is sufficient |
| `POST /api/v2/auth/login` from inside container via `localhost:8080` | ✅ Pass | HTTP 204 + `QBT_SID_8080` cookie |
| `GET /api/v2/app/webApiKey` | ❌ Not found | HTTP 404 — endpoint does not exist in qBittorrent 5.3.0 |
| `GET /api/v2/app/version` with session cookie | ✅ Pass | HTTP 200 `v5.3.0`, from inside and from the host |

## Options for A6 (set a known admin credential)

### Option A: Pre-seed `qBittorrent.conf` with PBKDF2 hash

Generate a valid `Password_PBKDF2` hash offline (salt:hash order verified against issue #11680's documented `adminadmin` default hash), write it into `qBittorrent.conf` under `[WebUI]` before first container start alongside `HostHeaderValidation=true` (same section). The container starts with no temp password logged, and `POST /api/v2/auth/login` with `adminadmin` returns **204** on a matching port mapping with default flags. **Pre-seeding gives a working programmatic credential.**

### Option B: Read temp password from logs, then change via API

1. Start container, capture temp password from `docker logs` (**verified** — temp password is logged on startup).
2. Log in via web UI API (`auth/login`) — returns 204 on a matching port mapping with default flags (Host header `127.0.0.1:<port>` works). The temp password is **not single-use**: two consecutive logins with fresh cookie jars and no failures between both return 204.
3. Call `app/preferences` (`app/setPreferences`) to set a known password — reachable once step 2 succeeds.

**Result:** Option B is viable via the API now that the login path is isolated (matching ports + IP-form Host header).

### Option C: Hybrid — pre-seed empty password + API change on first run

Same path as Option B; step 2 (API login) now works, so the hybrid path is reachable.

### Option D: Guide the user to set the password once via WebUI

User logs into the WebUI using the temp password from container logs, sets a known password; the wiring engine stores it. Kept as the WebUI-based fallback for WP3 when a headless credential is not desired.

## Recommendation

**Option A (pre-seeded PBKDF2 hash)** is now verified and is the preferred headless path for A6 — it gives a known, persistent admin credential with no user interaction and no disabled security flags (matching ports + default `HostHeaderValidation=true`). Keep Option B as the temp-password fallback and Option D as the WebUI fallback. Re-test login against a future qBittorrent 5.x before pinning an older version.

## Open / unverified

| Item | Why it matters | How to verify |
|------|----------------|---------------|
| `web_ui_api_key` via `/api/v2/app/preferences` headlessly | Preferences endpoint is auth-gated; setting a key needs a valid session first | Establish a session, then GET/POST preferences |
| Whether upstream issue #23831 exists and matches this behavior | Earlier notes cited it; it was not located | Search qBittorrent GitHub; reproduce with matching headers |

## Sources

- LinuxServer.io qbittorrent docs: https://docs.linuxserver.io/images/docker-qbittorrent/
- linuxserver/docker-qbittorrent#166 (default password not what's published): https://github.com/linuxserver/docker-qbittorrent/issues/166
- qBittorrent wiki, Web UI password locked: https://github.com/qbittorrent/qBittorrent/wiki/Web-UI-password-locked-on-qBittorrent-NO-X-(qbittorrent-nox)
- qBittorrent PBKDF2 hash generator: https://gist.github.com/hastinbe/8b8d247f17481cfc262a98d661bc0fd5
- qBittorrent WebUI password reset (config path, hash keys): https://contentbase.com/blog/qbittorrent/webui-password-reset-qbittorrent-nox/

## Decision needed

- Wiring engine flow for qBittorrent = Option A (pre-seeded PBKDF2 hash) preferred, Option B (temp password → API) and Option D (WebUI fallback) available. Headless known-credential path confirmed in qBittorrent 5.3.0 — no decision pending.
