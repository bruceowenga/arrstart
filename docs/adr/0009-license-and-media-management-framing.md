# ADR-0009: MIT license, no indexers, media-management framing

## Context

The tool installs media management software (Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr). It calls app APIs and generates compose files — the Sonarr/Radarr GPL licensing does not apply to this tool itself.

## Decision

- License: MIT
- Nothing in code, docs, tests, or fixtures names or preconfigures a real indexer or content source
- README and docs describe the software as media management only
- A plain disclaimer states users are responsible for what they download and for complying with local law
- Secret scanning with push protection enabled on the repo (gitleaks in pre-commit, secret scanning in CI)

## Consequences

- Low patent risk; easy for contributors and corporate users to adopt
- Legal framing must be reviewed whenever docs or the README change (docs-check in CI includes this)
- Automated secret scanning catches accidental key commits early
