# #4379 Typed managed-assets refusal on mutation paths

Fix Gentleman-Programming/gentle-ai#4379: the shared managed-asset gate behind
`review capture-result` (and every other authority-mutating verb) returns an
untyped error, so collect captures report `operation_outcome_unknown` instead
of the typed `managed_assets_outdated` refusal START and STATUS already emit.

## Specs

S1. Quote (issue #4379, "Suggested fix"): "When sync and capture resolve to
different binaries (global vs pinned relay), either warn with both resolved
paths + digests, or make the error message name the two digests so the
mismatch is diagnosable instead of looping on `run gentle-ai sync`."

S2. Quote (claim comment issuecomment-6194183960, scope item 1): "Return the
shared gate's refusal as the same typed preflight refusal with sync
continuation that START uses. Every verb behind it (capture-result, abandon,
recover, repair, bundle import, ...) then reports a not-started refusal with a
runnable continuation instead of an unknown outcome."

S3. Quote (claim comment, scope item 2): "Name both digests (recorded vs. this
binary's embedded) and the refusing binary's path in the refusal, as you
suggested, so a global/pinned binary split is visible at a glance."
Note: the refusing binary's path already travels unscrubbed in
`continuation.command` (#4434); `cause` redacts absolute paths, so the digests
go in the error text and the path rides in the continuation.

S4. Quote (claim comment, scope item 3): "Tests at the gate plus an end-to-end
capture-result case pinning the envelope shape."

S5. Quote (claim comment, out of scope): "recording the digest per Pi home.
That's #5528 (@ElCaaarnal), and this change stays compatible with it."

S6. Contract guard (discovered 2026-10-15, see L2): a failure published under
the v1 contract must satisfy the published v1 failure schema, which forbids
`continuation`. `managed_assets_outdated` carries the continuation only under
v2.

## Tasks

- [x] T1 (S2, S4) inline: typed refusal from `authorizeManagedReviewerAssets`; RED e2e capture-result + gate classification. Commit: `ff6baacaf`.
- [x] T2 (S1, S3) inline: refusal text names recorded and embedded digests. Commit: `a97c5ec97`.
- [x] T3 (S6) inline: contract-aware continuation (v2 only) in the preflight envelope and `Validate`. Commit: `c81b7495d`.
- [x] T4 (all) inline: full `go test ./...`, gofmtcheck, diff check, docs sweep (`docs/review-integration.md` stop-reason row). Commit: docs + this document (see git log).

## Log

L1. User request (2026-10-15): "Adelante con el #4379 entronces" (after the
claim comment scoped the work, see S2-S5).

L2. Probe (2026-10-15): on main `89f15238`, START with `--contract
review-integration/v1` and a stale digest emits a failure carrying
`continuation`, rejected by the published v1 schema ("additional properties
'continuation' not allowed"). Pre-existing for START; T1 alone would extend it
to negotiated `repair` under v1, which today emits a valid
`operation_outcome_unknown`. Hence T3.

L4. Evidence (2026-10-15):
- T1 RED: `TestCaptureResultClassifiesStaleManagedAssetsAsTypedRefusal` got
  `native_running`/`operation_outcome_unknown`/`mutation_outcome: unknown`,
  no continuation (the reported shape); `TestManagedAssetsGateRefusalIsTypedForEveryCaller`
  got `*errors.errorString`. GREEN after the gate returned
  `reviewPreflightRefusalWithContinuation`; existing managed-asset guards
  (START, STATUS, #4504 resume, executable anchoring, abandon/bundle-import
  boundary) passed.
- T2 RED: gate text lacked the digests. GREEN with
  `managedAssetProvenance.refusal()` used by the gate and START; START's exact
  cause assertion updated to the new text (requested, shared seam). No
  consumer matches the exact text (grep of gentle-ai and gentle-shell); the old
  sentence stays as prefix.
- T3 RED: START v1 and repair-gate v1 envelopes carried `continuation`;
  Validate accepted a v1 continuation. First GREEN attempt broke T1 because
  collect captures are upgraded to v2 by `runReviewCollectCaptureCommand`
  after classification; condition is now "contract v2 or CollectCapture".
- Focused matrix (provenance, START, STATUS, transport, provider artifact,
  failure contract, capture refusal envelopes): ok. Docs tests
  (`TestEveryReviewStopReasonCodeHasADocsContinuation` family,
  `TestReviewIntegrationDocumentationMatchesRuntimeContract`): ok.
- Full suite: `go test ./...` on the branch fails only in
  `internal/shellinstaller` (TestUserFrozenLockAcquisition,
  TestUserManagerCapabilityHelperUsesExecutableCustody) and
  `internal/reviewtransaction` (TestSnapshotIndexTemporaryPlacementAndCleanup,
  TestSnapshotInspectionPreservesGitDirectoryMetadataWithUsableProcessTemp).
  The identical four fail on a detached `upstream/main` worktree run
  sequentially on this host (umask 0002 temp dirs), so they are pre-existing
  and environmental. `internal/cli` exceeds the default 10m package timeout on
  this host on both sides; with `-timeout 40m` it passes on the branch
  (716s). Every other package: ok.
- Follow-up noted, not changed: the static reason message says "before
  starting review", which reads oddly on a capture refusal.

L3. Facts: capture verbs publish only v2 failures (`Validate`); abandon,
recover and bundle import are not negotiated (plain CLI error, text unchanged
plus digests); `--preflight` on capture-result skips the gate.
