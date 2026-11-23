# ADR-0007: Wizard drives `docker compose` by shelling out

## Context

The wizard must create, start, stop, and recreate containers during deploy. Two options: the Docker SDK for Go, or shelling out to the `docker compose` CLI. PRD §12.2 requires the wizard to bind localhost-only with a one-time token because it holds root-equivalent socket access.

## Decision

- The wizard shells out to `docker compose` (Docker CLI + compose plugin on the host), running the exact commands the user will run later — "no lock-in" literally (BUILD-PLAN §4.4).
- Compose files and `.env` are left plain on disk; the user can inspect, edit, and run `docker compose` themselves at any time (PRD §3 goal 5).
- The wizard requires Docker Engine + compose plugin installed by bootstrap (PRD B3); it does not embed a socket proxy or SDK-managed daemon connection.
- Wizard output streams compose logs so deploy progress is visible and matches what `docker compose logs` would show.

## Consequences

- No Docker SDK version drift; behavior matches the user's own CLI exactly.
- Depends on compose plugin presence (guaranteed by bootstrap; documented precondition otherwise).
- Errors from compose surface as plain text — straightforward to translate into plain-English wizard error screens (PRD W6).
- Shelling out is inherently slower and less "typed" than an SDK, but the wizard is a short-lived setup tool where correctness and transparency beat micro-performance.
