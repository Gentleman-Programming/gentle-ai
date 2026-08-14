# Apply Progress: Rollback-safe persona output-style transitions (#3163)

Change: `fix-persona-output-style-rollback`
Phase: apply — COMPLETE (all 11 tasks, 3 work-unit commits)
Mode: Strict TDD (RED → GREEN → TRIANGULATE → REFACTOR)
Artifact store: openspec

## What Was Implemented

Three additive changes closing the removal-failure gap in persona output-style
transitions (Gentleman → Neutral):

1. **Removal seam + typed error** (`internal/components/persona/inject.go`):
   `var RemoveFileFn = os.Remove` (exported test seam, mirrors
   `backup.UserHomeDirFn`); `removeFileAtomic` routes through it;
   `OutputStyleRemovalError{Path, Err}` with `Error()`/`Unwrap()` replaces the
   opaque `fmt.Errorf("remove retired output style: %w")` at the removal loop;
   `MessageRolledBackOutputStyle` constant added.
2. **CLI classification** (`internal/cli/persona_rollback.go`):
   `handleRolledBackPersonaTransition(exec pipeline.ExecutionResult) bool`
   returns true ONLY for `*persona.OutputStyleRemovalError` + `Rollback.Success`,
   prints `WARNING: <const>` to stderr. Wired at `run.go` (install) and
   `sync.go` (sync) BEFORE wrapping `Execution.Err`; handled rollback returns
   `result, nil` (exit 0) and skips post-apply verification.
3. **CLI e2e parity tests** (`internal/cli/persona_transition_test.go`):
   install + sync with identical Windows-safe failure injection (exported seam
   override, NOT chmod), asserting byte-for-byte restore of gentleman.md +
   settings.json, no partial neutral.md, user file untouched, hook called
   exactly once, warning on stderr, exit 0, install/sync parity.

Snapshot scope unchanged (settings.json already covered via component path
contract — D1 pinned). No retry loop (REQ-NO-RETRY), no in-function restore.

## Test Results

| Command | Result |
|---|---|
| `go test ./internal/components/persona/...` | ok (all tests, incl. 3 new) |
| `go test ./internal/cli/ -run 'TestPersonaOutputStyleTransitionRollback' -v` | 3/3 pass (install + parity/install + parity/sync) |
| `go test ./internal/cli/ -run 'TestHandleRolledBackPersonaTransition'` | 4/4 pass |
| `go test ./internal/cli/ -run 'TestPersonaSyncOutputStyleSwitchIsIdempotent\|TestSyncRollbackRestoresRemovedRetiredReviewPlugins\|TestRetiredOpenCodePluginBackupTargetsGuardRollback'` | 3/3 pass (guards) |
| `go vet ./...` | clean |
| `go test ./...` | 64 packages ok; **3 pre-existing `codex` failures** (see below) |

Pre-existing failures (confirmed identical at baseline `2ef223bf` before any
change, environment-dependent codex transport/manifest tests, untouched by this
change — reported, NOT fixed per strict-tdd policy):
- `TestEveryManifestDigestStaysByteStable/codex` (internal/agents/capabilitymanifest)
- `TestNegotiatedConsentEnvelopeBindsTheDeclaredRuntimeIdentity/codex` (internal/cli)
- `TestDirectRouteStillRefusesADeclaredRuntime/codex` (internal/cli)

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1/1.2 | `internal/components/persona/inject_test.go` | Unit | ✅ 3 guards + persona suite pass | ✅ Compile-fail (`undefined: RemoveFileFn`, `OutputStyleRemovalError`) | ✅ `TestInjectOutputStyleRemovalFailureReturnsTypedError` pass | ✅ `TestOutputStyleRemovalErrorCarriesPathAndUnwrapsCause` + `TestRemoveFileAtomicRoutesThroughSeam` (3 subtests) | ✅ gofmt clean; full persona suite + guards still green |
| 2.1/2.2 | `internal/cli/persona_rollback_test.go` | Unit | ✅ (new file) | ✅ Compile-fail (`undefined: handleRolledBackPersonaTransition`) | ✅ 4/4 classification cases pass | ✅ 4 distinct inputs cover all branches (typed+ok, typed+rollback-failed, generic, nil) + stderr leak assertions | ✅ helper minimal; run.go/sync.go wiring adds comments only |
| 2.3 | run.go:223 / sync.go:1599 | Integration | ✅ guards pass | N/A — wiring verified by 3.1/3.2 e2e (no production code before tests in this unit) | ✅ build + guards pass | ✅ (covered by parity loop) | ✅ — |
| 3.1 | `internal/cli/persona_transition_test.go` | E2E | ✅ (new file) | ✅ Test written first; RED exposed a test-setup bug (raw JSON string assert), fixed to JSON-parse assert | ✅ install e2e pass | ✅ parity loop adds sync leg (SEN-INSTALL-SYNC-PARITY) | ✅ helper extraction (setup/assert shared) |
| 3.2 | same file, parity loop | E2E | ✅ | — | ✅ 2/2 pass | ✅ — | ✅ — |
| 3.3 | guards | Integration | ✅ | — | ✅ 3/3 pass | ✅ — | ✅ — |
| 4.1/4.2 | full suite + docs | — | — | — | ✅ 64 pkgs ok; vet clean; docs grep empty | — | — |

## Work Unit Evidence

| Unit | Focused test + result | Runtime harness + result | Rollback boundary |
|------|----------------------|--------------------------|-------------------|
| 1 (seam+error) | `go test ./internal/components/persona/...` → ok | N/A — success paths byte-identical; CLI scenario proven by unit 3 | Revert `inject.go` only; old `os.Remove`/generic-error behavior restored |
| 2 (warning plumbing) | `go test ./internal/cli/ -run 'TestHandleRolledBackPersonaTransition\|TestPersona'` → ok | CLI e2e (unit 3) is the runtime harness | Revert run.go/sync.go edits + delete persona_rollback.go; exits 1 again |
| 3 (e2e parity) | `go test ./internal/cli/ -run 'TestPersonaOutputStyleTransitionRollback'` → 3/3 pass | The e2e tests ARE the harness (precedent sync_review_retirement_test.go:106) | Delete persona_transition_test.go only |

## Deviations from Design

None — implementation matches design.md. One test-implementation note: the
precondition assertion on the pre-transition settings.json initially used a
raw-string match that ignored `mergeJSONFile` pretty-printing; fixed to
JSON-unmarshal assertions. Production code unchanged by that fix.

## Commits

1. `919b7db1` `fix(persona): add removal seam and typed output-style removal error`
2. `e5a5d414` `fix(cli): warn and exit 0 after rolled-back persona output-style transition`
3. `5b2c7a58` `test(cli): prove persona output-style transition rollback parity`

No Co-Authored-By trailers. Openspec artifacts (proposal/spec/design/tasks/
apply-progress) remain untracked planning metadata; the PR is the 3 commits.

## Remaining Work

- Verify phase (sdd-verify): compare implementation against every spec scenario.
- The 3 pre-existing `codex` failures should be triaged separately (not part of
  this change; likely environment-dependent codex transport/manifest drift).
