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
| Android (Termux) | apt | Supported |

Derivatives are detected via `ID_LIKE` in `/etc/os-release` (Linux Mint, Pop!_OS, Manjaro, EndeavourOS, CentOS Stream, Rocky Linux, AlmaLinux, etc.). Termux is detected via the `TERMUX_VERSION` environment variable.

Release binaries are built for `linux`, `darwin`, and `windows` on both `amd64` and `arm64`. Android (Termux) is supported via source compilation (`go install`) since pre-built glibc binaries are incompatible with Android's Bionic libc.

---

## Termux (Android) Notes

- **apt** is used as the default package manager within Termux.
- **Prefix Awareness**: gentle-ai automatically detects the Termux `$PREFIX` and adjusts system paths accordingly (e.g., using `$PREFIX/bin/bash` instead of `/bin/bash`).
- **PATH Persistence**: When installing tools, gentle-ai will automatically append the appropriate `export PATH` commands to your `~/.bashrc` or `~/.zshrc`.
- **PIE Requirement**: All binaries updated via `gentle-ai self-update` on Termux are automatically compiled as Position Independent Executables (PIE), as required by Android.
- **Sub-agents**: Sub-agents like GGA are installed into `$PREFIX/tmp` during the setup process to ensure execution permissions.

---

## Windows Notes

- **winget** is used as the default package manager (pre-installed on Windows 10/11).
- **npm global installs** do not require `sudo` on Windows (user-writable by default).
- **curl** is pre-installed on Windows 10+ and does not require separate installation.
- **PowerShell** is the default shell when `$SHELL` is not set.
- Release archives use `.zip` format on Windows (`.tar.gz` on macOS/Linux).
- **GGA on Windows** works from both Git Bash and PowerShell. gentle-ai installs a `gga.ps1` shim that automatically delegates to Git Bash, so no manual shell switching is required.

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
