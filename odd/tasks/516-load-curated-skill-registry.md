# Feature: Load Curated Skill Registry (#516)

## 1. Diagnosis & Technical Proposal
- **Problem**:
  Currently, `gentle-ai skill-registry refresh` always regenerates `.atl/skill-registry.md` by scanning all project and user skill directories. Any manual curation (such as hand-authored compact rules, custom path conventions, or curated subsets across projects) gets wiped out on every refresh or startup hook.
  Furthermore, if `--load ""` or a missing path is passed, a naive implementation risks falling through to regeneration and overwriting a curated registry instead of failing closed.
  Also, in non-interactive environments like `pi -ns` (where startup skill loading is disabled), a curated registry load must be completely explicit, manual, and self-contained without triggering automatic background skill sweeps.
- **Chosen Approach**:
  1. Add `PrepareLoadRegistry` and `PreparedLoad.Commit` in `internal/skillregistry`.
  2. Require full validation of markdown shape: `# Skill Registry`, `## Skills`, `| Skill | Trigger / description | Scope | Path |`, and `| --- | --- | --- | --- |`.
  3. Support atomic pair commit (`.atl/skill-registry.md` and `.atl/.skill-registry.cache.json`) with rollback on failure.
  4. Fingerprint loaded registry with `loaded:<sha256>`, preserving it during non-forced refreshes (`manually-loaded`) and detecting drift (`loaded-drifted`).
  5. Support both `gentle-ai skill-registry load <path>` and `gentle-ai skill-registry refresh --load <path>`.
  6. Enforce strict **fail-closed** behavior: reject empty path (`--load ""`, `load ""`), missing arguments, or invalid format immediately before any filesystem mutation or fallback to regeneration.
- **Trade-offs**:
  Requiring exact markdown table headers prevents arbitrary markdown files from corrupting the skill registry contract, while preserving flexibility for curated rows and comments.

## 2. Technical Specification & Contracts
- **CLI Commands**:
  - `gentle-ai skill-registry load <path> [flags]`
  - `gentle-ai skill-registry refresh --load <path> [flags]`
- **Validation**:
  - `hasRegistryMarkers`: Must contain `# Skill Registry`, `## Skills`, table header, and table separator.
  - Fail-closed: empty load path returns an error without running `Regenerate`.
- **Files Modified**:
  - `internal/skillregistry/registry.go`
  - `internal/skillregistry/registry_test.go`
  - `internal/app/app.go`
  - `internal/app/app_test.go`
  - `internal/app/help.go`
  - `docs/skill-registry.md`

## 3. Tasks & Evidence
- [x] 1. TDD RED: Unit tests for registry loading, shape validation, rollback, and fail-closed arguments.
- [x] 2. TDD GREEN: Implement `PrepareLoadRegistry`, `Commit`, and CLI argument handling.
- [x] 3. Verification: Run tests for `internal/skillregistry` and `internal/app`, check build and help output.
- [ ] 4. Commit: Atomic work-unit commit on `feat/516-load-curated-skill-registry`.

## 4. Acceptance Criteria & Verification
- `go test ./internal/skillregistry/...` passes.
- `go test -run 'TestRunSkillRegistry' ./internal/app/...` passes.
- `gentle-ai skill-registry refresh --load ""` fails closed with an error and does not touch `.atl/skill-registry.md`.
