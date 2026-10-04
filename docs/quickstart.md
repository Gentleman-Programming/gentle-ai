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

- Git.
- Go 1.25.10+ (for building from source).
- Node.js 18+ and npm: `gentle-ai install` checks these as required prerequisites on every platform and prints a warning with an install hint if either is missing (the Node.js hints for the distros above are listed there; npm's hint asks you to install Node.js first) — regardless of which agents/components you select. It does not install them for you, and it does not install agent runtimes either: selected agents other than Pi are configured even when their runtime isn't detected, so install those yourself. For Pi, `gentle-ai install` refuses to install the Pi packages until `pi` is on `PATH`. Node.js/npm are strictly required if you select the CodeGraph community tool, which gentle-ai does install via `npm install -g`.
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
# Stable channel: the latest release (v4.0.0)
go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0
```

Go requires a major-version suffix (`/v4`) in the module path for major
version 2 and above. Installing the old `/v3` module path stays on the v3 line
and never reaches v4.

## Version Policy

The latest published stable release is [`v4.0.0`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v4.0.0). `@latest` on the `/v4` module path tracks the stable channel. Use `@main` only to test unreleased development changes.

### Install the stable channel

```bash
go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@latest
gentle-ai version
```

### Install unreleased development changes

Only use `main` when testing changes that are not part of a release yet:

```bash
# macOS / Linux
go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@main
gentle-ai version

# Windows (PowerShell)
go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@main
gentle-ai version
```

`go install` ignores `GENTLE_AI_CHANNEL`; `gentle-ai` reads it on later runs, where `beta` tracks `main` instead of the latest release. Tools that Homebrew manages keep updating through Homebrew. To update a beta installation later, preserve the beta channel:

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

> **Go module proxy cache**: `proxy.golang.org` can lag behind new commits on `main` for up to several hours. If manual `go install ...@main` does not update to the newest commit, bypass the cache with `GOPROXY=direct go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@main` (PowerShell: `$env:GOPROXY="direct"; go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@main`).

The managed install scripts select the latest version for their chosen channel and do not accept arbitrary release pins. Use `go install` with an exact tag when you need a reproducible prerelease or stable version.

## Run

```bash
gentle-ai install --dry-run
```

Use `--dry-run` first to validate selections and execution plan without applying changes. The dry-run output includes a `Platform decision` line showing the detected OS, distro, package manager, and support status.

From a source clone, run `go run ./cmd/gentle-ai install` (with or without `--dry-run`) instead.

## First real install

```bash
gentle-ai install
```

The installer detects your platform automatically — no flags needed to select macOS vs Linux. On Linux, it uses the first supported package manager it finds on `PATH`; see [Supported Platforms](platforms.md) for the full list and detection order.

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

When checks pass, the installer prints a ready message that names the agent commands you installed, for example:

``You're ready. Run `claude` or `opencode` and start building.``

If something looks wrong after install, run `gentle-ai doctor` for a read-only health check. It verifies tool binaries, `state.json` validity, Engram™ MCP reachability, and disk space — each check reports pass/warn/fail with a remedy hint.

For a Pi-only install, the plan shows the Pi package stack instead of Gentle AI components. It installs `gentle-pi` and `gentle-engram`, runs `pi-engram init` through the pinned `gentle-engram` package, then installs `pi-web-access` and `pi-btw`. Pi's built-in MCP support (Pi >= 0.99.0) runs the Engram and CodeGraph MCP servers from `mcp.json`. Gentle AI removes a previously installed `pi-mcp-adapter`, because an extension that registers `/mcp` replaces Pi's built-in MCP support.

## Start working with ODD

Open your agent in the project and describe an outcome, for example: "Add CSV export using the existing report filters." [Organic Driven Development (ODD)](usage.md#organic-driven-development-odd) is the development workflow: explore, implement authorized work, and check it. Substantial work keeps one recoverable feature document; small/read-only work avoids durable artifacts.

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
- `unsupported linux distro: no package manager found on PATH (detected distro <distro>).` The full error lists the package managers it searched, in order, and a command to check which ones your machine has.
