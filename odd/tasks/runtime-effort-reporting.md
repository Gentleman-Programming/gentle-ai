# Runtime telemetry: report effort and classify built-in agents

## Objective

Stop emitting `selected_effort="unavailable"` and `agent_class="unknown"` when the value is
actually knowable, so the public open-data page reflects real usage going forward.

## Problem (root cause, read-only investigation 2026-09-27)

Emitters initialise the label to `unavailable`/`unknown` and only overwrite it with selection
evidence; the collector, backfill and exporter pass labels through verbatim (no pipeline bug).
- OpenCode: `internal/telemetry/runtime_opencode.go:117` defaults to `unavailable`; only replaced
  when the gentle-ai assignment has an effort (`components/telemetryruntime/opencode.go:146-148`)
  or the v2 plugin sends `selectedEffort` (`:130-139`). Dominant contributor (~5.67M rows).
- Claude Code: `internal/telemetry/runtime_claude.go:227` drops the agent definition on `Stop`,
  so the orchestrator never reports effort (`:231`); subagents only when frontmatter has `effort`.
- Claude Code: `ClaudeNamedAgent` (`runtime_claude.go:122-127`) excludes `explore`/`worker`, so
  built-ins `general-purpose`/`Explore` land in `custom/unknown`.
- Codex: observed effort goes to `effective_effort` only; small volume. Out of scope.

## Scope (authorized by the user, 2026-09-27: "si a todo")

- T1 Claude: map built-in `general-purpose` → worker and `Explore` → explore.
- T2 Claude: report the orchestrator's selected effort on `Stop` from the effort Claude Code
  actually exposes (settings/env), only when verified from primary evidence; otherwise keep
  `unavailable` and document why.
- T3 OpenCode: resolve the selected effort from the agent/model configuration (opencode.json
  variant/reasoningEffort) when the plugin does not send it; keep `unavailable` when no
  selection evidence exists.

Out of scope: rewriting historical data, collector renormalisation, Codex fallback.

## Constraints

- Privacy: only enumerated effort values; never free text.
- Test-first with `go test ./internal/telemetry/... ./internal/components/telemetryruntime/...`.
- Only forward-looking: needs a gentle-ai release; push/PR/release are user decisions.
- Forecast ~300 authored changed lines; delivery strategy ask-on-risk.

## Tasks

- [x] T1 Claude built-in agent mapping (test-first). Route: delegated writer. Risk tier: high
  (telemetry/privacy surface). `ClaudeNamedAgent` keeps excluding the generic class names on
  purpose: it gates reading `~/.claude/agents/<name>.md` and a user agent literally named
  `worker`/`explore` is not a built-in (commit 2c566abef, comment "generic aggregate classes are
  not installable Claude agent names"). A separate exact, case-sensitive map adds only the
  built-in `general-purpose` → `worker` and `Explore` → `explore`; no definition file is read
  for them. Evidence: RED `TestClaudeRuntimeBuiltInSubagentsMapToAggregateClasses` failed with
  `general-purpose: got custom/unknown, want built_in/worker` and
  `Explore: got custom/unknown, want built_in/explore`; GREEN after the change; the full
  verification set passed (see Verification evidence). Commit: pending (parent-owned).
- [x] T2 Claude orchestrator effort (evidence-gated, test-first). Route: delegated writer.
  Risk tier: high. Primary evidence (fetched 2026-09-27): https://code.claude.com/docs/en/hooks
  common input field `effort` = object with `level` in `low|medium|high|xhigh|max`, "the effort
  level in effect when the hook runs", reporting the level Claude Code ran after fallback
  (ultracode reports as `xhigh`), present on `Stop`/`SubagentStop` when the model supports
  effort. Because it is the level in effect, it maps to `effective_effort` only, honoring the
  documented contract that selected and effective effort are independent evidence (the Codex
  adapter follows the same rule). Applied to `Stop` only; `SubagentStop` keeps `unavailable`
  because the contract does not establish whether that level is the subagent's or the session's.
  `selected_effort` for the orchestrator stays `unavailable`: settings `effortLevel` /
  `modelSettings` (https://code.claude.com/docs/en/settings-reference) are saved defaults that
  `/effort` session picks, `--effort`, `CLAUDE_CODE_EFFORT_LEVEL`, project/managed files, and
  per-model rules override (https://code.claude.com/docs/en/model-config), so reading
  `~/.claude/settings.json` would guess the selection. A malformed/non-object `effort` never
  discards the hook. Evidence: RED `TestClaudeRuntimeStopReportsHookEffortAsEffective` failed
  for the five documented levels (`effective effort = "unavailable", want "low"` ...); GREEN
  after the change. Commit: pending (parent-owned).
- [x] T3 OpenCode effort from configuration (test-first). Route: delegated writer. Risk tier:
  high. V1 already read `agent.<name>.variant` / `model#variant`; the gap was agents with no
  variant. V1 now falls back to the documented `reasoningEffort` option: `agent.<name>` options
  (https://opencode.ai/docs/agents/, "Additional options") override the response model's
  `provider.<id>.models.<id>.options` (https://opencode.ai/docs/models/, "Configure models").
  Only `off|minimal|low|medium|high|xhigh|max` (the V2 plugin allowlist) is accepted; OpenAI's
  `none`, meta values, free text, and non-strings stay `unavailable`. Any configured agent
  variant (valid, custom, or model-less) withholds the fallback because it can override the
  options. V2 is unchanged: docs/telemetry.md states V2 "never consults ambient configuration"
  (commit e6c64e02b) and the plugin already sends the runtime `model.variant`. Evidence: RED
  `TestSendOpenCodeResolvesConfiguredReasoningEffort` failed on the four positive cases
  (`selected effort = "unavailable" ... want selected "high"` ...), then on the added
  model-less-variant guard case (`selected effort = "high"`); GREEN after each change.
  Commit: pending (parent-owned).
- [ ] T4 Native review and handoff (push/PR decision to the user). Route: parent.

## Acceptance criteria

- Unit tests prove: Claude `general-purpose`/`Explore` rows classify as worker/explore; Claude
  `Stop` rows carry the configured effort when present; OpenCode rows carry the configured
  effort when present; all fall back to `unavailable` without evidence.
- `go test ./...` for the touched packages green; `go vet` clean.

## Progress

- 2026-09-27: feature document created from the read-only root-cause investigation.

## Verification evidence

- T1: `go test ./internal/telemetry/... ./internal/components/telemetryruntime/...` ok;
  `go vet` same packages clean; `gofmt -l internal/telemetry internal/components/telemetryruntime`
  printed nothing; `go build ./...` ok; extra focused `go test ./internal/cli/ -run Claude` ok.
  Runtime harness: N/A (pure normalizer change covered by unit tests).
  Rollback boundary: `claudeBuiltInAgentClass` and its call in `NormalizeClaude` plus the test.
- Final run after T1–T3 (all with `-count=1` for tests):
  `go test ./internal/telemetry/... ./internal/components/telemetryruntime/...` ok (both packages);
  `go vet ./internal/telemetry/... ./internal/components/telemetryruntime/...` exit 0;
  `gofmt -l internal/telemetry internal/components/telemetryruntime` printed nothing;
  `go build ./...` exit 0; extra focused `go test ./internal/cli/ -run 'Claude|OpenCode'` ok.
  Runtime harness: N/A for all three (pure normalizer/adapter changes covered by unit tests).
  Rollback boundaries: T2 = `EffortLevel`, `claudeHookEffort`, the `Effort` source field, the
  `Stop` branch assignment, and its test; T3 = `readOpenCodeConfig`, `openCodeAssignment`,
  `openCodeOptionEffort`, the V1 branch in `sendOpenCode`, and its test.

## Open items for the parent

- Commits were not created: the delegated writer contract forbids staging/committing.
- `docs/telemetry.md` is outside the writer's edit surface and is now stale in three places:
  lines 274-276 (Claude `SubagentStop` classification must mention `general-purpose` → `worker`
  and `Explore` → `explore`), lines 310-313 ("The hook contract does not expose effective
  effort" is no longer true for `Stop`), and lines 246-253 (V1 now also reads the documented
  `reasoningEffort` agent/model options when no variant is configured).

## Next step

Start after the landing feature `open-data-mobile-and-coverage` T1–T3 (single writer at a time).
