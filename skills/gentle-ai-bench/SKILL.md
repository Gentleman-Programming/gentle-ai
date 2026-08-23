---
name: gentle-ai-bench
description: "Trigger: bench, journey, journeys, driven mode, gentle-ai-bench, journey corpus, j-numbers, bench axis. Author and verify gentle-ai bench journeys; go test ./bench never proves driven execution."
license: Apache-2.0
metadata:
  author: "Gentleman-Programming"
  version: "1.0"
---

## Activation Contract

Load when touching `bench/` in gentle-ai, adding or changing a journey, changing a product semantic a journey might pin, or diagnosing a bench failure in CI's Unit Tests job.

## Hard Rules

- `go test ./bench` validates corpus declarations only. It does NOT execute journeys. The only driven proof is building the harness and the product binary and running the harness against it; a green `go test ./bench` claims nothing about execution.
- Reproduce CI, do not guess invocations: read the Unit Tests step in `.github/workflows/ci.yml` and copy its exact build and `gentle-ai-bench run --binary ...` commands. Use `--only <journey-id>` to drive one journey.
- Journey IDs are unique across every `journeys_*.go` file. The collision guard fails loudly naming both files; pick an unused ID by reading the corpus, never reuse a retired one.
- Every journey declares `Review:` — `reviewOptedIn` (the runner enables receipt-driven development globally before the first step, uncounted, and fails the journey if the switch does not come on) or `reviewUntouched` (its subject IS the switch, or it has nothing to do with reviews). The declaration is mandatory; `validateCorpus` fails the run without it. Never let a journey inherit the product's default: reviews are opt-in, and a journey that assumed otherwise measures a review-refused flow while still reporting `completed`.
- Every `execute` transition must carry a runnable command; the dead-execute guard fails the run otherwise.
- When a ratified product semantic changes, grep the corpus for journeys pinning the OLD behavior before shipping. The corpus is a second test surface beyond unit tests; a journey asserting the defect keeps the defect green.
- `dead_end` prints `n/a` unless the run actually measured one. Never fabricate a value to move the column.
- A `by_design` exemption costs a shape from the closed vocabulary plus a verified quote of the product's own next-action text. If the quote no longer tells the operator what to do, it is a defect wearing an exemption.
- Prefer a NEW `journeys_*.go` file when the shared ones are owned by open PRs; bump the core journey-count pin in the same change.

## Execution Steps

1. Read the corpus area you touch and the CI invocation before writing.
2. Author or adapt the journey; update its title, step names, and comment to say WHY the expectation holds (cite the issue or ratified decision).
3. Run `go test ./...` in `bench/` for declarations, THEN the driven harness for execution; both results go in the PR body.
4. On semantic changes, list the journeys you checked for stale pins.

## Output Contract

PR evidence includes the driven-mode summary line (completed / unsupported / failed counts) from a locally built binary, not only `go test` output.
