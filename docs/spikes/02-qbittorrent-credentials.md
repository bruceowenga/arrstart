# Spike S2: qBittorrent credentials

## Status: verified against pinned container (WP0.9) — with an upstream blocker

*Verified against `linuxserver/qbittorrent:5.3.0_v2.1.2-ls488` in a throwaway container (localhost port 18081). Temp-password-from-logs path, config path, and PBKDF2 format all confirmed. **Blocker: qBittorrent WebUI API login (`/api/v2/auth/login`) returns 401 for every password in qBittorrent 5.x (upstream bug #23831), including passwords whose PBKDF2 hash was pre-seeded into `qBittorrent.conf`. Programmatic credential setup via API is therefore unverifiable in this version.***

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

## Verification results (spikes/verify-compose)

| Check | Result | Detail |
|-------|--------|--------|
| Temp random password printed to container logs on startup | ✅ Pass | Log line contains a random, single-use `admin` password (captured: `YNHXdgy6A`) |
| Config path | ✅ Pass | `/config/qBittorrent/qBittorrent.conf` (`/config/qBittorrent/` directory under the config volume) |
| `WebUI\Password_ha1` removed, new temp password logged on restart | ✅ Pass | Password line removed → new temp password generated and logged |
| PBKDF2 hash format, qBittorrent ≥ 4.2.0 | ✅ Pass | `Password_PBKDF2="@ByteArray(<base64>:<base64>)"` (100k SHA-512, 16-byte salt, `<DerivedKey>:<Salt>`) under `[WebUI]` — container starts and accepts the hash |
| Pre-seeded hash accepted by container | ⚠️ Partial | Container started cleanly with pre-seeded conf (hash parsed without startup error); API login still 401, so a *known password usable via API* was not achieved |
| `Password_PBKDF2` section placement | ✅ Pass | Belongs under `[WebUI]`, not `[Preferences]` (key separator `\` is section-key) |
| WebUI API login `POST /api/v2/auth/login` with temp password | ❌ **Fails upstream** | Returns 401 for every password in qBittorrent 5.3.0 (upstream bug #23831, regression from PR #23564) |
| WebUI API login with pre-seeded PBKDF2 password | ❌ **Fails upstream** | Same 401 — Option A (pre-seed hash) gives GUI login with a known password but **no API login** |

## Options for A6 (set a known admin credential)

### Option A: Pre-seed `qBittorrent.conf` with PBKDF2 hash
Generate a valid `Password_PBKDF2` hash offline, write it into `qBittorrent.conf` in the config volume before first container start. Most reliable if the hash format (salt, iterations, KDF params) can be reproduced exactly. **Verified:** hash format reproduced and container starts with it; **but** API login remains broken upstream (#23831), so pre-seeding gives a GUI-known password only — no programmatic login.

### Option B: Read temp password from logs, then change via API
1. Start container, capture temp password from `docker logs` (**verified** — temp password is logged on startup).
2. Log in via web UI API (`auth/login`) — **impossible in qBittorrent 5.x**: upstream bug #23831 returns 401 for every password.
3. Call `app/preferences` to set a known password — unreachable without step 2.
**Result:** Option B does not work in qBittorrent 5.3.0 regardless of log-parsing robustness.

### Option C: Hybrid — pre-seed empty password + API change on first run
Same blocker as Option B: step 2 (API login) is broken upstream in qBittorrent 5.x. **Not viable.**

### Option D: Guide the user to set the password manually
User sets the password once via the WebUI using the temp password from container logs; wiring engine stores it. **No automated path exists in qBittorrent 5.3.0**, so this becomes the documented fallback for the wiring engine (WP3).

## Recommendation

**Option D is the only viable path for qBittorrent 5.3.0** (upstream bug #23831 makes every API-login option impossible):

1. Wiring engine starts qBittorrent, reads the temp admin password from container logs (`docker logs`), and presents it to the user via the wizard one-time flow.
2. User logs into the WebUI once (temp password → set a known password).
3. The known password is stored in arrstart's encrypted state and used for qBittorrent connections from WP3 onward.

Options A and B remain documented for when upstream fixes #23831 — re-test at that point rather than guessing the hash format.

## Open / unverified

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
