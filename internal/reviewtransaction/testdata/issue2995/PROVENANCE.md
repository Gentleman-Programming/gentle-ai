# issue #2995 released-record fixtures

These two `review-state.json` files are byte-identical copies of compact
review authority produced by the released gentle-ai v2.2.0 binary. They are
forensic evidence for issue #2995 (classification honesty for released
v2.2.x records): a genuine CLI transaction wrote them, and every hash below
pins the exact bytes. Never edit them in place; a drifted fixture proves
nothing.

## Release provenance

- Released asset: `gentle-ai_2.2.0_linux_amd64.tar.gz`
- Released asset SHA-256: `56efe0611c2af21913cd7fa768e05a0e6130d026a4157b909a5227e0e0a3b266`
- Tag: `v2.2.0` (commit `ee83e83d56f0d149c52f93fd13b3296858f5147f`)

## Fixtures

### released-v2.2.0-approved-review-state.json

- Record SHA-256: `bdc0db81f30867ba11eb6179f68d8a76651d77c4b0af668dabd855c23961ccbb`
- Lineage: `review-a39b858db5f00bbb`
- Terminal state: `approved`, with the retired `lens_results` array
  non-empty (one admitted lens result) and retired
  `findings`/`classifications`/`outcomes`/`follow_ups` projections present
  and empty.
- Produced by a real v2.2.0 CLI transaction: ordinary `review start`, a
  headless `review capture-result` for the single selected lens, then
  `review finalize` to approval.

### released-v2.2.0-escalated-review-state.json

- Record SHA-256: `3d9a686b5a9f43abcb8b474e44986f8b72e57631cf851619096eefaeba854b12`
- Lineage: `review-a3118da0f4a4c425`
- Terminal state: `escalated`, carrying the retired top-level
  `result_dispositions` array (one audited disposition) that v2.5.0 deleted
  from the schema.
- Produced by a real v2.2.0 CLI transaction: ordinary `review start`, a
  headless `review capture-result` whose preserved result was then rejected
  through `review dispose-result`, escalating the lineage terminally.

## Why the SHA-256 pins matter

Each record's `revision` field binds the exact persisted state bytes under
the `gentle-ai.review-state/v2` formula, and the record SHA-256 above binds
the whole file. Any edit to these fixtures breaks both pins, so a test
failure against them always means the reader changed — never the evidence.
