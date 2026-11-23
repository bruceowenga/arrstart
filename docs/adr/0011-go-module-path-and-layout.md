# ADR-0011: Go module path and repo layout

## Context

The repo needs a Go module path, a single binary that serves both the wizard and the helper CLI (doctor, status, update, backup, add, remove, rewire, uninstall), and a place for embedded assets (compose templates, presets, UI templates/static) that `go:embed` can actually reach.

`go:embed` cannot read into parent directories, so a top-level `/templates` folder only embeds if it carries its own `embed.go` exposing an `embed.FS`. If templates, presets, and web live at top level without their own embed gates, the build breaks mid-implementation.

The PRD originally split `doctor` across `/doctor` and `/wizard/internal/doctor`. A single source of truth keeps check logic and fix text together.

## Decision

- Module path: `github.com/bruceowenga/arrstart`, `go.mod` at the repo root (not inside a `/wizard` subfolder, which would make the path `.../arrstart/wizard`).
- Single binary at `cmd/arrstart/main.go` with subcommands; wizard is the default subcommand. All application code under `internal/` — nothing is meant to be imported by outsiders.
- `/templates`, `/presets`, and `/web` each carry their own `embed.go` exposing an `embed.FS`.
- Single `internal/doctor` package with checks and fix text together; no separate `/doctor` folder.
- `/spikes` for throwaway research code (findings still documented in `docs/spikes/`), excluded from lint and release builds.
- Per-package `testdata/` directories for golden files.
- PRD §18 layout updated to match.

## Consequences

- The one binary image (`ghcr.io/bruceowenga/arrstart`) doubles as wizard container and helper CLI (`arrstack doctor`, `arrstack update`, etc.), matching ADR-0010's single-image decision.
- Adding a new embedded asset requires an `embed.go` gate in its directory — a small, repeatable convention worth documenting in CONTRIBUTING.md.
- `internal/` keeps the API surface small; forks can still copy code, which is fine for an open-source installer.
