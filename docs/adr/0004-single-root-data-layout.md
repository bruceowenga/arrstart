# ADR-0004: Single-root /data layout, one folder asked

## Context

Bad folder layouts silently double disk usage and are the top cause of "why is my disk full" support threads (PRD §1). Apps need downloads and media on the same filesystem for hardlinks; qBittorrent needs complete/incomplete split and Sonarr/Radarr need separate TV/Movies roots.

## Decision

- Single media root chosen once by the user (one folder prompt, PRD L2/B5).
- Layout under `<root>/`:
  - `data/downloads/complete`, `data/downloads/incomplete` (qBittorrent default save path `/data/downloads`)
  - `data/media/tv`, `data/media/movies` (Sonarr/Radarr root folders)
  - `config/{sonarr,radarr,prowlarr,qbittorrent,jellyfin,seerr}` (per-app config volumes)
- Every app that touches media mounts the same `/data` (PRD L1).
- No separate download/media folder prompts; presets only change which apps are installed, not the layout.

## Consequences

- One filesystem boundary to reason about; hardlinks possible between downloads and media (Windows hardlink behavior validated in WP0.7/S5).
- Simplest permission model (one owner discussion than separate roots).
- Users with existing separate download/media folders must consolidate or accept moving content; documented in install guides.
