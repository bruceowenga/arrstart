# ADR-0013: Apply engine shared by headless mode and wizard UI

Status: Proposed (not yet decided — awaiting user approval in PR #27).

## Context

The installer has two entry points to the same outcome: a headless path for CI e2e and reproducible bug reports (`arrstart apply --answers answers.yaml`) and a browser-based wizard (HTMX UI) for human users. Both must produce the same `docker-compose.yml` and `.env`, start the same stack, and run the same wiring (WaitHealthy/Configure/Verify per app) so results are identical regardless of entry point. Duplicating the apply logic per entry point would let them diverge (WP1.x wizard not built yet; wiring clients for sonarr, radarr, prowlarr, qbittorrent, jellyfin, seerr already exist).

## Decision

- Single `cmd/arrstart/main.go` binary. Wizard is the default subcommand; `apply --answers answers.yaml` is the headless subcommand; `version`, `doctor`, `status`, `update`, `backup`, `add`, `remove`, `rewire`, `uninstall` follow (PRD §10.6, C1–C7).
- New `internal/apply/` package: parse `answers.yaml` → generate `docker-compose.yml` + `.env` (generator) → `docker compose up -d` (shell out to compose per ADR-0007) → wait healthy → run the wiring runner per app → print the `Summary`.
- The wizard UI produces the same `answers.yaml` and delegates to `internal/apply` after the review screen, so UI and headless paths share one code path.
- CI e2e (`make e2e`, CI e2e job) runs headless: Gate 1 (healthy stack on clean Ubuntu) and Gate 3 (mock indexer scenario) are verified before any wizard screen exists, per PRD W9 and BUILD-PLAN WP1.9/WP1.11.

## Consequences

- Gate 1 and Gate 3 do not block on wizard UI work; UI screens (WP1.7/WP1.8) can follow Gate 1.
- One code path to test: `internal/apply` unit tests plus real-container e2e cover both headless and UI installs.
- `answers.yaml` becomes a documented contract: hand-editable, reproducible, storable for bug reports.
