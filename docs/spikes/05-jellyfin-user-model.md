# Spike S5: jellyfin user model

Verified by the project owner against jellyfin/jellyfin:12.4 on 2026-11-25.
Do not re-verify; record the facts here.

## Facts

- The image defines no PUID/PGID variables. Its environment is `JELLYFIN_*`
  only: `JELLYFIN_CACHE_DIR=/cache`, `JELLYFIN_DATA_DIR=/config`,
  `JELLYFIN_CONFIG_DIR=/config/config`, `JELLYFIN_LOG_DIR=/config/log`.
- `/config` and `/cache` exist in the image as world-writable root-owned dirs.
- Started with `--user 1234:1234` and empty bind mounts owned by `1234:1234`
  for `/config` and `/cache`: served `/System/Info/Public` (Version 12.4.0,
  StartupWizardCompleted false) in ~7s, no permission errors, all files under
  `/config` created as `1234`.

## Consequence

jellyfin/jellyfin uses the compose `user:` mechanism (ADR-0006-style), not
the LSIO `PUID`/`PGID` env convention:

- generator emits `user: "${PUID}:${PGID}"` for jellyfin
  (`user_rules.compose_user: true`);
- generator must ensure `<root>/config/jellyfin` and
  `<root>/config/jellyfin-cache` exist and are owned by PUID:PGID before
  `compose up`;
- generator must NOT inject `PUID`/`PGID`/`TZ` environment — the image
  ignores them;
- cache lives at `/cache`, volume `<root>/config/jellyfin-cache -> /cache`
  (not under `/config`); media stays at `/data` read-only.
