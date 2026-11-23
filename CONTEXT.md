# Arr Stack Installer — domain glossary

## Terms

**arrstart** — The project name. Resolved: **arrstart**. (Alternate considered: "Arrival", rejected as hard to search. Avoid pirate jokes to keep media-management framing.)

**Arr stack** — Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr, plus an optional gluetun VPN and Tailscale remote access. Homepage dashboard is included. Bazarr and subtitle wiring are post-v1.

**Bootstrap** — `install.sh` (Linux) / `install.ps1` (Windows): detects/installs Docker, asks for the media root folder, launches the wizard container, opens the browser, installs the `arrstack` helper wrapper.

**Wizard** — Go backend + HTMX + Go templates, SSE progress screen, single static binary, multi-arch image. Bind to localhost only, random one-time token in URL, `--rm`, exits when done.

**Generator** — template-per-app compose fragments merged into plain `docker-compose.yml` + `.env` + `README.txt`, left on disk for the user to own (no lock-in). Pinned image tags, never `latest`.

**Wiring** — native Go clients, one package per app, `WaitHealthy/Configure/Verify` interface. No Buildarr dependency. Recyclarr/Configarr only as optional post-v1 quality-profile add-ons.

**doctor** — shares wiring's API clients; plain-English checks with suggested fixes; redacted diagnostic report.

**Single-root layout** — one media root folder asked of the user; `<root>/data/{downloads,media}` and `<root>/config/{sonarr,radarr,prowlarr,qbittorrent,jellyfin,seerr}`.

**One-time token** — random token required in the wizard URL; authentication on arr apps enabled with one generated admin credential shown once.

**No real indexers** — nothing in code, docs, tests, or fixtures names or preconfigures a real indexer or content source. Mock Torznab indexer used in tests only.

**No secrets in git** — fixtures use obviously fake keys; no real `.env` or generated compose from real runs committed.

## Decided (bootstrap / WP0.1)

- Code of conduct: Contributor Covenant (use project contact / GitHub private reporting for enforcement, not a personal email)
- `.gitignore`: standard Go + OS noise, plus `.env`/`.env.*` (keep `.env.example`), `/dist/`, `/bin/`, `go.work`, local test media folders
- `.gitattributes`: force LF for `*.sh`, `*.go`, `*.yaml`, `testdata/**`; `install.ps1` kept ASCII-only
- Add `.editorconfig` and `SECURITY.md` (private vulnerability reporting path)
- README in first commit: one-paragraph legal framing = media management software, no indexers, no content
- Initial commit on `main`: README (with framing), LICENSE (MIT), PRD, BUILD-PLAN only
- WP0.1 scaffold lands as a PR from `chore/scaffold` — no direct commits to `main` after the initial one
- Wizard UI stack: HTMX + Go templates + SSE (PRD §17, marked decided).
- Wiring: native Go clients (PRD §17, marked decided).
- Media server: Jellyfin only for v1, swappable slot (PRD §17, marked decided).
- License: MIT (PRD §17, marked decided).
- Seerr image: `ghcr.io/seerr-team/seerr`, compose `user:` + own config folder (PRD §17, marked decided).

## Decided (registry / image / Go layout)

- GitHub repo: github.com/bruceowenga/arrstart (personal account, not an org)
- Image registry: GHCR only, ghcr.io/bruceowenga/arrstart (no Docker Hub for v1)
- Image name: single `arrstart` image, multi-arch (amd64, arm64)
- Tags: pinned to repo versions (0.0.1-alpha.1, ...), never `latest` in the one-liner
- Registry path kept in a single constant so the namespace can change later
- Package made public after first push; anonymous pull verified
- Go module path: github.com/bruceowenga/arrstart (go.mod at repo root)
- Single binary at cmd/arrstart/main.go with subcommands (wizard default, doctor, status, update, backup, add, remove, rewire, uninstall); all code under internal/
- /templates, /presets, /web each carry their own embed.go exposing an embed.FS (go:embed can't cross parent directories)
- Single internal/doctor package (checks + fix text together); no separate /doctor folder
- /spikes for throwaway research code (excluded from lint/release builds); per-package testdata/ for golden files

## Decided (spike findings)

- Servarr env-var API key injection confirmed: `<APPNAME>__AUTH__APIKEY` works for Sonarr (`SONARR__AUTH__APIKEY`), Radarr (`RADARR__AUTH__APIKEY`), Prowlarr (`PROWLARR__AUTH__APIKEY`); also `AUTH__ENABLED`, `AUTH__METHOD`, `AUTH__REQUIRED`. Pattern: `<APPNAME>__<NAMESPACE>__<CONFIGITEM>` overrides `config.xml`. Source: docs/spikes/01-servarr-api-keys.md.
- config.xml path and XML element name for stored API key: unverified — needs testing against real pinned containers.

## Open

- Windows default location — result of Phase 0 hardlink spike (PRD §12.3).
- VPN starting provider set — confirm gluetun WireGuard docs for Mullvad/ProtonVPN.
- Pin exact image tags (WP0.9).
- Verify env-var injection + config.xml fallback against real pinned Sonarr/Radarr/Prowlarr containers.
