# Review Integration Contract

← [Back to README](../README.md)

`gentle-ai.review-integration/v1` is the versioned provider contract for consumers that coordinate Gentle AI's native bounded review lifecycle. It lets a consumer negotiate capabilities, reconstruct one target after restart, drive explicit review operations, and validate the resulting receipt without reading provider-private authority files.

## Negotiate the provider first

Resolve the exact `gentle-ai` executable that will perform review operations, then query it outside a repository:

```bash
gentle-ai review capabilities \
  --contract gentle-ai.review-integration/v1
```

The response identifies the protocol major, package and build identity, executable SHA-256, operations, five gates, projections, schemas, mandatory and optional features, and compatibility window. The executable digest is self-reported evidence; compare it with the published release manifest before trusting the binary.

Consumers MUST reject an incompatible protocol major, an unsupported mandatory feature, an unknown mandatory enum, or a schema identity mismatch. Unknown optional fields may be ignored only under the advertised additive-minor policy. Existing unnegotiated CLI responses remain separate compatibility surfaces and do not gain negotiated fields silently.

Pass the same contract explicitly to negotiated repository operations:

```bash
gentle-ai review start --contract gentle-ai.review-integration/v1 --cwd .
gentle-ai review status --contract gentle-ai.review-integration/v1 --cwd .
gentle-ai review finalize --contract gentle-ai.review-integration/v1 --cwd . --lineage <lineage> ...
gentle-ai review validate --contract gentle-ai.review-integration/v1 --cwd . --gate pre-commit
gentle-ai review bind-sdd --contract gentle-ai.review-integration/v1 --cwd . --change <change> --lineage <lineage> --expected-binding-revision=<revision>
```

## Keep provider and consumer ownership separate

| Gentle AI provider owns | Consumer owns |
| --- | --- |
| Git-derived immutable snapshot identity and projection | User interaction and explicit maintainer confirmation |
| Deterministic risk reasons, tier, lenses, and correction budget | Reviewer, validator, and correction actor execution |
| Compact-v2 authority transitions, lock, and expected-revision CAS | Process invocation, cancellation, and transport diagnostics |
| Receipt derivation and exact receipt publication replay | Rendering native outcomes without weakening them |
| Target applicability, replayability, and gate evaluation | Rechecking command intent immediately before execution |
| Approved-receipt binding for SDD | Derived worktree and temporary-view lifecycle |

Consumers MUST NOT reconstruct receipts, derive canonical hashes, inspect the Git common-dir authority store, select an ambiguous lineage automatically, or infer that a transport interruption did not mutate state. Gentle AI does not choose models, run arbitrary user commands, or replace a consumer's command-safety policy.

## Drive the bounded operation set

| Operation | Mutation boundary | Contract behavior |
| --- | --- | --- |
| `review.capabilities` | None | Reports the deterministic repository-independent provider surface. |
| `review.start` | Compact authority | Freezes one target, tier, lens set, and correction budget; it never starts because a gate was invoked. |
| `review.status` | None | Reconstructs target-scoped applicability, projection, lifecycle, and one next action. |
| `review.finalize` | Compact authority and derived receipt | Accepts the selected lens results and bounded correction evidence, or performs an exact receipt-publication replay. |
| `review.validate` | None | Revalidates one existing content-bound receipt at a named lifecycle gate. |
| `review.bind_sdd` | SDD binding artifact | Binds only an approved receipt to an SDD change. |

`review.start` is the only ordinary entry point that creates a review budget. Finalize continues that frozen lifecycle. Status, validation, and gates are read-only and never allocate a reviewer, actor, lineage, or correction budget.

### Validate exactly five gates

| Gate | Required boundary |
| --- | --- |
| `post-apply` | Revalidate the implemented candidate against the terminal receipt. |
| `pre-commit` | Revalidate the intended staged candidate before commit. |
| `pre-push` | Revalidate the committed candidate before publication. |
| `pre-pr` | Revalidate the candidate, selected remote base, and compatible-base evidence before opening or updating a PR. |
| `release` | Revalidate the immutable release tree, configuration, generated manifest, provenance, publication boundary, and evidence freshness. |

There is no `archive` gate. An advisory preflight is not delivery authorization; the native live gate result is authoritative.

### Follow applicability and action, not inventory

| Applicability | Meaning |
| --- | --- |
| `current_target` | Exactly one validated authority applies to the requested Git target. |
| `unrelated` | No authority applies, even when unrelated historical authority exists. |
| `ambiguous` | More than one authority applies or a required lineage selector is missing. |
| `corrupted` | Authority required for classification cannot be validated safely. |

The provider returns one action from `start`, `finalize`, `validate`, `recover`, `maintainer_action`, `select_lineage`, `repair_authority`, or `stop`. Applicable non-terminal legacy-v1 authority always returns `stop`; it never recommends finalize or another mutation. A consumer MUST stop on ambiguity, corruption, invalidation, escalation, or `stop` and present the provider's required action. It MUST NOT silently choose, reset, quarantine, or create a lineage.

### Preserve the uniform failure envelope

Every failed operation explicitly negotiated through `gentle-ai.review-integration/v1` emits `gentle-ai.review-integration.failure/v1` and still exits nonzero. Capabilities uses this envelope by default; repository operations use it when `--contract` is present. Unnegotiated command errors retain their compatibility behavior.

| Field | Runtime meaning |
| --- | --- |
| `operation`, `phase`, `code`, `message` | Stable operation identity, failure boundary, machine code, and bounded package-controlled message. |
| `mutation_outcome` | Exactly `not_started`, `unknown`, or `committed`; uncertainty is never weakened to a no-mutation claim. |
| `authority_applicability` | `current_target`, `unrelated`, `ambiguous`, `corrupted`, or `not_evaluated`. |
| `retry_safe`, `replayability` | Independent retry and replay safety. Unknown mutation requires status; exact replay requires the declared identity. |
| `lineage_id`, `request_digest` | Present only when the provider has safe canonical replay evidence. |
| `required_inputs`, `next_action` | The bounded input names and one safe follow-up action. |

Messages never contain authority or receipt paths, locks, tokens, raw provider stderr, or canonical store bytes. Invalid or unsupported explicit contracts fail before mutation through the same envelope. A negotiated gate denial is a failure envelope, not a successful operation result; gate evaluation remains read-only.

## Reconcile interruptions before replay

| Replayability | Consumer behavior |
| --- | --- |
| `not_replayable` | Do not repeat the mutation from transport evidence alone. |
| `exact_replay_safe` | Replay only the provider-declared canonical request with every required input unchanged. |
| `status_required` | Run target-scoped status before deciding whether any replay is safe. |
| `manual_action_required` | Stop and obtain the named maintainer action or repair prerequisite. |

Finalize commits terminal compact authority before publishing its derived receipt. If receipt publication fails after that commit, the failure envelope reports `mutation_outcome: committed`, `exact_replay_safe`, the lineage, and the canonical request digest. That declaration permits the exact explicit-lineage finalize replay with no new review inputs; target status independently reports the same publication-pending condition after restart. The replay derives the same receipt bytes and does not mutate authority or open another budget.

An ambiguous or lost transport result is never proof of `not_started`. Reconcile it with `review.status`; do not launch another reviewer, correction, or lineage while the outcome is unknown.

## Preserve compatibility without reopening legacy mutation

Compact-v2 is the sole ordinary mutable authority. Legacy-v1 is in an active, release-based compatibility window with these guarantees:

- Valid applicable historical receipts remain readable and evaluable at supported gates.
- Ordinary legacy mutation through START, finalize, BIND-SDD, invalidation, and direct append—including the `review-step` compatibility route—returns the typed `LegacyReadOnlyError`, preserves `errors.Is(ErrLegacyReadOnly)`, and exposes stable code `legacy_v1_read_only` without changing authority bytes across retries or restarts.
- Negotiated wrappers preserve that typed cause as `legacy_v1_read_only` with `mutation_outcome: not_started`, retry and replay disabled, `next_action: stop`, and a package-controlled message that contains no provider paths or raw diagnostics.
- Applicable non-terminal legacy status returns the deterministic read-only action `stop`; applicable approved legacy receipts remain evaluable at supported gates.
- Applicable approved legacy status validates the canonical published v1 receipt and reports its SHA-256 identity as `present`. Legacy-v1 never reports `publication_pending`; a missing, corrupt, or wrong legacy receipt fails closed as corrupted authority without compact exact-replay semantics.
- Frozen tier, authored-line count, and correction budget are compact-v2 fields. Historical `ordinary_4r` legacy status omits `frozen` rather than inventing values; compact current targets still require the complete frozen object.
- Unrelated valid legacy history does not block a current compact target.
- Same-lineage mixed v1/v2 authority and unclassifiable corruption fail closed.
- Explicit maintenance transport import/export may preserve historical compatibility.
- Removal is not scheduled and requires at least one compatibility release plus separate reachability evidence.

The provider does not auto-upgrade, migrate, rewrite, quarantine, or delete legacy authority. A later deletion is a separate compatibility decision, not part of protocol v1 negotiation.

## Respect compatibility and non-goals

Protocol v1 supports `workspace` and `staged` projections and preserves existing compact authority and receipt schemas. Published archives contain the versioned JSON Schemas and conformance fixtures under `contracts/review-integration/v1/`; consumers should validate against those packaged bytes rather than copying private Go structs.

This contract does not implement Gentle Pi, select a model or provider, transmit repository data, add remote telemetry, claim Windows runtime durability, define an archive coordinator, defend against a malicious actor with local filesystem access, or authorize a command merely because review passed.

## Consume the contract from Gentle Pi

Gentle Pi should remain a thin consumer:

1. Resolve and independently verify the exact Gentle AI executable.
2. Negotiate capabilities before repository work and cache them only for that executable identity.
3. Use negotiated status to reconstruct the provider-selected projection after restart.
4. Execute reviewers and validators, then pass their typed results to finalize without constructing authority bytes.
5. Preserve native actions, gate results, replayability, and mutation outcomes without semantic remapping.
6. Reconcile uncertain mutations through status before an exact replay.
7. Keep command interception, worktrees, user confirmation, and final intent rederivation on the Pi side.

Pi adoption, fallback retirement, package pinning, and Pi release sequencing are separate consumer work. They do not change Gentle AI's provider authority or release ownership.

## Inspect packaged contract artifacts

Each release archive contains:

- `contracts/review-integration/v1/schemas/` — six strict JSON Schemas.
- `contracts/review-integration/v1/fixtures/` — eight deterministic conformance fixtures, including all four target-applicability states.
- `docs/review-integration.md` — this ownership and consumption guide.

Repository maintainers can verify source inventory or a complete GoReleaser snapshot:

```bash
scripts/test-review-contract-package.sh
scripts/test-review-contract-package.sh dist
```

The archive assertion compares every packaged contract file with the repository source by SHA-256 and verifies each platform archive against `checksums.txt`.

### Next steps

- Read the [review authority threat model](review-authority-threat-model.md) before integrating delivery authorization.
- Query `review capabilities` from the exact executable you intend to run.
- Validate the packaged fixtures before implementing or updating a consumer.

← [Back to README](../README.md)
