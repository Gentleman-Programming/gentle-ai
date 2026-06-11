# Supported Platforms

← [Back to README](../README.md)

---

| Platform | Package Manager | Status |
|----------|----------------|--------|
| macOS (Apple Silicon + Intel) | Homebrew | Supported |
| Linux (Ubuntu/Debian) | apt | Supported |
| Linux (Arch) | pacman | Supported |
| Linux (Fedora/RHEL family) | dnf | Supported |
| Windows 10/11 | Scoop | Supported |

Derivatives are detected via `ID_LIKE` in `/etc/os-release` (Linux Mint, Pop!_OS, Manjaro, EndeavourOS, CentOS Stream, Rocky Linux, AlmaLinux, etc.).

Release artifacts are produced by CI, but Windows users should install through Scoop so upgrades stay consistent.

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
| Hermes | `%USERPROFILE%\.hermes\` (config.yaml, SOUL.md, skills/) |
