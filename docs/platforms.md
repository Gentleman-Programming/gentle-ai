# Supported Platforms

← [Back to README](../README.md)

---

| Platform | Package Manager | Status |
|----------|----------------|--------|
| macOS (Apple Silicon + Intel) | Homebrew | Supported |
| Linux (Ubuntu/Debian) | apt | Supported |
| Linux (Arch) | pacman | Supported |
| Linux (Fedora/RHEL family) | dnf | Supported |
| Android (Termux) | apt | Supported |
| Windows 10/11 | Scoop | Supported |

Derivatives are detected via `ID_LIKE` in `/etc/os-release` (Linux Mint, Pop!_OS, Manjaro, EndeavourOS, CentOS Stream, Rocky Linux, AlmaLinux, etc.). Termux is detected as Android/`GOOS=android` in the Go application; the shell installer also recognizes the Termux environment.

Release binaries are built for `linux`, `darwin`, and `windows` on both `amd64` and `arm64`. Android (Termux) is supported via source compilation (`go install`) since pre-built glibc binaries are incompatible with Android's Bionic libc.

Windows release artifacts are produced by CI, but Windows users should install through Scoop so upgrades stay consistent.

---

## Termux (Android) Notes

- **apt** is used as the default package manager within Termux.
- **Prefix Awareness**: gentle-ai automatically detects the Termux `$PREFIX` and adjusts system paths accordingly (e.g., using `$PREFIX/bin/bash` instead of `/bin/bash`).
- **PATH Persistence**: When installing tools, gentle-ai will automatically append the appropriate `export PATH` commands to your `~/.bashrc` or `~/.zshrc`.
- **PIE Requirement**: All binaries updated via `gentle-ai self-update` on Termux are automatically compiled as Position Independent Executables (PIE), as required by Android.
- **Sub-agents**: Sub-agents like GGA are installed into `$PREFIX/tmp` during the setup process to ensure execution permissions.

---

## Windows Notes

- **Scoop** is the supported Windows install path for Gentle AI.
- **npm global installs** do not require `sudo` on Windows (user-writable by default).
- **curl** is pre-installed on Windows 10+ and does not require separate installation.
- **PowerShell** is the default shell when `$SHELL` is not set.
- **GGA on Windows** works from both Git Bash and PowerShell. gentle-ai installs a `gga.ps1` shim that automatically delegates to Git Bash, so no manual shell switching is required.
- **PowerShell installer output** is forced to UTF-8 to avoid garbled icons, and the installer persists the install directory to the user `PATH` while updating the current session for verification.
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
