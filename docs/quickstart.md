# Quickstart

## Prerequisites

### macOS

- Homebrew installed and available in PATH.
- `git` available.

### Ubuntu/Debian (and derivatives like Linux Mint, Pop!_OS)

- `apt-get` available (standard on these distros).
- `sudo` access for package installs.
- `git` available.

### Arch Linux (and derivatives like Manjaro, EndeavourOS)

- `pacman` available (standard on these distros).
- `sudo` access for package installs.
- `git` available.

### Fedora / RHEL family (Fedora, CentOS Stream, Rocky Linux, AlmaLinux)

- `dnf` available (standard on these distros).
- `sudo` access for package installs.
- `git` available.
- Node.js installs use NodeSource LTS setup + `dnf install -y nodejs` during dependency remediation.

### All platforms

- Go 1.24+ (for building from source).
- Node.js / npm if installing Claude Code (agent is installed via `npm install -g`).
- Pi installed and available as `pi` on `PATH` if you select the Pi agent.

### Windows

- Scoop installed. Gentle AI recommends Scoop as the Windows install path.

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

## Verification outcome

When checks pass, installer reports:

`You're ready. Run 'claude' or 'opencode' and start building.`

For a Pi-only install, the plan shows the Pi package stack instead of Gentle AI components. It installs `gentle-pi`, `gentle-engram`, `pi-subagents`, `pi-intercom`, `@juicesharp/rpiv-ask-user-question`, `pi-web-access`, `pi-lens`, `@juicesharp/rpiv-todo`, and `pi-btw` through `pi install npm:*` commands.

## Hardening recommendations for users

Gentle AI pins versions and disables postinstall scripts on every npm install it generates. For broader protection across npm packages you install yourself, set these once on your machine:

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
