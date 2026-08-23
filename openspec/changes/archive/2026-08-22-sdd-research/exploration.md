## Exploration: Optional source-backed `sdd-research` lane

### Current State

**Fresh repository verification (2026-08-21)**

- The current lifecycle still starts native planning at proposal. `resolveNextRecommended` returns `propose` whenever the proposal is missing, `ArtifactPaths` has no exploration or research field, and the frozen v1 projection accepts neither a research artifact nor a research route (`internal/sddstatus/status.go`, `internal/sddstatus/artifact_states.go`, `internal/sddstatus/status_v1.go`, `internal/assets/skills/_shared/sdd-status-contract.md`). Existing tests explicitly pin missing-proposal routing to `propose` and reject non-v1 route values.
- Pre-proposal behavior is prompt-orchestrated. The 12 source orchestrator templates start `/sdd-new` with exploration and proposal, while their dependency graph begins at proposal. The templates currently offer product questions only in interactive mode; automatic mode has no equivalent product-discovery step before proposal (`internal/assets/{antigravity,claude,codex,cursor,gemini,generic,hermes,kimi,kiro,opencode,qwen,windsurf}/sdd-orchestrator*.md`).
- Proposal ownership remains duplicated. The shared `sdd-propose` skill and Claude, Cursor, Kiro, and Kimi proposal agents instruct the proposal executor to conduct an interactive question round, even though product discovery is an orchestrator responsibility and automatic mode cannot safely invent unanswered product choices (`internal/assets/skills/sdd-propose/SKILL.md`, `internal/assets/{claude,cursor,kiro,kimi}/agents/sdd-propose.*`).
- External-source capability is still heterogeneous. Claude explicitly grants `WebFetch` and `WebSearch`; Kiro explicitly grants `@context7`; Cursor only declares terminal/MCP availability; Kimi inherits default tools while excluding delegation and file mutation; OpenCode's shipped overlays grant `read`, `write`, `edit`, and `bash` but no named external-source tool (`internal/assets/{claude,cursor,kiro,kimi}/agents/sdd-explore.*`, `internal/assets/opencode/sdd-overlay-*.json`).
- The canonical `AgentCapabilityManifest` is an immutable v1 integration projection. Its fields cover output styles, commands, file sub-agents, skills, system prompts, MCP, and workflows; all 16 registered agents advertise MCP, so MCP cannot discriminate documentation or open-web research capability. The manifest also intentionally contains no live runtime observations (`internal/agents/capabilitymanifest/manifest.go`, `internal/agents/capability_manifest.go`).
- Hybrid recovery is documented but underspecified for a new pre-proposal gate. The persistence contract requires both Engram and `state.yaml`, while the only state example tracks the last phase and proposal/spec/design/tasks booleans. No current `openspec/changes/**/state.yaml` exists, and no `sdd/sdd-research/state` observation exists, so there is no typed state shape for research intent, evidence classes, admission, or confirmed product decisions (`internal/assets/skills/_shared/{persistence-contract,engram-convention,openspec-convention}.md`).
- Phase installation is distributed across several inventories: `SkillID` and catalog entries, injector skill IDs, `opencode.SDDPhases`, `profilePhaseOrder`, Claude/Kiro/Codex model presets and TUI pickers, OpenCode overlays, native agent/command directories, uninstall lists, and golden/static tests. Current profile tests pin 10 phase agents plus one orchestrator (11 agents per profile; 33 for three profiles), so adding research changes those expectations to 12 and 36 respectively (`internal/model/types.go`, `internal/catalog/skills.go`, `internal/opencode/models.go`, `internal/components/sdd/{inject,profiles,prompts}.go`, `internal/tui/screens/*model_picker.go`).
- The source orchestrator specification requires parity across 12 templates and maps them to all 16 supported AgentIDs. Windsurf remains a separate lifecycle hazard: its native workflow writes `.sdd/proposal.md` and `.sdd/spec.md` before approval instead of using the shared OpenSpec/Engram order (`openspec/specs/sdd-orchestrator-assets/spec.md`, `internal/assets/windsurf/workflows/sdd-new.md`).
- The prior exploration's core conclusions remain valid, but its runtime/inventory scope was too narrow and its restart recommendation lacked a concrete state admission boundary. The fresh evidence supports a separate capability contract rather than extending or inferring from the generic v1 manifest.

**Confirmed maintainer inputs**

- `sdd-research` is optional at lifecycle selection, first-class once selected, source-backed, separate from local `sdd-explore`, capability-gated, and fail-closed.
- Product discovery remains orchestrator-owned after exploration/research and before proposal. `sdd-propose` consumes confirmed decisions and does not interview the user.
- Bash, generic MCP availability, and unverified inherited tools are not research capability evidence.

### Affected Areas

- `internal/assets/skills/sdd-research/SKILL.md` and shared persistence/OpenSpec conventions — define phase admission, evidence schema, `research.md`, Engram topic `sdd/{change-name}/research`, and blocked/partial behavior.
- `internal/agents/capabilitymanifest/` or a sibling research-capability package — add a separate versioned source-class declaration; do not infer from or silently widen the immutable generic v1 feature manifest.
- `internal/assets/{claude,cursor,kiro,kimi}/agents/sdd-research.*` and `internal/assets/opencode/sdd-overlay-{single,multi}.json` — install dedicated executors with explicit evidence-tool grants and default-deny unsupported classes.
- `internal/assets/{claude,opencode}/commands/`, `internal/components/sdd/commands.go`, and Windsurf workflow assets — expose the first-class command where the host supports commands and reconcile lifecycle order.
- All 12 `internal/assets/*/sdd-orchestrator*.md` templates — add optional parallel dispatch, the pre-proposal gate, automatic-mode product-decision blocking, recovery behavior, and platform-accurate capability wording.
- `internal/assets/skills/sdd-propose/SKILL.md` and proposal agents — remove interview ownership; require confirmed product decisions and accept optional exploration/research references.
- `internal/model/`, `internal/opencode/models.go`, `internal/components/sdd/{inject,profiles,prompts}.go`, TUI model pickers, catalog, uninstall, and persisted assignment handling — register the phase and its model/profile identity everywhere.
- `internal/sddstatus/` and `internal/assets/skills/_shared/sdd-status-contract.md` — compatibility tests and orchestrator guidance only for the recommended approach; native status v1 remains byte-shape and token compatible.
- Asset, component, CLI, model, TUI, state, profile, capability, and golden tests — verify inventory parity, explicit grants, default denial, state recovery, proposal ownership, and unchanged status-v1 routing.
- `bench/journeys_sdd.go` and CI benchmark evidence — affected only if native binary routing changes; prompt/asset routing is not proven by a binary journey.

### Approaches

1. **Fold external research into `sdd-explore`** — let the existing phase use whatever network mechanism happens to be available.
   - Pros: Smallest inventory change.
   - Cons: Violates the confirmed responsibility split, conflates local and external evidence, and cannot distinguish real source capability from ubiquitous MCP/Bash claims.
   - Effort: Low

2. **Prompt-only research phase with self-reported tools** — add `sdd-research` assets and let each executor decide whether it appears capable.
   - Pros: Keeps native status unchanged and avoids new Go capability types.
   - Cons: Capability truth remains duplicated across prompts, inherited tool sets can be misreported, and cross-runtime drift has no machine-checked authority.
   - Effort: Medium

3. **Versioned research capability plus orchestrator pre-proposal gate** — add a dedicated source-class contract and first-class phase, persist pre-proposal state, but leave native status v1 unchanged.
   - Pros: Fails closed with typed evidence classes, preserves legacy status consumers, supports parallel local/external investigation, and centralizes proposal readiness without making research mandatory for old changes.
   - Cons: Broad asset/model/profile churn; native status still reports `propose` before the orchestrator applies its pre-proposal gate.
   - Effort: Medium/High

4. **Native status v2 with research artifacts and routes** — expose research directly through a new native status version and dispatcher.
   - Pros: Native visibility and direct resumability.
   - Cons: Requires a versioned wire contract, downstream decoder migration, CLI changes, and driven bench coverage; optional intent and product decisions still need persisted state.
   - Effort: High

### Recommendation

Use approach 3.

1. Define a dedicated versioned research-capability declaration with closed evidence classes such as `documentation` and `open-web`. Missing classes or unknown declarations deny. Keep it separate from the immutable generic capability v1; `MCP: true`, Bash, shell-installed clients, or inherited unnamed tools never satisfy admission.
2. Apply two-layer admission: the adapter/runtime declaration states the maximum supported source classes, then the phase actor verifies that the exact granted tool for every requested class is present before accessing sources. Initially advertise Claude for `documentation` plus `open-web`, Kiro for `documentation`, and deny every other runtime/class until an explicit asset and parity test prove support.
3. Treat the lane as optional only before selection. Once requested, persist its required evidence classes before dispatch and require `done` for proposal readiness; `blocked` or `partial` must not silently degrade into skipped research. A blocked phase should persist its admission record without source claims so restart behavior is deterministic.
4. Make `research.md` source-backed and auditable: admission verdict and observed grants; research questions; source records with stable IDs, class, title, publisher, URL, access date, and bounded excerpts; claim-to-source mappings; contradictions, uncertainty, and freshness limits; and a handoff that explicitly separates evidence from product choices.
5. Orchestrate: intake/preflight → persist research intent → run `sdd-explore` and admitted `sdd-research` concurrently when supported → validate both artifacts → orchestrator-owned product discovery → persist confirmed decisions → launch `sdd-propose`. Only the orchestrator writes shared DAG state, preventing parallel phase races.
6. Expand the state contract with a version/revision and a `preproposal` block covering exploration outcome, research requested/classes/admission/outcome/artifact references, product-decision status, and `proposal_ready`. In hybrid mode write equivalent state to both backends; if either write fails or recovered revisions disagree, stop before proposal rather than choosing the more convenient copy.
7. Make product discovery mode-independent. Interactive mode may run iterative question rounds. Automatic mode may use already-confirmed facts, but unresolved product choices produce one lossless grouped blocking prompt and stop; research evidence never becomes implicit product consent.
8. Remove question-round logic from every proposal executor. `sdd-propose` must require `product_decisions: confirmed`, read optional exploration/research artifacts, and return `blocked` if the orchestrator handoff is absent or unresolved.
9. Keep status v1 unchanged. When native or manual status returns `propose`, every orchestrator first evaluates the persisted pre-proposal gate. Existing changes with a proposal continue unchanged; changes with no research request skip only the research branch, not product-decision confirmation. A status v2 is separate future scope.
10. Update the complete 12-template/16-AgentID asset surface, command and skill inventories, model presets/pickers, profile generation, uninstall ownership, and Windsurf's divergent workflow. Avoid repeating the current explore contradiction where the skill permits a named artifact but runtime agent prose prohibits every project-file write; source access is read-only, while persistence is narrowly limited to `research.md` and Engram.
11. Test declaration/default-deny logic, exact tool-grant parity, evidence-schema acceptance, hybrid restart mismatch, automatic-mode blocking, proposal handoff enforcement, command/agent/profile inventories, model assignment round trips, Windsurf ordering, and golden outputs. Add explicit status-v1 regressions proving an empty change still routes to `propose` and a research token remains rejected by v1.
12. Do not add a bench journey for prompt-only routing. If implementation changes native `sdd-status`/`sdd-continue` semantics, audit the journey corpus for stale old-route pins and prove the new journey through CI's locally built `gentle-ai-bench run --binary ...` invocation; `go test ./bench` alone is declaration validation, not driven evidence.

The implementation is likely to exceed the session's 800-line review budget because it spans shared contracts, 12 orchestrator templates, runtime assets, inventories, tests, and goldens. With `ask-on-risk`, `sdd-tasks` should plan reviewable slices and request the delivery decision before apply if its forecast remains high.

### Risks

- Static capability declarations can overstate a live runtime unless exact phase grants are checked and tested as a second gate.
- Prompt-owned orchestration can bypass research if any of the 12 templates launches proposal directly from native `propose` status.
- Hybrid state has no atomic cross-store transaction; revision mismatch must block rather than silently prefer stale Engram or filesystem state.
- Broad inventory duplication can install a skill without its command, model assignment, profile agent, uninstall ownership, or runtime permission.
- Moving product discovery out of proposal must be atomic across shared skills and runtime-specific proposal agents to avoid either duplicate questions or no questions.
- Windsurf can continue generating incompatible `.sdd/` artifacts unless its workflow is migrated or explicitly retired.
- The expected change is high-risk against the 800-line review budget and should be sliced without separating tests from their owning contract changes.

### Ready for Proposal

Yes. The proposal should formalize approach 3 as a new `sdd-research` capability plus modifications to `sdd-orchestrator-assets`: versioned evidence-class admission, source/claim schema, restart-safe pre-proposal state, mode-independent orchestrator product discovery, proposal handoff ownership, status-v1 compatibility, complete runtime inventory parity, and the conditional bench rule. The maintainer decisions supplied for this change are confirmed, so no additional product question round is required before `sdd-propose`.
