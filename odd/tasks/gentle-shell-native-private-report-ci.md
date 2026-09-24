# Gentle-Shell Native Private Report CI

## Goal
Establish PR-run Linux, macOS, and Windows evidence for the optional local failure-report storage boundary without enabling unsupported platforms.

## Authority
- Approved issue: [Gentleman-Programming/gentle-ai#4935](https://github.com/Gentleman-Programming/gentle-ai/issues/4935).
- Branch: `feat/shell-installer-native-evidence-01-privatefile`, based on canonical Main `c5da5fd0f5f0a0b34cfc9bca0a8b5dd5a46213a8` in a dedicated worktree.
- The parent is authorized to commit this scoped work unit and push/open a fork draft PR for native CI. No merge, publication, preview, user configuration change, or real Apply is authorized.

## Tasks
- [x] Bring the self-contained Linux owner-only report writer and its tests into this clean branch; retain non-Linux `ErrUnsupported` with no destination creation.
- [x] Run focused PR CI steps on native macOS and Windows with an exact refusal-test presence guard.
- [x] Independently verify locally and inspect corrected native CI `35949939912`: 11 jobs passed, including Darwin/Windows refusal and Linux full suite/format.
- [x] Commit reviewed work units `8113d435` and `1466c43b`; open [draft PR #4936](https://github.com/Gentleman-Programming/gentle-ai/pull/4936), with no merge approval.

## Scope and limits
This first slice establishes Linux report storage and non-Linux fail-closed behavior only. Native macOS/Windows CI passing an unsupported-path test is not evidence of secure report writing on those platforms, nor of a working `gentle-shell` launcher route. The dirty `gentle-shell-runtime` worktree and reserved `main-current` checkout are not edited. No Apply or report writes under the actual user home.

## Evidence
Bounded work-unit evidence (371 initial authored lines; 372 total after correction):
- RED: `go test -p 1 ./internal/privatefile -count=1 -timeout=300s` failed because the package was absent.
- GREEN: with disposable writable `HOME`, `XDG_CONFIG_HOME`, `GOCACHE`, and
  `GOPROXY=off GOTOOLCHAIN=local GOMODCACHE=/home/devel/go/pkg/mod`, the same
  focused test and `go vet ./internal/privatefile` passed.
- `GOOS=windows GOARCH=amd64 go test -c -o <temporary location> ./internal/privatefile`
  and the same command with `GOOS=darwin` passed (compile-only, not native proof).
- Verbose Linux rerun passed with no skips as nonroot. `gofmt -l` on touched Go files, `git diff --check`, and untracked whitespace inspection were clean.
- Independent read-only verifier confirmed all five source files match the source-only originals byte-for-byte and focused package/vet/format/whitespace checks pass. macOS/Windows cross-compiles alone proved no runtime behavior.
- Initial CI `35949091000`: Windows refusal step passed; Darwin guard exited 141 before tests because quiet grep closed the pipe under `pipefail`. One-line full-consuming grep correction passed independent Linux pipefail simulation and a separate approved/acknowledged native review.
- [Corrected native CI `35949939912`](https://github.com/Gentleman-Programming/gentle-ai/actions/runs/35949939912) at `1466c43b`: all 11 jobs passed. Native Darwin and Windows each ran the registered fail-closed privatefile test (one package `ok` each); Linux `go test ./...` and Go Format passed. These results **do not** prove non-Linux report writing.
- Runtime harness: N/A; no launcher/Apply route is present in this slice.
- Rollback boundary: remove `internal/privatefile/`, the two CI steps, and
  this tracker update; no other worktree or actual user home was changed.
