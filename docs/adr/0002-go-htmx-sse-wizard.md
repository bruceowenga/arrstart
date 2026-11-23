# ADR-0002: Go backend with HTMX + Go templates + SSE

## Context

The wizard UI must run inside a small container image, require no Node build step (PRD 12.1), and drive a live progress screen during deploy (PRD W5). WP0.8 spike S6 compared candidate UI stacks (docs/spikes/wp08-consolidation.md).

## Decision

- Wizard: Go HTTP server, HTML via Go templates, interactivity via HTMX, live progress via server-sent events (SSE). UI templates and static assets embedded with `go:embed` in a single static binary (PRD §12.2).
- No JavaScript framework (Svelte/etc.) unless the wizard grows well beyond ~5 screens (PRD §12.2).
- Progress reported over SSE from real container health checks and streamed logs — not fake timers (PRD 11.2).
- Headless mode via `--answers answers.yaml` (proposed PRD W9) shares the same HTTP backend; browser and headless are two entry points into one wizard (BUILD-PLAN §4.4).

## Consequences

- One static binary and one container image; no Node/npm toolchain in CI or the wizard image.
- Server-rendered HTML works without a JS-heavy client; HTMX keeps screen-to-screen navigation simple and testable.
- SSE ties progress to real health checks, so deploy status is accurate but depends on container health endpoints behaving (verified for Sonarr/Radarr/Prowlarr/qBittorrent/Jellyfin/Seerr in WP0.9).
- HTMX SSE event-stream contract becomes an internal interface; changes need wizard + test updates together.
