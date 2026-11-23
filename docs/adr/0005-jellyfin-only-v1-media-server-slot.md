# ADR-0005: Jellyfin only for v1; media server is a swappable slot

## Context

The stack needs a media server for Seerr to connect to and for users to watch content. Jellyfin is open source, needs no account, and has headless setup. Plex/Emby may be wanted later.

## Decision

- Jellyfin is the default and only media server in v1 (PRD §9).
- Templates treat the media server as a swappable slot: templates and wiring interfaces do not hard-code Jellyfin, so Plex/Emby can be added later as one more template (PRD G8).
- Jellyfin wiring is scoped to: admin user created headless via the public startup wizard (`POST /Startup/Configuration` → `POST /Startup/User` → `POST /Startup/Complete`, then `POST /Users/AuthenticateByName` returns an admin `AccessToken`, verified in WP0.9/S3), libraries for /data/media/tv and /data/media/movies (PRD A7), Seerr connection settings.

## Consequences

- v1 scope stays small and testable against one pinned image (jellyfin/jellyfin:12.4).
- Adding Plex/Emby later means a new template fragment plus a media-server wiring adapter implementing the same interface; no changes to the core generator or other app clients.
- Users who want Plex/Emby in v1 are out of scope; documented as post-v1 (PRD Phase 6).
