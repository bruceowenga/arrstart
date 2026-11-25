# ADR-0001: Docker-based, one-liner bootstrap, temporary wizard container

## Context

The target user has never used Docker and comes from "nothing installed" (PRD §1). The tool must install Docker if missing, start the stack, and not leave a long-running management process on the user's machine. The wizard needs the Docker socket to create containers, which is root-equivalent on the host (PRD §12.2).

## Decision

- Users install via a one-liner: `install.sh` on Linux (Debian/Ubuntu via the official Docker repo first), `install.ps1` on Windows (winget `Docker.DockerDesktop`).
- Bootstrap handles reboots/sign-outs with a resume mechanism (scheduled task or RunOnce on Windows; systemd unit or flag file on Linux) — PRD B4.
- The wizard runs as a `--rm` container on localhost only (default port 8765, auto-fallback if busy) with a random one-time token in the URL (PRD B7/B8/W8).
- The wizard container exits when the wizard finishes; the `arrstart` helper wrapper remains for `doctor`, `status`, `update`, `backup`, `add`, `remove`, `rewire`, `uninstall` (PRD C1–C7).
- The Docker socket is mounted into the wizard container only, never into any long-running app container (PRD §12.2).

## Consequences

- Simple mental model: one action (paste the one-liner), one short-lived container manages setup.
- The wizard briefly holds root-equivalent access via the socket; mitigated by localhost-only binding, one-time token, `--rm`, and plain documentation (PRD §15).
- After the wizard exits, no arrstart process remains on the host — all ongoing operations go through `arrstart` subcommands that run short-lived containers as needed.
- Resume logic adds state files and branching complexity to bootstrap scripts, but is required for the Windows reboot story.
