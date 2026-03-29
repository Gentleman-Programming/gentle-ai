# Tasks: Integrate RTK into Gentleman AI Ecosystem

## Phase 1: Foundation (Types & Catalog)

- [x] 1.1 Add `ComponentRTK ComponentID = "rtk"` to `internal/model/types.go`
- [x] 1.2 Add RTK entry to `mvpComponents` in `internal/catalog/components.go` with ID, name, description
- [x] 1.3 Add `resolveRTKInstall()` case in `ResolveComponentInstall` switch in `internal/installcmd/resolver.go`

## Phase 2: Core Implementation (RTK Package)

- [x] 2.1 Create `internal/components/rtk/config.go` — `AgentFlags()` mapping all 8 agents to rtk init flags
- [x] 2.2 Create `internal/components/rtk/install.go` — `InstallCommand()` using `installcmd.Resolver`, mirror GGA pattern
- [x] 2.3 Create `internal/components/rtk/runtime.go` — `ConfigureAgentHook()`, `ConfigureAllHooks()` with graceful failure loop
- [x] 2.4 Create `internal/components/rtk/verify.go` — `VerifyInstalled()`, `VerifyVersion()` with `exec.LookPath` mocking

## Phase 3: Integration (Pipeline Wiring)

- [x] 3.1 Add RTK case to `resolveRTKInstall()` — Homebrew: `brew install rtk`, Linux/Windows: download binary
- [x] 3.2 Wire RTK into pipeline stages — ensure RTK install runs before agent hook configuration
- [x] 3.3 Add `AgentHookResult` return type and progress reporting for TUI

## Phase 4: Testing

- [x] 4.1 Create `internal/components/rtk/config_test.go` — table-driven tests for `AgentFlags()` across all 8 agents
- [x] 4.2 Create `internal/components/rtk/verify_test.go` — test `VerifyInstalled()` with/without rtk in PATH
- [x] 4.3 Create `internal/components/rtk/runtime_test.go` — test `ConfigureAgentHook()` command args, graceful failure

## Phase 5: Documentation

- [x] 5.1 Add RTK row to component table in `docs/components.md`
- [x] 5.2 Add RTK to component sync list in `docs/usage.md`
