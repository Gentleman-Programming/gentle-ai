# Changelog

Cross-cutting notes for gentle-ai capabilities, one entry per change. PR
release notes still live in `docs/releases/v*.md`; this file is the index
for capability additions that span multiple slices or PRs.

## Unreleased

- `model-capability-discovery` capability (foundation): the Codex
  adapter now exposes a synchronous `Capabilities(ctx, lookup, modelID)`
  sibling to `Tier()` that returns a `model.CapabilityRecord` with
  `reasoning`, `speed_tiers`, `service_tiers`, `multi_agent_version`,
  and `capability_source` fields. Discovery is bounded by a 2-second
  timeout and falls back to a curated row for the requested `modelID`
  (or the conservative `unknown` row when the model id has no curated
  entry) whenever the runtime `codex debug models` call is missing,
  times out, parses with errors, or names a slug not in the catalog.
  The picker never blocks. The real `codex debug models` envelope
  `{"models":[{"slug":...,"reasoning":...,"speed_tiers":...,
  "service_tiers":...,"multi_agent_version":...}, ...]}` is parsed
  per-model via `RecordFromRuntimeForModel`; the legacy flat top-level
  shape stays accepted for empty-modelID callers. The curated matrix
  (`gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`) is pinned in
  `internal/model/codex_capabilities_test.go` — `luna` never carries
  `ultra`, and `fast` never appears in `reasoning`. Partition
  validation (`firstOverlap` + `containsValue`) is now case-insensitive
  on both paths so a mixed-case payload cannot let one partition check
  slide past while the other catches it. See
  Gentleman-Programming/gentle-ai#2218.
