# Telemetry collector /metrics cardinality

Locator: `odd/tasks/telemetry-metrics-cardinality.md` (worktree `gentle-ai-worktrees/telemetry-metrics-cardinality`, branch `fix/telemetry-metrics-cardinality` from `origin/main` 465452ebe). Engram mirror: `odd/telemetry-metrics-cardinality/tasks`.

## Objective
Keep the collector's `GET /metrics` exposition (and the collector's memory) bounded so it never crosses VictoriaMetrics' `-promscrape.maxScrapeSize` (64 MiB).

## Problem
Watchdog alert 2026-09-24: `/metrics` = 50.4 MB (75% of 64 MiB), growing linearly ~7.5 MB/day since the 2026-09-18 restart; the cap is reached in ~2 days, after which the whole scrape is dropped and every runtime panel reads 0. Collector RSS 720 MB on a 3.6 GB VPS.

Evidence (production snapshot, 259,217 series):
- `internal/telemetrycollector/metrics.go` `RuntimeMetrics` never evicts a series (no timestamp, reset only by restart). `provider`/`model` labels are client free-form: 355 providers, 637 models, 8,713 base combinations.
- `add()` creates a series even when `delta == 0`: 125,281 series (48%) = 24.8 MB are permanently 0 (token_fields_total 87,422; tokens_total 20,517; launches/duration ~17,200; responses 134).

## Why
Raising the cap only moves the limit to VPS RAM (collector + VictoriaMetrics both grow with series). The registry has to bound itself.

## Scope
- Authorized: `internal/telemetrycollector/metrics.go`, its tests, `cmd/gentle-telemetry/main.go` flag wiring and its tests, `docs/telemetry-collector.md`.
- Out of scope: bucketing provider/model into a closed set (product decision, loses long-tail detail), dashboard changes.
- VPS deploy: authorized by the user per delivery (done for T1+T2; authorized again for T3).

## Constraints
- Counters stay monotonic while a series lives; eviction is a counter reset downstream, already handled by `increase()`/`rate()`.
- TTL must be far above the 15 s scrape interval so the last increment is always scraped before eviction.
- Strict TDD (openspec/config.yaml `strict_tdd: true`). Runner: `go test ./internal/telemetrycollector/... ./cmd/gentle-telemetry/...`.
- Delivery strategy: `ask-on-risk`; forecast ~200 authored lines, single PR.

## Tasks
- [x] T1 — Skip creating a series for a zero delta (existing series unchanged). Route: delegated direct (writer covers T1+T2: 2+ non-trivial files).
- [x] T2 — Idle-series TTL: `lastUpdate` per series, evict idle series during `WriteTo`, injectable clock, `--runtime-metrics-ttl` flag (default 24h, `0` disables), doc update. Route: delegated direct (same writer).
- [x] T4 — Record delivery: T1+T2 pushed directly to `main` (user decision, no PR; the collector is not shipped by goreleaser) and deployed to the VPS. Route: direct inline (this document only).
- [x] T3 — Render before evicting: `WriteTo` renders every live series, including ones past the TTL, then deletes the expired ones, so each series is scraped at least once after its last increment even after a scrape outage longer than the TTL. Route: delegated direct (writer trigger: metrics.go + tests + docs).

## Acceptance criteria
- A delta of 0 on an absent series renders nothing; a delta of 0 on an existing series leaves it rendered unchanged.
- A series not updated for longer than the TTL is absent from the next `WriteTo`; an updated series is kept; TTL 0 never evicts.
- A series evicted and observed again restarts from its new delta.
- A series past the TTL is rendered by the `WriteTo` that evicts it and absent from the next one.
- Docs describe both behaviors and the flag.

## Checks
- `go test ./internal/telemetrycollector/... ./cmd/gentle-telemetry/...`
- `go vet ./internal/telemetrycollector/... ./cmd/gentle-telemetry/...`
- `gofmt -l internal/telemetrycollector cmd/gentle-telemetry`

## Progress
- 2026-09-24: diagnosis done on the VPS (read-only), worktree created, document created.
- T1 commit 0281553d1; review assess (base origin/main, committed-only): medium, under_budget.
- T1 done: zero deltas no longer create series (writer, strict TDD RED->GREEN observed by writer). Checks: go test, go vet, gofmt -l all clean; parent spot check `go test` ok.

- T2 done: idle-series TTL evicted during WriteTo, NewRuntimeMetricsWithTTL, --runtime-metrics-ttl (default 24h, 0 disables, negative rejected), docs. Writer strict TDD RED (build failure on undefined API) -> GREEN. Checks: go test, go test -race, go vet, gofmt -l clean; parent spot check `go test -count=1` ok.
- (Resolved by T3) Former limit: eviction ran before rendering, so if VictoriaMetrics stops scraping for longer than the TTL, increments landed during that outage on series that then went idle are dropped instead of scraped once. Accepted for now; the fix is to render before evicting.

- Delivery: user chose a direct push to `main` without a PR (collector is not in `.goreleaser.yaml`). Pushed 465452ebe..480f2e3bd on 2026-09-24; the ruleset's required status checks were bypassed by the maintainer account.
- Full suite before push: `go vet ./...` clean; `go test ./...` had 4 failures that fail identically on `origin/main` (pre-existing, unrelated): `internal/reviewtransaction` TestStoreLoadsLegacyBoundedLineageAndCompletesFixWithoutNewBudgetSemantics, TestStoreRejectsFreshLegacyShapedBoundedGenesis, TestStoreLoadChainBindsGenesisHeadAndOrderedIdentity; `internal/update/upgrade` TestConfigPathsForBackup_ExcludesPiSessionRuntimeFile.
- Deploy 2026-09-24 15:17 CST: VPS checkout moved to `origin/main`, `install.sh --local-source`, then `systemctl restart gentle-telemetry` (install.sh does not restart a running collector). `/metrics` 50,566,896 -> 511,920 bytes (2,721 series); collector memory 720 MB -> 33 MB; VictoriaMetrics target up. Rollback binary: `/usr/local/bin/gentle-telemetry.bak-20260924`.

- T3 done: WriteTo renders expired series once more and evicts only after a successful write; a failed write keeps them. Writer strict TDD RED (TTL and failing-writer tests) -> GREEN. Checks: go test, go test -race, go vet, gofmt -l clean; parent spot check `go test -count=1` ok.

## Next step
Push, deploy, and a baseline measurement. Re-measure `/metrics` on 2026-09-25/26 to confirm it plateaus.
