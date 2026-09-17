# External research: retain native authority, delete duplicate interpretation

**Outcome: done; proposal readiness remains false for parent evidence/gate readback.** Spec Kit now has a native workflow engine as well as agent-mediated commands. The useful precedent is not “replace native workflow with prompts”: it is sharing executable definitions and separating dispatch, content, and evidence. Its resume implementation also supplies concrete counterexamples to equating saved progress with exactly-once execution.

```yaml
schema: gentle-ai.sdd-research/v1
revision: 3
change: sdd-runtime-simplification
tracker: https://github.com/Gentleman-Programming/gentle-ai/issues/4484
artifact_store: openspec
request_id: sdd-runtime-simplification-external-r1
supplemental_request_id: sdd-runtime-simplification-external-r1-quality-addendum
reuse_request_id: sdd-runtime-simplification-external-r1-reuse-addendum
outcome: done
proposal_ready: false
proposal_gate: parent evidence and confirmed-constraint readback required
skill_resolution: paths-injected
skills:
  injected: /home/gentleman/work/gentle-ai/skills/cognitive-doc-design/SKILL.md
  phase_fallback: internal/assets/skills/sdd-research/SKILL.md
internal_context_only: [preproposal.md, exploration.md, evidence.md]
admission:
  outcome: admitted
  documentation:
    outcome: done
    observed_grants: [fetch_content]
    exercised: [fetch_content]
    denial_reason: null
  open-web:
    outcome: done
    observed_grants: [web_search, source_check, fetch_content, get_search_content]
    exercised: [web_search, source_check, fetch_content, get_search_content]
    denial_reason: null
  recovered_failures: GitHub API throttling, missing source paths, intermittent search failures
  validation_method: direct inspection of fetched original passages and implementation
  automated_source_check_verdicts: unclear; never represented as positive verification
product_decisions:
  status: confirmed
  authority: preproposal.md#confirmed-product-constraints
  supplemental_authority: preproposal.md#current-session-scope-addendum
  new_decisions: none
questions:
  R1: {outcome: answered, claims: [C1, C2, C3, C4, C5, C6, C7]}
  R2: {outcome: answered, claims: [C3, C4, C5, C6, C7, C8]}
  R3: {outcome: answered, claims: [C6, C7, C9, C10]}
  R4: {outcome: answered, claims: [C8, C11, C12, C13]}
  R5: {outcome: answered, evidence: 'C1–C13; recommendations and counterexamples below'}
  R6: {outcome: answered, claims: [C14, C15, C16, C17, C18, C19], evidence: 'S13–S16 and existing S3–S7'}
  R7: {outcome: answered, claims: [C20, C21, C22, C23, C24, C25], evidence: 'S17–S21; existing S2–S7/S15; internal evidence.md; non-authoritative reuse assessment below'}
```

## Questions retained verbatim

- **R1 — Spec Kit today:** What is present in the latest verifiable official Spec Kit release and current upstream source, what materially changed recently, and how are phase prerequisites, state discovery, artifact dependencies, execution, and recovery implemented? Pin release/tag/commit and retrieval time; distinguish released behavior from main-only changes.
- **R2 — Deterministic boundary:** Which Spec Kit mechanisms are enforced by executable code and which rely on agent instructions or human invocation? What can Gentle AI reuse or delete while retaining a deterministic workflow? Do not equate deterministic helper scripts with deterministic end-to-end phase dispatch, and do not claim the whole upstream project is nondeterministic without scoped evidence.
- **R3 — Finite execution and recovery:** What independently documented patterns support idempotent admission/settlement, lost-response reconciliation, interruption, and truthful failure without duplicate side effects or a second workflow authority? Identify the limits of exactly-once guarantees and which facts must remain durable.
- **R4 — Shared contracts and evidence subjects:** What official precedents support producer-derived consumer contracts, explicit schema evolution, and evidence bound to the actual artifact or command subject? Identify where these ideas remove duplicated interpretation and where adoption would introduce unnecessary infrastructure.
- **R5 — Recommendations and counterexamples:** For each useful external pattern, map adopt/adapt/reject to a concrete Gentle AI/Pi deletion or preserved invariant. Identify counterexamples to unsafe simplifications, and state remaining evidence gaps rather than converting analogy into implementation authority.

## R1: latest verified release and source novelty

| Reference | Observed identity | What this proves |
|---|---|---|
| Official publisher | GitHub repository `github/spec-kit`; release HTML identifies owner `github`, repository `1042367133`, `is_fork=false`; README links `github.github.io/spec-kit` | First-party upstream identity, not a similarly named fork. [S1] |
| Latest endpoint | `v1.0.6`, release ID `386307373`, `draft=false`, `prerelease=false`, published `2026-09-10T13:27:05Z` | Latest release returned by the official API during collection. `target_commitish=main` alone is not a commit pin. [S1] |
| Release source | `96c9bd657bfd5de0d651a6165084932b7304ac99`, linked by the official v1.0.6 release HTML | Exact release commit; API also reports `immutable=false`, so retain the commit rather than trust a tag forever. [S1] |
| Current observed main | `c173bf19a6654e3b05386ec3599349a55282b897`, first entry on official main commit listing, dated September 10, 2026 | Snapshot of main, not a promise about future HEAD. Release page showed eight commits since release. [S2] |
| Main-only deletion example | `ce593cdcb10dee03fa48058e2767c70e8566734a`, present in that main history after release | Expression validator now asks the evaluator for leaves rather than maintaining another grammar walker. Exact main source confirms the mechanism. [S2] |

**Recent released novelty, not a complete release survey:** the release-pinned changelog records 1.0.0 on August 21; 1.0.3 adds `--require-spec`; 1.0.5 adds workflow slots and removes an unused bundled workflow input; 1.0.6 adds per-step integration configuration, reports unreadable hook YAML, caps generated event-dispatcher stdin, and repairs bundler rollback registry reads. The last two are release-note observations only, not implementation audits. [S1]

Implementation inspected for the load-bearing changes: command steps resolve `integration_args`/`integration_options`, validate through the integration, record the resolved configuration, and dispatch; the implement template reports unreadable `extensions.yml` **then continues**. Thus “reports the error” does not mean “fails closed.” [S4, S7]

**Present in the release, not asserted newly introduced in 1.0.6:** explicit feature context, prerequisite JSON, native YAML workflows and gates, persisted run state, typed shell/command results, and the `converge` template. The bundled workflow is version `1.0.1` with schema `1.0`, distinct from CLI release `1.0.6`. It lists `specify → review-spec → plan → review-plan → tasks → implement`; it does not include `converge`. The README quickstart separately recommends repeating implement/converge. These are different entry paths, not evidence of one universal dispatcher. [S1, S3–S7]

Main's catalog tag-pinning change at `c173bf1` and JSON list-output change at `8b5ea9019696688449e297e542975eeb1e1fa07c` are **commit-list observations only**. No behavior claim or adoption decision relies on their titles. The source-audited main novelty is the evaluator/validator consolidation. [S2]

## R2: Spec Kit vs Gentle AI deterministic boundary

Gentle AI entries below are the parent's confirmed constraints/internal audit, not claims independently established by external sources.

| Boundary | Spec Kit observed behavior | Gentle AI boundary to preserve / deletion opportunity |
|---|---|---|
| Feature discovery | Bash resolver uses `SPECIFY_FEATURE_DIRECTORY`, then `.specify/feature.json`, otherwise errors. `SPECIFY_FEATURE` supplies a label, not a directory. `--no-persist` suppresses the context write. [S3] | One native resolver; Pi receives resolved identity/store. Delete consumer rediscovery, not worktree/candidate bindings. |
| Read-only inspection | `--paths-only` calls resolution with `--no-persist` and exits before prerequisite validation. Ordinary resolution can persist context. [S3] | Separate status inspection from execution authorization. Do not copy filesystem state as authoritative admission. |
| Artifact prerequisites | Executable checks require feature directory and plan; optional flags require spec/tasks. Optional documents are discovered by existence. [S3] | Keep native artifact dependency predicates; existence alone cannot prove validity, freshness, consent, or completion. |
| Interactive phase flow | Templates instruct the agent to run scripts, inspect checklists, parse task dependencies, execute and mark tasks, invoke hooks, and report completion. [S4] | Models author content, never choose lifecycle transitions. Delete prompt/Pi phase routers, retain content instructions. |
| Native multi-step flow | YAML engine dispatches registry-selected step types, branches/loops, maps typed outcomes, and persists progress. Bundled YAML defines order and review gates. [S5–S7] | Spec Kit also has executable routing. Gentle AI's differentiator must remain **its enforced admission/settlement invariants**, not an inaccurate claim that Spec Kit has no engine. No second engine in Pi. |
| Content versus success | Command step maps integration exit zero to `COMPLETED`; semantic task/checklist completion is instructed in templates. [S4, S7] | Process success is evidence, not automatic phase acceptance; native validation decides admission/transition. |
| Recovery | Engine accepts paused/failed resume, reloads saved YAML/inputs/results, and re-executes current top-level step. Nested pause replays parent/body. [S6] | Native recovery must reconcile the existing attempt before authorizing a successor. Do not replace CAS history with a cursor. |
| Persistence/concurrency | RunState uses in-process locks and individually atomic JSON replacements; state/inputs/log are separate writes. [S6] | Preserve immutable common-directory authority and CAS. Atomic file replacement is not cross-process admission or multi-file transactional settlement. |
| Permission and hooks | Workflow `requires` is advisory, not a sandbox; templates can continue after unreadable hook configuration; shell interpolation is unescaped. [S4–S7] | Preserve real authorization and consent binding. Reject model-built shell routes and fail-open treatment of selected required capabilities. |
| Remediation | Converge prompts append traceable tasks and recommend implement; they do not establish Gentle AI's failed-evidence-bound remediation semantics. [S4] | Preserve a distinct bounded typed remediation executor; unsupported never means apply. |

## R3: finite execution, reconciliation, and truthful closure

### What the sources establish

AWS describes caller-supplied request identity, semantic replay, parameter/intent matching, and atomic coupling of deduplication with mutations. Stripe provides an independent operational contract: replay saved status/body including errors, reject changed parameters, bound key retention, and treat server-error effects as potentially indeterminate. [S8, S9]

Spec Kit provides **bounded examples, not end-to-end exactly-once proof**: shell timeout defaults to 300 seconds, invalid/non-finite timeouts are rejected by a shared helper, timeout/nonzero exit becomes a typed failure, and workflow loops have an iteration cap. Its engine maps KeyboardInterrupt to paused, ordinary exceptions to failed, and permits resume only from paused/failed. [S6, S7]

### Application to existing authority — recommendations, not adopted schema

| Boundary / failure | Truthful native behavior to design | Durable facts that cannot be replaced by prose |
|---|---|---|
| Admission response lost | Reconcile or replay the same request against native authority before launching anything; changed subject/intent is not the same request. [S8, S9 analogy] | Stable operation/attempt identity, admitted action and authority revision, candidate/worktree identity, relevant consent and request parameters. |
| Settlement committed, response lost | Read/replay the authoritative settlement; do not execute work or spend/refund budget again to obtain an acknowledgement. [S8, S9 analogy] | Settlement identity, predecessor/CAS linkage, finite outcome, real evidence/diagnosis and accounting. |
| Worker crashes or times out before settlement | Record interruption/failure only with facts supported by observation; reconcile process/cleanup and effects before a native-selected recovery. An unknown effect remains unknown. [S6–S9 analogy] | Admission record, command/subject, process outcome where known, cleanup state, evidence location/identity, unresolved-effect diagnosis. |
| External mutation happened but acknowledgement did not | Use the target's idempotency/reconciliation mechanism if it exists. Without it or atomic coupling, report uncertainty and block unsafe replay; never invent successful cleanup. [S8, S9] | External operation/correlation identifier and original parameters, returned result if available, reconciliation evidence. |
| No Git diff | Still account for admitted execution. A failed command can have remote effects, and evidence can be about a command/artifact outside the changed-files diff. [S9, S11, S12 analogy + confirmed constraint] | Attempt and actual subject; existing bounded refund predicates, not a new semantic-work exemption. |

**Exactly-once limit:** a single CAS-protected native settlement can be at-most-once in its own authority domain. That does not make an arbitrary subprocess, remote mutation, and ledger write one transaction. AWS's atomicity precondition is precisely why a local marker is insufficient; Stripe's cached `500` demonstrates that repeated identical responses need not mean no effects. Retention expiry and reuse of a new key can permit another operation. “Exactly once” requires a specified domain, durable identity, deduplication lifetime, and atomicity/reconciliation assumptions—not a blanket execution promise. [S8, S9]

**Truthful finite outcome is not guaranteed eventual success:** bounded execution may terminate failed/interrupted with unresolved external effects and a blocked recovery. It must not terminate passed merely because a timeout, empty output, no diff, or optimistic retry exhausted the budget. This is a recommendation under confirmed constraints, not an upstream guarantee.

## R4: one producer contract and evidence about the real subject

- **Producer-derived interpretation:** main's evaluator exposes its actual leaves to validation, deleting the second grammar walk. The released workflow engine already obtains valid step types from a registry and delegates step-specific validation. These are direct deletion precedents, not a suggestion to add an expression DSL. [S2, S6, S7]
- **Explicit contract evolution:** OpenAPI describes machine-readable contracts usable for client/server generation and separates specification version from API version. Its discriminator is only a hint and MUST NOT change validation results. Adapt these ideas to one AI-owned versioned native action/result contract and producer fixtures; do not adopt HTTP/OpenAPI infrastructure merely to describe a CLI. Exact format and compatibility floor remain design work. [S10]
- **Evidence subject:** in-toto Statement v1 requires a digest for each subject and explicitly matches artifacts by digest rather than name. SLSA separates a build definition, externally controlled parameters, resolved dependency revisions, run identity, and byproducts. Adapt those distinctions to the existing candidate/action/evidence bindings; a filename or report about another worktree is not equivalent evidence. [S11, S12]
- **Schema policy is domain-specific:** SLSA permits monotonic envelope extensions but recommends rejecting unexpected external parameters. Its extension rules say ignoring an extension SHOULD NOT turn DENY into ALLOW. Do not copy permissive unknown-field handling onto mutation-authorizing action tags, remediation, consent, or evidence requirements. [S12]
- **Delete redundant facts, not irrecoverable facts:** derive current readiness/action/status from native records; derive consumer types/fixtures from the producer contract; retain durable planning documents and irrecoverable attempt/evidence/consent history. SLSA explicitly recommends making boilerplate implicit and keeping useful non-reproducible byproducts, not storing every intermediate file. These are narrow analogies, not approval of a signing framework or another ledger. [S10, S12]

## R5: adopt / adapt / reject matrix

All entries are non-authoritative design recommendations for the parent, within its confirmed constraints.

| Pattern | Recommendation | Concrete deletion or preservation in AI/Pi | Evidence / unsafe counterexample |
|---|---|---|---|
| Single executable interpreter | **Adopt principle** | Remove Pi's phase vocabulary/arity copies and prose routing; derive from the AI producer. Consolidate shared native readiness predicates. | S2, S6: a second parser misses a grammar change even when both look reasonable. |
| Pure resolution versus admission | **Adapt** | Remove irrelevant review/delivery execution gates from read-only status; keep launch gates at native mutation boundary. | S3: ordinary path resolution writes feature state; naming a command “status” is insufficient. |
| Native typed step dispatch | **Adapt** | Expose one native-admitted action and bounded typed consumer outcome. Delete competing Pi dispatcher. | S5–S7: executable dispatch exists upstream, but exit zero alone is not semantic acceptance. |
| Shared timeout validation and typed failure | **Adopt principle** | Reuse existing native validation/execution rules instead of separate permissive consumer checks. Preserve finite failed/interrupted outcomes. | S7: bool/non-finite timeout acceptance breaks bounds; a timeout says nothing about rollback. |
| Idempotent admission/settlement replay | **Adapt to existing ledger** | Delete retry-by-new-attempt/consumer reconstruction where canonical replay is available; preserve identity/CAS/accounting. | S8, S9: same payload can mean two intended operations; new key after lost response can duplicate effects. |
| Saved cursor as execution authority | **Reject** | Do not replace immutable common-directory records with mutable status/step files or a second Pi ledger. | S6: nested resume repeats parent body; process death can leave `running`, which engine resume rejects. |
| Atomic rename as settlement proof | **Reject** | Keep CAS and full subject/consent/evidence binding. | S6, S8: state and inputs are separately replaced; external effects are outside those file writes. |
| Traceable unmet-work findings | **Adapt content only** | Models may author evidence-linked remediation tasks; native code admits bounded remediation and fresh verification. | S4: converge's append/recommend loop is not authorization to alias remediate to apply or reset budgets. |
| Governed producer schema and fixtures | **Adapt** | Replace independently maintained exact-three decoder assumptions; consumer rejects unsupported authority-bearing actions explicitly. | S10, S12: discriminator hints and ignored extension fields cannot supply authorization. No universal compatibility reader. |
| Subject digest plus invocation evidence | **Adapt existing bindings** | Remove duplicate interpretations of “what was verified”; preserve candidate/worktree/action/failed-evidence identity and command receipts. | S11, S12: the same filename in another worktree, or logs from another invocation, prove nothing about this subject. Digest alone does not prove consent or correctness. |
| Reusable instruction templates / registries | **Adapt narrowly** | Generate presentation and managed consumers from native definitions; keep durable OpenSpec/Engram design content. | S4, S7: printed `EXECUTE_COMMAND` is not execution; unreadable mandatory hooks must not become a silent waiver. |
| General workflow platform, queue/database, signing framework | **Reject as dependency** | Do not add a second workflow engine, attempt ledger, universal signed context, or broad overlay/catalog surface. | S5, S8, S12 are precedents, not dependency requirements. |

### Safety counterexamples and explicit limits

1. A top-level parent runs an external mutation, then a nested gate pauses. Spec Kit's source replays the parent/body on resume; saved progress is not exactly-once effects. [S6]
2. A process dies after a step side effect but before its result is saved. The shown engine has no atomic coupling between `step_impl.execute` and state persistence. A hard kill need not reach KeyboardInterrupt handling; `running` is not an accepted engine resume state. This is a source-level failure-window analysis, not a reproduced bug or a claim about every upstream recovery tool. [S6, S8]
3. Shell timeout becomes FAILED with synthetic exit `-1`, empty stdout, and `stderr=timeout`. This is not proof that descendants stopped, that external effects rolled back, or that useful output never existed. No universal process-tree cleanup guarantee was established. [S7]
4. Unreadable hook YAML is reported but the implement template continues, including when mandatory hooks could have been registered. That template policy is unsuitable for Gentle AI's selected-research, consent, or native authority gates. [S4]
5. A workflow shell expression interpolates model-produced text. Official docs warn that quoting is not a security boundary and gates do not resolve/inspect the next command. Preserve typed native dispatch; never infer runnable remediation from prose. [S5]
6. Stripe replays a `500`, yet the original operation can have user-visible effects. A stable error response, zero diff, or empty local report cannot prove “nothing happened.” [S9]
7. A generated client accepts a shape but ignores a new authority-bearing action/requirement. Shape derivation is useful only with explicit version/action semantics and rollout tests; generation alone is not compatibility or admission proof. [S10, S12 analogy]

## Validated claim index

“Validated” means inspected original text/code supports this scoped statement. It does **not** mean tests ran, an automated checker returned supported, or the parent admitted the proposal.

| Claim | Validated statement | Sources |
|---|---|---|
| C1 | Official latest API returned v1.0.6; release HTML binds it to `96c9bd6…`; observed main is `c173bf1…`. | S1, S2 |
| C2 | Recent released changes include per-step integration config and hook-error reporting; main separately consolidates expression validation. | S1, S2, S4, S7 |
| C3 | Feature discovery is explicit env/persisted context; pure paths mode suppresses persistence; prerequisite checks are executable existence checks. | S3 |
| C4 | Implement/converge templates prescribe agent task/checklist/hook behavior and content-based convergence; they are not themselves native settlement code. | S4 |
| C5 | Release has native YAML step routing and a bundled phase/gate sequence; workflow requirements are advisory, not capability sandboxing. | S5, S6, S7 |
| C6 | RunState uses individual atomic JSON replacements and in-process locks; engine resume accepts paused/failed and can replay a nested parent/body. | S6 |
| C7 | Command step success follows integration exit status; shell step validates a finite positive timeout, captures output, and returns typed failure on timeout/nonzero exit. | S7 |
| C8 | Main expression validation obtains leaves from its evaluator; released step validation obtains types from the executable registry. | S2, S6, S7 |
| C9 | AWS's idempotency pattern uses caller request identity, atomic token/mutation coupling, semantic replay, intent matching, and service-dependent retention. | S8 |
| C10 | Stripe caches first-result status/body including 500, rejects parameter mismatch, limits key lifetime, and documents indeterminate effects/reconciliation. | S9 |
| C11 | OpenAPI supports generated consumers and explicit version interpretation; discriminator cannot alter schema validity. | S10 |
| C12 | in-toto Statement v1 requires subject digests and matches subjects by digest rather than name. | S11 |
| C13 | SLSA distinguishes inputs, resolved dependencies, invocation identity and byproducts; unrecognized external parameters and non-monotonic extensions are unsafe. | S12 |

## Source and tool ledger

### Retrieval-time convention and provenance

All sources were fetched in this executor session using approved tools; no remembered release or search snippet was promoted to evidence. `fetch_content` responses did not expose per-request wall-clock timestamps. Therefore their `accessed_at` is **session-observed, exact instant unavailable**. An actual clock checkpoint available for this collection session is Unix UTC milliseconds `1789117428235` (the last inspected `source_check` artifact timestamp). It is **not** a per-source fetch timestamp or a bound on parallel call completion. Precisely timestamped source-check fetches are identified below. Source publication dates are never relabeled retrieval dates.

Source-check tool output was retrieved as structured data; it contains original passage spans/hashes, but its automatic verdicts were `unclear` (0.30). Those verdicts are preserved, not described as successful verification. Independent direct original reads support C1–C13. Intermittent discovery failures were recovered through subsequent successful calls within the same approved class; no class was skipped. Older refs returned by search/checking are discovery-only, not authority for v1.0.6 or current main.

There are **12 evidence families**, with multiple original files where implementation and documentation must be compared. The individual URL count exceeds the approximate 8–12-source target because exact release identity, templates, engine, and adapters were needed to avoid a false “prompts only” comparison. This remained a narrow first-party comparison, not a competitor survey.

### Sources (IDs resolve to exact fetched URLs)

Each source inherits the `accessed_at` convention above unless an exact timestamp is supplied. Class is the collection lane; documentation grants remain only `fetch_content` even when a supporting page was also checked in open-web.

| ID / class | Publisher, version, URL(s), retrieval record | Short supporting excerpts |
|---|---|---|
| **S1 / open-web** — upstream identity and released history | GitHub. `fetch_content` raw: [latest API](https://api.github.com/repos/github/spec-kit/releases/latest), response `mtwq659owzlyor`, index 1; [release HTML](https://github.com/github/spec-kit/releases/tag/v1.0.6), `mtwq8lvvzzn3bg`, index 0; [release-pinned changelog](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/CHANGELOG.md), `mtwq9mlfy5ta2h`, index 3. [Official README](https://github.com/github/spec-kit), `mtwq6s7mjvqexz`, index 0, identity/entry-path context only. | `"tag_name":"v1.0.6"`; `"published_at":"2026-09-10T13:27:05Z"`; release link `/commit/96c9bd657bfd5de0d651a6165084932b7304ac99`; `feat(workflows): add per-step integration configuration (#4425)`. |
| **S2 / open-web** — observed main and deletion | GitHub. [Main commits](https://github.com/github/spec-kit/commits/main/), `mtwq6s7mjvqexz`, index 2; [exact change](https://github.com/github/spec-kit/commit/ce593cdcb10dee03fa48058e2767c70e8566734a), `mtwq7w9lwqt1bg`, index 2; [expressions at observed main](https://raw.githubusercontent.com/github/spec-kit/c173bf19a6654e3b05386ec3599349a55282b897/src/specify_cli/workflows/expressions.py), `mtwq9mlfy5ta2h`, index 2. | `_collect_leaves` calls `_evaluate_simple_expression`; `_unresolvable_term`: “Asks the evaluator which names it will look up”; former code “had to be kept in step ... by hand.” |
| **S3 / open-web** — feature/prerequisite scripts | GitHub, release v1.0.6 / `96c9bd6…`. [common.sh exact commit](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/scripts/bash/common.sh), `mtwqaho5fa11li`, index 2; [check-prerequisites.sh at release tag](https://raw.githubusercontent.com/github/spec-kit/v1.0.6/scripts/bash/check-prerequisites.sh), `mtwq78u95aokba`, index 2. Tag-to-commit binding: S1. | `get_feature_paths --no-persist`; “1. SPECIFY_FEATURE_DIRECTORY ... 2. .specify/feature.json ... 3. Error”; `if [[ ! -f "$IMPL_PLAN" ]]`; `$REQUIRE_TASKS`. |
| **S4 / open-web** — implementation/convergence templates | GitHub, v1.0.6 / `96c9bd6…`. [implement exact commit](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/templates/commands/implement.md), `mtwqaho5fa11li`, index 1; [converge release tag](https://raw.githubusercontent.com/github/spec-kit/v1.0.6/templates/commands/converge.md), `mtwq78u95aokba`, index 5. | “Emitting the block alone does not run the hook”; unreadable YAML: “then continue normally”; “Parse tasks.md structure”; converge: “APPEND-ONLY, NEVER REWRITE.” |
| **S5 / documentation (reference), open-web (bundled YAML)** — native workflow contract and shipped sequence | GitHub, v1.0.6. [workflows reference](https://raw.githubusercontent.com/github/spec-kit/v1.0.6/docs/reference/workflows.md), `mtwq78u95aokba`, index 1; [bundled YAML](https://raw.githubusercontent.com/github/spec-kit/v1.0.6/workflows/speckit/workflow.yml), `mtwq8lvvzzn3bg`, index 2. | `state.json`, `inputs.json`, `log.jsonl`; “requires is an advisory pre-condition block”; “There is no shell-escaping filter”; YAML schema `1.0`, workflow version `1.0.1`. |
| **S6 / open-web** — engine and recovery implementation | GitHub, [engine at release commit](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/src/specify_cli/workflows/engine.py), `mtwqaho5fa11li`, index 0. Full release-tag engine read in slices 0–26000, 26000–56000, 56000–81697 via `mtwq8b0rbadvg8`, index 0; same mechanism inspected at exact release commit. [Observed-main engine](https://raw.githubusercontent.com/github/spec-kit/c173bf19a6654e3b05386ec3599349a55282b897/src/specify_cli/workflows/engine.py), same response index 2, no distinct main-only engine claim. | `state.status not in (RunStatus.PAUSED, RunStatus.FAILED)`; “resume will re-run the parent step and its nested body”; `_atomic_write_json(state.json)` then `_atomic_write_json(inputs.json)`; `result = step_impl.execute(...)` precedes result persistence. |
| **S7 / open-web** — typed executors and registry | GitHub, release commit `96c9bd6…`. [shell](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/src/specify_cli/workflows/steps/shell/__init__.py), [command](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/src/specify_cli/workflows/steps/command/__init__.py), `mtwq9mlfy5ta2h`, indices 0/1; [registry](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/src/specify_cli/workflows/__init__.py), `mtwq94ss12lqmt`, index 0. | `timeout = config.get("timeout", 300)`; `except subprocess.TimeoutExpired`; `_timeout_error` shared by execute/validate; `dispatch_result["exit_code"] != 0`; `STEP_REGISTRY: dict[str, StepBase]`. |
| **S8 / documentation** — idempotent API engineering | Amazon Web Services Builders' Library. [Making retries safe with idempotent APIs](https://aws.amazon.com/builders-library/making-retries-safe-with-idempotent-APIs/). Living article, no publication/version date exposed in inspected text. Readable extraction was incomplete (`mtwq6rlmced0m7`, index 0); raw original recovered full relevant text (`mtwq79b4a8ze4m`, index 0). | “unique caller-provided client request identifier”; token plus mutations “must meet ... atomic, consistent, isolated, and durable”; “lifetime of the resource, plus an interval” for late requests. |
| **S9 / documentation** — replay, retention and indeterminate effects | Stripe. [Idempotent requests](https://stripe.com/docs/api/idempotent_requests), `mtwq6rlmced0m7`, index 1, displayed API `2026-08-26.dahlia`; canonical [API page](https://docs.stripe.com/api/idempotent_requests). [Advanced error handling](https://docs.stripe.com/error-low-level), `mtwq8b0rbadvg8`, index 3, living unversioned guide. Supporting open-web check fetches: `1789117288361` and `1789117336868` Unix UTC ms, respectively. | “same result, including `500` errors”; keys pruned after “at least 24 hours”; new key discouraged because “the original key may have produced side effects”; “Treat ... `500` errors as indeterminate.” |
| **S10 / documentation** — shared versioned contracts | OpenAPI Initiative. [OpenAPI Specification 3.1.1](https://spec.openapis.org/oas/v3.1.1.html), `mtwq6rlmced0m7`, index 2; explicit specification version, not claimed latest. | “code generation tools to generate servers and clients”; `openapi` “not related to ... info.version”; `discriminator` “MUST NOT change the validation outcome.” |
| **S11 / documentation** — evidence subject | in-toto project. [Statement v1.0 pinned to `7aefca35…`](https://raw.githubusercontent.com/in-toto/attestation/7aefca35a0f74a6e0cb397a8c4a76558f54de571/spec/v1/statement.md), fetched raw with inline result; pin linked by SLSA v1.1. Earlier main copy (`mtwq6rlmced0m7`, index 3) was discovery; claim uses pinned text. | “Each element MUST have digest set”; “Subject artifacts are matched purely by digest, regardless of content type.” |
| **S12 / documentation** — provenance and narrow schema evolution | SLSA project. [Provenance, specification v1.1](https://slsa.dev/spec/v1.1/provenance); raw `mtwqaho5fa11li`, index 3, followed by readable inline full relevant text. Predicate remains `https://slsa.dev/provenance/v1`; not a claim that 1.1 is latest. | “externalParameters ... untrusted”; “Verifiers SHOULD reject unrecognized or unexpected fields”; `invocationId` identifies “this particular build invocation”; ignored extensions “SHOULD NOT turn a DENY decision into an ALLOW.” |

### Executed tool inputs and checks

- **`web_search`**: one three-query discovery call, `workflow=none`, `numResults=4`, no provider override: `github spec-kit latest release workflow check prerequisites implement scripts`; `AWS builders library making retries safe idempotent APIs late arriving requests`; `official OpenAPI schema generator in-toto statement subject digest provenance`. Response `mtwq664ehbz7mi`. Its stale Spec Kit snippets were not evidence.
- **`fetch_content`**: exact URLs and raw/readable routes appear in S1–S12. GitHub HTML/raw/API originals, AWS raw recovery, and Stripe readable recovery were actually fetched, not suggested future calls. Repository URLs caused the approved tool to create its own temporary clone/cache; no clone command, source execution, install, or local-cache evidence route was invoked by this executor. Tool-returned suggestions to read/run cached code were ignored.
- **`get_search_content`**: read bounded originals and exact matches: release `repository_nwo`, `/commit/`, `datetime=`; main commit listing; script `get_feature_paths`, `--no-persist`; complete implement/converge; complete release engine; executor files; evaluator `_collect_leaves`/`_unresolvable_term`; AWS `atomic`, `ACID`, `late arriving`; Stripe `indeterminate`; OAS `code generation`, `versions`, `discriminator`. Relevant response IDs/indices are in the source table. Source-check records were retrieved in full, not just their summaries.
- **`source_check` initial pivotal checks**: compound Spec Kit native-engine/template claim with queries `site:github.com/github/spec-kit "WorkflowEngine" "resume"` and `site:github.com/github/spec-kit "v1.0.6" "workflows"` (`mtwq7yvnlls8gv`); Stripe replay/indeterminate claim with queries `site:docs.stripe.com/error-low-level "indeterminate"` and `site:docs.stripe.com/api/idempotent_requests "24 hours"` (`mtwq7uqcqmtdah`). Both partially hit transient “No search provider available”; successful results existed, but verdicts were unclear.
- **`source_check` recovered Stripe replay**: claim “Stripe saves status code and body for an idempotency key, including 500 errors, and can prune keys after at least 24 hours”; query `site:docs.stripe.com/api/idempotent_requests`, two results, fetched originals. `mtwq8muj7fcqx4`, exact fetch `1789117288361`. Passages `p-1-1` (saving result), `p-1-2` (24 hours), `p-1-3` (500 replay) directly support C10 despite classifier `unclear`.
- **`source_check` recovered Stripe uncertainty**: claim “Stripe advises treating 500 responses as indeterminate because side effects can still occur”; query `site:docs.stripe.com/error-low-level "indeterminate"`, two results requested. `mtwq9o9wyy6rwv`, exact fetch `1789117336868`. Passages `p-1-1` (new-key warning), `p-1-2` (user-visible effects) plus the fetched original section support C10; verdict remained unclear.
- **`source_check` Spec Kit recovery**: narrow nested-resume query failed (`mtwq9mf9vr3hv0`); broader claim “Spec Kit workflows have native run, status and resume commands with persisted state”, query `Spec Kit workflows run resume state engine`, three results, fetched originals (`mtwqa3y8kjxu3y`, fetch `1789117357177`) recovered official docs. Passages establish the existence of workflows, not release-specific crash guarantees. C5/C6 were validated against S5/S6 instead of stale search refs.
- **`source_check` release cross-check**: claim “The official github/spec-kit release v1.0.6 was published on September 10, 2026 and includes per-step integration configuration”; query exact v1.0.6 release URL, two results. `mtwqbmrvo1x1we`, timestamp `1789117428235`, fetched results timestamp `1789117428232`. Search returned releases listing and v1.0.1, so this check does **not** validate the latest version. S1's original latest API, release HTML and exact-commit changelog provide the validation.

### Failed/recovered calls and limits

- GitHub API repository, main commit and tag-ref requests returned rate-limit JSON, not repository evidence: `https://api.github.com/repos/github/spec-kit`, `/commits/main`, `/git/ref/tags/v1.0.6`. Recovered identity/pins via official release HTML, main listing and raw exact source. Latest-release API succeeded.
- Guessed raw paths `src/specify_cli/workflows.py`, `workflow/engine.py`, `workflows/state.py`, `workflows/steps.py`, `workflows/shell.py`, `workflows/command.py`, `workflows/expression.py`, and `workflows/steps/{shell,command}.py` returned 404/error bodies. No claims use them. Registry imports led to the actual step packages, successfully fetched at the release commit.
- Parent-notification helper was denied by the child's launch allowlist. It supplied no evidence and was not added to either class grant. Skill fallback and retrieval-clock limits are reported here instead.
- Exact per-`fetch_content` instants remain unavailable; only the disclosed session clock checkpoint and source-check fetch timestamps are available. Parent should retain this provenance limit rather than manufacture precise times.
- No claims of universal Spec Kit safety/nondeterminism, exactly-once subprocess execution, process-tree cleanup, crash-tested durability, current packaged Gentle AI/Pi behavior, or closure of any issue are made. Platform adapters, PowerShell/Python parity, all extension hooks, and every catalog/rollback implementation were not audited.
- No source tests, lifecycle authority operations, configuration/package changes, GitHub writes, commits, or delivery were performed. No new product choice, schema, dependency, compatibility release floor, or implementation was approved.

## Handoff

Parent: read this artifact and the unchanged immutable intent/confirmed constraints in `preproposal.md`; assess the source-level conclusions and disclosed automated-check limitations before final proposal admission. Research was explicitly selected and is now source-backed; the exploration's earlier optional-unselected note is historical, not the current gate. The strongest next design input is to remove duplicated producer/consumer interpretation **while retaining native deterministic transitions and immutable admission/settlement authority**.

## R6 addendum: preserve quality and distinguish capability boundaries

**Outcome: done, with scoped source-backed answers; proposal readiness remains false.** This is the additive request `sdd-runtime-simplification-external-r1-quality-addendum`, read from preproposal revision 3. R1–R5, C1–C13, S1–S12 and their provenance caveats are retained. Four additional load-bearing originals were inspected at the already verified release commit `96c9bd657bfd5de0d651a6165084932b7304ac99` (Spec Kit v1.0.6); no latest-release rediscovery was performed.

**R6 — quality and capability boundary:** At the already pinned current Spec Kit release/source, what TDD/test-first guidance, review/verification gates, implementer–verifier separation, and capability/prerequisite/write-scope handling are actually present? Distinguish executable enforcement, templates, optional/user-configured controls, and facts not established by the inspected sources. Map only useful patterns to preserving Gentle AI's quality guarantees and preventing the reported research-to-artifact false blockers. Prefer 2–4 additional primary source files and reuse existing fetched engine/workflow evidence; do not restart the full research.

### Confirmed constraints versus reported symptoms

Gentle AI's **deterministic native workflow, strict RED/GREEN/TRIANGULATE/REFACTOR, verification stages, and required implementer–verifier independence** are user constraints for this plan. They are not new external claims that the existing implementation already enforces every invariant. Different responsibilities and independent evidence do not require selecting different model vendors. Simplification must delete duplicated interpretation, not quality requirements or evidence subject/consent/CAS binding; it must not add redundant gates just to name the pattern.

The report linked in preproposal at [#4484 comment 5632285029](https://github.com/Gentleman-Programming/gentle-ai/issues/4484#issuecomment-5632285029) describes research/webfetch refusal → OpenSpec write refusal → apparently permanent blocking. It remains **user-supplied symptom evidence**: no runtime/version/configuration/exact logs were provided. This addendum neither re-fetches the issue nor infers a root cause, reproduction, fix, or closure.

### Quality and capability comparison

| Topic | What the pinned sources establish | Enforcement classification / limits | Gentle AI implication under confirmed constraints |
|---|---|---|---|
| Test generation and test-first | Tasks command says tests are optional unless the specification requests them or the user requests TDD; requested contract tests precede implementation, and phases are independently testable. Existing implement instructions say execute test tasks before corresponding implementation and validate tests/coverage. [S13, S4] | **Template guidance with explicit user/spec conditions.** This is real TDD/test-first guidance, not proof of native RED evidence, triangulation, or independent test execution. | Preserve strict TDD rather than importing the optional default. Carry the already confirmed requirement through task generation and native evidence acceptance; do not ask the user to select it again. |
| Pre-implementation quality review | Analyze checks spec/plan/tasks consistency, constitution MUST principles, requirement/task coverage and ordering; constitution conflicts are CRITICAL, and critical findings lead to a recommendation to resolve before implement. Tasks declares Analyze and Implement handoffs. [S14, S13] | **Agent-mediated analysis and handoff metadata.** This is artifact quality analysis, not execution of the application's tests. It is incorrect to claim Spec Kit lacks review because engine.py has no specialized TDD code. | Retain useful requirement-to-task/evidence traceability. Native acceptance—not severity prose or an implementer's completion claim—owns required transitions. |
| Executable human review gates | GateStep validates options/on_reject/verdict_input during execute, pauses without a TTY when no bound verdict is available, stores choice, and maps abort/retry/skip to typed outcomes. Existing bundled YAML includes review-spec/review-plan. [S15, S5, S6] | **Executable, configuration-dependent control.** A matching supplied verdict can avoid an interactive prompt. Configured rejection `skip` returns COMPLETED; `retry` pauses and clears a consumed bound verdict; `abort` returns failure with aborted=true. A gate's presence alone is not independent verification. | Preserve real native consent and required verification semantics; do not copy auto-verdict/skip behavior onto non-waivable quality or consent obligations. |
| Implementer–verifier separation | Analyze and converge have separate command responsibilities; per-step integrations/models can be configured (S5/S7). In the inspected bundled workflow, only specify/plan/tasks/implement command steps and spec/plan review gates are configured; there is no separate post-implement verifier step in that YAML. [S4–S7, S13, S14] | **Distinct commands / optional composition**, not an established universal independent-verifier invariant. The inspected templates, bundled YAML, engine and GateStep do not establish a mandatory different verifier identity or independent evidence check. Integration-specific isolation and community workflows are **unknown**, not asserted absent. “Independently testable story” is not “independent verifier.” | Keep required verifier independence as a native acceptance invariant, including real subject/evidence linkage. Do not downgrade verify to implementer self-report or add a second Pi authority. |
| Tool availability and feature discovery | Core docs describe offline `specify check` for installed agent CLIs (IDE agents skipped), and `specify version --features --json` for installed CLI features. Existing command executor validates integration configuration and returns dispatched=false failure when it cannot dispatch. [S16, S7] | **Documented discovery surface** plus **inspected dispatch-time checks**. Installed CLI/features do not prove a child has a particular webfetch tool, provider credentials, or artifact-write grant. No Spec Kit-to-Gentle AI capability equivalence is established. | Keep producer/consumer/child agreement on exact tool names, actual calls and existing write scope. Delete contradictory guessed capability lists, not real unavailable-selected-research stops. |
| Project, feature and artifact scope | Core docs distinguish project selection (`SPECIFY_INIT_DIR`) from feature selection; invalid explicit project context errors rather than falling back. Existing prerequisite scripts resolve explicit context and validate required files. Analyze explicitly forbids writes and requires approval before follow-up editing; tasks explicitly instructs generating tasks.md. [S16, S3, S14, S13] | **Documented root selection**, **executable prerequisite checks**, and **phase-specific instruction scopes**. Analyze's no-write rule is not a universal denial of all later authorized artifact writes. Docs describe command-specific symlink handling, not one universal confinement policy. | Carry the admitted artifact backend/path/write scope across research-to-persistence handoff; do not re-infer a phase's permissions from another phase's read-only text. Retain wrong-worktree/out-of-scope refusal and real write errors. |
| Recovery after missing input or failed dispatch | Missing artifacts are named with prerequisite instructions; missing CLI is explicit failure; existing engine resumes only paused/failed and can re-execute the parent body. [S14, S3, S7, S6] | **Typed refusal/recovery plus template advice**, not proof of automatic safe end-to-end recovery, exactly-once execution, or no permanent blocks. A read failure on gate `show_file` becomes a display notice, not automatic gate failure. [S15] | Re-evaluate corrected facts through native predicates; a persisted old blocker is not proof it remains true. Preserve CAS/freshness/attempt accounting and do not blindly replay work or auto-clear a genuine block. |

### Supplemental adopt / adapt / reject rows

These are recommendations only, not source implementation or new product authority.

| Pattern | Recommendation | Concrete consolidation / invariant | Counterexample or limitation |
|---|---|---|---|
| Explicit test-first ordering tied to tasks | **Adapt, strengthen to confirmed strict TDD** | One native quality requirement carried into AI artifacts and Pi execution; delete repeated consumer decisions about whether TDD applies. Preserve RED/GREEN/TRIANGULATE/REFACTOR and required evidence. | S13's optional tests must not weaken Gentle AI's confirmed requirement. Test ordering in prose alone does not prove a RED run. |
| Artifact analysis separated from implementation | **Adapt responsibilities, not a new gate** | Preserve required verifier independence and traceable evidence; remove implementer-as-verifier aliases and duplicate phase dispatch rather than add redundant reviews. | S14 analyzes planning artifacts. A new command name, subagent label, different model, or self-reported pass is not sufficient proof of independent verification. |
| Typed gate input validation | **Adopt principle** | Share native validation with execution, so an invalid value cannot silently choose an allow/skip path. | S15 explicitly guards malformed values at execution because standalone validation may have been skipped. Its legitimate configurable skip mode is not a Gentle AI waiver. |
| Capability discovery distinct from actual grant | **Adapt** | Use one native contract for required routes and outcomes, then verify actual child-local provisioning/calls. Preserve selected-class failure when genuinely unavailable. | S16's installed CLI check does not prove web access; S7's dispatched=false does not diagnose the reported Gentle AI journey. |
| Phase-specific artifact write authority | **Adapt existing scope** | Carry authorized OpenSpec writes and resolved worktree/subject through the same admitted action; delete contradictory no-write instructions or re-interviewing for already granted writes. | S14's intentionally read-only analyze command cannot justify a blanket persistence ban; S13's task-writing prompt cannot grant arbitrary writes. |
| Permission bypass or unconditional unblock | **Reject** | No fabricated tools, research skipping, widened write scope, stale-evidence acceptance, wrong-worktree fallback, automatic budget reset or second ledger. | An actual unavailable tool/write denial must remain truthful; a mere old blocked label cannot independently justify permanent refusal. Source research does not identify which case occurred in the report. |

### Supplemental validated claims and unknowns

| Claim | Scoped validated statement | Sources |
|---|---|---|
| C14 | The release tasks command makes test generation conditional on specification/user TDD requests and orders requested contract tests before implementation; implement also contains test-first and completion-validation guidance. | S13, existing S4 |
| C15 | Analyze is read-only cross-artifact quality analysis with explicit approval before follow-up editing, constitution-conflict severity and coverage reporting; tasks declares analyze/implement handoffs. | S14, S13 |
| C16 | GateStep implements configurable pause/abort/retry/skip and bound-verdict behavior with execution-time input validation; it does not equate rejection with failure for configured skip. | S15, existing S6 |
| C17 | The inspected bundled workflow does not configure a separate post-implementation verifier step. Separate analysis/convergence commands and configurable integrations do not, by themselves, establish required independent verifier execution. | Existing S4–S7, S13, S14 |
| C18 | Official core docs expose offline installed-tool checks and CLI-feature JSON, separate project from feature selection, and describe explicit-root errors and command-specific write confinement. These docs do not claim child web-tool/write grants. | S16; existing S3/S7 for inspected resolver/dispatch mechanisms |
| C19 | Analyze's no-write instruction and tasks' output-writing instruction are phase-specific; GateStep can still prompt after an unreadable review file notice. Neither provides a universal artifact-write authorization/verification guarantee. | S13, S14, S15 |

**Not established:** project-wide absence of TDD or reviews; a mandatory independent verifier across all Spec Kit integrations/extensions; native enforcement of the full Gentle AI TDD cycle; runtime parity of all documented core checks; Spec Kit handling of OpenSpec-specific permissions; cause or reproduction of the community report. Search surfaced an integration-specific analyze-fork PR, but it was not inspected at the pin and supplies no claim here. These limits answer the unknown branch of R6 rather than inventing a project-wide absence or expanding this bounded study.

### Additional source ledger — original S1–S12 retained

All four originals are published by the already verified official GitHub `github/spec-kit` repository at release commit `96c9bd657bfd5de0d651a6165084932b7304ac99`. `accessed_at`: this supplemental collection; exact per-fetch instant unavailable because `fetch_content` did not expose it. Do not reuse the original round's clock checkpoint as a timestamp for these new fetches.

| ID / class | Exact source / retrieval | Short original supporting excerpts |
|---|---|---|
| **S13 / open-web** — task generation and TDD conditions | [templates/commands/tasks.md](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/templates/commands/tasks.md). `fetch_content` raw batch `mtwqvp4mmyr5ze`, index 0; `get_search_content` complete 11378 characters. | “Only generate test tasks if explicitly requested in the feature specification or if user requests TDD approach”; “contract test task [P] before implementation”; handoffs `agent: speckit.analyze` and `agent: speckit.implement`; “Generate tasks.md”. |
| **S14 / open-web** — read-only analysis and approval | [templates/commands/analyze.md](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/templates/commands/analyze.md). Same raw batch, index 1; complete 11758 characters retrieved. | “STRICTLY READ-ONLY: Do not modify any files”; “user must explicitly approve before any follow-up editing commands”; “Constitution conflicts are automatically CRITICAL”; “If CRITICAL issues exist: Recommend resolving before” implement. |
| **S15 / open-web** — executable human-review gate | [src/specify_cli/workflows/steps/gate/__init__.py](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/src/specify_cli/workflows/steps/gate/__init__.py). Same raw batch, index 2; complete 15697 characters retrieved. | `if not sys.stdin.isatty(): ... StepStatus.PAUSED`; `if on_reject not in ("abort", "skip", "retry")`; `context.inputs[bound_verdict_input] = ""`; `# on_reject == "skip" → completed, downstream steps decide`; read error returns “(could not read file: ...)”. |
| **S16 / documentation** — core capability and scope reference | [docs/reference/core.md](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/docs/reference/core.md). Same raw batch, index 3; complete 8494 characters retrieved. | “This command stays offline”; `specify version --features --json`; “Two resolution axes”; invalid explicit root “errors and does not fall back”; “each command keeps its existing cwd-path stance.” |

### Supplemental capability admission and tool record

Child-local inventory was reconfirmed before collection. Documentation grants/execution remain **[fetch_content]**; open-web grants/execution remain **[web_search, source_check, fetch_content, get_search_content]**. Both classes ran. There were **nine approved external tool calls** in this supplemental round, with four new load-bearing primary files; no new API/latest discovery, install, execution, issue audit or subagent orchestration.

1. `fetch_content`, raw, four exact S13–S16 URLs in one batch: response `mtwqvp4mmyr5ze`. All four succeeded.
2. `web_search`, no provider override, `workflow=none`, `numResults=2`: queries `site:github.com/github/spec-kit "tasks" "Tests are OPTIONAL"` and `site:github.com/github/spec-kit "analyze" "READ-ONLY"`; response `mtwqvpcz3gd327`. Discovery only; snippets, an old tasks-template ref and an analyze-fork PR were not promoted to release evidence.
3. Four `get_search_content` calls retrieved indices 0–3 completely, with limits 13000, 13000, 18000 and 10000, respectively. These original passages validate the supplemental claims.
4. `source_check` claim “Spec Kit's tasks command guidance makes test-task generation conditional on the feature specification or an explicit user request for TDD”; query `site:github.com/github/spec-kit/templates/commands/tasks.md "Tests are OPTIONAL"`, two results requested. Response `mtwqwbk8m1qwh1`: **missing-evidence**, confidence 0.20, search-provider error; no positive verification claimed.
5. Bounded recovery `source_check` claim “The Spec Kit tasks template says to generate test tasks if explicitly requested in the feature specification or if the user requests a TDD approach”; query `site:github.com/github/spec-kit "user requests TDD approach"`, two results requested. Response `mtwqwlox4yxvak`: **unclear**, confidence 0.30, but successfully fetched originals. It returned an older tasks source at `4a323449` and discussion 1873; neither establishes the current release nor the reported Gentle AI failure.
6. `get_search_content` retrieved the recovery check's full structured record. Actual metadata: artifact `timestamp=1789118406609`, source `fetch_timestamp=1789118406607` (Unix UTC milliseconds). Passage `p-1-1`, span 6085–6220, repeats the exact optional-test/TDD condition; passage hash `sha256:4ad2dbe4fba0956e1298a30868d17dfa5459c1b3aa4ec450c19fd8529e0ccdc3`. This is corroboration at an older ref, not a timestamp for S13–S16. C14 is validated by the direct release-pinned S13 original despite the automatic classifier uncertainty. The discussion's anecdotes are not load-bearing evidence.

**Skill diagnostic:** project/user `skill_resolution` is corrected to **paths-injected**: the supplied cognitive-doc-design skill was loaded and re-read. The executor phase skill previously needed the explicit fallback `internal/assets/skills/sdd-research/SKILL.md` and shared references; that diagnostic is retained separately, not classified as a project-skill injection failure. All earlier provenance caveats remain in force.

### Supplemental handoff

R6 now has validated scoped answers, including explicit unknowns; collection stopped without expanding into integration/plugin audits. Parent should read this addendum and the current-session scope constraints before admitting the proposal. Preserve native deterministic transitions, strict TDD and required verifier independence through the entire research → authorized artifact write → continuation journey. A proposed acceptance scenario must test that journey and its negative controls later; this research has not executed it. `proposal_ready` remains false.

## R7 addendum: simplify inside the existing native owner

**Recommend adapting the small separation-of-responsibilities pattern inside the existing Go authority, not importing or porting the workflow engine.** A producer-owned action/result definition can remove duplicate Pi interpretation while retaining immutable admission, settlement and recovery. Direct reuse is permitted under the inspected license's conditions, but its operational semantics are not equivalent to the required guarantees. This is a research recommendation, not implementation approval, a dependency decision or a final wire schema.

**Outcome: done; proposal readiness remains false for parent readback.** Additive request `sdd-runtime-simplification-external-r1-reuse-addendum` was read from preproposal revision 5. R1–R6 and all original evidence remain intact. Same official publisher and release pin: GitHub `github/spec-kit`, v1.0.6, commit `96c9bd657bfd5de0d651a6165084932b7304ac99`, published `2026-09-10T13:27:05Z` [S1]. No latest-version rediscovery or issue investigation was performed.

**R7 — code-level reuse assessment:** Inspect the pinned engine's states, events, transition predicates, result handling, recovery, dependencies, and relevant upstream tests. Compare direct code reuse, narrow extraction/porting, and adapting the design within the existing native owner. Identify actual native/Pi duplicate mechanisms each useful pattern would remove. Recommend the smallest compatible option, not a new workflow engine by default.

### Execution state is not artifact readiness

`RunStatus` declares **created, running, paused, completed, failed, aborted**. `StepStatus` separately declares **pending, running, completed, failed, skipped, paused**. `StepResult` contains `status` (default COMPLETED), `output`, `next_steps`, and `error`; `StepContext` carries inputs, previous results, project/run identity and integration configuration. These represent execution data, not valid planning artifacts, strict TDD, verifier independence, consent or immutable candidate binding. Declared enum members do not establish that every executor emits every member. [S21]

Events below describe inspected entry/return conditions, not a proposed event vocabulary. This is a compact model of the inspected engine/gate, not every plugin, loop, overlay or concurrent branch. [S6, S7, S15, S21]

| State / event | Predicate and mechanism | Result / recovery | Guarantee not supplied |
|---|---|---|---|
| New run → execute | Definition validation is a separate API; typed inputs are resolved. RunState supplies CREATED; engine enters RUNNING before steps. | Saves context, traverses definition through executable registry. | Calling execute is not independent artifact validation or authorization; workflow requires is advisory [S5]. |
| RUNNING → step dispatch | Registry selects `StepBase.execute(config, context)`; result is recorded after execution. | Status/output/error and possible nested steps are returned; traversal continues unless a halting branch applies. | Default COMPLETED or process exit zero is not acceptance. Effects can precede persistence. |
| RUNNING → gate lacks verdict / interrupt | No usable bound verdict and no TTY, or engine catches KeyboardInterrupt. | PAUSED; interrupt logged/saved. | Not rollback or process-tree cleanup; hard kill need not reach the handler. |
| RUNNING → failed step | FAILED without aborted output; bypass requires literal `continue_on_error is True`. | Normally FAILED with error; literal true retains failure, logs `step_continue_on_error`, advances. | A completed run can contain a handled failed step. This must not waive required quality evidence. |
| RUNNING → deliberate abort | Failed result has truthy `output.aborted`, checked before continue-on-error. | ABORTED and saved; bypass flag cannot override it. | Not resumable through engine resume. Gate rejection configured as skip instead returns COMPLETED [S15]; gate presence is insufficient. |
| RUNNING → ordinary exception | Engine catches an execution exception. | FAILED, logs/saves error, re-raises. | No atomic coupling with external effects. |
| RUNNING → traversal ends | State still RUNNING after `_execute_steps`. | Converted to COMPLETED and saved. | End-of-list is not artifact freshness, verification or archive readiness. |
| PAUSED/FAILED → resume | Only these statuses accepted; saved definition/inputs/results loaded; replacements merged and type-resolved. | Re-executes current top-level step at saved index; nested pause/failure can replay parent/body. | Not exactly-once effects or immutable same-intent replay. `StepBase.can_resume` defaults true; its existence proves no stronger recovery guarantee. |
| Saved RUNNING/COMPLETED/ABORTED → engine resume | Fails paused-or-failed predicate. | This resume path refuses. | No automatic hard-kill reconciliation established; stale RUNNING is not evidence of actual process/effect state. |

Artifact existence checks [S3], template quality analysis [S4/S13/S14], configured gates [S15], and process/step outcomes are **different predicates**. Gentle AI must derive readiness from its own authoritative history and required subject-bound evidence. RunState's per-file atomic replacement and in-process locking are mechanisms, not cross-process CAS or a transaction with a subprocess [S6].

### Targeted tests: inspected, not executed

The exact-commit `tests/` directory disclosed the actual paths; search snippets did not establish them. No dependency was installed and no test ran. Only targeted passages of the large workflow test file were inspected; the smaller standalone file was read completely. Assertions are test designs, **not passing evidence, suite coverage proof or maintainer approval**. [S19, S20]

| Inspected test anchor | Original assertion / behavior | Useful proof target and limit |
|---|---|---|
| `tests/test_workflows.py::test_resume_with_input_reruns_step_with_new_value` | Initial exit 1 yields FAILED; `engine.resume(state.run_id, {"cmd": "exit 0"})`; asserts COMPLETED and cmd exit 0. | Tests changed-input rerun, not same-intent replay. Gentle AI needs changed-intent refusal or an explicitly authorized successor. |
| Same file, `test_resume_without_input_preserves_inputs` | Resume without replacement asserts FAILED again and `resumed.inputs["cmd"] == "exit 1"`. | Preserved inputs do not prevent re-execution; lost-response recovery must not rerun merely to obtain acknowledgement. |
| Same file, `test_resume_merges_and_coerces_typed_input` / `test_resume_invalid_typed_input_raises` | Count string "5" becomes integer 5 in state/inputs.json; "not-a-number" expects ValueError. | Shared typed validation is useful consolidation, not consent/CAS/candidate binding. |
| `tests/test_workflow_run_without_project.py::TestWorkflowRunWithoutProject.test_workflow_run_failing_yaml_without_project` | YAML exit 1; asserts `result.exit_code == 1` and `"Status: failed"` in output. | Truthful CLI failure mapping; exit code is not required verification evidence. |
| Same class, `test_workflow_run_yaml_without_project` | Asserts completion, marker creation and `.specify/workflows/runs` creation without an existing project. | Intended execution/persistence side effects; do not copy this convenience into read-only status or infer authorization from no project/diff. |
| Same file, `TestWorkflowRunJsonErrorStream.test_run_json_validation_error_not_on_stdout` | Invalid shell definition: exit 1, validation error absent from stdout, present on stderr. | Machine result and diagnostic-stream separation; not runtime compatibility or semantic acceptance. |

Other partial snippets (fan-out, installed-workflow ownership, gate error persistence) are context only; no broad coverage/reliability claim relies on them. Concurrent admission, interrupted effects, complete recovery and Gentle AI quality guarantees were **not tested here**.

### License and language/dependency cost

At the exact release commit, LICENSE is **MIT License**, holder **Copyright GitHub, Inc.** It permits use, copying, modification, merging, publication, distribution, sublicensing and sale subject to this exact condition [S17]:

> The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

It also states the software is provided **"AS IS", WITHOUT WARRANTY OF ANY KIND**. The inspected text does not require advertising or product endorsement. Preserve the full license with copied or substantially translated code, including copied tests; translation into Go/TypeScript is not a safe basis for stripping notices or copied-code provenance. Whether a future extraction is substantial is not decided here. Adapting a general design does not itself establish copied code; retain research provenance and assess the actual future implementation before a licensing determination. Neutral product-facing language cannot remove legally required notices. This is the license text and conservative handling advice, not legal clearance of an uninspected patch.

`pyproject.toml` specifies version 1.0.6, **Python >=3.11**, entry point `specify = specify_cli:main`, and dependencies Typer, Click, Rich, platformdirs, readchar, PyYAML, packaging, pathspec and json5. Hatchling builds the package; test extras are pytest/pytest-cov. The package bundles command templates, scripts, extensions, workflows and presets. These are declared package dependencies/assets, not a measured minimal engine dependency closure. [S18]

| Option | Python / Go / TypeScript coupling and maintenance | Imported complexity / invariant risk | Assessment |
|---|---|---|---|
| Direct package/engine reuse | Adds Python deployment or embedding/subprocess adapter alongside Go owner and TS consumer; upgrades require pin/adapter/behavior tests. | Package imports more than dispatch; engine includes registry, YAML/expressions, nested control flow, fan-out, mutable run files and cursor resume [S5–S7/S18/S21]. Wrapper supplies no CAS, strict TDD or verifier independence automatically. | **Reject for this plan:** competing progress authority or substantial translation layer while existing authority remains necessary. License permission is not architectural fit. |
| Narrow extraction / port | Copy a small validator/result helper or translate into Go; avoiding Python means owning port maintenance. TS remains consumer, not a policy port. | Must audit selected unit's imports, license, semantics and tests. Engine port still imports cursor/replay/general-language complexity; ported tests do not prove Gentle AI invariants. | **Conditional only for a demonstrated isolated deletion.** No unit selected/approved here; prefer existing native helpers. Reject an engine port. |
| Pattern adaptation in existing Go owner | Keep Go admission/settlement/recovery; derive TS decoding/dispatch fixtures from producer-owned contract. No upstream runtime dependency or second workflow policy owner. | Keep typed results/shared validation, not a YAML DSL, plugin/overlay platform, permissive gate skip or replay cursor. Requires coordinated consumer uptake proof. | **Recommend:** matches internal deletion map without importing unrelated semantics. |

Neutral downstream description: **one native-selected action, one producer-owned contract, one bounded consumer result, one authoritative settlement/recovery path**. This is a direction, not a wire record. Named provenance belongs here; later proposal/spec/design/task prose can use Gentle AI's own terms.

### Five concrete simplifications and deletion proofs

These are non-authoritative recommendations using the existing internal audit, not new source/runtime findings. AI anchors use `3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3`; Pi uses `564ae19b70bdf32f8f2e9865ed068742a1908834`; exact links are in [evidence.md#direct-source-observations](evidence.md#direct-source-observations). D1/D2 concern directly observed contradictions; D3–D5 identify mapped consolidation boundaries whose complete duplicate-call inventory belongs to later design. No uninspected handler is asserted defective or already deleted.

| Candidate / existing source anchors | Keep and derive | Delete/consolidate only after proof | Required acceptance proof, not current results |
|---|---|---|---|
| **D1 — producer-owned interpretation.** AI `internal/cli/sdd_status.go:20–45`, `RunSDDStatus` → `ProjectStatusV2`; `internal/sddstatus/status_v2.go:13–160`; Pi `lib/native-review-cli.ts:1379–1398`. | Native vocabulary/typed validation, producer-owned schema/fixtures, explicit unsupported action. S2/S6/S7 demonstrate shared executable definitions. | Pi's independent exact-three key/phase assumptions, duplicated vocabulary and route derivation; no dual-format compatibility readers. | Seven-dependency/four-instruction fixture accepted by intended consumer; unknown/malformed authority-bearing actions refused; remediation never aliases apply. Prove managed asset uptake, not only decoder units. |
| **D2 — inspection without launch preflight.** AI `internal/sddstatus/status.go:475–533`; `internal/assets/opencode/commands/sdd-status.md:20`, `sdd-continue.md:13`. | One declared-store resolver; display intrinsic blocks without execution permission. S3 is a pure-resolution precedent. | Irrelevant review/delivery checks in read-only status and contradictory blanket Engram-native-dispatch ban, after actual route alignment. | OpenSpec/Engram status inspectable when launch/review/delivery refused; no admission/authority mutation or new permission from status. Wrong root/missing facts remain truthful. |
| **D3 — one native action, bounded consumer.** AI `internal/sddstatus/status.go:1974–2055` acquire/settle/remediation instructions; Pi decoder above. | Ledger chooses action; consumer executes only it. Keep failed-evidence-bound remediation and fresh independent verification. S7's dispatch/result separation is useful, not exit-zero acceptance. | Competing prose/phase dispatch, action reconstruction and independent success rules where boundary inventory identifies them. Keep presentation non-authoritative and unsupported remediation explicit until typed execution exists. | Apply/verify/remediate/archive fixtures plus unknown/unsupported/alias negatives; results feed native settlement, never self-advance. Strict RED/GREEN/TRIANGULATE/REFACTOR and required implementer–verifier independence accepted only for the bound subject. |
| **D4 — shared native recovery predicates.** `RuntimeStatus`, AI `internal/sddstatus/runtime_ledger.go:26–56,436–493`; reconciliation `3607–3625`, callers `1221–1230,1451–1458,1712–1744,1828–1834,1949–1955`. | Existing reset/rescope/supersede/handoff/repair predicates, consent, current capture and strict fresh selection. S6/S8/S9 rule out cursor replay as sufficient. | Consumer recovery-verb selection and duplicate prose reconstruction. Consolidate presentation only if every predicate remains represented; no history deletion or automatic reset. | Lost responses reconcile/replay same authority operation; concurrent callers cannot both admit. Historical untracked→tracked replay works while fresh selection/drift remain strict. Unknown effects stay unknown; successor cannot launder budget/consent. |
| **D5 — one settlement interpretation, derived status.** AI `runtime_ledger.go:26–56`, settlement/capture `1221–1230`; `status.go:1974–2055`. | Immutable common-directory CAS; candidate/worktree/evidence/consent; finite outcomes and existing refund accounting. Keep irrecoverable command/process/cleanup evidence; derive display/readiness. | Duplicate client pass/fail/readiness inference and status-as-authority reconstruction where inventory finds them. No replacement ledger/marker; do not remove legitimate capped productive/harness-invalidated refund predicates. | Exit zero, no diff, timeout, empty output and implementer self-report alone never accept verification. Failed/interrupted work closes with supported facts; duplicate settlement cannot double charge/refund or rerun. Crash after effect/before save reconciles or blocks truthfully. |

**Cross-cutting acceptance:** retain the full research → authorized OpenSpec write → deterministic continuation journey. Required routes, actual child-local exact grants/calls, backend/path and existing write scope must agree; completed research must not inherit another phase's read-only prohibition. Genuine selected-tool unavailability, write errors, out-of-scope writes, stale evidence and wrong worktrees remain stops. Corrected facts are re-evaluated by native predicates, not a permanently trusted blocked label. Scope anchors are inspected `internal/assets/skills/sdd-research/SKILL.md` capability/persistence rules and `internal/assets/skills/_shared/sdd-phase-common.md` retrieval/persistence rules, plus preproposal's confirmed journey. Precise provisioning handler/root cause is not established; [exploration.md](exploration.md) retains the managed-uptake gap. No fabricated tools, permission bypass, automatic waiver, second capability authority or reporter-cause claim.

No deletion is justified merely by line count. Later proof must include independent verification, native-authority negative controls, cross-repository fixture and actual managed child uptake. Models author content, never lifecycle transitions. These recommendations do not execute that proof or authorize more planning files.

### R7 validated claims and exact sources

“Validated” means direct original inspection, not an automatic supported verdict or passing tests. D1–D5 are recommendations, not new implementation claims.

| Claim | Scoped statement | Sources |
|---|---|---|
| C20 | Separate run/step enums and StepResult/StepContext represent execution data; result defaults to COMPLETED with nested steps/error; StepBase declares execute/validate/can_resume. | S21 |
| C21 | Engine maps pause/failure/abort/exception/end separately; literal continue_on_error=true bypasses ordinary failure, not aborted output. Resume accepts paused/failed and can replay nested parent/body. | Existing S6/S15; S21 for declared types |
| C22 | Exact-release MIT license names GitHub, Inc., permits listed reuse operations, requires copyright/permission notices in copies/substantial portions, disclaims warranty. | S17 |
| C23 | Package 1.0.6 declares Python >=3.11, listed dependencies/assets, Hatchling and pytest extras; not a minimal engine dependency audit. | S18 |
| C24 | Inspected tests assert changed-input resume reruns to completion, unchanged-input resume can fail again, typed input coercion/rejection affects resumed state. No tests run. | S19 |
| C25 | Inspected standalone CLI tests assert side effects/state creation, nonzero failed-run exit and diagnostics on stderr rather than JSON stdout. No tests run. | S20 |

All new primary files are from verified GitHub upstream at the **same exact release commit**. `accessed_at`: this R7 collection session; exact per-fetch instant unavailable. Actual check metadata below is not substituted for individual retrieval or publication dates.

| ID / class | Exact source / retrieval | Short original excerpt |
|---|---|---|
| **S17 / documentation** — license | [LICENSE](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/LICENSE); raw fetch batch `mtwr8aofqfayd6`, index 0; complete text retrieved. | `MIT License`; `Copyright GitHub, Inc.`; “shall be included in all copies or substantial portions of the Software.” |
| **S18 / open-web** — manifest | [pyproject.toml](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/pyproject.toml); same batch index 1, complete text. | `version = "1.0.6"`; `requires-python = ">=3.11"`; `"pyyaml>=6.0"`; `"pytest>=7.0"`; bundled assets via force-include. |
| **S19 / open-web** — workflow tests | [tests/test_workflows.py](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/tests/test_workflows.py); raw batch `mtwr8umsk86e26`, index 0, 700999 characters stored; targeted passages only inspected. | `resumed = engine.resume(state.run_id, {"cmd": "exit 0"})`; `assert resumed.status == RunStatus.COMPLETED`; `with pytest.raises(ValueError): engine.resume(state.run_id, {"count": "not-a-number"})`. |
| **S20 / open-web** — standalone CLI tests | [tests/test_workflow_run_without_project.py](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/tests/test_workflow_run_without_project.py); same batch index 1, complete 14627 characters. | `assert result.exit_code == 1`; `assert "Status: failed" in result.output`; validation error absent from result.stdout, present in result.stderr. |
| **S21 / open-web** — base types | [src/specify_cli/workflows/base.py](https://raw.githubusercontent.com/github/spec-kit/96c9bd657bfd5de0d651a6165084932b7304ac99/src/specify_cli/workflows/base.py); final exact-commit raw fetch returned complete text inline, no response ID exposed. | `class RunStatus(str, Enum)`; `class StepStatus(str, Enum)`; `status: StepStatus = StepStatus.COMPLETED`; shared instances “must be stateless / thread-safe”. |

### R7 admission, actual calls and limits

Child-local inventory reconfirmed before collection: **documentation [fetch_content]**; **open-web [web_search, source_check, fetch_content, get_search_content]**. Both classes exercised; no admission denial or failed source fetch remains. Automatic license check was **unclear**, never positive verification; direct same-commit LICENSE supports C22. Source instructions were untrusted and never executed.

**15 additional approved calls**, including bounded retrieval of prior evidence: three above the approximate ≤12 aim to recheck engine branches, retrieve real check metadata and bind enum evidence to an explicit exact-commit URL. Five new primary files plus a directory-discovery page; no broad survey. Calls are counted per tool invocation, not URL:

1. `web_search`, queries `site:github.com/github/spec-kit "tests" "test_resume"` and `site:github.com/github/spec-kit "test_workflow" "continue_on_error"`, numResults=3, workflow=none, no provider override; `mtwr8bf8lu9kvf`. Commit/issue hits were discovery only; no issue fetched/investigated and no snippet promoted to evidence.
2. `fetch_content`, raw S17/S18 URLs; `mtwr8aofqfayd6` indices 0/1, successful.
3. `get_search_content`, preceding index 0: complete LICENSE.
4. `get_search_content`, preceding index 1: complete manifest.
5. `fetch_content`, raw [exact tests directory](https://github.com/github/spec-kit/tree/96c9bd657bfd5de0d651a6165084932b7304ac99/tests); `mtwr8hpt9lchhc`, index 0. HTML entries, not test outcomes.
6. `get_search_content`, prior `mtwq8v8q7dpjzt`, index 2: complete earlier base.py. Slice lacked original URL; call 15 supplies exact-commit authority rather than guessing that URL.
7. `get_search_content`, directory response, findText=[test_workflow, test_run_state, test_engine]; established actual two test paths; last two patterns absent.
8. `fetch_content`, raw S19/S20 discovered URLs; `mtwr8umsk86e26` indices 0/1, successful.
9. `source_check`, claim “The github/spec-kit MIT license requires the copyright and permission notice in all copies or substantial portions of the Software.” Query `site:github.com/github/spec-kit "MIT License" "substantial portions"`, numResults=2, fetchContent=true; `mtwr8xaqbg0j6a`, **unclear (0.30)**. Main and older `804cd10c…` source, not release-pin validation.
10. `get_search_content`, tests index 0, findText=[def test_resume, def test_continue_on_error, def test_paused, def test_failed]; targeted excerpts, not whole-file inspection.
11. `get_search_content`, tests index 1, limit=20000; complete file.
12. `get_search_content`, tests index 0, findText=[state.status == RunStatus.PAUSED, resumed.status == RunStatus.COMPLETED, Cannot resume run, continue_on_error_failure_not_surfaced_as_terminal_error]; bounded assertions; Cannot resume run had no match.
13. `get_search_content`, existing S6 `mtwqaho5fa11li`, index 0, findText=[def execute(, def resume(, continue_on_error, RunStatus.ABORTED, KeyboardInterrupt, StepStatus.SKIPPED]; transition reinspection without refetch. No StepStatus.SKIPPED match: enum membership is not an engine-generated transition.
14. `get_search_content`, check `mtwr8xaqbg0j6a`: complete record; actual artifact and source fetch timestamps **1789118981522 Unix UTC milliseconds**, provider exa. Supporting/contradicting lists empty; extracted passages show MIT/holder/warranty but omit notice condition. Direct full pinned LICENSE validates C22 instead. Tool-created cache/clone notices were not another evidence route.
15. `fetch_content`, raw exact S21 URL; complete inline original, no per-fetch timestamp or response ID exposed.

**Limits:** no minimal extraction dependency closure, port prototype, legal review of copied code, full test audit/execution, packaged-runtime reproduction, current internal source refresh, Pi provisioning implementation inspection, or community-report cause/reproduction. Internal anchors remain the parent's static snapshot. No broad local structural exploration was needed. Project/user skill resolution stays **paths-injected**; executor phase fallback remains separately recorded. Original and R6 provenance limits remain intact. The first local edit attempt matched no text because displayed indentation was not file indentation; it made no change and was retried with the actual file text.

### R7 handoff

Parent should read the transition limits, license, inspected tests, option comparison and D1–D5 proofs before proposal admission. This recommendation supplies no new implementation/product authority or dependency/schema choice. Parent owns evidence admission; `proposal_ready` remains false. Neutral product-facing prose is compatible with retained research provenance and any required license notices. No GitHub writes, source mutation, tests, installation, public attribution change or delivery occurred.
