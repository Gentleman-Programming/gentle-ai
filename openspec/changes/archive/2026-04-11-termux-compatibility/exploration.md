## Exploration: termux-compatibility

### Current State
`gentle-ai` relies on `runtime.GOOS` for platform detection and assumes standard Unix paths (`/usr/bin/bash`) or Windows-specific tools (`powershell`). It currently does not recognize Android/Termux as a specific platform profile, which leads to path resolution issues and potential execution failures on Android due to missing PIE (Position Independent Executable) support.

### Affected Areas
- `internal/system/detect.go` — Needs to recognize `GOOS=android` as the canonical Android/Termux platform profile.
- `internal/system/path.go` — Needs to handle PATH persistence in Termux (`~/.bashrc` instead of Windows registry).
- `internal/update/upgrade/strategy.go` — Needs to handle `android/arm64` binary downloads and PIE requirements.
- `internal/installcmd/resolver.go` — Installation of sub-agents needs to be prefix-aware (avoiding hardcoded `/usr/bin`).

### Approaches
1. **Prefix-Aware Routing (Recommended)** — Dynamically resolve system paths using an internal `ResolvePath(path string)` helper that prepends `$PREFIX` if running in Termux.
   - Pros: Transparent to the rest of the codebase, handles non-standard Termux paths.
   - Cons: Requires wrapping common path operations.
   - Effort: Medium

2. **Environment-Specific Profiles** — Add a dedicated Android/Termux profile in `PlatformProfile` that overrides default Unix behavior for installation and updates.
   - Pros: Explicit and maintainable, allows for Termux-specific features (Termux:API).
   - Cons: More complex detection logic.
   - Effort: Medium

### Recommendation
Combine both approaches: add an Android/Termux platform profile keyed by `GOOS=android` and implement a path resolver utility. This ensures `gentle-ai` feels native in Termux while maintaining clean architecture for other platforms and avoiding a half-detected `linux + termux` state.

### Risks
- **PIE Compilation**: Failure to compile with `-extldflags=-pie` will cause the binary to crash on modern Android versions.
- **Permission Denied**: Installing binaries in `/sdcard` or outside `$HOME` will fail due to `noexec` mounts. We must ensure the installer defaults to `$HOME` or `$PREFIX`.

### Ready for Proposal
Yes — I have enough information to define the architectural changes and implementation tasks.
