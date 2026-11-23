# Spike S2: qBittorrent credentials

## Status: doc-only, unverified

*Findings below were gathered from public documentation only. Not yet verified against a real pinned qBittorrent container. Re-run against the pinned image (WP0.9 tag) before trusting any env var, config path, or hash format.*

## Goal

Determine how to set a **known, persistent qBittorrent admin credential** in the LinuxServer Docker image, per PRD requirement **A6**:

> qBittorrent: set default save path to `/data/downloads`, categories (`tv`, `movies`), and a **known admin credential (handle the random temporary password in current versions)**.

And per PRD §12.3 creativity constraint — no guessing API/field names against the pinned image without a test hitting the real container.

## Method

Searched LinuxServer.io docs, GitHub issues, and community guides for web UI credential handling. Did **not** run against real pinned containers yet — verification remains open.

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

`qBittorrent.conf` lives in the `/config` volume. Exact container path is uncertain between sources — either `/config/qBittorrent.conf` or `/config/qBittorrent/qBittorrent.conf` — **must be verified against the pinned image**.

The web UI password is stored under the `[Preferences]` section:

| qBittorrent version | Config key | Hash |
|---------------------|------------|------|
| < 4.2.0 | `WebUI\Password_ha1` | HA1 |
| ≥ 4.2.0 | `WebUI\Password_PBKDF2` | PBKDF2 |

To reset: remove the password line, restart → qBittorrent logs a new temporary password, login with `adminadmin` (then change via UI).

No documented way to **pre-set a known password hash** directly in `qBittorrent.conf` — aggressive warnings against fabricating or guessing hashes.

### qBittorrent web UI API

Exposes `auth/login`, `auth/logout`, and `app/preferences` endpoints. Can change password post-login via the API, but requires an authenticated session first.

## Options for A6 (set a known admin credential)

### Option A: Pre-seed `qBittorrent.conf` with PBKDF2 hash
Generate a valid `Password_PBKDF2` hash offline, write it into `qBittorrent.conf` in the config volume before first container start. Most reliable if the hash format (salt, iterations, KDF params) can be reproduced exactly. **Risk:** hash format not fully documented; wrong hash → qBittorrent won't start or won't accept login.

### Option B: Read temp password from logs, then change via API
1. Start container, capture temp password from `docker logs`.
2. Log in via web UI API (`auth/login`).
3. Call `app/preferences` to set a known password.
**Risk:** log parsing is fragile (log rotation, multi-line output), two-step dance, temp password only valid until restart.

### Option C: Hybrid — pre-seed empty password + API change on first run
Pre-seed `qBittorrent.conf` with no password line → qBittorrent generates temp password → read from logs → API-change to known password → on subsequent starts the known password persists. Combines A and B but still log-dependent on first run.

### Option D: Guide the user to set the password manually
Violates the PRD goal ("no manual config file editing", zero manual settings changes). **Reject.**

## Recommendation

**Attempt Option A first** (pre-seed with PBKDF2 hash). If the hash format can be confirmed (generate a test hash with a known password, start a container with it, verify login), this gives a deterministic, single-path credential flow with no log parsing. The `gist.github.com/hastinbe/8b8d247f17481cfc262a98d661bc0fd5` PBKDF2 generator may help reproduce the format.

Fallback to Option B if Option A proves infeasible.

## Open / unverified

| Item | Why it matters | How to verify |
|------|----------------|---------------|
| Exact `qBittorrent.conf` container path (`/config/qBittorrent.conf` vs `/config/qBittorrent/qBittorrent.conf`) | Needed to mount/write config volume correctly | Launch pinned image, `find /config -name 'qBittorrent.conf'` |
| PBKDF2 hash format (salt, iterations, params) for v≥4.2.0 | Needed for Option A pre-seed | Generate test hash, start container, verify login |
| Whether `Password_PBKDF2` alone is sufficient or `Password_ha1` also needed | Compatibility across qBittorrent versions | Test against pinned version |
| Wiring flow: categories (`tv`, `movies`) and save path via API | Part of A6 beyond credentials | Test app/preferences API against pinned container |
| Pinned qBittorrent image tag | Version affects hash format (4.2.0 boundary) | Decided at WP0.9 |

## Sources

- LinuxServer.io qbittorrent docs: https://docs.linuxserver.io/images/docker-qbittorrent/
- linuxserver/docker-qbittorrent#166 (default password not what's published): https://github.com/linuxserver/docker-qbittorrent/issues/166
- qBittorrent wiki, Web UI password locked: https://github.com/qbittorrent/qBittorrent/wiki/Web-UI-password-locked-on-qBittorrent-NO-X-(qbittorrent-nox)
- qBittorrent PBKDF2 hash generator: https://gist.github.com/hastinbe/8b8d247f17481cfc262a98d661bc0fd5
- qBittorrent WebUI password reset (config path, hash keys): https://contentbase.com/blog/qbittorrent/webui-password-reset-qbittorrent-nox/

## Decision needed

- Confirm pinned qBittorrent image tag (WP0.9) before verification containers can run.
- Decide between Option A (pre-seed hash) and Option B (logs→API) for the wiring engine implementation.
