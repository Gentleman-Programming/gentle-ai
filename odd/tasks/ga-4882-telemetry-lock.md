# ga-4882: telemetry lock loses same-process updates on Windows

Claimed 2026-09-23 (issuecomment-5796980665). Branch fix/4882-telemetry-increment-syncs
(worktree ~/gentleman/gentle-ai-4882) from origin/main a773ccfb.

## Root-cause position (verified)

- Update (lock -> EnsureState -> mutate -> Save) is the only serialization point; Save is
  atomic (temp+rename) on telemetry.json; nothing deletes telemetry.json.lock.
- Defect: same-process LockFileEx intermittently fails to mutually exclude on Windows
  (17/20 observed on the lane; Linux flock never reproduces).
- Range landscape: our ^uint32(0),^uint32(0) matches Go stdlib lockedfile (not proven
  wrong); gofrs/flock and bbolt use a concrete 1-byte range after real same-process races
  (bbolt#121); this repo's reviewtransaction store lock uses exactly 1 byte at offset 0.

## Tasks

1. [done] RED: TestLockStateExcludesSameProcessCallers (contract) +
   TestUpdateSameProcessExclusionSurvivesBrokenFileLock (injectable lockFileExclusive,
   no-op swap, concurrent Updates lose zero).
2. [done] Implement: 1-byte range at offset 0 (lock+unlock), path-keyed same-process
   mutex hybrid in lockState, EnsureState under-Update invariant comment.
3. [done] GREEN + full internal/telemetry suite + gofmt/vet (delegated): focused GREEN,
   suite ok 8.2s, gofmt/vet clean, GOOS=windows build ok.
4. [done] Commit 7ce9baa7 + push; PR #4914 opened (Closes #4882, type:bug requested from maintainer).
5. [done] Native review lineage review-d61a45dd63798350 (medium, 1 lens): approved + acknowledged/burned. Advisories: R3-1 WARNING state_lock_windows.go:27, R3-2/R3-3 SUGGESTION state_lock.go:50-64 — all informational, separate later work.

## Evidence

- Task 1 (RED): stage 0 — build failure (lockFileExclusive not yet a var, tests written
  first: cannot assign to lockFileExclusive, 4 sites). After the behavior-neutral seam
  (pure rename to lockFileExclusivePlatform + package var), stage 1 — both configs of
  TestUpdateSameProcessExclusionSurvivesBrokenFileLock fail with the mechanism
  reproduced: "Counters.Syncs = 1, want 20: same-process updates were lost"; WriteFileAtomic
  independently flagged "the replacement did not land" (lost-update window observed).
  Contract test TestLockStateExcludesSameProcessCallers passes on Linux (flock excludes
  same-process fds); it pins the bar Windows must meet. Files: state_lock_test.go (new).
- Task 2 (GREEN): state_lock.go (hybrid: path-keyed mutex map keyed by absolute lock path,
  acquired before open/lock; unlock = file lock -> close -> mutex; var seam comment),
  state_lock_windows.go (1 byte at offset 0, lock+unlock, rationale comment),
  state_lock_unix.go (rename only), state.go (EnsureState concurrency invariant comment).
  Focused run all PASS; suite ok 8.2s; lane test -count=5 ok; gofmt empty; vet clean;
  go build ./... ok; supplementary GOOS=windows build of internal/telemetry ok.

- 2026-09-23: worker RED `Counters.Syncs = 1, want 20` (broken-lock sabotage, Linux); GREEN zero lost both configs; suite ok 8.2s; gofmt/vet/GOOS=windows clean. Review approved+burned same day. Windows lane green on the PR #4914 branch only
  (CI run 35883778782: Unit Tests, Windows Runtime, Darwin, E2E all pass); this does not
  establish the Windows suite on main, which stays pending until a passing main run is
  recorded. Also pending external: type:bug label (maintainer-side).

- CI 2026-09-23: Unit Tests red once on TestDocumentedInvocationsRunAsDocumented/executed/sync_# (TempDir RemoveAll cleanup race, internal/app). Triaged as pre-existing environment flake: telemetry writer dev-gated off in this job, diff adds no writer/file, local x15 green branch+base. Triage comment on PR (issuecomment-5797855289). type:bug label pending maintainer.
