# ADR-0010: Registry, image name, and namespace

## Context

The installer one-liner must pull from a versioned release asset, never from `main` (PRD §9). The wizard image needs a registry home, a name, and pinned tags (never `latest` in the one-liner). These choices affect the install one-liner, CI image builds, and any future namespace change, so they should live in one place.

## Decision

- GitHub repo: github.com/bruceowenga/arrstart (personal account, not an org)
- Image registry: GHCR only, `ghcr.io/bruceowenga/arrstart` — no Docker Hub for v1
- Image name: single `arrstart` image, multi-arch (amd64, arm64)
- Tags: pinned to repo versions (e.g. `0.0.1-alpha.1`), never `latest` in the one-liner
- Registry path kept in a single Go constant so the namespace can change in one place
- Package made public after first push; anonymous pull verified as part of release

## Consequences

- The install one-liner references `ghcr.io/bruceowenga/arrstart:<version>` and a versioned release asset URL.
- Moving the namespace later requires updating the constant, the one-liner, CI, and docs — contained but not zero-cost.
- GHCR authentication for pushes uses the personal `id_ed25519_personal` SSH key / GitHub token; no Docker Hub credentials are needed for v1.
- Multi-arch images mean CI must build both amd64 and arm64 (already planned in BUILD-PLAN §4.3).
