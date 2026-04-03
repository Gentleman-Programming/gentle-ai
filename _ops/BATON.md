# BATON — Gentle AI Session Handoff
> Read this FIRST before any work.

## Last Update: 2026-04-03

### Current State
- Go CLI toolkit for AI-assisted development
- Recent work: SDD auto-init guard, TOML Windows fix, GGA upgrade path, **backup exclude fix**
- Has e2e/ test directory and testdata/ — one of the few projects with test infra
- OpenSpec directory for SDD artifacts

### What's Working
- CLI commands functional
- SDD flow integration (auto-init guard resolves Strict TDD from config)
- E2E test infrastructure
- **Pre-upgrade backup**: excludes runtime dirs (projects/, sessions/, plugins/, cache/) — fixes hang on large ~/.claude/ dirs

### Pending
- P2: Expand e2e test coverage
- P2: Documentation updates for v1.11.0 features
- P3: Dashboard/UI improvements
- P1: GGA git clone tests fail on Windows (TestGGAScriptUpgradeUsesGitClone, TestRunStrategy_GGAUsesGitClone)

### For Next Session
1. Read this BATON.md
2. Read AGENTS.md for skill routing
3. Run `go test ./...` to verify
4. Check GitHub issues
