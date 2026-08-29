# Organic RDD — atomic review architecture

← [Back to README](../../README.md)

Receipt-Driven Development (RDD) reviews a finished candidate without taking ownership of repository operations. Native code freezes one local Git candidate, coordinates bounded review, retains acknowledged approval for exact pre-commit validation, and returns control to the human.

## The model

- **Review follows work.** A candidate exists before review begins; the parent asks native STATUS to preflight that current worktree only.
- **Native owns review mechanics.** Go derives risk, frozen trees, lenses, provider bindings, admission, refutation, one bounded correction, repository evidence, and targeted validation.
- **Humans own delivery.** Approval never commits, pushes, opens a PR, or overrides repository policy.

## Atomic transaction lifecycle

**The switch is a switch, and it starts off.** RDD is opt-in: until someone runs
`gentle-ai review mode enable --scope global`, it does not govern the candidate.
Nothing blocks or gates delivery; ordinary repository policy applies. `gentle-ai
review mode disable` returns to that state. Enabling RDD revalidates the current
candidate instead of resuming stale obligations.

```text
selectorless STATUS -> exact START -> bound collection -> approved -> exact acknowledgement -> exact pre-commit gate
```

### Preflight and START

Selectorless STATUS does not scan or resume ambient authority. It preflights the current worktree candidate and returns one exact START invocation. START creates one compact transaction whose lineage, worktree, and target are explicit and immutable.

The parent retains the lineage, revision, and target returned by START. Every subsequent STATUS and capture call uses those exact tokens. An exact active START replay can report `replayed`; a genuinely new START is independent. An acknowledged terminal lineage is never restarted.

This prevents a historical authority, a sibling worktree, or a stale lifecycle response from steering the current candidate.

### Cross-repository root continuity

A session rooted in repository A can review an explicitly user-authorized nested target in unrelated repository B. Go resolves the requested path to B's canonical worktree root; adapters remain opaque and never parse authorization or roots. Once B is selected, the host retains B through STATUS, consent, collection, correction, validation, acknowledgement, and exact pre-commit validation. Provider-issued tokens remain exact; an invocation without `--cwd` runs with process cwd B.

Opaque `repository_context` can materialize or capture from process cwd A, but remains bound to B. Identical lineage text in A and B names independent authority: acknowledgement retains B's approval only and leaves A unchanged. Other repository operations remain human-owned and run in B only.

Only Claude Code, Codex, OpenCode, and Pi receive this lifecycle. Unsupported runtimes fail before repository or authority mutation.

### Review and acknowledgement

Reviewers receive provider-issued immutable context, not live workspace state. Adapters are opaque transport: they do not parse bindings, build prompts, admit findings, or decide workflow state. Only candidate-caused severe findings can block. Native review permits one bounded correction and only a validator that can inspect the frozen trees may return a verdict.

A successful final capture records `approved` plus one exact acknowledgement continuation. That acknowledgement advances the compact revision and clears only its token; the approved authority, candidate binding, and admitted evidence remain the sole durable owner. No receipt mirror, tombstone, witness, or sidecar is created.

Malformed, unavailable, wrong, stale, or replayed operations never become approval. The parent retains the exact lineage, revision, and target, queries bound STATUS, and follows only the returned action.

## Gate behavior

RDD needs local Git only; no remote or upstream. Gate behavior is narrow:

| Request | Result |
| --- | --- |
| Disabled | `disabled/unmanaged` |
| Enabled without exact lineage, or not `pre-commit` | `invalidated/unmanaged` |
| Exact lineage, acknowledgement pending | managed denial |
| Acknowledged lineage, exact staged base/tree/paths | `allow` |
| Acknowledged lineage, staged drift | `scope-changed` denial |

## Delivery boundary

A gate allow authorizes only the exact pre-commit continuation. Commit, push, PR, merge, release, and archive remain governed by repository policy and explicit human authorization. When B was selected from A, any authorized repository operation runs in B only.

## Runtime boundary

The atomic lifecycle is rendered only for Claude Code, OpenCode, Codex, and Pi. Generic and non-RDD runtime guidance keeps ordinary SDD behavior and makes no review-transport promise. Pi receives the lifecycle dynamically over the generic base only when its compiled capability advertises the provider contract.

## Historical compatibility

Older contracts and historical artifacts may be read through explicit manual compatibility operations. They do not participate in the ordinary atomic lifecycle, replace retained compact authority, or decide delivery.
