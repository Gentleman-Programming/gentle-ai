## Intent
Enable `gentle-ai` to run natively within the Termux environment on Android by addressing path resolution issues, platform detection gaps, and Android's mandatory PIE (Position Independent Executable) requirement.

## Maintainability & Regression Strategy
To ensure zero impact on existing platforms (Windows, macOS, standard Linux):
- **Abstract Path Resolver**: Replace hardcoded paths with a `system.Resolver` interface that handles prefix-awareness.
- **Environment Mocking**: All Termux-specific logic MUST be unit-tested using explicit Android profile inputs and environment mocks for `$PREFIX`/shell filesystem layouts.
- **Strict Isolation**: Termux logic will be encapsulated in platform-specific adapters to prevent "spaghetti" `if` blocks in the core logic.
- **Regression Suite**: Existing tests for Windows (PowerShell) and Linux (standard paths) must remain unchanged and pass.

## Scope

### In Scope
- Recognize Android/Termux through the canonical `GOOS=android` platform profile.
- Implement prefix-aware path resolution for system binaries (e.g., `/usr/bin/bash` -> `$PREFIX/bin/bash`).
- Add support for PATH persistence in Termux shells (`.bashrc`, `.zshrc`).
- Ensure `go build` for Android targets uses PIE flags.
- Update `internal/update` to handle `android/arm64` releases correctly.

### Out of Scope
- Integration with `termux-api` for notifications (deferred to a future change).
- Support for running `gentle-ai` outside of the Termux environment (e.g., standard Android shell).

## Capabilities

### New Capabilities
- `termux-support`: Core logic for detecting and adapting to the Termux prefix environment.
- `android-pie-compilation`: Build-time support for Position Independent Executables.

### Modified Capabilities
- `system-detection`: Update `PlatformProfile` to include Android/Termux as a supported platform profile.
- `update-strategy`: Adapt binary download and replacement for Android's filesystem layout.

## Approach
Implement a hybrid approach:
1.  **Detection**: Update `internal/system/detect.go` so `GOOS=android` resolves to the Android/Termux profile.
2.  **Path Resolution**: Create a helper that returns the correct path for the active `$PREFIX`.
3.  **Compilation**: Configure update-time Go install flags to include `-extldflags=-pie` on Android.
4.  **Installation**: Adjust `AddToUserPath` to append PATH exports to shell configuration files when the platform profile is Android/Termux.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/system/detect.go` | Modified | Add Android/Termux platform detection logic. |
| `internal/system/path.go` | Modified | Add shell-config persistence for Termux. |
| `internal/update/upgrade/strategy.go` | Modified | Add Android/PIE build flags and arch detection. |
| `internal/installcmd/resolver.go` | Modified | Use prefix-aware paths for sub-agent installation. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Regression on Windows/Linux | Medium | Exhaustive unit testing with mocked environments; no changes to core logic paths. |
| PIE Crash | High | Ensure all build steps for Android include PIE flags. |
| Permission Denied | Medium | Default all binary paths to `$HOME` or `$PREFIX/bin`. |

## Rollback Plan
Since this change mostly adds conditional logic for Android/Termux, a rollback involves reverting the Android branch in `internal/system/detect.go`, which will cause the system to stop treating Termux as a supported first-class platform.

## Dependencies
- Termux environment (v0.118+ recommended).
- Go 1.22+ installed within Termux.

## Success Criteria
- [ ] `gentle-ai` starts correctly in Termux without path errors.
- [ ] `gentle-ai self-update` works and produces an executable binary.
- [ ] Sub-agents can be installed and run within the Termux `$PREFIX`.
- [ ] **100% of existing tests for Windows and Linux pass without modification.**
