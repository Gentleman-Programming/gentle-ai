# pi in-process reviewer contract + assess slice transition

- Feature: `pi-inprocess-reviewer-assess-transition`
- Branch: `fix/pi-inprocess-reviewer-assess-transition` (worktree `~/work/gentle-ai-worktrees/pi-inprocess-reviewer`, base `origin/main` 15ea98ed)
- Engram mirror: topic `odd/pi-inprocess-reviewer-assess-transition/tasks` (project gentle-ai)
- Sibling feature (gentle-pi): `odd/tasks/inprocess-reviewer-completion.md` in `~/work/gentle-pi-worktrees/inprocess-reviewer`
- TDD: strict (source: gentle-ai session config "Strict TDD Mode: enabled"); runner `go test ./...`
- Delivery: `single-pr` with `size:exception` (user decision 2026-09-18); work-unit commits per task
- RDD: on (global). Candidate per task = the work-unit commit; assess after each commit with `--base-ref <last reviewed boundary> --committed-only`

## Objective

Make the pi runtime never spawn a `pi --print` child for any reviewer role, and make `review assess` emit the exact review preflight transition after a work-unit commit so the orchestrator executes tokens instead of interpreting prose.

## Problem

1. Issue #4611: reviewer children run `pi --print --no-extensions`, so providers registered by pi extensions (e.g. `nan/deepseek-v4-flash`) resolve as `Model not found`, and `piRuntimeEnvironment` strips `$ENV` API keys. Lenses are host-relayed by gentle-pi, but refuter/validator are still spawned by Go (`reviewProviderRoleHostAdapter` → `PiAdapter`), and roles have no host submission mode.
2. Orchestrators skipped the RDD preflight after a `medium` commit that closed a ~400-line slice (NaN-builders T5): the ODD rule is prose only; `review assess` reports size/tier but no consequence.

## Scope

In:
- Role submission mode (`--input`) for `review capture-refuter` / `review capture-validation`; pi role collect inputs rendered as materialize + submission descriptor (status schema bump).
- Remove the Go-owned pi spawn (`internal/reviewerprovider/pi_adapter.go`, `internal/agents/pi/review_routing.go`) once unused; update `scripts/crosslane/hostpi.go`.
- `review assess`: additive fields `candidate.consumed`, `review_due`, `next_transition` (exact STATUS preflight tokens), optional `--agent`; schema JSON update.
- Docs/contract text: role submission contract, ODD "after each work-unit commit run assess; if `review_due`, execute `next_transition` verbatim".

Out:
- gentle-pi in-process completion (sibling feature, own PR).
- Token-aware lens budget (#4680), automatic RDD chaining, `GENTLE_PI_REVIEW_RELAY_EXTENSIONS` allowlist in Go (rejected).

## Constraints

- Go never authors a verdict: `--input` bytes are admitted through the existing raw admitters with the same binding (`--lineage --target --expected-revision [--request-hash]`).
- Status schema: role `submission` descriptor is a documented contract change → bump `ReviewIntegrationStatusSchema` (v7 → v8) and update every consumer/test/bench constant.
- Assess never disagrees with START: reuse `AssessSnapshotRisk`, `CompactTargetConsumed`, and the STATUS token builders; `review_due = high || (medium && changed_lines >= LargeChangeLines)`, false when consumed or passive.
- No `PiAdapter` fallback, no retry, no synthetic result.

## Known environmental failures

- `internal/reviewtransaction` fails under the macOS default `TMPDIR` (`/var/folders/...`: "maintenance authority component /var is unsafe", "review store lock could not be acquired: not a directory") on untouched `main`; passes with `TMPDIR=$HOME/.cache/gentle-ai-gotest`. Run full suites with that TMPDIR.

## Tasks

- [x] T1 — Role submission mode: `capture-refuter`/`capture-validation` accept `--input=<path|->` (mutually exclusive with `--materialize`/`--execute`; host-relay runtimes only), admit via `reviewProviderAdmitRefuterRaw` / `reviewProviderAdmitTargetedValidatorRaw`, close the slot exactly like `--execute` does. Tests in `review_provider_role_materialize_test.go`.
- [x] T2 — STATUS renders pi role collect inputs as `--materialize=true` + `submission` descriptor (mirror lens `input.submission`), status schema v7→v8; `--execute` for pi refuses typed (host-mediated). Delete `reviewProviderRoleHostAdapter`, `pi_adapter.go`(+tests), `internal/agents/pi/review_routing.go`(+tests), `review_pi_role_routing_test.go`; update `scripts/crosslane/hostpi.go` and bench schema constants.
- [x] T3 — `review assess`: `--agent` (optional), `candidate.consumed`, `review_due`, `next_transition{operation,arguments}`; schema JSON additive; tests for passive/medium-under-budget/medium-at-budget/high/consumed.
- [x] T4 — Docs and contract: `docs/usage.md`, `internal/assets/skills/_shared/sdd-orchestrator-sections.md`, review integration docs (role submission), ODD assess rule; CHANGELOG entry if the repo keeps one.

## Acceptance criteria

- `go test ./...` green; `cd bench && go test ./...` green.
- No Go code path constructs `reviewerprovider.NewPiAdapter` (grep returns nothing).
- STATUS for a pi lineage at refuter/validator collection returns `--materialize=true` tokens plus a submission descriptor whose `{{value}}` slot substitutes into `--input`.
- `review assess --base-ref X --committed-only --json` on a medium candidate with `changed_lines >= 400` returns `review_due: true` and a `next_transition` whose arguments are exactly `review status --cwd <repo> --contract gentle-ai.review-integration/v2 [--agent <a>] --next-transition --base-ref X --committed-only`.

## Forecast

~900 authored changed lines (T1 ~200, T2 ~400 incl. deletions, T3 ~250, T4 ~80). Exceeds 400 → `size:exception` accepted by user.

## Progress / evidence

- T1 — commit `08d14841`. Checks: focused `go test ./internal/cli/ -run 'TestReviewCapture(Refuter|Validation)'` ok; full `go test ./internal/cli/...` ok (674s); `go vet`, `gofmt -l` clean; parent spot check `go test ./internal/cli/...` exit 0. Decision: `--input` does not require `--repository-context` (matches `--execute`); `--agent` stays mandatory. Assess: medium, 2 paths, 298 lines → deferred to slice (boundary `15ea98ed`).
- T2 — commit `55eefed3` (34 files, +633/−1147; deletes `pi_adapter.go`, `agents/pi/review_routing.go`; adds `contracts/review-integration/v2/schemas/status-v9.schema.json`). Checks: `go build ./...`, `go vet ./...`, `gofmt` clean; focused cli tests ok; `go test ./...` ok under HOME TMPDIR; `cd bench && go test ./...` ok; `go test ./scripts/crosslane/...` ok. Decisions: status schema v8→v9 (v5 `transition_input` forbade `submission` on role inputs); capabilities schema NOT bumped (closed enum, out of scope — comment left in `review_capabilities.go`); crosslane battery authors a minimal admissible role verdict from the materialized prompt and submits via `--input` (no live model path for roles there). Gap: `--with-host` pi lane not executed locally. Assess (slice T1+T2 from `15ea98ed`): high, 35 paths, 2061 lines (`process_boundary`) → immediate candidate; consent granted; lineage `review-d400267bdff6745d`, four lenses → approved with no correction → acknowledged. Reviewed boundary: `55eefed3`.
- T3 — commit `71a47477` (3 files, +416/−14). Checks: `go build`, `gofmt`, `go vet` clean; `go test ./internal/cli/ -run TestReviewAssess` ok; `go test ./internal/cli/...` ok with `-timeout 30m` (607s; the default 10m ceiling is not a failure); `go test ./...` ok under HOME TMPDIR. Decisions: schema stays `/v1` (additive; `review_due`, `review_due_reason`, `candidate.consumed` required, `next_transition` optional); `--agent` renders before `--next-transition`; ASCII `->` in human output. Dev-build proof: assess on the T1+T2 range reports `already_reviewed`; on T3 reports `review_due: true (slice_budget_reached)` with the exact STATUS tokens. Assess: medium, 430 lines → slice budget reached → preflight run from the emitted tokens; consent granted; lineage `review-6d77250921c4610e`, one lens (reliability) → approved, no correction → acknowledged. Reviewed boundary: `71a47477`.
- T4 — commit `972446f1` (docs/usage.md, docs/review-integration.md, and the installed ODD routing text in `internal/components/agentguidance/routing.go` + its test). Checks: `go test ./internal/components/agentguidance/` ok (RED→GREEN on the rule text); `go test ./internal/assets/...` ok. Assess: medium, 62 lines → feature end closes the slice; consent granted; lineage `review-2f8e023ce75a2afd`, one lens → approved → acknowledged. Reviewed boundary: `972446f1`.
- Final branch verification: `go run ./internal/gofmtcheck` ok; `TMPDIR=$HOME/.cache/gentle-ai-gotest go test -timeout 30m ./...` no failures; `cd bench && go test ./...` ok. Branch: 42 files, +1438/−1115.
- GitHub: issue #4783 created (feature form, readback confirmed); #4611 → `status:approved`+`type:bug`; #4783 → `status:approved`+`type:feature`; `size:exception` authorized for the PR.

## Next step

Push, open the PR (Closes #4611, #4783; type:feature; size:exception), CI, merge. Then release and bump the gentle-pi pin.
