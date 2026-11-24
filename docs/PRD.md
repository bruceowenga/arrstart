# PRD: Arr Stack Installer

> Working title: `arrstack-easy` (placeholder, see Open Questions)
> Status: Draft v1.1 (open questions resolved, see section 17)
> How to use this doc: every `- [ ]` is a checkable task. Phases are ordered; each ends with an acceptance gate.

---

## 1. Summary

A GitHub repo that lets someone with no Docker experience go from "nothing installed" to a working, fully connected media automation stack (Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr) on an old PC or their everyday computer, by pasting one line into a terminal and clicking through a short web wizard.

## 2. Problem

- Many people want to self-host the arr stack but are blocked by Docker, compose files, volume paths, permissions, and the manual wiring between apps.
- Existing compose files solve the smallest part of the problem. The painful parts are: installing Docker, getting folder paths right, and connecting the apps to each other (API keys, download client, indexer sync, root folders, request app).
- Bad folder layouts silently double disk usage (no hardlinks) and are the most common cause of "why is my disk full" support threads.

## 3. Goals

1. **Zero-to-working in under 20 minutes** on a fresh Windows 11 or Ubuntu machine, with no manual config file editing.
2. **No Docker knowledge required.** The tool installs Docker if missing and explains what it is doing in plain language.
3. **Apps arrive already connected.** After install, a user can open Seerr, request a movie, and the request flows to Radarr and the download client without the user touching a settings page.
4. **Correct by default.** Single-root `/data` layout, correct permissions, hardlink-friendly.
5. **No lock-in.** Output is a plain `docker-compose.yml` and `.env` the user owns and can edit.
6. **Fit for old hardware.** Presets scale to the machine.

## 4. Non-goals (v1)

- Not a general-purpose Docker GUI or app store.
- No bundled or preconfigured indexers, trackers, or links to content. The tool sets up media management software only.
- No standalone desktop app (Go/Tauri). Docker-based only.
- No cloud hosting, accounts, or telemetry.
- No Kubernetes, Swarm, or multi-host setups.
- macOS is best-effort after Linux and Windows ship.

## 5. Target users

| Persona | Description | Needs |
|---|---|---|
| **Newcomer** | Has an old PC or a laptop, never used Docker | One line to run, plain-English guidance, no surprises |
| **Family admin** | Wants to give family a Netflix-like request experience | Seerr working, Jellyfin working, remote access without port forwarding |
| **Tinkerer** | Knows Docker, hates wiring boilerplate | Clean compose output they can take over and edit |

## 6. Success metrics

- Time from paste-the-one-liner to first successful Seerr request (target: under 20 min on a clean machine, excluding download time).
- Install success rate on the test matrix (target: 95%+ of clean-VM runs end healthy with zero manual steps).
- Share of support issues solvable by `doctor` output alone.
- GitHub stars and issues are vanity; track "issues caused by path or permission mistakes" and drive it toward zero.

---

## 7. Product principles

1. **Explain every step** in plain language before doing it.
2. **Nothing hidden.** Every action is logged and every generated file is left on disk.
3. **Safe defaults.** Nothing exposed to the internet, no default passwords.
4. **Resumable.** Any step that needs a reboot or retry must pick up where it left off.
5. **Boring tech.** Pinned images, plain compose, official APIs.

---

## 8. User journey

1. User opens the README, copies one line for their OS (`curl ... | bash` or a PowerShell one-liner).
2. Bootstrap checks for Docker. If missing, it explains and installs it. If a reboot or sign-out is needed, it registers a resume step and tells the user exactly what will happen.
3. Bootstrap asks for the **media root folder** (native folder picker on Windows, prompt on Linux/macOS).
4. Bootstrap starts the **wizard container** on a high local port (default 8765, auto-fallback if busy) and opens the browser.
5. Wizard: pick a **preset** (or custom), review the suggested apps (Prowlarr suggested and pre-ticked), toggle optional extras (VPN, Tailscale), review the plan.
6. Wizard writes `docker-compose.yml` and `.env`, pulls images, starts containers, and shows a **live progress screen** driven by health checks.
7. Wizard **auto-wires** the apps and shows each step as it completes.
8. Wizard finishes with a **home page and "what to do next" checklist**, then shuts itself down.
9. A small helper command (`arrstack`) stays on the machine for `doctor`, `update`, `backup`, `add`, and `uninstall`.

---

## 9. Scope: apps

| App | Role | v1 | Notes |
|---|---|---|---|
| Sonarr | TV management | Yes | |
| Radarr | Movie management | Yes | |
| Prowlarr | Indexer manager | Yes (suggested, pre-ticked) | Ships empty. User adds their own indexers |
| qBittorrent | Download client | Yes | Required for a complete stack |
| Jellyfin | Media player/server | Yes (default on, optional) | Jellyfin only for v1. Media server is a swappable slot in the templates so Plex/Emby can be added later |
| Seerr | Requests | Yes | Official successor to Overseerr and Jellyseerr. Image: `ghcr.io/seerr-team/seerr` (also `seerr/seerr` on Docker Hub). Config at `/app/config`, runs as non-root: use compose `user:` and chown the config folder |
| gluetun | VPN container | Optional, "advanced", off by default | Routes qBittorrent through VPN. Starting providers: Mullvad, ProtonVPN, plus bring-your-own WireGuard/OpenVPN config |
| Tailscale | Remote access | Optional | No port forwarding |
| Homepage | Dashboard | Yes | Generated config with links and widgets |
| Bazarr | Subtitles | Post-v1 | |
| Recyclarr/Configarr | Quality profile sync | Post-v1, optional one-shot container | Quality profiles and custom formats only, not app wiring |

---

## 10. Functional requirements

IDs are for traceability in issues and PRs.

### 10.1 Bootstrap
- **B1** One-liner for Linux (`install.sh`) and Windows (`install.ps1`).
- **B2** Detect whether Docker is installed and the daemon is running (`docker info`).
- **B3** If missing: Windows uses `winget install Docker.DockerDesktop`; Linux uses the official Docker convenience/repo method per distro (Debian/Ubuntu first).
- **B4** Handle reboot, sign-out, and WSL2 enablement with a resume mechanism (scheduled task or RunOnce key on Windows; systemd unit or flag file on Linux).
- **B5** Folder selection: native picker on Windows, prompt on Linux/macOS. Validate: exists or creatable, writable, not a network share or USB (warn), enough free space (warn).
- **B6** Detect hardware (RAM, CPU cores, free disk) and pass results to the wizard.
- **B7** Launch the wizard container with the media root, host OS, PUID/PGID (Linux/macOS), and hardware info as env vars.
- **B8** Pick a free port (default 8765), open the default browser.
- **B9** Install the `arrstack` helper wrapper script (`arrstack.sh` / `arrstack.ps1`) on the user's PATH or in the install folder.

### 10.2 Wizard
- **W1** Preset selection: Movies only, Family media server, Full automation, Custom.
- **W2** App selection with plain-language descriptions. Prowlarr pre-ticked with a "why you want this" note.
- **W3** Hardware-aware defaults: low RAM (about 4 GB or less) gets a lean preset and a warning on heavy extras.
- **W4** Review screen showing exactly what will be created (folders, containers, ports).
- **W5** Deploy screen with live progress from container health checks and streamed logs (collapsed by default).
- **W6** Error screen in plain English with a "copy diagnostics" button.
- **W7** Final screen: service links, generated credentials, and a "what to do next" checklist.
- **W8** Wizard binds to localhost only and requires a random one-time token in the URL (see Security).

### 10.3 Generator
- **G1** Template-per-app compose fragments, merged based on selection.
- **G2** Outputs `docker-compose.yml`, `.env`, and a short `README.txt` in the install folder.
- **G3** Normalize Windows paths to forward slashes (`C:/Media`).
- **G4** Set `PUID`/`PGID` only on Linux/macOS; set `TZ` from the host. Seerr does not use the LSIO PUID/PGID convention: set compose `user:` and chown its config folder to match (change the user or the owner, not both).
- **G5** Pin image tags. Prefer linuxserver.io images where available.
- **G6** Auto-resolve port conflicts and record the chosen ports in `.env`.
- **G7** Idempotent: re-running with the same inputs yields the same files; re-running with changes produces a diff, not a clobber.
- **G8** Media server is a swappable slot: Jellyfin is the only v1 implementation, but templates and wiring interfaces must not hard-code it, so Plex/Emby can be added as one more template.

### 10.4 Folder layout (single root)
```
<media-root>/
├── data/
│   ├── downloads/        # qBittorrent
│   │   ├── complete/
│   │   └── incomplete/
│   └── media/
│       ├── tv/
│       └── movies/
└── config/               # per-app config volumes
    ├── sonarr/
    ├── radarr/
    ├── prowlarr/
    ├── qbittorrent/
    ├── jellyfin/
    └── seerr/
```
- **L1** Every app that touches media mounts the same `/data`.
- **L2** The wizard asks for **one root folder**, not separate download and media folders.

### 10.5 Auto-wiring
- **A1** Pre-generate API keys and inject via environment variables where the app supports it (Servarr apps support `APP__SECTION__KEY` style overrides; verify per version). Fall back to reading `config.xml` from the config volume.
- **A2** Wait for each app's health endpoint before configuring it.
- **A3** Sonarr: add qBittorrent as download client, set root folder `/data/media/tv`, set naming and hardlink options.
- **A4** Radarr: same, with `/data/media/movies`.
- **A5** Prowlarr: register Sonarr and Radarr as applications so indexers sync automatically. Add no indexers.
- **A6** qBittorrent: set default save path to `/data/downloads`, categories (`tv`, `movies`), and a known admin credential (handle the random temporary password in current versions).
- **A7** Jellyfin: complete the startup wizard via API with an admin user; add libraries for `/data/media/tv` and `/data/media/movies`.
- **A8** Seerr: connect to Jellyfin, Sonarr, Radarr; set default quality profile and root folders.
- **A9** Enable authentication on the arr apps with one generated admin credential, shown once on the final screen and stored in `.env`.
- **A10** Every wiring step is idempotent and re-runnable (`arrstack rewire`).
- **A11** Failures in one step do not abort the others; report a summary at the end.

### 10.5b Optional extras
- **X1** gluetun VPN: provider picker, credentials form, qBittorrent routed through it, labelled "advanced".
- **X2** Tailscale: auth key or login link flow; gives Seerr/Jellyfin remote access with no port forwarding.
- **X3** Homepage dashboard: generated `services.yaml` with every service link and widgets using the generated API keys.

### 10.6 Helper CLI (`arrstack`)
- **C1** `doctor`: diagnose and explain common failures (see 11.1).
- **C2** `status`: container health and URLs.
- **C3** `update`: pull new images and recreate containers, with a pre-update config backup.
- **C4** `backup`: archive `config/` (not media) to a chosen path.
- **C5** `add <app>` / `remove <app>`: modify the stack and regenerate compose.
- **C6** `rewire`: re-run auto-wiring.
- **C7** `uninstall`: stop and remove containers, with explicit choice to keep or delete config and media.

---

## 11. Quality-of-life features

### 11.1 `doctor` (highest-value QoL)
Plain-English checks with a suggested fix for each:
- [ ] Docker daemon reachable; compose version OK
- [ ] All containers healthy; show last log lines for unhealthy ones
- [ ] Root folder exists and is writable by the container user
- [ ] **Hardlink test:** create a file in `downloads/`, hardlink into `media/`, verify inode match. If it fails, explain why and what it costs (double disk usage).
- [ ] **Path mismatch detection:** compare each app's configured root folders and download paths against actual mounts
- [ ] Disk space low warning
- [ ] Port conflicts
- [ ] Time/timezone mismatch between host and containers
- [ ] Windows only: folder is on a local drive, not a network share or USB; WSL2 memory limits sane
- [ ] Prowlarr has zero indexers (info, not an error, with a neutral pointer to docs)
- [ ] API connectivity between each wired pair of apps
- [ ] "Copy a redacted diagnostic report" for GitHub issues (strip keys and passwords)

### 11.2 Install experience
- [ ] Resumable Windows install through reboots
- [ ] Hardware-aware presets and warnings
- [ ] Auto-fallback ports
- [ ] Automatic PUID/PGID so permission errors never occur
- [ ] Plain-language narration of each step
- [ ] Live progress screen tied to real health checks, not fake timers

### 11.3 After install
- [ ] Generated home page with links, credentials location, and a "what to do next" checklist (add indexers in Prowlarr, add a Seerr user, install the Jellyfin app on the TV, etc.)
- [ ] Tailscale option for remote family access
- [ ] One-command updates with automatic pre-update backup
- [ ] One-command backup and restore
- [ ] No lock-in: compose and `.env` always on disk, documented

### 11.4 Presets
| Preset | Contents |
|---|---|
| Movies only | Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr |
| Family media server | Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr, Homepage |
| Full automation | Everything above plus Bazarr (post-v1), Tailscale, optional VPN |
| Lean (auto-suggested on low RAM) | Radarr or Sonarr, Prowlarr, qBittorrent, Jellyfin |

---

## 12. Technical design

### 12.1 Components
- **Bootstrap scripts** (`install.sh`, `install.ps1`): thin, readable, no logic that belongs in the wizard.
- **Wizard** (Go backend + embedded UI): single static binary in a small multi-arch image (amd64, arm64). UI via **HTMX + Go templates**, with server-sent events for the live progress screen, embedded with `go:embed`. No Node build step. Revisit Svelte only if the wizard grows well beyond about five screens.
- **Templates:** compose fragments per app, one file each, with metadata (ports, volumes, env, health check, RAM estimate).
- **Wiring engine:** native Go clients, one package per app, with an interface: `WaitHealthy()`, `Configure()`, `Verify()`. Only the small set of calls needed (download client, root folders, Prowlarr app links, Seerr connections). No Buildarr dependency.
- **Doctor:** shares the same API clients as wiring, so a check and a fix use the same code.
- **Helper wrapper:** runs the wizard image with a subcommand (`docker run --rm ... arrstack doctor`).

### 12.2 Why Docker socket access, and the risk
The wizard needs the Docker socket to create containers. That is root-equivalent on the host, so:
- [ ] Bind wizard to `127.0.0.1` only
- [ ] Random one-time token required in the URL
- [ ] Wizard container is `--rm` and exits when done
- [ ] Document this plainly in the README and the security section
- [ ] Never mount the socket into any long-running app container

### 12.3 Windows specifics
- [ ] Docker Desktop via winget; handle WSL2 enablement and the "Docker Desktop requires restart" case
- [ ] Folder picker via `System.Windows.Forms.FolderBrowserDialog`
- [ ] Forward-slash path normalization
- [ ] No PUID/PGID
- [ ] **Test hardlink behavior on Windows bind mounts.** Decision rule: if hardlinks pass on an NTFS bind mount, default to a simple path like `C:\Media` (visible in Explorer). If they fail, default to a WSL-native location and add an Explorer shortcut to it. Document the result either way.
- [ ] Warn against network shares and USB drives
- [ ] gluetun on WSL2 marked advanced

### 12.4 Linux specifics
- [ ] Install Docker Engine plus compose plugin on Debian/Ubuntu first, then Fedora and Arch as follow-ups
- [ ] Add user to `docker` group or use sudo, with a clear explanation
- [ ] PUID/PGID from `id -u` / `id -g`
- [ ] `zenity` folder picker if a desktop is present, else terminal prompt

### 12.5 Versioning and updates
- [ ] Images pinned to specific tags in generated compose
- [ ] Wizard image versioned with the repo; `arrstack update` checks for new wizard releases
- [ ] Wiring code tested against the pinned app versions in CI

---

## 13. Delivery plan

### Phase 0: Foundations
- [ ] Create repo, add MIT license, code of conduct, issue templates
- [ ] Pick final name from shortlist (Arrival, arrstart) and check availability (GitHub org/repo, Docker Hub namespace, domain)
- [ ] README skeleton with the legal framing (media management only, no content, no indexers)
- [x] Decide wizard UI stack: HTMX + Go templates + SSE (confirm with a half-day spike of the progress screen)
- [x] Spike: confirm Servarr env-var API key injection works for current Sonarr, Radarr, Prowlarr
- [x] Spike: confirm qBittorrent credential handling in current linuxserver image
- [x] Spike: Jellyfin startup wizard via API
- [x] Spike: Seerr setup via API, including non-root `user:` and config folder ownership at `/app/config`
- [x] Pin exact Seerr, Jellyfin, and arr image tags — recorded in `templates/versions.yaml` (WP0.9)
- [ ] Spike: hardlink behavior on Windows bind mounts
- [x] Decide wiring approach: native Go clients; Recyclarr/Configarr only as an optional post-v1 add-on for quality profiles
- [ ] Set up CI (lint, unit tests, image build)
- [ ] Set up VM-based test environments (Ubuntu, Windows 11)

**Gate 0:** spikes documented; no blocking unknowns for wiring or Windows paths.

### Phase 1: MVP on Linux (bootstrap, wizard, compose generation)
- [ ] `install.sh`: Docker detection and install
- [ ] Hardware detection and folder prompt
- [ ] Wizard container skeleton, token auth, localhost binding
- [ ] App selection UI with descriptions and Prowlarr suggestion
- [ ] Template system and compose generator
- [ ] Folder creation with single-root layout and correct ownership
- [ ] Deploy with live health-based progress screen
- [ ] Output `docker-compose.yml`, `.env`, `README.txt`
- [ ] Port conflict auto-fallback
- [ ] Wizard self-exit and helper wrapper install

**Gate 1:** on a clean Ubuntu VM, one line produces running, healthy, unconnected containers with correct folder layout.

### Phase 2: Windows
- [ ] `install.ps1`: Docker check and winget install
- [ ] Resumable flow across reboot/sign-out (scheduled task or RunOnce)
- [ ] Native folder picker and path normalization
- [ ] Windows-specific compose differences (no PUID/PGID, path handling)
- [ ] Local-drive validation and warnings
- [ ] Windows test pass on clean Windows 11 VM (with and without WSL2 pre-enabled)
- [ ] Windows troubleshooting page with screenshots/GIFs

**Gate 2:** a clean Windows 11 machine reaches the same state as Gate 1, including through a forced reboot.

### Phase 3: Auto-wiring
- [x] API client packages for Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr
- [x] Health waiting and retry logic
- [x] Generated API keys and credentials flow
- [ ] Wire qBittorrent to Sonarr and Radarr, set root folders, naming, hardlinks
- [ ] Wire Prowlarr to Sonarr and Radarr (no indexers added)
- [x] Jellyfin admin user and libraries
- [ ] Seerr connected to Jellyfin, Sonarr, Radarr
- [ ] Enable auth on arr apps with generated credentials
- [ ] Idempotent re-run (`rewire`)
- [ ] Wiring summary screen with per-step result

**Gate 3:** after install, a Seerr request reaches Radarr and the download client with zero manual settings changes (verified with a test item that does not require any real indexer, e.g. a mock indexer in CI).

### Phase 4: Quality of life
- [ ] `doctor` command with all checks in 11.1
- [ ] Redacted diagnostic report
- [ ] Presets and hardware-aware recommendations
- [ ] Homepage dashboard generation
- [ ] "What to do next" checklist screen
- [ ] `status`, `update`, `backup`, `add`, `remove`, `uninstall`
- [ ] Tailscale option
- [ ] gluetun VPN option (advanced, off by default): Mullvad, ProtonVPN, and custom WireGuard/OpenVPN config
- [ ] Optional: Recyclarr/Configarr one-shot container for quality profiles (post-v1 if time is short)

**Gate 4:** a user can break paths or permissions on purpose and `doctor` names the problem and the fix.

### Phase 5: Docs, testing, release
- [ ] README: hero GIF, one-liners per OS, FAQ
- [ ] Windows and Linux install guides
- [ ] "If you see this error, do this" page
- [ ] Architecture doc and contributor guide
- [ ] Security doc (socket access, exposure defaults, credentials)
- [ ] Full test matrix run (see section 14)
- [ ] Tag v0.1.0; publish multi-arch image
- [ ] Announce (blog post, r/selfhosted, r/homelab, relevant Discords) with the media-management framing

**Gate 5:** three people who have never used Docker complete the install unassisted.

### Phase 6: Post-launch
- [ ] macOS support (Docker Desktop or OrbStack)
- [ ] Bazarr and subtitle wiring
- [ ] Additional distros
- [ ] Community preset contributions
- [ ] Optional hosted "compose generator only" static page for people who want the file without running anything

---

## 14. Test matrix

| Environment | Docker present | Notes |
|---|---|---|
| Ubuntu 22.04/24.04 VM | No | Full install |
| Ubuntu | Yes | Skip-install path |
| Debian 12 | No | |
| Windows 11 | No, WSL2 off | Reboot path |
| Windows 11 | No, WSL2 on | |
| Windows 11 | Docker Desktop running | Skip-install path |
| Low-spec VM (2 CPU, 4 GB RAM) | Either | Lean preset |
| Port 8765 and 8989 pre-occupied | Either | Fallback ports |
| Non-ASCII/space in folder path (Windows) | Either | Path normalization |

Automated:
- [ ] Unit tests for generator and path normalization
- [ ] Integration test: compose up, wiring, verify via APIs
- [ ] CI smoke test of the wizard image on every PR
- [ ] Idempotency test: run the wizard twice, expect no change on the second run

---

## 15. Security and legal

- Default exposure: LAN only. Nothing forwarded to the internet.
- Generated admin credential, no default passwords. Stored in `.env` with restrictive permissions.
- Wizard is localhost-only with a one-time token and exits when finished.
- Repo and docs describe the software as media management. No indexers preconfigured, no links to content sources, no example indexer names in docs.
- Include a plain disclaimer: users are responsible for what they download and for complying with local law.
- No telemetry. If crash reporting is ever added, opt-in only.
- Pin and periodically review image sources; document which images are used.

---

## 16. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Windows Docker Desktop install and reboot flow breaks | High, where most beginners drop off | Resumable flow, heavy testing, clear narration, troubleshooting page |
| App APIs change between versions | Medium | Pin versions, CI tests wiring, abstract each client |
| qBittorrent auth changes break wiring | Medium | Spike early, fall back to guiding the user |
| Hardlinks fail on Windows bind mounts | Medium | Test in Phase 0, document, offer a WSL-native location, `doctor` detects |
| Docker socket exposure | Medium | Localhost, token, short-lived container, docs |
| Legal or takedown attention | Medium | Media-management framing, no indexers, no content links |
| Support load | High | `doctor`, redacted reports, FAQ, issue templates |
| Scope creep | Medium | Phase gates; Bazarr, macOS, Recyclarr are post-v1 |

---

## 17. Decisions and remaining open questions

### Decided

- [x] **Seerr image:** official `ghcr.io/seerr-team/seerr` (Overseerr and Jellyseerr are merged into it; the LinuxServer Overseerr image is deprecated). Set compose `user:` and own the config folder to match. Pin a version tag.
- [x] **Wizard UI stack:** HTMX + Go templates, SSE for progress. No Node build step.
- [x] **Wiring:** native Go clients for the narrow set of calls needed. Buildarr is Python and adds a runtime dependency; Recyclarr/Configarr are quality-profile tools, kept as an optional post-v1 add-on.
- [x] **Media server:** Jellyfin only for v1 (open source, no account, headless setup). Media server is a swappable slot so Plex/Emby can be added later.
- [x] **VPN providers:** no long hardcoded list. Start with Mullvad and ProtonVPN plus a custom WireGuard/OpenVPN config option. Off by default, labelled advanced. Add more via community PRs.
- [x] **License:** MIT. (Apache-2.0 only if an explicit patent grant becomes important. The tool only calls app APIs and generates compose files, so Sonarr/Radarr GPL licensing does not apply.)

### Still open

- [ ] **Final project name:** shortlist is "Arrival" (playful, hard to search) and "arrstart" (plain, easy to find). Avoid pirate jokes to keep the media-management framing. Check GitHub, Docker Hub, and domain before committing.
- [ ] **Windows default location:** resolved by the Phase 0 hardlink spike using the decision rule in 12.3.
- [ ] **Seerr details:** confirm in the Phase 0 spike that the `user:` and config ownership approach works cleanly on both Linux and Windows.
- [ ] **VPN starting set:** confirm gluetun WireGuard docs for Mullvad and ProtonVPN are good enough to ship without per-provider hacks.

---

## 18. Proposed repo layout

```
install.sh
install.ps1
go.mod               # module github.com/bruceowenga/arrstart
Makefile
CLAUDE.md
cmd/arrstart/main.go # subcommands: wizard (default), doctor, status,
                      # update, backup, add, remove, rewire, uninstall
internal/
  generator/         # + testdata/ golden files
  wiring/            # sonarr, radarr, prowlarr, qbittorrent, jellyfin, seerr
  doctor/            # checks + fix text together
  hardware/
  server/            # HTTP, token auth, SSE
web/                 # UI templates + static, with embed.go exposing embed.FS
templates/           # compose fragments per app + metadata, versions.yaml,
                      # with embed.go exposing embed.FS
presets/             # preset definitions, with embed.go exposing embed.FS
spikes/              # throwaway research code; excluded from lint and release builds
tests/               # VM scripts, integration tests, mock-indexer
docs/
  install-windows.md
  install-linux.md
  troubleshooting.md
  security.md
  architecture.md
  adr/
  spikes/
  build-log.md
.github/             # workflows, issue templates
```

---

## 19. Definition of done (v0.1.0)

- [ ] A beginner on a clean Windows 11 PC and a clean Ubuntu machine can run one line and end with Jellyfin, Seerr, Sonarr, Radarr, Prowlarr, and qBittorrent running and connected
- [ ] Folder layout supports hardlinks (or `doctor` clearly explains when it cannot)
- [ ] Plain compose and `.env` left on disk
- [ ] `doctor`, `update`, `backup`, `uninstall` work
- [ ] No indexers preconfigured; legal framing in README
- [ ] Docs cover install, troubleshooting, and security
- [ ] Test matrix passes
