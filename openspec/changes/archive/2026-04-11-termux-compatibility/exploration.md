## Exploration: termux-compatibility

### Current State
`gentle-ai` relies on `runtime.GOOS` for platform detection and assumes standard Unix paths (`/usr/bin/bash`) or Windows-specific tools (`powershell`). It currently does not recognize Termux as a specific environment, which leads to path resolution issues and potential execution failures on Android due to missing PIE (Position Independent Executable) support.

### Affected Areas
- `internal/system/detect.go` — Needs to recognize Termux via `TERMUX_VERSION` or `ID=termux` in os-release.
- `internal/system/path.go` — Needs to handle PATH persistence in Termux (`~/.bashrc` instead of Windows registry).
- `internal/update/upgrade/strategy.go` — Needs to handle `android/arm64` binary downloads and PIE requirements.
- `internal/installcmd/resolver.go` — Installation of sub-agents needs to be prefix-aware (avoiding hardcoded `/usr/bin`).

### Approaches
1. **Prefix-Aware Routing (Recommended)** — Dynamically resolve system paths using an internal `ResolvePath(path string)` helper that prepends `$PREFIX` if running in Termux.
   - Pros: Transparent to the rest of the codebase, handles non-standard Termux paths.
   - Cons: Requires wrapping common path operations.
   - Effort: Medium

2. **Environment-Specific Profiles** — Add a dedicated `Termux` profile in `PlatformProfile` that overrides default Unix behavior for installation and updates.
   - Pros: Explicit and maintainable, allows for Termux-specific features (Termux:API).
   - Cons: More complex detection logic.
   - Effort: Medium

### Recommendation
Combine both approaches: Add a `Termux` distro/profile and implement a path resolver utility. This ensures `gentle-ai` feels native in Termux while maintaining clean architecture for other platforms.

### Risks
- **PIE Compilation**: Failure to compile with `-extldflags=-pie` will cause the binary to crash on modern Android versions.
- **Permission Denied**: Installing binaries in `/sdcard` or outside `$HOME` will fail due to `noexec` mounts. We must ensure the installer defaults to `$HOME` or `$PREFIX`.

### Ready for Proposal
Yes — I have enough information to define the architectural changes and implementation tasks.
