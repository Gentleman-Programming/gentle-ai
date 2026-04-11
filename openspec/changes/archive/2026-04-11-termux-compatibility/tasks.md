# Tasks: termux-compatibility

## Phase 1: Infrastructure & Platform Detection (TDD)
- [ ] 1.1 **RED**: Add failing unit test in `internal/system/detect_test.go` to simulate Termux environment via `TERMUX_VERSION`.
- [ ] 1.2 **GREEN**: Update `internal/system/detect.go` to recognize `TERMUX_VERSION` and assign `LinuxDistroTermux`.
- [ ] 1.3 **REFACTOR**: Ensure `detectFromInputs` remains clean and platform-agnostic.
- [ ] 1.4 **VERIFY**: Run `go test ./internal/system/...` and confirm 100% pass for all distros.

## Phase 2: Prefix-Aware Path Resolver (TDD)
- [ ] 2.1 **RED**: Create `internal/system/resolver_test.go` with cases for standard Linux vs. Termux `$PREFIX` resolution.
- [ ] 2.2 **GREEN**: Implement `PathResolver` interface and `TermuxResolver` in `internal/system/resolver.go`.
- [ ] 2.3 **REFACTOR**: Extract prefix-aware logic into a reusable helper.
- [ ] 2.4 **VERIFY**: Ensure resolver tests pass and do not affect non-Termux paths.

## Phase 3: PATH Persistence & Android/PIE Strategy (TDD)
- [ ] 3.1 **RED**: Add integration test in `internal/system/path_test.go` for `AddToUserPath` in Termux mode (mocking `.bashrc`).
- [ ] 3.2 **GREEN**: Update `internal/system/path.go` to append PATH exports to shell config files in Termux.
- [ ] 3.3 **RED**: Add unit test in `internal/update/upgrade/strategy_test.go` to verify `-extldflags=-pie` for Android builds.
- [ ] 3.4 **GREEN**: Update `internal/update/upgrade/strategy.go` to include PIE flags when `GOOS=android`.

## Phase 4: Integration & Verification
- [ ] 4.1 Update `internal/installcmd/resolver.go` to use `system.Resolver` for sub-agent installation paths.
- [ ] 4.2 **FINAL VERIFY**: Run full test suite (`go test ./...`) and ensure no regressions on Windows/Linux.
- [ ] 4.3 Update `README.md` or `docs/platforms.md` with Termux-specific installation notes.
