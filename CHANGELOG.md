# Changelog

Cross-cutting notes for gentle-ai capabilities, one entry per change. PR
release notes still live in `docs/releases/v*.md`; this file is the index
for capability additions that span multiple slices or PRs.

## Unreleased

- `model-capability-discovery` capability (foundation): the Codex
  adapter now exposes a synchronous `Capabilities(ctx, lookup)` sibling
  to `Tier()` that returns a `model.CapabilityRecord` with
  `reasoning`, `speed_tiers`, `service_tiers`, `multi_agent_version`,
  and `capability_source` fields. Discovery is bounded by a 2-second
  timeout and falls back to a curated `gpt-5.6-sol` row whenever the
  runtime `codex debug models` call is missing, times out, or parses
  with errors; the picker never blocks. The curated matrix
  (`gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`) is pinned in
  `internal/model/codex_capabilities_test.go` — `luna` never carries
  `max` or `ultra`, and `fast` never appears in `reasoning`. See
  Gentleman-Programming/gentle-ai#2218.
