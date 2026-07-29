# Proposal: Organic DX

## Intent

This release is RDD's debut, so the experience is the product. The user asks for work, the tool asks one natural question — "verify this with RDD?" — and from there the machinery is invisible: validations run, and when the lifecycle blocks or wedges internally, the tool recovers itself. The user must never read "it got stuck, it recovered, now run this". Friction at the moment of failure is what makes people abandon a tool.

Invisible is not silent. The tool narrates its **work** — "running the lenses now", "a lens failed, here is the finding and why, here is what I do next" — confidently and in domain vocabulary. It never narrates **itself**: no receipt/authority/lineage anomalies leaking into the human surface, and no uncertainty or exploration talk ("I'll look into what I need to do"). Either the continuation is known and applied, or the state is genuinely terminal.

Today the opposite happens. This session's audits proved every one of the 10 `--maintainer-authorization` bindings is a deterministic string the CLI derives itself (two commands already emit it via `--prepare`/`--preflight`), so hand-assembly is a typing tax, not a control; one deadlock class survived to HEAD because routing said `stop` where a legal native edge existed; users are escalated by `cumulative_correction_lines`, a number no surface prints; ~82% of CLI refusals name concepts, not commands; and the orchestrator defaults to Interactive, generating prompts its own spec calls unnecessary in auto mode.

## Scope

### In Scope

| # | Stream | Shape |
|---|---|---|
| 1 | Self-driving recovery | all 10 gated commands self-emit their binding; the facade consumes it for the recovery shapes already proven legal, stamping actor from git identity and a machine reason into the same CAS audit trail |
| 2 | No-stop-with-successor invariant | unit + contract test asserting no reachable state emits `stop` while a legal native continuation edge exists; the accounting-only fix is the template |
| 3 | Visible numbers | escalation output carries spent / remaining / total with distinct labels, extending the sddstatus fields to the review CLI surfaces |
| 4 | Narration contract | the three-tier output contract below, replacing the ad-hoc human-vocabulary sweep |
| 5 | Prompt budget | auto becomes the default execution mode; zero prompts after scope approval on the happy path, at most one actionable prompt per recoverable failure, gatekeeper summarizes instead of interrupting |

- Explicit bindings stay accepted; an explicitly wrong binding stays rejected. The one-time consent prompt remains the single human ceremony.

### Out of Scope

- The stop-schema escape/continuation field — a versioned wire-contract change, designed post-release once streams 1–2 shrink the stop surface.
- Identity-bound (signed) authorization: the ed25519 pattern exists (`prepr.go:304-309`) but binding identity is a separate product decision.
- gentle-pi forward tolerance (rides the pin-bump PR).
- Any weakening of fail-closed behavior: corrupted authority, wrong explicit bindings, and failed-criteria escalations refuse exactly as today.
- Scope changes and permanent disable stay human decisions.

## Narration contract (stream 4, requirement-shaping)

Every user-facing emission classifies into exactly one tier.

| Tier | Events | Human surface |
|---|---|---|
| A — domain | lens selection, lens running, findings with reasons, correction being applied, verification outcome | narrated, confident, domain vocabulary — this is the product's voice |
| B — machinery | receipt/authority anomalies, recovery, retries, drift resets, quarantines | never narrated; stream 1 self-recovers, the CAS trail records it, `--json`/verbose exposes it for agents and operators. On successful self-recovery, Tier-A narration continues as if the bump never happened |
| C — terminal | states genuinely needing a human decision | exactly one statement: the outcome in domain terms plus the single decision or command |

- Uncertainty and exploration phrasing ("I'll figure out what to do", "I don't know how to solve this") is banned from every surface, Tier C included.
- Internal identifiers (lineage, ordinal, CAS, facade, receipt) stay in the negotiated/JSON envelopes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `review-findings-ledger`: self-derived recovery authorization, the no-stop-with-successor invariant, visible correction spend, and the three-tier narration contract.
- `sdd-orchestrator-assets`: auto-by-default execution mode and the documented prompt budgets.

## Approach

Streams land in dependency order 1 → 2 → 3 → 4 → 5. Stream 1 generalizes the existing `--prepare` emitter to the eight commands lacking it, then routes it at the facade so the deterministic cases never reach the operator; stream 2 turns the class of unrouted stops into a structural test rather than case-by-case fixes; stream 3 is surface work over already-computed values; stream 4 classifies the emission surface into the three tiers and asserts it; stream 5 is asset text pinned by the existing `assets_test` anchors. RED-first per stream, delivered in PR #1801 under the accepted `size:exception`, riding the live community re-test rc loop.

## Affected Areas

`internal/cli/review_{repair,reconcile,abandon,dispose_result,reopen_results,final_verification_retry,legacy_*}.go`, `internal/cli/review_{facade,next_transition,status_contract}.go`, `internal/assets/*/sdd-orchestrator*.md`, `e2e/organicruntime/`.

## Risks

| Risk | Mitigation |
|---|---|
| Auto-applied recovery weakens an authority surface | negative tests per command proving corrupted state, wrong explicit binding, and active attempt still refuse |
| `actor` shifts from typed-by-human to derived-from-git-identity | accepted trade, stated in the audit trail semantics and release notes |
| The stream-2 invariant reveals more unrouted stops | each becomes a fix, never a test exemption |
| Auto-by-default changes behavior for Interactive users | release notes call it out explicitly |
| Tier-B suppression hides something an operator needed | nothing is dropped: the CAS trail and `--json`/verbose keep every machinery event |
| Tier boundaries drift as new emissions are added | classification asserted by test over the emission surface, not by convention |

## Rollback Plan

Each stream is an independently revertible commit; `git revert` the stream. Stream 1 additionally degrades safely: reverting the facade routing leaves the emitters in place, restoring today's manual flow. The kill switch remains the runtime escape.

## Dependencies

- Community re-test rc loop is live; this change rides the same loop.
- Accounting-only stop fix and the sddstatus budget fields, both landed this session.

## Success Criteria

- [ ] A happy-path run from request to receipt shows exactly one human prompt and zero authorization tokens.
- [ ] Every deterministic recovery shape completes without operator argv, with the same CAS audit entry it writes today.
- [ ] No reachable state emits `stop` while a legal continuation edge exists, asserted by test.
- [ ] Every escalation prints spend, remaining, and total with distinct labels.
- [ ] Paired scenario, one underlying machinery failure: (a) recoverable → the human surface shows uninterrupted Tier-A narration and zero Tier-B output; (b) terminal → the human surface shows exactly one Tier-C decision statement.
- [ ] No human surface prints internal identifiers or uncertainty phrasing.
