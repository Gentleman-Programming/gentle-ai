# Post-Apply Review Ledger

review_stage: post-apply
target_revision: sha256:22087f42def765edcb5d1b2ef2be93f75eba5a711e5ca962269e14f844883785
canonical_ledger: `openspec/changes/pi-optional-codegraph-integration/review-ledger.md`

| id | lens | location | severity | status | evidence |
|---|---|---|---|---|---|
| R4-001 | resilience | `openspec/changes/pi-optional-codegraph-integration/apply-progress.md:269` | CRITICAL | verified | Effective project MCP precedence is now resolved and a broken project override fails closed; remediation GREEN and scoped re-review verified the configured runtime/adapter path. |
| R4-002 | resilience | `openspec/changes/pi-optional-codegraph-integration/apply-progress.md:270` | CRITICAL | verified | Deselect snapshot coverage includes the manifest and manifest-owned overlay; remediation GREEN and scoped re-review verified journal-before-mutation behavior. |
| R4-003 | resilience | `openspec/changes/pi-optional-codegraph-integration/apply-progress.md:271` | CRITICAL | verified | Transactional uninstall restores prior MCP/overlay bytes after cleanup failure and retains the manifest for retry; scoped re-review verified recovery. |
| R4-004 | resilience | `openspec/changes/pi-optional-codegraph-integration/apply-progress.md:271` | CRITICAL | verified | Unreadable managed children fail cleanup before mutation and preserve the manifest; scoped re-review verified fail-closed behavior. |
| R4-005 | resilience | `openspec/changes/pi-optional-codegraph-integration/apply-progress.md:272` | CRITICAL | verified | Reconcile and uninstall return both the triggering failure and journal-restore failure; scoped re-review verified joined errors. |
| R4-006 | resilience | `openspec/changes/pi-optional-codegraph-integration/apply-progress.md:273` | CRITICAL | verified | An unmatched managed marker returns an error while leaving child bytes unchanged; scoped re-review verified renderer integrity. |
| R4-007 | resilience | `openspec/changes/pi-optional-codegraph-integration/reviews/post-apply/review-ledger.md:15` | WARNING | info | Stale package overlays after sync remain informational; no fix or re-review was authorized. |
| R4-008 | resilience | `internal/agents/pi/adapter.go`, `internal/components/communitytool/pi_codegraph.go` | CRITICAL | fixed | Project override MCP resolution retains `$PI_CODING_AGENT_DIR` as the adapter runtime location; focused adapter and runtime-seam tests pass. |
| R4-009 | resilience | `internal/cli/run.go` | CRITICAL | fixed | The Pi reconciliation step participates in pipeline rollback and removes manifest-owned dynamic overlays. |
| R4-010 | resilience | `internal/cli/run.go`, `internal/app/app.go`, `internal/tui/model.go`, `internal/tui/screens/complete.go` | CRITICAL | fixed | Pi CodeGraph manual actions are rendered by both normal CLI completion and TUI completion. |
| R4-011 | resilience | `internal/components/communitytool/pi_codegraph.go:523-568` | CRITICAL | fixed | The Pi runtime probe requires exactly one parsed `codegraph: <state>` record with an exact healthy state. Adversarial runtime-seam tests reject negated, malformed, unrelated, and ambiguous output while accepting explicit `connected` and `ready` states. |

The canonical ledger carries the complete first-pass, refuter, fix, and scoped re-review normalization. This stage-scoped copy remains contract-complete for post-apply consumers. R4-008 through R4-010 are verified; R4-011 is fixed.
