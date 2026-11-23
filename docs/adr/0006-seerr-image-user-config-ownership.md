# ADR-0006: Seerr official image, `user:` and config ownership approach

## Context

PRD G4: "Seerr does not use the LSIO PUID/PGID convention: set compose `user:` and chown its config folder to match (change the user or the owner, not both)." WP0.9 pinned seerr/seerr:v3.5.2 on Docker Hub (ghcr.io/seerr-team/seerr publishes no version tags — reconcile at WP0.12). Spike S4 verified behavior against the real container (docs/spikes/04-seerr-setup.md).

## Decision

- Image: `seerr/seerr:v3.5.2`, pinned in templates/versions.yaml. Not `latest`; not ghcr.io/seerr-team/seerr until versioned tags appear there.
- Linux compose: set `user: "${PUID}:${PGID}"` on the Seerr service (default 1000:1000). Do NOT add PUID/PGID env vars — Seerr does not honor them.
- Before first start, the wizard chowns `<root>/config/seerr` to PUID:PGID so Seerr can write settings.json/db/logs inside /app/config. No-op when ownership already matches (idempotent re-runs).
- Windows: no `user:`; Docker Desktop creates the config dir — behavior to be verified on the WP0.11 Windows VM (WP0.7/S5 dependency).
- Seerr runs non-root by default (uid 1000 `node`, verified). With `user:` ≠ host dir owner, Seerr exits 1 (verified: uid 1001 against a 1000:1000 dir failed) — doctor checks and names this failure.
- First-run setup: no public headless admin API in Seerr 3.5.2 — `/api/v1/settings/initialize` requires an authenticated connect.sid session and nothing anonymous issues one. The admin user is created once through the web UI at /setup; after that, `POST /api/v1/settings/{jellyfin,sonarr,radarr}` connections and API-key flows run against the authenticated API (bodies to be recorded from a real session as fixtures before WP3.7).

## Consequences

- One privileged chown step on Linux at setup; covered by install.sh (folder creation already sets ownership for the layout — PRD L1/L2).
- If ownership mismatches, Seerr fails to start with a clear cause the doctor/rewire path can fix.
- Seerr cannot be fully initialized headless in v3.5.2 — a one-time browser step (/setup) is a documented product gap, not something the wiring engine can automate.
- Registry decision for Seerr remains open for release (WP0.12): Docker Hub for now.
