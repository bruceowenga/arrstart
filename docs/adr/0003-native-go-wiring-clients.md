# ADR-0003: Native Go wiring clients (no Buildarr)

## Context

Auto-wiring (PRD Phase 3, requirements A1–A11) needs per-app API clients: wait for health, configure download clients/root folders/library links, verify results. Existing tools like Buildarr are Python and would add a runtime dependency to a Go-only stack.

## Decision

- Native Go API client packages under `internal/wiring/` — one per app: sonarr, radarr, prowlarr, qbittorrent, jellyfin, seerr.
- Shared interface: `WaitHealthy()`, `Configure()`, `Verify()`, with retries-with-backoff and per-step result reporting (WP3.1).
- Only the small set of calls actually needed (download client, root folders, naming/hardlinks, Prowlarr app links, Seerr connections, API keys).
- Buildarr is not a dependency; Recyclarr/Configarr are kept as optional post-v1 quality-profile add-ons, not app wiring (PRD §17 Decided).
- Client fixtures recorded from the pinned app versions in real containers (WP0.9 verification); unit tests use `httptest` servers with those fixtures (BUILD-PLAN §5.1).

## Consequences

- Single static binary deploys; no Python runtime for users.
- More code to own and maintain than delegating to existing tools; mitigated by narrow scope (only the calls wiring needs).
- Every client must be re-verified when app versions bump (pins in templates/versions.yaml); CI tests wiring against pinned versions (PRD 12.5).
