# Codex agent-role parity with OpenCode / Claude Code

Locator: `odd/tasks/codex-agent-role-parity.md` · Engram mirror: `odd/codex-agent-role-parity/tasks` (not written — no callable `mem_*` tool in this session; see Notes)
Branch: `feat/codex-agent-role-parity` (worktree `~/gentle-ai-worktrees/codex-agent-role-parity`, base `origin/main` 3a19dbf8)

## Objective

`codex` exposes the same named agents that gentle-ai already projects into `claude-code` and `opencode`, so a user who opens Codex finds the gentle-ai default agents already available there — on this machine immediately (T1) and for every user through `gentle-ai install/sync --agent codex` (T2–T4).

## Problem

- gentle-ai ships named agents for `claude-code` (19 files in `~/.claude/agents/`) and for `opencode` (agent entries in `opencode.json`), but the Codex adapter has no named-agent projection at all. `internal/assets/codex/` contains only `orchestrator.md`, and the installed `~/.codex/config.toml` has `[agents] max_depth/max_threads` with zero role entries.
- Result: Codex has the gentle-ai prompt (100 KB `AGENTS.md`), skills, and MCP servers, but no `spawn_agent` role catalogue. SDD/JD/review phases cannot be delegated to the canonical role names there.
- The existing Codex multi-agent path is prompt-only (`features.multi_agent` + capability text in `AGENTS.md`), so reviewers/orchestrators that expect `sdd-*` / `jd-*` / `review-*` agents find nothing to route to.

## Why

User requirement (2026-09-26): `nan`, `gentle-ai` and `gentle-shell` must work in Codex, OpenCode and Claude Code, and entering any of those three tools must already expose the gentle-ai default agents. OpenCode and Claude Code already comply; Codex is the gap.

## Evidence: Codex role mechanism (verified against 0.151.0 on this machine)

Codex supports user-defined agent roles through **two** independent routes (`codex-rs/agent-roles/src/{loader,discovery,agent_role_config}.rs`):

1. Declared in a config layer: `[agents.<name>] description = "..."` + optional `config_file = "<path>"` / `nickname_candidates = [...]`.
2. **Directory discovery, always on**: every `*.toml` under `<config-folder>/agents/`, recursively. `config_file` is not required.

Verified empirically on 0.151.0 (`CODEX_HOME` scratch home + `codex doctor`):

- A malformed role file under `<CODEX_HOME>/agents/` is surfaced as `startup warning  Ignoring malformed agent role definition: ...` — so discovery is live and `codex doctor` is a deterministic offline oracle.
- Discovery does **not** require `features.multi_agent = true`.
- Role-file schema (`parse_agent_role_file_contents`, `deny_unknown_fields`): a TOML table with non-empty `name`, a non-blank `description`, non-blank `developer_instructions`, optional `nickname_candidates` (≥1 entry, no blanks, no duplicates, ASCII letters/digits/spaces/hyphens/underscores only), plus any valid `ConfigToml` key as that role's own config layer. Unknown keys are rejected outright.
- `developer_instructions` is the role's own config layer, so a role file needs **no** `~/.codex/config.toml` change at all.

Consequence for gentle-ai: writing `~/.codex/agents/<role>.toml` is enough, and it must not collide with the existing `[agents]` table in `config.toml`.

## Scope (authorized)

### T1 — this machine (already delivered)

- Role set (user decision, 2026-09-26): identical to the OpenCode/Claude Code managed set as installed by gentle-ai 3.7.0 on this machine — `sdd-init`, `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify`, `sdd-archive`, `sdd-onboard`, `sdd-research`, `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`, `review-risk`, `review-resilience`, `review-readability`, `review-reliability`, `review-refuter` (**19 roles**). No ODD worker roles (`gentle-ai-explore/worker/verify`) in this slice.
- Format: one `~/.codex/agents/<role>.toml` per role, generated from the installed gentle-ai agent assets; runtime-adapted (skill-root paths, delegation/tool vocabulary), never a blind copy.
- `~/.codex/config.toml` is **out of scope**: directory discovery makes it unnecessary, and editing it risks the existing `[agents]` table.

### T2–T4 — upstream (scope narrowed 2026-09-26)

Scout result on `origin/main` `3a19dbf8`: the SDD retirement (`e219644b2`) deleted `internal/assets/*/sdd-*` agent assets for **every** runtime, so upstream ships **8 owned agents only**, enumerated by `reviewassets.NativeAgentManifest` (`internal/components/reviewassets/install.go:22`): `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`, `review-risk`, `review-resilience`, `review-readability`, `review-reliability`, `review-refuter`. The 11 local `~/.claude/agents/sdd-*.md` files are gentle-ai 3.7.0 remnants, not something `main` reinstalls.

- **User decision (2026-09-26):** the upstream Codex projection covers **only those 8 managed roles**. Restoring `sdd-*` assets is explicitly NOT in scope; parity means "the same named agents gentle-ai manages for every runtime", not "the stale 3.7.0 set".
- Consequence for T1's local generator: if a future gentle-ai upgrade removes `~/.claude/agents/sdd-*.md`, `project_from_source` fails loudly with the missing role names instead of silently writing a smaller set. Accepted; T2 supersedes it.

Out of scope / non-goals:

- `SupportsSubAgents()` must stay `false` (see Constraints).
- `nan` in Claude Code (user decision, 2026-09-26: do not touch). `api.nan.builders` is OpenAI-compatible; `POST /v1/messages` returns 404, so Claude Code cannot use it without an Anthropic-compatible proxy.
- No change to OpenCode or Claude Code projections: both are already at parity on this machine.
- No `codex` model/reasoning mapping from Claude agent frontmatter (`model: sonnet` is meaningless for a Codex provider).

## Constraints

- **Do not flip `SupportsSubAgents()` / `SubAgentsDir()` / `EmbeddedSubAgentsDir()`.** `TestAdapterSubAgentsStayFalse` is a regression guard: those accessors are consumed by `sdd/inject.go` and `uninstall/service.go`, which copy `assets.FS.ReadDir(embeddedDir)` — an empty `EmbeddedSubAgentsDir()` there would copy the whole embedded root into `~/.codex/agents/`. The Codex projection needs its own targeted writer with an explicit role allowlist.
- Idempotent re-runs; untouched user-defined `~/.codex/agents/*.toml` (no gentle-ai ownership marker) are never modified or deleted.
- Generated role files must satisfy the Codex schema exactly; `nickname_candidates` names are ASCII-sanitized and unique per role.
- Generated technical artifacts in English; keep existing repo conventions (`gofmt`, `go vet`, tests) for T2–T4.

## Tasks

- [x] T1 — Local projection on this machine (immediate value; artifact lives outside the repo at `~/.local/bin/` and `~/.codex/agents/`): single-file idempotent generator with `--dry-run`, `--check`, `--selftest`; generates the 19 role TOMLs from `~/.claude/agents/*.md` with a documented adaptation table; `--selftest` builds a scratch `CODEX_HOME` and asserts against `codex doctor` with a negative control. Route: inline (one authored file). Done 2026-09-26 — 19/19 roles generated and loaded; see Delivery.
- [ ] T2 — Upstream Codex agent-role writer for the 8 managed roles: reuse `reviewassets`' manifest, RDD gating and ownership ledger; add an `md` → Codex role-TOML rendering path; add Codex-format agent assets for the 8 roles; wire the install and sync stage plans; inventory test asserting all 8 roles are written with schema-valid TOML. Route: delegated (writer trigger: 2+ non-trivial files).
- [ ] T3 — Lifecycle: `sync` idempotency (byte-identical second run), retired-role reconciliation against owned files, and uninstall parity with claude-code (native agents are preserved today — `TestCompleteUninstallPreservesNativeReviewAndJudgmentDayAgents`), plus regression tests. Route: delegated (same writer, sequential).
- [ ] T4 — Sandbox e2e in an isolated HOME: fresh `install --agent codex` → the 8 role files present and `codex doctor` reports zero malformed-role warnings, re-run byte-identical, uninstall leaves owned files intact as designed. Resolve the open inheritance question here. Route: inline (bounded action).

## Acceptance criteria

1. T1: 19 role files exist under `~/.codex/agents/`; `codex doctor` on the real HOME reports zero `Ignoring malformed agent role definition` warnings; `--selftest` proves the oracle is not vacuous (negative control warns); `--check` reports no drift; re-run is byte-identical.
2. T1: `~/.codex/config.toml` is byte-identical before and after the projection.
3. T2–T4: `gentle-ai install/sync --agent codex` produces the 8 managed roles in an isolated HOME; each generated role file is accepted by Codex (zero malformed-role warnings); `go test ./...` green; `go vet` and `gofmt -l .` clean.
4. `SupportsSubAgents()`, `SubAgentsDir()` and `EmbeddedSubAgentsDir()` stay `false`/empty for Codex; `TestAdapterSubAgentsStayFalse` passes unchanged; `~/.codex/config.toml` is never written by the new path.
5. A pre-existing user-authored `~/.codex/agents/<role>.toml` that gentle-ai does not own survives install, sync and uninstall untouched.

## Checks

- T1: `~/.local/bin/gentle-codex-agent-roles.py --dry-run`, `--selftest`, `--check`; `codex doctor` on the real HOME (before/after warning count); `diff` of `~/.codex/config.toml` before/after.
- T2–T4: `go test ./internal/agents/... ./internal/assets/... ./internal/components/... ./internal/cli/...`; `go test ./...`; `go vet ./...`; `gofmt -l .`; isolated-HOME sandbox run.

## Delivery

- 2026-09-26 · T1 committed as `docs(odd): track codex agent-role parity feature` (`bffa2d63`) plus the T1 close-out commit that records this evidence and the `T1 done` state. The T1 artifact itself is machine-local (`~/.local/bin/gentle-codex-agent-roles.py` + `~/.codex/agents/*.toml`) and intentionally not committed to this repo: T2 replaces it with the managed writer.
- No PR yet — push and PR are the user's decision.

## Evidence — T1 (2026-09-26, Codex 0.151.0, this machine)

Test-first, oracle = `codex doctor` in scratch `CODEX_HOME` homes (offline, ~5 s per run):

- RED (before generation): `gentle-codex-agent-roles.py --selftest` → exit 1, `SELFTEST FAILED — target ~/.codex/agents not in sync: missing=19 stale=0 extra=0`; `~/.codex/agents/` did not exist. Same run: `negative control warnings 1` (a deliberately description-less role file IS reported), so the oracle can fail.
- GREEN (after generation): `SELFTEST PASSED · 19 roles · 19 files in sync`; `clean projection warnings 0`; `roles proven loaded 19/19` — positive proof comes from a second file declaring each role name, which makes Codex emit `duplicate agent role name \`<role>\` discovered in …` for every one of the 19 roles.
- Real HOME: `codex doctor` reports zero role warnings (no `startup warning` line at all) and `auth ✓`.
- `~/.codex/config.toml` sha256 `dad9bb61a3dc1e1dfe988b26c7eb80032890936520de3d465a331608e705ea58` before and after the projection — byte-identical, criterion 2 satisfied without touching the file.
- Idempotency: direct re-run → 0 add / 0 update / 0 remove / 19 unchanged; directory digest identical.
- `--check` → `IN SYNC`, exit 0. `--dry-run` → writes nothing.

Adaptation applied to every body (checked: zero remaining `.claude` references in the generated set): `~/.claude/skills/` → `~/.codex/skills/`, `~/.claude/agents/` → `~/.codex/agents/`, “the Task tool” → “the `spawn_agent` tool”, `TodoWrite` → `update_plan`, `multi_tool_use.parallel` → “parallel tool calls”, plus a short Codex runtime-profile header.

## Notes / open questions

- 2026-09-26: exploration (read-only) established the mechanism before any write: Codex 0.151.0 role discovery verified with scratch `CODEX_HOME` homes, `codex-rs/agent-roles/*` read from `openai/codex@main`, the 19-role set confirmed in `~/.claude/agents/`, and `~/.codex/skills/_shared/` confirmed to mirror the Claude skill tree (so skill-root rewriting is the only required path adaptation).
- 2026-09-26: repo scout (delegated, `gentle-ai-explore`) mapped the upstream install/sync/uninstall path and found that the SDD retirement removed all `sdd-*` agent assets upstream — basis for the T2–T4 scope narrowing above. It also reported: no repo-side reference to `nickname_candidates`, `developer_instructions`, or `~/.codex/agents`; `sddSubAgentPaths` (`internal/cli/run.go:3325`) and `internal/update/upgrade/executor.go:245` copy every entry of `EmbeddedSubAgentsDir()` behind a `SupportsSubAgents()` guard only; `reviewassets` owns the only agents-directory writer plus a per-file sha256 ownership ledger (`.gentle-ai-native-agent-ownership.json`) and a retired-agent manifest; uninstall has no `SubAgentsDir` branch and deliberately preserves native agents; the Codex capability manifest is digest-pinned (`internal/agents/capabilitymanifest/manifest_test.go:134,218`).
- Codex applies a role as a **bounded override layer** over the parent-derived child config (`codex-rs/core/src/agent/role.rs`: `AgentRoleOverrides` = `developer_instructions`, `model`, `model_reasoning_effort`, `model_reasoning_summary`, `model_verbosity`, `personality`, `service_tier`, `features`, `skills`; a role may only reduce capabilities). Two consequences: (a) a role file needs no `config.toml` change, and (b) per-role `model` / `model_reasoning_effort` overrides are available for a later parity slice if the OpenCode/Claude model mapping should be mirrored.
- Open question for T4: whether the child receives the parent's `model_instructions_file` (engram instructions) *in addition* to the role's `developer_instructions`, or instead of it. Resolving this needs one live spawn (`codex exec -p nan …`), which is a billable call — do it in T4, not inline.
- The `review-*` and `jd-*` personalities in the Claude set are wired to the native RDD review transport (`GENTLE_AI_REVIEW_BINDING` / `GENTLE_AI_REVIEW_CONTEXT` injection). As Codex roles they are inert for native review and only useful for manual invocation until Codex grows an equivalent transport — accepted as part of the requested parity set.
- The Engram mirror was not written: this session has no callable `mem_*` tool.
