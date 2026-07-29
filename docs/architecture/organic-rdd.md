# Organic RDD — architecture and change record

> Technical reference for PR [#1801](https://github.com/Gentleman-Programming/gentle-ai/pull/1801). 154 commits, 340 files, +58,379 / −6,586. For the story behind it, see [the-organic-rdd-story.md](the-organic-rdd-story.md).

## 1. What changed at the top

Review-Driven Development used to be a control plane. A change was routed into a work-run, the run carried capabilities, and the capabilities decided ceremony. That plane was deleted (`feat!: delete the retired work-routing control plane`) and replaced by three ideas that fit in a paragraph each.

**Review happens after the candidate, not before the work.** There is no plan to approve, no run to open. You change something, and if it is worth reviewing, a review is offered on the exact bytes you produced.

**Tier is decided by evidence, never by size.** A thousand-line documentation change is tier 0 and gets no reviewer. Two lines touching authentication are tier 2 and get four. The classifier names its own reason, so the cost is never unexplained.

**The switch is a switch.** `gentle-ai review mode disable` means RDD does not exist: nothing blocks, nothing gates, delivery falls to ordinary repository policy. Turning it back on re-validates from the current state rather than resuming stale obligations.

## 2. The lifecycle

```
review start ──▶ reviewing ──▶ validating ──▶ approved ──▶ gates
     │              │              │              │
   frozen       reviewer       verification    receipt
  candidate      results         evidence      governs
```

Every transition is bound to an immutable candidate identity. Authority never advances on anything but the exact bytes that were frozen.

**`review start`** freezes the candidate, classifies risk, selects lenses, and creates the authority. It renders the frozen reviewer context *before* committing anything, so a candidate that cannot be expressed as reviewer work never becomes an authority.

**`review capture-result`** admits one reviewer result per lens, bound to the frozen subject hash. `gentle-ai review schema reviewer` emits the schema with a working example.

**`review finalize`** consumes captured results, then verification evidence, then reaches a terminal receipt.

**`review validate --gate <gate>`** is the delivery boundary. Gates are `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`. They discover and validate the same receipt and never launch reviewers.

## 3. The negotiated contract

Two output modes, selected by the presence of `--contract gentle-ai.review-integration/v1`:

| | human form | negotiated form |
|---|---|---|
| audience | a person reading a terminal | a tool driving the lifecycle |
| shape | prose refusals, exit codes | typed JSON envelopes |
| carries | the reason | the reason, the code, the next action |

**Mode divergence was the largest defect class in this branch.** The two modes emitted different data, the mode was selected by an invisible flag, and nothing told the caller. Four separate community reports traced back to it. It is now guarded: a conformance test fails CI when contract documentation names a schema, command, or field that no code emits, and a parity test requires the negotiated envelope to be at least as specific as the human surface for the same condition.

### Transitions are literally executable

`next_transition` no longer carries only a dotted operation name. Every argument carries its exact argv `token`, and an `execute` transition carries the complete `command`:

```json
{
  "kind": "execute",
  "execute": {
    "operation": "review.start",
    "command": "gentle-ai review start --contract=... --target=sha256:... --projection=workspace",
    "arguments": [
      { "name": "target", "value": "sha256:...", "token": "--target=sha256:..." }
    ]
  }
}
```

The verb is derived from `reviewIntegrationOperationRegistry`, the same table that already owned the mapping, so there is no second source to drift. Values carrying operator free text are POSIX-quoted, because `review repair` takes `--reason` and `--actor` and joined raw the shell split them into positionals every verb refuses.

A `collect` transition carries tokens too — its arguments *are* the flags of `review capture-result` — but deliberately no `command`, because `--input` points at an artifact that does not exist until a model has run the lens.

## 4. Recovery

The governing rule of this branch, stated once:

> **A message may name a command only if running that command resolves the block.**

Naming a dead end is worse than naming nothing. That rule is the reason several fixes here look larger than the defect that prompted them.

**Scope-changed** at `pre-push` has two classes. One completes assessment and derives diagnostics; the other errors during discovery and previously carried none. Both now name a recovery, and the named recovery is gate-conditional: at `pre-push` over an already-committed delivery, a bare `recover` freezes an empty successor that re-trips the same rule, so the denial names the committed base-diff shape with a derived merge-base rather than a remote ref that can move between reading and running.

One sub-case keeps the honest fallback on purpose: when committed content is byte-identical to what was approved and only commit topology changed, no single `recover` expresses it, and naming a two-step chain whose first step does not clear the gate would repeat the defect.

**Preflight refusals** in the negotiated envelope collapsed into one opaque code with an empty `required_inputs`. The specific reason now travels in `cause`, set once at the collapse point rather than at eighty call sites. A stale snapshot gets its own code and `next_action: review.status`, because `correct_request` is actively wrong there: no edit to the request makes a stale snapshot fresh.

**Git trust refusals** are typed rather than collapsed. gentle-ai never provisions `safe.directory` and never relaxes an ownership check; the fix is diagnostic only, and detection requires three independent signals from one failure so a miss degrades to the generic message and can never mislabel.

## 5. The kill switch

Three consultation points existed in the whole binary, two of them behind `if !negotiated`. Any caller passing `--contract` never received the escape at all, and `internal/sddstatus` consulted it nowhere.

It now reaches: the negotiated gate for every discovery kind, both non-stale ambiguous compositions, corrupted authority, mixed compact/legacy authority, the SDD remediation obligation, and the SDD archive gate.

Three invariants hold while disabled:

- **It never fabricates approval.** `disabled/unmanaged` keeps `allowed: false`. It exits 0 because it defers, not because it approved.
- **It never destroys information.** An outcome the gate could not decide says so and carries its typed cause.
- **An unreadable switch is not a disabled switch.** It resolves to managed, so a damaged or tampered mode record can never manufacture an unmanaged result.

Declining consent deliberately does *not* suppress the gate. The prompt's own off-path text says a decline is not the kill switch, and each decline is scoped to one candidate; making it suppress delivery would silently turn "skip once" into "off".

## 6. Platform work

**Windows self-upgrade.** It never worked: the routing short-circuited to a binary strategy before the Go check. With Go on PATH it now upgrades through a pinned `go install`, verified by the Go checksum database — a different trust anchor than our minisign key, not a missing one. Linux and macOS keep the authenticated binary download, enforced structurally rather than by ordering: gentle-ai routes through a helper whose only go-install exit is gated on Windows, so declaring `GoImportPath` cannot revive the previously-dead generic rule on every platform at once.

On every platform, an upgrade now verifies that `go install` wrote where the user actually executes from, and names both absolute paths on mismatch.

**macOS.** Four defects had escaped because CI has no Darwin lane: `/var` path aliasing, `EPERM` under managed profiles, reviewer-result publication on ExFAT, and first-use store contention. All four are now fixed and, for the first time, verified on real hardware.

**Codex.** The permissions component stopped writing to `~/.codex/config.toml` entirely. It no longer injected a profile; what remained was a migration that removed the `default_permissions` pointer unconditionally while removing the profile table only when empty, so anyone who had customized that profile lost the pointer, kept the table, and Codex refused to start. Probing Codex directly showed every formulation of a surgical fix is also invalid, so the cleanup is gone rather than narrowed.

## 7. The guards

Ten defects in this branch shared one shape: **tests verified something was emitted, never that a consumer could act on it.** `review recover` is a real verb with real flags and the existence test passed, while the recovery it named dead-ended when run.

Four mechanical guards now cover that class, all derived from source rather than hand-maintained lists:

| Guard | What it proves |
|---|---|
| `TestPrePushScopeChangeNamedRecoveryReachesAllow` | reads the recovery out of the frozen diagnostics the denial carries, runs it, requires `allow` |
| `TestEveryNamedReviewContinuationIsStructurallyReal` | AST-walks refusal strings; every named verb and flag resolves against the real dispatch and `FlagSet` |
| mode parity in `review_preflight_reason_test.go` | every distinguishing token of the human refusal is recoverable from the negotiated envelope |
| `scripts/deadcode-ratchet.sh` | fails on a new unreachable function; the 230 already present are frozen |
| [guard population declarations](guard-population.md) | AST-binds eight scoped population claims to production guards and rejects exact registry drift |

The ratchet is a ratchet on purpose. Demanding zero before it could exist would have meant it never existed.

## 8. The friction benchmark

`bench/` is a separate Go module that drives a real `gentle-ai` binary through 36 end-to-end journeys and reports where the operator gets stuck. It is the evidence behind every friction claim in this branch, and it ships so the claims are reproducible rather than asserted.

```
cd bench && go run . run --binary $(command -v gentle-ai)
```

It classifies every block into exactly one class, and the split is the measurement, not the total:

| Class | Meaning |
|---|---|
| `in_band` | the refusal names a command that runs and clears it |
| `out_of_band` | the operator is stopped with nothing runnable named |
| `by_design` | a correct refusal for which no command can honestly exist |
| `dead_end` | nothing resolves it, anywhere |
| `self_recovered` | the flow continued with no extra command |

Two rules keep it from grading itself generously. Mechanical evidence outranks corpus annotation: a named runnable command classifies as `in_band` regardless of what the journey declared. And a `by_design` declaration costs a shape from a closed vocabulary plus the exact substring of the product's own next-action text, which is **verified present in the emitted bytes** before the exemption applies. A refusal with nothing to quote cannot be exempted, so an invalid declaration can only make a block look worse.

Two harness defects found by pointing it at itself are worth knowing about, because both produced plausible numbers:

- A lifecycle gate answering `disabled/unmanaged` at exit 0 was counted as an out-of-band block. It carries `allowed: false` because RDD is declining to express an opinion, not declining the delivery. The kill switch working was being reported as friction the product caused.
- A missing `GIT_TRACE` file was read as unobservable rather than zero, so one journey that legitimately spawns no git erased the subprocess total for the whole corpus.

`bench/README.md` carries the honesty contract: ten entries naming what the instrument does not measure, cannot measure, or measures with a known bias. Current known gap: `human_surface_bytes` varies by a byte or two across runs because `os.MkdirTemp` suffixes vary in length and two journeys quote that path back. Every classification and every count is stable.

## 9. Contract surface

`contracts/review-integration/v1` is published and digest-pinned. Three digests moved in this branch, each deliberately:

| Artifact | Change | Observable by a pinned consumer |
|---|---|---|
| `schemas/status.schema.json` | `command` property, required `token` on execute arguments, corrected `$comment` | added optional properties; strict validators still accept |
| `fixtures/status.fixture.json` | token keys on collect arguments | additive; `name`/`value` byte-identical |
| `schemas/failure.schema.json` | not pinned; gained `cause` usage | none |

`recovery_required_inputs` remains pinned at exactly six entries, which is why the gate-conditional recovery selectors are rendered in the human message and deliberately not projected into the negotiated envelope.

## 10. Known open

- **SDD `verify` still blocks with reviews off.** The archive gate honors the switch; `applyPreVerifyReviewRouting` blocks verify one phase earlier with `next: "review"`, and `review start` refuses. Unblocking it decides whether verify may run with no review at all.
- **The `sdd-archive` assets still require `reviewGate.result: allow`** in prose, so the agent-facing contract blocks where the native projection now allows.
- **`review status` and `--next-transition` do not carry escalation numbers.** `finalize` and the gates do.
- **Reviewer-result authoring is discover-by-iteration strict.** `finding.lens` must be the unprefixed name; supplying the selector's own output string is rejected.
- **`max` reasoning effort does not exist.** The Codex effort type accepts `low`, `medium`, `high`, `xhigh`. Codex itself validates nothing, so an unknown value would be silently ignored rather than rejected.
