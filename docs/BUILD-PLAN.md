# Build Plan: Arr Stack Installer

> Companion to `arr-stack-installer-PRD.md`. The PRD says **what** we build. This doc says **how** we build it: repo conventions, commit strategy, work packages, testing, releases, and how we run each working session.
> IDs like `B1`, `G4`, `A3`, `C1` refer to PRD requirements. Phases 0 to 6 match the PRD.

---

## 1. How to use this doc

- Every `- [ ]` is a work item. Tick it in the same PR that completes it.
- A **work package (WP)** is one PR-sized unit of work, named `WP<phase>.<n>`.
- Each phase ends with a **gate**. Do not start the next phase's feature work until the gate is green. Spikes and docs may run ahead.
- Size tags: **S** (under 2 hours), **M** (half a day), **L** (a day or more). If something feels bigger than L, split it.

---

## 2. Working agreement

1. **You own decisions and scope.** I propose, you approve. Anything that changes the PRD gets a PRD edit and a decision record (section 8) in the same PR.
2. **One WP at a time.** Finish, merge, then start the next. No parallel half-finished branches.
3. **Verify against reality, not memory.** App API behavior (Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr) is checked against the pinned versions running in real containers. Never trust recalled endpoints or field names without a test hitting the real app.
4. **Say what is unverified.** Every WP summary ends with: what was tested, what was not, and what could break.
5. **No indexers, ever.** Nothing in code, docs, tests, or fixtures names or preconfigures a real indexer or content source. Tests use a mock.
6. **No secrets in git.** Fixtures use obviously fake keys.

---

## 3. Repo conventions

### 3.1 Branching: trunk-based, short-lived branches

- `main` is always releasable and always green.
- Branch per WP, lives for hours or a couple of days, never weeks.
- Branch names: `<type>/<prd-id>-<short-desc>`

| Type | Use | Example |
|---|---|---|
| `feat` | New capability | `feat/g1-compose-fragments` |
| `fix` | Bug fix | `fix/b5-folder-validation` |
| `spike` | Throwaway research, findings documented | `spike/windows-hardlinks` |
| `docs` | Docs only | `docs/windows-troubleshooting` |
| `test` | Tests only | `test/generator-golden-files` |
| `chore` | Tooling, deps, CI | `chore/ci-skeleton` |

- `main` is protected: PR required (even solo, it gives you a CI gate and a history), CI must pass, no force-push.
- Delete branches after merge.

### 3.2 Commit messages: Conventional Commits

Format:

```
<type>(<scope>): <imperative summary, max 72 chars>

<why this change, not what; wrap at 72>

Refs: <PRD IDs>
```

**Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`, `build`, `perf`.

**Scopes:** `bootstrap`, `wizard`, `ui`, `generator`, `templates`, `wiring`, `doctor`, `cli`, `presets`, `docs`, `ci`, `deps`, plus an app name for app-specific wiring (`sonarr`, `radarr`, `prowlarr`, `qbittorrent`, `jellyfin`, `seerr`).

**Examples:**

```
feat(generator): merge per-app compose fragments into one file

Refs: G1, G2

fix(bootstrap): normalize Windows paths to forward slashes

Docker Desktop rejects backslash paths in bind mounts when the
path contains a drive letter.

Refs: G3

test(generator): add golden files for lean preset

Refs: G1, W3

docs(spike): record hardlink results on Windows bind mounts
```

**Rules:**
- One logical change per commit. Tests ship in the same commit as the code they cover.
- Do not mix refactors with features.
- Breaking change: add `!` after the scope (`feat(generator)!: ...`) and a `BREAKING CHANGE:` footer.
- Always add a `Refs:` footer with the PRD IDs the commit touches. This makes "which commit implemented A3?" a one-line `git log --grep`.
- Never commit generated output (`docker-compose.yml`, `.env`) from a real run. Golden test files live under `testdata/`.
- `WIP` or messy commits are fine on a branch because of squash merge (below), but keep them tidy if the branch will be rebase-merged.

### 3.3 Pull requests and merging

- **Default: squash merge.** The PR title is the Conventional Commit message, so `main` history is one clean commit per WP.
- **Rebase merge** only for PRs whose commits are already atomic and each worth keeping (for example a multi-step refactor).
- PR size guide: aim for under about 400 changed lines, excluding golden files and lockfiles. Bigger means split it.
- PR template (put in `.github/pull_request_template.md`):

```markdown
## What and why
<!-- one paragraph -->

## PRD / WP
Refs: <IDs>, WP<phase>.<n>

## How I verified
- [ ] `make check` passes
- [ ] Ran against real containers (if wiring/doctor)
- [ ] Tested on: Linux / Windows / N/A

## Not verified / risks
<!-- be honest -->

## Checklist
- [ ] PRD / BUILD-PLAN checkboxes ticked
- [ ] Docs updated if behavior changed
- [ ] Decision record added if a decision was made
- [ ] No secrets, no real indexer names
```

### 3.4 Versioning

- SemVer. Stay on `0.x` until Gate 5.
- **Alpha tags** for early testers: `v0.0.1-alpha.1` after Gate 1 (Linux), `v0.0.2-alpha.1` after Gate 2 (Windows), `v0.0.3-alpha.1` after Gate 3 (wiring). Get real users on these before polishing.
- `v0.1.0` at Gate 5.
- Automate changelog and version bumps from Conventional Commits (release-please or equivalent) once the repo has a few merged PRs.

---

## 4. Tooling and guardrails

### 4.1 Local: `Makefile` targets (define in WP0.1)

| Target | Does |
|---|---|
| `make check` | Lint plus unit tests. Must pass before any PR. |
| `make fmt` | `gofmt`, formatting for shell and PowerShell |
| `make build` | Build the wizard binary and image locally |
| `make e2e` | Headless end-to-end run against real containers (Linux) |
| `make docs-check` | Link check on docs |

### 4.2 Pre-commit hooks

- [ ] `gofmt` / `goimports`
- [ ] `shellcheck` on `install.sh` and scripts
- [ ] `PSScriptAnalyzer` on `install.ps1`
- [ ] `gitleaks` (or similar) to catch API keys and tokens
- [ ] Commit message lint (Conventional Commits)

### 4.3 CI (GitHub Actions)

| Job | Runs on | Purpose |
|---|---|---|
| lint | ubuntu | golangci-lint, shellcheck, PSScriptAnalyzer, markdown lint |
| unit | ubuntu | `go test ./...` with race detector |
| compose-validate | ubuntu | Generate compose for every preset, run `docker compose config` on each |
| build-image | ubuntu | Multi-arch build (amd64, arm64), push `edge` on main |
| e2e-linux | ubuntu | Run wizard headless, bring up the real stack, run wiring, verify via APIs |
| pester | windows | Unit-test PowerShell logic (path normalization, parsing). **Cannot run the full Docker flow**, see 5.3 |
| pr-title | ubuntu | Enforce Conventional Commit PR titles |

### 4.4 Required derived work (adds to the PRD)

These are not in the PRD yet and the plan depends on them:

- **Headless mode:** the wizard accepts `--answers answers.yaml` and runs with no browser. Needed for CI e2e, reproducible bug reports, and power users. (Proposed PRD requirement **W9**.)
- **ADR: how the wizard drives Docker.** Recommendation: shell out to `docker compose` (Docker CLI plus compose plugin inside the wizard image) rather than the Docker SDK. It runs the exact commands the user will run later, so "no lock-in" is literally true.
- **Mock Torznab indexer** in `tests/mock-indexer/` for the Gate 3 end-to-end test, so we never touch a real indexer.

---

## 5. Testing plan

### 5.1 Layers

| Layer | What | How |
|---|---|---|
| Unit | Path normalization, port picking, hardware tiers, preset resolution, secret generation | Table-driven Go tests |
| Golden | Generated `docker-compose.yml` and `.env` per preset and OS | Compare to files in `testdata/`; update with a `-update` flag, reviewed in PR |
| Client | Each app's API client | `httptest` servers using fixtures recorded from real pinned versions |
| Integration | Wiring orchestrator against real containers | CI e2e job, `make e2e` locally |
| Doctor | Each check against a deliberately broken setup | Scripted fault injection (wrong owner, separate mounts, stopped container) |
| Bootstrap | `install.sh`, `install.ps1` | Shell/Pester unit tests plus clean-VM runs |
| Manual | Real beginner run on clean machines | Test matrix in PRD section 14 |

### 5.2 Gate 3 end-to-end scenario (no real indexers)

1. Headless wizard brings up the stack.
2. Wiring completes; `rewire` run a second time changes nothing.
3. Mock Torznab indexer is added to Prowlarr by the test, not by the product.
4. Create a Seerr request for a test movie.
5. Assert Radarr receives it and **sends a grab to qBittorrent** (the test stops at the grab, not a real download).

### 5.3 Windows testing reality

- GitHub's Windows runners cannot run Linux containers through Docker Desktop, so the full Windows flow **cannot** be automated there.
- Plan: Pester and PSScriptAnalyzer in CI for script logic; full flow on a local Windows 11 VM with **snapshots** (clean, WSL2 enabled, Docker Desktop installed).
- Each Windows-touching PR records the VM run result in the PR description.
- Revisit a self-hosted Windows runner only if Windows regressions keep slipping through.

---

## 6. Execution plan: work packages by phase

### Phase 0: Foundations and spikes (Size: M total)

- [ ] **WP0.1 Repo scaffold** [S]: MIT license, code of conduct, `.gitignore`, `Makefile`, pre-commit config, issue and PR templates, `CLAUDE.md` (section 7.2), empty directory layout from the PRD.
- [ ] **WP0.2 CI skeleton** [S]: lint, unit, pr-title jobs. Green on an empty repo.
- [x] **WP0.3 Spike S1: Servarr API keys** [S]: confirm env-var key injection (`APP__SECTION__KEY` style) works for Sonarr, Radarr, Prowlarr; fall back to reading `config.xml`. Findings in `docs/spikes/01-servarr-api-keys.md`.
- [x] **WP0.4 Spike S2: qBittorrent credentials** [S]: how the current image handles the temporary random password; choose how we set a known credential.
- [x] **WP0.5 Spike S3: Jellyfin headless setup** [M]: startup wizard via API, admin user, libraries.
- [x] **WP0.6 Spike S4: Seerr setup** [M]: setup via API, non-root `user:`, `/app/config` ownership on Linux and Windows.
- [ ] **WP0.7 Spike S5: Windows hardlinks** [M]: run the hardlink test on NTFS bind mount vs WSL-native path; apply the PRD 12.3 decision rule.
- [x] **WP0.8 Spike S6: HTMX + SSE progress screen** [S]: prove the live progress screen pattern.
- [x] **WP0.9 Pin image tags** [S]: choose and record exact tags for every app image in `templates/versions.yaml`.
- [ ] **WP0.10 Decision records** [S]: write ADRs 0001 to 0008 (section 8).
- [ ] **WP0.11 Test VMs** [M]: scripted Ubuntu VM and Windows 11 VM with snapshots; document how to reset.
- [x] **WP0.12 Name** [S]: pick final name, claim GitHub org/repo, Docker Hub namespace, domain. Namespace/registry decided in ADR-0010 (GHCR only, ghcr.io/bruceowenga/arrstart, pinned tags never latest); registry-path Go constant added (internal/registry/registry.go). Dockerfile + `.github/workflows/publish.yml` added: builds the minimal bootstrap image and pushes it to `ghcr.io/${{ github.repository_owner }}/arrstart` using the workflow-scoped `GITHUB_TOKEN` with `packages: write` (no PAT), actions pinned by commit SHA, and the `org.opencontainers.image.source` OCI label. First public push v0.0.0-ci.1 done; anonymous pull verified (`docker pull ghcr.io/bruceowenga/arrstart:0.0.0-ci.1` after `docker logout ghcr.io`).

**Gate 0:** all spikes documented; PRD updated with results; no blocking unknowns for wiring or Windows paths.

### Phase 1: Linux MVP (Size: L total)

Build as **vertical slices**: get a thin end-to-end path working first, then widen.

- [ ] **WP1.1 Wizard skeleton** [M]: HTTP server, localhost bind, one-time token (W8), health endpoint, multi-arch Dockerfile. `W8`
- [ ] **WP1.2 Hardware detection** [S]: RAM, CPU, free disk; tier logic. `B6`, `W3`
- [ ] **WP1.3 Template format** [M]: per-app fragment schema (ports, volumes, env, healthcheck, RAM estimate, slot type for media server) plus Sonarr, Radarr, Prowlarr, qBittorrent, Jellyfin, Seerr fragments. `G1`, `G5`, `G8`
- [ ] **WP1.4 Generator** [L]: merge fragments, ports, PUID/PGID/`user:`, TZ, idempotent diff, golden tests for every preset. `G1` to `G8`
- [ ] **WP1.5 Folder layout** [S]: create single-root layout with correct ownership. `L1`, `L2`
- [ ] **WP1.6 `install.sh`** [L]: Docker detection and install (Debian/Ubuntu), folder prompt, launch wizard, open browser. `B1` to `B3`, `B5`, `B7`, `B8`
- [ ] **WP1.7 Wizard UI: select and review** [M]: preset screen, app selection with descriptions, review screen. `W1` to `W4`
- [ ] **WP1.8 Deploy and progress** [L]: drive `docker compose`, health-based progress over SSE, plain-English errors. `W5`, `W6`
- [ ] **WP1.9 Headless mode** [M]: `--answers answers.yaml`; wire into `make e2e` and CI. (`W9`)
- [ ] **WP1.10 Finish and helper wrapper** [M]: final screen stub, wizard self-exit, install `arrstack.sh`. `B9`, `W7`

**Gate 1:** on a clean Ubuntu VM, one line produces running, healthy, unconnected containers with the correct folder layout. Tag `v0.0.1-alpha.1`.

### Phase 2: Windows (Size: L total)

- [ ] **WP2.1 `install.ps1` Docker check and install** [M]: `docker info`, `winget install`. `B2`, `B3`
- [ ] **WP2.2 Resumable flow** [L]: scheduled task or RunOnce, reboot and sign-out handling, clear narration. `B4`
- [ ] **WP2.3 Folder picker and validation** [M]: native picker, local-drive check, warnings. `B5`
- [ ] **WP2.4 Windows generator differences** [S]: path normalization, no PUID/PGID, tested in golden files. `G3`, `G4`
- [ ] **WP2.5 Pester tests** [S]: path logic, parsing, resume state.
- [ ] **WP2.6 VM matrix pass** [M]: WSL2 off, WSL2 on, Docker Desktop already running, spaces and non-ASCII in path.
- [ ] **WP2.7 Windows docs** [M]: install guide and troubleshooting page with screenshots.

**Gate 2:** clean Windows 11 reaches the Gate 1 state, including through a forced reboot. Tag `v0.0.2-alpha.1` and hand to one or two Windows testers.

### Phase 3: Auto-wiring (Size: L total)

Order matters: wire the stack from the bottom up.

- [x] **WP3.1 Wiring framework** [M]: `WaitHealthy/Configure/Verify` interface, retries with backoff, per-step result reporting, failure isolation. `A2`, `A10`, `A11`
- [ ] **WP3.2 Credentials and keys** [M]: pre-generated API keys, one admin credential, `.env` storage, permissions. `A1`, `A9`
- [x] **WP3.3 qBittorrent client** [M]: save path, categories, known credential. `A6`
- [x] **WP3.4 Sonarr and Radarr clients** [L]: download client, root folders, naming, hardlinks. `A3`, `A4`
- [x] **WP3.5 Prowlarr client** [M]: app links to Sonarr and Radarr, no indexers added. `A5`
- [x] **WP3.6 Jellyfin client** [L]: admin user, libraries. `A7`
- [x] **WP3.7 Seerr client** [L]: connect Jellyfin, Sonarr, Radarr; defaults. `A8`
- [ ] **WP3.8 Mock indexer and Gate 3 e2e test** [L]: section 5.2.
- [ ] **WP3.9 `rewire` command and idempotency tests** [M]: `A10`, `C6`
- [ ] **WP3.10 Wiring summary screen** [S]: per-step results in the wizard.

**Gate 3:** the section 5.2 scenario passes in CI with zero manual settings changes. Tag `v0.0.3-alpha.1`.

### Phase 4: Quality of life (Size: L total)

- [ ] **WP4.1 Doctor framework** [M]: check interface, plain-English output, suggested fix per check. `C1`
- [ ] **WP4.2 Doctor checks batch A** [L]: Docker, container health, folder ownership, hardlink test, path mismatch.
- [ ] **WP4.3 Doctor checks batch B** [M]: disk space, ports, timezone, Windows drive checks, app connectivity, Prowlarr empty-indexer info.
- [ ] **WP4.4 Redacted diagnostic report** [S]: strip keys and passwords; copy button.
- [ ] **WP4.5 Fault-injection tests for doctor** [M]: break things on purpose, assert the diagnosis.
- [ ] **WP4.6 Presets and hardware recommendations** [S]: Movies only, Family, Full, Lean.
- [ ] **WP4.7 Homepage dashboard** [M]: generated config with links and widgets. `X3`
- [ ] **WP4.8 "What to do next" checklist screen** [S]
- [ ] **WP4.9 CLI: status, update, backup, add, remove, uninstall** [L]: `C2` to `C5`, `C7`; update takes a pre-update backup.
- [ ] **WP4.10 Tailscale option** [M]: `X2`
- [ ] **WP4.11 gluetun option** [M]: `X1`, advanced, off by default.
- [ ] **WP4.12 Optional: Recyclarr/Configarr one-shot** [M]: post-v1 if time is short.

**Gate 4:** break paths or permissions on purpose; `doctor` names the problem and the fix.

### Phase 5: Docs, hardening, release (Size: M total)

- [ ] **WP5.1 README** [M]: hero GIF, one-liners per OS, FAQ, legal framing.
- [ ] **WP5.2 Docs set** [M]: Linux install, Windows install, troubleshooting, security, architecture, contributor guide.
- [ ] **WP5.3 Release hardening** [M]: see section 9 (pinned one-liner, checksums).
- [ ] **WP5.4 Full matrix run** [L]: PRD section 14, results logged in `docs/test-results/`.
- [ ] **WP5.5 Beginner trial** [M]: three people who have never used Docker, unassisted; fix what they hit.
- [ ] **WP5.6 Release** [S]: tag `v0.1.0`, publish multi-arch image, announce with the media-management framing.

**Gate 5:** the three-beginner trial succeeds unassisted.

### Phase 6: Post-launch (backlog, unordered)

- [ ] macOS support
- [ ] Bazarr and subtitle wiring
- [ ] More Linux distros
- [ ] Plex/Emby templates via the media-server slot
- [ ] Community presets
- [ ] Static "compose generator only" page

---

## 7. Working with Claude

### 7.1 Session protocol

Each build session is **one WP**.

1. **Start:** attach or paste `PRD`, this plan, and the latest entry of `docs/build-log.md`. State the WP ID.
2. **Plan:** I reply with the approach in about ten lines: files touched, tests, risks. You approve or adjust.
3. **Build:** small commits following section 3.2. Run `make check` before calling anything done.
4. **Report:** every session ends with four parts: what changed, how it was verified, what was **not** verified, and suggested next WP.
5. **Close:** tick checkboxes in the PRD and this plan, add a build-log entry, open the PR.

### 7.2 `CLAUDE.md` (repo root, created in WP0.1)

Put these rules in it so any Claude session, including Claude Code, follows them without being told:

```markdown
# Project rules
- Read docs/PRD.md and docs/BUILD-PLAN.md before starting. Work on one WP at a time.
- Commits: Conventional Commits with a `Refs:` footer listing PRD IDs.
- Run `make check` before declaring work done. Run `make e2e` for wiring/doctor changes.
- Verify app API behavior against the pinned images in real containers. Do not rely on memory.
- Never add, name, or preconfigure real indexers or content sources. Use the mock indexer.
- Never commit secrets, real `.env` files, or generated compose files from real runs.
- Pin image tags. Do not use `latest` in generated output.
- Generated compose and `.env` must stay plain and hand-editable (no lock-in).
- Finish every task with: changed, verified, NOT verified, next step.
- Update PRD/BUILD-PLAN checkboxes and docs/build-log.md in the same PR.
```

### 7.3 Build log

`docs/build-log.md`, newest first, three lines per session:

```markdown
## 2026-11-24 WP0.3 Spike S1: Servarr API keys
Result: env-var injection works for Sonarr/Radarr; Prowlarr needs config.xml fallback.
Open: confirm behavior on arm64. Next: WP0.4.
```

This is the handoff between sessions. It keeps context small and stops us re-deciding things.

### 7.4 Prompt template for starting a WP

```
Work package: WP<id> <name>
PRD refs: <IDs>
Context: attached PRD, BUILD-PLAN, last build-log entry.
Constraints: <anything specific, e.g. Linux only, no new deps>
Done when: <acceptance criteria from the PRD / gate>
```

---

## 8. Decision records (ADRs)

Short files in `docs/adr/`, one per decision: context, decision, consequences. Write them in WP0.10 and add one whenever a decision changes.

- [x] ADR-0001: Docker-based, one-liner bootstrap, temporary wizard container
- [x] ADR-0002: Go backend with HTMX + Go templates + SSE
- [x] ADR-0003: Native Go wiring clients (no Buildarr); Recyclarr/Configarr optional later
- [x] ADR-0004: Single-root `/data` layout, one folder asked
- [x] ADR-0005: Jellyfin only for v1; media server is a swappable slot
- [x] ADR-0006: Seerr official image, `user:` and config ownership approach
- [x] ADR-0007: Wizard drives `docker compose` by shelling out (vs Docker SDK)
- [ ] ADR-0008: Windows default location (result of spike S5) — blocked until WP0.7 runs on the WP0.11 Windows VM
- [ ] ADR-0009: MIT license; no indexers, media-management framing

---

## 9. Release and supply-chain plan

- **The one-liner must not pull from `main`.** `curl ... | bash` from a moving branch is a risk. Point it at a **versioned release asset** and publish a SHA256 checksum next to it. Show the checksum in the README for people who want to verify.
- Pin the wizard image tag in the script to the same release version.
- Build images in CI from tagged commits; keep `edge` for main builds only, never in the README one-liner.
- Publish release notes generated from Conventional Commits.
- Optional hardening later: sign images with cosign, and publish an SBOM.
- Keep a short `SECURITY.md` with a private reporting path.

---

## 10. Tracking

- **GitHub Milestones:** one per phase (`Phase 0` to `Phase 6`).
- **Issues:** one per WP, titled `WP<phase>.<n> <name>`, body links the PRD IDs and acceptance criteria.
- **Labels:** `phase:0` to `phase:6`, `type:*` (same as branch types), `os:windows`, `os:linux`, `needs-spike`, `blocked`, `good first issue` (for post-launch contributors).
- **Project board:** columns `Backlog`, `Next`, `In progress` (limit 1), `In review`, `Done`.
- Keep the PRD and this plan as the source of truth; issues mirror them. If they disagree, fix the docs.

---

## 11. First week

Concrete first moves, in order:

1. [ ] Create the repo and add the PRD and this plan under `docs/`.
2. [ ] WP0.1 scaffold with `CLAUDE.md`, Makefile, hooks, templates.
3. [ ] WP0.2 CI skeleton, green on an empty repo.
4. [ ] WP0.3 Spike S1 (Servarr API keys).
5. [ ] WP0.4 Spike S2 (qBittorrent credentials).
6. [ ] WP0.5 Spike S3 (Jellyfin headless setup).
7. [ ] WP0.6 Spike S4 (Seerr setup and ownership).
8. [ ] WP0.7 Spike S5 (Windows hardlinks), needs the Windows VM from WP0.11.
9. [ ] Update the PRD with spike results; write ADRs.
10. [ ] Gate 0 review: go or adjust.

---

## 12. Risks to the process

| Risk | Mitigation |
|---|---|
| Scope creep mid-phase | Gates; new ideas go to Phase 6 backlog unless they block a gate |
| Long-lived branches drift | Max about two days per branch; split the WP instead |
| Wiring built on guessed APIs | Spikes first; client fixtures recorded from real pinned versions |
| Windows regressions unnoticed | VM run recorded in every Windows-touching PR; alpha testers at Gate 2 |
| Docs rot | Docs updated in the same PR as behavior changes; `docs-check` in CI |
| Context lost between sessions | `build-log.md` handoff plus `CLAUDE.md` rules |
| Burnout on a big build | Alpha tags at each gate give early feedback and visible progress |
