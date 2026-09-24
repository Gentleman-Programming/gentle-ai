# Gentle-Shell Native Private Report CI

## Goal
Establish PR-run Linux, macOS, and Windows evidence for the optional local failure-report storage boundary without enabling unsupported platforms.

## Authority
- Approved issue: [Gentleman-Programming/gentle-ai#4935](https://github.com/Gentleman-Programming/gentle-ai/issues/4935).
- Branch: `feat/shell-installer-native-evidence-01-privatefile`, based on canonical Main `c5da5fd0f5f0a0b34cfc9bca0a8b5dd5a46213a8` in a dedicated worktree.
- The parent is authorized to commit this scoped work unit and push/open a fork draft PR for native CI. No merge, publication, preview, user configuration change, or real Apply is authorized.

## Tasks
- [x] Bring the self-contained Linux owner-only report writer and its tests into this clean branch; retain non-Linux `ErrUnsupported` with no destination creation.
- [x] Add focused PR CI steps on native macOS and Windows runners with an exact test-selection guard (not yet executed on CI).
- [ ] Independently verify locally, then inspect native CI results; independent local package/vet/cross-compile checks passed, native CI pending.
- [ ] Parent: commit the independently verified work unit, use applicable native review, and open the linked draft PR; no merge approval.

## Scope and limits
This first slice establishes Linux report storage and non-Linux fail-closed behavior only. Native macOS/Windows CI passing an unsupported-path test is not evidence of secure report writing on those platforms, nor of a working `gentle-shell` launcher route. The dirty `gentle-shell-runtime` worktree and reserved `main-current` checkout are not edited. No Apply or report writes under the actual user home.

## Evidence
Local evidence (uncommitted; no CI run IDs):
- RED: `go test -p 1 ./internal/privatefile -count=1 -timeout=300s` failed because the package was absent.
- GREEN: with disposable writable `HOME`, `XDG_CONFIG_HOME`, `GOCACHE`, and
  `GOPROXY=off GOTOOLCHAIN=local GOMODCACHE=/home/devel/go/pkg/mod`, the same
  focused test and `go vet ./internal/privatefile` passed.
- `GOOS=windows GOARCH=amd64 go test -c -o <temporary location> ./internal/privatefile`
  and the same command with `GOOS=darwin` passed (compile-only, not native proof).
- Verbose Linux rerun passed with no skips as nonroot. Native macOS/Windows
  execution is pending. `gofmt -l` on touched Go files, `git diff --check`,
  and untracked whitespace inspection were clean.
- Independent read-only verifier confirmed all five source files match the source-only originals byte-for-byte, package tests/vet/format/whitespace pass, and macOS/Windows compile-only checks pass. Full repository suite remains pending CI; native runtime behavior has not been observed.
- Runtime harness: N/A; no launcher/Apply route is present in this slice.
- Rollback boundary: remove `internal/privatefile/`, the two CI steps, and
  this tracker update; no other worktree or actual user home was changed.
