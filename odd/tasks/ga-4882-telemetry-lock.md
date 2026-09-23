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
3. [pending] GREEN + full internal/telemetry suite + gofmt/vet (delegated).
4. [pending] Work-unit commit + push; PR (Closes #4882).
5. [pending] Native review preflight for the candidate.

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
