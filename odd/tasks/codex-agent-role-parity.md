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

- Role set (user decision, 2026-09-26): identical to the OpenCode/Claude Code managed set — `sdd-init`, `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify`, `sdd-archive`, `sdd-onboard`, `sdd-research`, `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`, `review-risk`, `review-resilience`, `review-readability`, `review-reliability`, `review-refuter` (**19 roles**). No ODD worker roles (`gentle-ai-explore/worker/verify`) in this slice.
- Format: one `~/.codex/agents/<role>.toml` per role, generated from the installed gentle-ai agent assets; runtime-adapted (skill-root paths, delegation/tool vocabulary), never a blind copy.
- `~/.codex/config.toml` is **out of scope**: directory discovery makes it unnecessary, and editing it risks the existing `[agents]` table.

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

- [ ] T1 — Local projection on this machine (immediate value; artifact lives outside the repo at `~/.local/bin/` and `~/.codex/agents/`): single-file idempotent generator with `--dry-run`, `--check`, `--selftest`; generates the 19 role TOMLs from `~/.claude/agents/*.md` with a documented adaptation table; `--selftest` builds a scratch `CODEX_HOME` and asserts against `codex doctor` with a negative control. Route: inline (one authored file).
- [ ] T2 — Upstream Codex agent-role writer: embedded role assets (`internal/assets/codex/agent-roles/*.toml` or an equivalent rendered source), a targeted install writer in `internal/agents/codex` (or the Codex component), ownership marker, and an inventory test asserting all 19 roles are written with valid schema. Route: delegated (writer trigger: 2+ non-trivial files).
- [ ] T3 — Lifecycle: `sync` idempotency (byte-identical second run), ownership-safe upgrade, `uninstall` cleanup of owned role files, and regression tests. Route: delegated (same writer, sequential).
- [ ] T4 — Sandbox e2e in an isolated HOME: fresh `install --agent codex` → 19 role files present, `codex doctor` reports zero malformed-role warnings, re-run is byte-identical, uninstall removes owned files only. Route: inline (bounded action).

## Acceptance criteria

1. T1: 19 role files exist under `~/.codex/agents/`; `codex doctor` on the real HOME reports zero `Ignoring malformed agent role definition` warnings; `--selftest` proves the oracle is not vacuous (negative control warns); `--check` reports no drift; re-run is byte-identical.
2. T1: `~/.codex/config.toml` is byte-identical before and after the projection.
3. T2–T4: `gentle-ai install/sync --agent codex` produces the same 19 roles in an isolated HOME; `go test ./...` green; `go vet` and `gofmt -l .` clean.
4. Uninstall removes only owned role files; a pre-existing user role file survives install, sync and uninstall.

## Checks

- T1: `~/.local/bin/gentle-codex-agent-roles.py --dry-run`, `--selftest`, `--check`; `codex doctor` on the real HOME (before/after warning count); `diff` of `~/.codex/config.toml` before/after.
- T2–T4: `go test ./internal/agents/... ./internal/assets/... ./internal/components/... ./internal/cli/...`; `go test ./...`; `go vet ./...`; `gofmt -l .`; isolated-HOME sandbox run.

## Delivery

(no PR yet — push and PR are the user's decision)

## Notes

- 2026-09-26: exploration only — no writes. Verified Codex 0.151.0 role discovery and `codex doctor` warning behaviour with scratch `CODEX_HOME` homes; read `codex-rs/agent-roles/*` from `openai/codex@main`; confirmed the 19-role set exists in `~/.claude/agents/`; confirmed `~/.codex/skills/_shared/` mirrors the Claude skill tree (so skill-root rewriting is the only required path adaptation).
- The Engram mirror was not written: this session has no callable `mem_*` tool.
