# Supported Platforms

← [Back to README](../README.md)

---

| Platform | Package Manager | Status |
|----------|----------------|--------|
| macOS (Apple Silicon + Intel) | Homebrew | Supported |
| Linux (Ubuntu/Debian) | apt | Supported |
| Linux (Arch) | pacman | Supported |
| Linux (Fedora/RHEL family) | dnf | Supported |
| Windows 10/11 | winget | Supported |

Derivatives are detected via `ID_LIKE` in `/etc/os-release` (Linux Mint, Pop!_OS, Manjaro, EndeavourOS, CentOS Stream, Rocky Linux, AlmaLinux, etc.).

Release binaries are built for `linux`, `darwin`, and `windows` on both `amd64` and `arm64`.

---

## Windows Notes

- **winget** is used as the default package manager (pre-installed on Windows 10/11).
- **npm global installs** do not require `sudo` on Windows (user-writable by default).
- **curl** is pre-installed on Windows 10+ and does not require separate installation.
- **PowerShell** is the default shell when `$SHELL` is not set.
- Release archives use `.zip` format on Windows (`.tar.gz` on macOS/Linux).
- **GGA on Windows** only works inside Git Bash. After installation, run `gga init` and `gga install` from Git Bash — not from PowerShell or CMD. This is a GGA limitation, not a gentle-ai limitation.

---

## Windows Security Verification

Some antivirus products can flag unsigned Go binaries heuristically.

Use the release checksum to verify integrity:

```powershell
# 1) Download checksums.txt from the same release tag
# 2) Compute local hash
Get-FileHash .\gentle-ai_<VERSION>_windows_amd64.zip -Algorithm SHA256

# 3) Compare the hash with checksums.txt entry for that file
```

If the hash matches `checksums.txt`, the file is authentic for that release.

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
| Antigravity | `%USERPROFILE%\.gemini\antigravity\` |
