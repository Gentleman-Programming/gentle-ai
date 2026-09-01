# Supported Platforms

← [Back to README](../README.md)

---

| Platform | Package Manager | Status |
|----------|----------------|--------|
| macOS (Apple Silicon + Intel) | Homebrew | Supported |
| Linux (Ubuntu/Debian) | apt | Supported |
| Linux (Arch) | pacman | Supported |
| Linux (Fedora/RHEL family) | dnf | Supported |
| Windows 10/11 | `go install` (Go toolchain) | Supported (binary distribution held) |

Derivatives are detected via `ID_LIKE` in `/etc/os-release` (Linux Mint, Pop!_OS, Manjaro, EndeavourOS, CentOS Stream, Rocky Linux, AlmaLinux, etc.).

Release archives are currently produced for macOS and Linux only. Windows source compatibility remains supported, but official Windows executable/archive assets and Scoop publication are temporarily unavailable pending the [Authenticode restoration gate](release-signing.md#windows-distribution-restoration-gate).

## OpenCode Managed Launcher

When OpenCode background subagents are enabled through `gentle-ai install` or `gentle-ai sync`, Gentle AI writes only its own launcher files under `~/.gentle-ai/bin/`. POSIX systems use `~/.gentle-ai/bin/opencode`; Windows uses `opencode.cmd` and `opencode.ps1`. The launcher sets `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true` only when the variable is not already defined, so an explicit `false` always selects foreground execution.

Deactivation removes managed launcher files but may leave `~/.gentle-ai/bin/` in `PATH`; Gentle AI does not clean up shell profiles.

Restart OpenCode after enabling managed activation. Restart the shell if the launcher directory has not entered PATH. OpenCode `serve`, `attach`, Desktop, or any session started outside the managed launcher uses foreground fallback rather than receiving an unsafe partial activation.

---

## Windows Notes

- **Install from source** with Go 1.25.10+:
  `go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest`.
- **`gentle-ai upgrade` updates itself automatically on release channels when Go 1.25.10+ is on `PATH`.** It runs `go install …/cmd/gentle-ai@vX.Y.Z` pinned to the exact release tag. The module is verified against the Go checksum database (`sum.golang.org`) — a different trust anchor than the minisign signature used for the Linux/macOS release binaries, not a missing one.
  Because `go install` writes to `GOBIN` (or `GOPATH\bin`), which is not necessarily the directory your shell resolves, the upgrade checks the destination afterwards and warns — naming both full paths — if a different `gentle-ai.exe` earlier on `PATH` would keep running.
   On the beta/development channel, `$env:GENTLE_AI_CHANNEL="beta"; gentle-ai upgrade` advances the binary from `main` and refreshes managed tools. If a manual source install sees stale `main` commits, run `GOPROXY=direct go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main` (PowerShell: `$env:GOPROXY="direct"; go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main`).
   Re-running either installer defaults to stable, so preserve beta explicitly: `curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash -s -- --channel beta` on macOS/Linux, or `$env:GENTLE_AI_CHANNEL="beta"; irm https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.ps1 | iex` in PowerShell.
- **Without Go on `PATH`, the upgrader fails closed.** It downloads and executes nothing, and prints the runnable `go install` command instead.
- **Scoop and official Windows binaries are still temporarily unavailable.** No unsigned artifact is ever downloaded and `gentle-ai upgrade` never executes a remote update script.
- **npm global installs** do not require `sudo` on Windows (user-writable by default).
- **curl** is pre-installed on Windows 10+ and does not require separate installation.
- **PowerShell** is the default shell when `$SHELL` is not set.
- **GGA on Windows** works from both Git Bash and PowerShell. gentle-ai installs a `gga.ps1` shim that automatically delegates to Git Bash, so no manual shell switching is required.
- **PowerShell source-installer output** is forced to UTF-8 and installs through Go's configured `GOBIN`/`GOPATH`.
- **Fresh install detection** falls back to known Engram/GGA install locations when the running process has a stale `PATH`.

---

## Windows Config Paths

| Agent | Windows Config Path |
|-------|-------------------|
| Claude Code | `%USERPROFILE%\.claude\` |
| OpenCode | `%USERPROFILE%\.config\opencode\` |
| Gemini CLI | `%USERPROFILE%\.gemini\` |
| Cursor | `%USERPROFILE%\.cursor\` |
| VS Code Copilot | `%APPDATA%\Code\User\` (settings, MCP, prompts) + `%USERPROFILE%\.copilot\` (skills) |
| Codex | `%USERPROFILE%\.codex\` |
| Windsurf | `%USERPROFILE%\.codeium\windsurf\` (skills, MCP, rules) + `%APPDATA%\Windsurf\User\` (settings) |
| Kimi | `%USERPROFILE%\.kimi\` (includes `config.toml`, system prompt, agents, MCP) |
| Antigravity | `%USERPROFILE%\.gemini\antigravity\` |
| Kiro IDE | `%USERPROFILE%\.kiro\steering\` (prompts) + `%USERPROFILE%\.kiro\skills\` (skills) + `%USERPROFILE%\.kiro\agents\` (SDD agents) + `%APPDATA%\kiro\User\settings.json` (settings) + `%USERPROFILE%\.kiro\settings\mcp.json` (MCP) |
| OpenClaw | `%USERPROFILE%\.openclaw\openclaw.json` (global MCP/settings) + active workspace from `agents.defaults.workspace` for `AGENTS.md` / `SOUL.md` / workspace-scoped SDD skills |
| Trae | `%USERPROFILE%\.trae\` (skills) + `%APPDATA%\Trae\User\user_rules.md` (rules) + `%APPDATA%\Trae\User\mcp.json` (MCP) |
| Pi | `%USERPROFILE%\.pi\` (Pi config, project agents/chains, Gentle AI support assets) |
| Hermes | `%USERPROFILE%\.hermes\` (config.yaml, SOUL.md, skills/) |
