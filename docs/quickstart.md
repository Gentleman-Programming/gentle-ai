# Quickstart

## Prerequisites

### macOS

- Homebrew installed and available in PATH.
- `git` available.
- If Homebrew requires trust, run `brew trust --formula gentleman-programming/tap/gentle-ai` once for Gentle AI™ only.
  - To install several tools from this tap, use `brew trust gentleman-programming/tap` instead. It trusts all current and future formulas, casks, and external commands published in the tap.

### Ubuntu/Debian (and derivatives like Linux Mint, Pop!\_OS)

- `apt-get` available (standard on these distros).
- `sudo` access for package installs.
- `git` available.
- If Node.js is missing, `gentle-ai install` prints this install hint: NodeSource LTS setup + `apt-get install -y nodejs` (npm comes bundled).
- If using Homebrew on Linux, Bubblewrap may require unprivileged user namespaces; see `docs/usage.md#homebrew-upgrade-troubleshooting`.

### Arch Linux (and derivatives like Manjaro, EndeavourOS)

- `pacman` available (standard on these distros).
- `sudo` access for package installs.
- `git` available.
- If Node.js is missing, `gentle-ai install` prints this install hint: `pacman -S --noconfirm nodejs npm`.

### Fedora / RHEL family (Fedora, CentOS Stream, Rocky Linux, AlmaLinux)

- `dnf` available (standard on these distros).
- `sudo` access for package installs.
- `git` available.
- If Node.js is missing, `gentle-ai install` prints this install hint: NodeSource LTS setup + `dnf install -y nodejs` (npm comes bundled).

### All platforms

- Git 2.38+.
- Go 1.25.10+ (for building from source).
- Node.js 18+ and npm: `gentle-ai install` checks these as required prerequisites on every platform and prints a warning with a distro-specific install hint (see above) if either is missing — regardless of which agents/components you select. It does not install them for you, and it does not install agent runtimes either: if a selected agent isn't detected, `gentle-ai install` refuses and prints the exact `npm install -g` (or equivalent) command for you to run yourself. Node.js/npm are strictly required if you select the CodeGraph community tool, which gentle-ai does install via `npm install -g`.
- Pi installed and available as `pi` on `PATH` if you select the Pi agent.

### Windows

- Go 1.25.10+, because Windows installs and upgrades through `go install`.
  Official Windows binaries and the Scoop bucket are temporarily unavailable
  while publicly trusted Authenticode signing is provisioned, so nothing
  unsigned is ever fetched. With Go on `PATH`, `gentle-ai upgrade` updates
  itself automatically by running `go install …/cmd/gentle-ai@vX.Y.Z` pinned to
  the release tag and verified against the Go checksum database; without Go it
  fails closed and just prints that command. See [platforms.md](platforms.md)
  and the
  [restoration gate](release-signing.md#windows-distribution-restoration-gate).

```powershell
# Stable channel (`@latest`, currently v2.3.0)
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest

# Opt-in prerelease (v2.4.0-rc.1)
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@v2.4.0-rc.1
```

Both commands use the `/v2` module path. Go requires that suffix for major
version 2 and above.

## Version Policy

Receipt-Driven Development (RDD) began in `v1.47.0` on 2026-07-10, and `v2.2.0` made it the supported stable path. Those are historical milestones. The negotiated public review contract was published in `v2.1.6`.

The current stable release is [`v2.3.0`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.3.0). `@latest` explicitly tracks this stable channel. The current opt-in prerelease is [`v2.4.0-rc.1`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.4.0-rc.1). `@main` installs unreleased development changes.

### Install the stable channel

```bash
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest
gentle-ai version
```

### Install the opt-in prerelease

```bash
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@v2.4.0-rc.1
gentle-ai version
```

### Install unreleased development changes

Only use `main` when testing changes that are not part of a release yet:

```bash
# macOS / Linux
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main
gentle-ai version

# Windows (PowerShell)
$env:GENTLE_AI_CHANNEL="beta"; go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main
gentle-ai version
```

To update a beta installation later, preserve the beta channel:

```bash
# macOS / Linux
GENTLE_AI_CHANNEL=beta gentle-ai upgrade

# Windows (PowerShell)
$env:GENTLE_AI_CHANNEL="beta"; gentle-ai upgrade
```

`gentle-ai upgrade` advances the `gentle-ai` binary from `main` and refreshes managed tools on macOS, Linux, and Windows with Go on `PATH`.

If you re-run an installer, pass beta explicitly because both installers default to stable:

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash -s -- --channel beta

# Windows (PowerShell)
$env:GENTLE_AI_CHANNEL="beta"; irm https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.ps1 | iex
```

> **Go module proxy cache**: `proxy.golang.org` can lag behind new commits on `main` for up to several hours. If manual `go install ...@main` does not update to the newest commit, bypass the cache with `GOPROXY=direct go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main` (PowerShell: `$env:GOPROXY="direct"; go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main`).

The managed install scripts select the latest version for their chosen channel and do not accept arbitrary release pins. Use `go install` with an exact tag when you need a reproducible prerelease or stable version.

## Run

```bash
go run ./cmd/gentle-ai install --dry-run
```

Use `--dry-run` first to validate selections and execution plan without applying changes. The dry-run output includes a `Platform decision` line showing the detected OS, distro, package manager, and support status.

## First real install

```bash
go run ./cmd/gentle-ai install
```

The installer detects your platform automatically — no flags needed to select macOS vs Linux. Install commands are resolved through the appropriate package manager (brew, apt, pacman, or dnf) based on detection.

After completion, verify that agent configs and selected components were installed to their expected paths.

The agents you select during install become the default scope for future `gentle-ai sync` runs. Gentle AI records that selection in `~/.gentle-ai/state.json` and does not automatically sync every agent config directory that exists on your machine. To check what will be updated after an upgrade, run:

```bash
gentle-ai sync --dry-run
```

To update a different set explicitly, pass every target agent:

```bash
gentle-ai sync --agent claude-code --agent opencode
```

## Verification outcome

When checks pass, installer reports:

`You're ready. Run 'claude' or 'opencode' and start building.`

If something looks wrong after install, run `gentle-ai doctor` for a read-only health check. It verifies tool binaries, `state.json` validity, Engram™ MCP reachability, and disk space — each check reports pass/warn/fail with a remedy hint.

For a Pi-only install, the plan shows the Pi package stack instead of Gentle AI components. It installs `gentle-pi`, `gentle-engram`, and `pi-mcp-adapter`, runs `pi-engram init` through the pinned `gentle-engram` package, then installs `pi-subagents-j0k3r`, `@juicesharp/rpiv-ask-user-question`, `pi-web-access`, `@juicesharp/rpiv-todo`, and `pi-btw`.

## Hardening recommendations for users

Gentle AI pins versions and disables postinstall scripts on every npm install it generates. When you install the `permissions` component, a sensitive-paths deny list is applied to Claude Code and OpenCode blocking access to `~/.ssh/*`, `**/*.pem`, `**/*.key`, `**/.env*`, `~/.aws/credentials`, and other credential paths. See [Components](../docs/components.md) for the full list.

For broader protection across npm packages you install yourself, set these once on your machine:

- `npm config set ignore-scripts true` — blocks postinstall scripts globally; the primary supply-chain attack vector.
- `npm config set min-release-age 3` — skip packages published in the last 3 days; catches malicious typosquats before you install them.
- `npm config set allow-git none` — block git: dependencies, which can be moving targets.

Optional wrapper tools for extra defense:

- [`npq`](https://github.com/lirantal/npq) — audits a package against several heuristics before it installs.
- [`sfw`](https://socket.dev/) (Socket Firewall) — runtime guard that intercepts suspicious behavior at install/run time.

## Unsupported platforms

If you run the installer on an unsupported OS or Linux distro, it exits immediately with an error:

- `unsupported operating system: only macOS, Linux, and Windows are supported (detected <os>)`
- `unsupported linux distro: Linux support is limited to Ubuntu/Debian, Arch, and Fedora/RHEL family (detected <distro>)`
