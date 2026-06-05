# Proposal: fix-skill-duplication-and-migrate

## Intent

Claude Code v2.x indexes both `~/.claude/skills/` and `~/.claude/commands/` into the same `/` slash-command picker, so every SDD phase that gentle-ai installs in BOTH directories shows up TWICE in the picker (`sdd-apply`, `sdd-archive`, `sdd-explore`, `sdd-init`, `sdd-onboard`, `sdd-verify`). That duplication is confusing — users see two visually identical entries for the same workflow and don't know which to pick.

This change hides the SDD `SKILL.md` files from the user-facing picker (and from the model's auto-invocation) by adding two YAML frontmatter flags. Agents and commands that read those files via `Read` keep working unchanged, because the path doesn't move.

Success looks like: in Claude Code v2.1.131 with the next gentle-ai sync, each `sdd-*` slash command appears EXACTLY ONCE in `/`, and that single entry comes from `~/.claude/commands/sdd-*.md`. No path migration, no orphan files, no cross-adapter ripple.

## Scope

### In scope

1. Edit the embedded SKILL.md files for the 10 SDD phases under `internal/assets/skills/`:
   - `sdd-apply`, `sdd-archive`, `sdd-design`, `sdd-explore`, `sdd-init`, `sdd-onboard`, `sdd-propose`, `sdd-spec`, `sdd-tasks`, `sdd-verify`
   - Add `user-invocable: false` and `disable-model-invocation: true` to the YAML frontmatter (placement inside `metadata:` vs top-level — see open question 1).
2. Update `internal/assets/skills_frontmatter_test.go` so the linter accepts the two new fields wherever they're placed (extend the allowlist OR validate them as nested `metadata` keys).
3. Regenerate any golden files affected by the frontmatter byte changes (the exploration already lists `testdata/golden/skills-*.golden` and `testdata/golden/sdd-*-skill-*.golden` as candidates — `sdd-tasks` will confirm exact set).
4. Brief documentation: a short note in `CONTRIBUTING.md` (or the closest equivalent) explaining why SDD `SKILL.md` files carry the visibility flags, plus a one-line comment in `inject.go` near the SDD skill write block pointing at the same reason.
5. Validation: confirm Claude Code v2.1.131 actually honors the flags (cite the docs, smoke test on a real install).

### Out of scope (explicitly)

- Path relocation to `~/.claude/sdd-lib/` (Option C in the exploration). Rejected — see "Why Option A over Option C" below.
- Cross-adapter changes for Cursor, Kiro, OpenCode, Windsurf, etc. None of them have the duplication.
- Changes to `internal/components/sdd/inject.go` write paths. The destination is unchanged; only the file CONTENT picks up two new frontmatter fields.
- Migration logic. Existing installs pick up the new frontmatter automatically on the next `gentle-ai sync` — same path, new content.
- Uninstall path changes. None needed.
- Frontmatter changes to non-SDD skills (`judgment-day`, `branch-pr`, `chained-pr`, `cognitive-doc-design`, etc.). Those are intentionally user-invocable.

## Approach

**Option A — frontmatter visibility flags.**

For each of the 10 SDD `SKILL.md` files under `internal/assets/skills/sdd-*/SKILL.md`, add:

```yaml
user-invocable: false
disable-model-invocation: true
```

The exact placement (top-level vs nested under `metadata:`) is settled in the spec phase based on the linter constraint described below. Either way:

- Claude Code stops surfacing these `SKILL.md` files in the `/` picker (`user-invocable: false`).
- Claude Code stops auto-loading them as contextual skills based on `description:` (`disable-model-invocation: true`).
- The `~/.claude/commands/sdd-*.md` files keep producing the single canonical `/sdd-*` entry users actually invoke.
- Agents and commands that do `Read ~/.claude/skills/sdd-{phase}/SKILL.md` keep working — `Read` is unaffected by these flags.

### Why Option A over Option C (path relocation)

| Dimension | Option A (frontmatter) | Option C (relocate to `sdd-lib/`) |
|---|---|---|
| Files touched | 10 SKILL.md + 1 test + golden regen + 1 doc note | 10 SKILL.md (relocate) + `inject.go` + 8 Claude agents + 6 Claude commands + 10 Cursor agents + 10 Kiro agents + adapter additions + uninstall service + migration logic + ~25 golden files + e2e |
| Migration risk | None (same path, new content) | High (must atomically swap path references AND clean stale `~/.claude/skills/sdd-*/`; user-edited files complicate cleanup) |
| Cross-adapter ripple | None | Cursor + Kiro agent files all need rewriting even though they don't have the bug |
| Reversibility | Trivial — drop the two fields | Hard — once paths move, every reference must move back |
| Cleanliness long-term | SDD payload still lives in `~/.claude/skills/`, hidden by flags | Cleaner separation: `skills/` for user skills, `sdd-lib/` for orchestration internals |
| Risk if Claude Code semantics change | Flags become no-op → duplication returns; mitigated by monitoring + fall back to C | Path scanning rules change → broader breakage, harder to revert |

Option A is **~10x less code surface**, has **zero migration risk**, leaves cross-adapter agents alone, and is **trivially reversible** if Claude Code ever changes how these flags work. The cleanliness argument for C is real but cosmetic; the bug we're fixing is user-visible duplication, and A fixes that with the smallest blast radius.

### Affected files (concrete)

**Edit (10 files)**:
- `internal/assets/skills/sdd-apply/SKILL.md`
- `internal/assets/skills/sdd-archive/SKILL.md`
- `internal/assets/skills/sdd-design/SKILL.md`
- `internal/assets/skills/sdd-explore/SKILL.md`
- `internal/assets/skills/sdd-init/SKILL.md`
- `internal/assets/skills/sdd-onboard/SKILL.md`
- `internal/assets/skills/sdd-propose/SKILL.md`
- `internal/assets/skills/sdd-spec/SKILL.md`
- `internal/assets/skills/sdd-tasks/SKILL.md`
- `internal/assets/skills/sdd-verify/SKILL.md`

**Edit (test + lint)**:
- `internal/assets/skills_frontmatter_test.go` — extend allowlist or add nested-`metadata` validator (depending on placement decision).

**Possibly edit (1 file, scoped in spec)**:
- `internal/assets/skills/_shared/SKILL.md` — see open question 2.

**Regenerate (test data)**:
- `testdata/golden/skills-*.golden` and `testdata/golden/sdd-*-skill-*.golden` — exact list confirmed by `sdd-tasks` after a dry run.

**Documentation (1 file + 1 inline comment)**:
- `CONTRIBUTING.md` (or closest equivalent) — short note explaining the flags.
- `internal/components/sdd/inject.go` — one-line comment at the SDD skill write block pointing at the rationale.

**NOT touched**:
- `internal/components/sdd/inject.go` write logic (destination unchanged).
- `internal/components/uninstall/service.go` (paths unchanged).
- Any `internal/agents/{claude,cursor,kiro,...}/adapter.go` (no new dirs).
- Any non-SDD `SKILL.md` (`judgment-day`, `branch-pr`, etc.).
- Any `~/.claude/commands/sdd-*.md` or `~/.claude/agents/sdd-*.md` files.

## Validation strategy

1. **Frontmatter linter**: `internal/assets/skills_frontmatter_test.go` must pass with the two new fields present on all 10 SDD `SKILL.md` files. Decision in `sdd-spec`: widen the top-level allowlist OR require both fields nested under `metadata:`.
2. **Golden tests**: regenerate affected goldens; review the diff is byte-for-byte the two added lines (no incidental changes).
3. **Docs check**: confirm Claude Code's official skill schema docs document `user-invocable` and `disable-model-invocation` as supported v2.x fields. The exploration referenced this; `sdd-spec` will lock in the canonical doc URL with a context7 lookup. If the docs don't confirm both flags, fall back to `sdd-design` to pick a verified-supported alternative (e.g. only `user-invocable: false`, or relocate path).
4. **Smoke test on a real install**: after `gentle-ai sync` against a Claude Code v2.1.131 install, type `/` and verify each of `sdd-apply`, `sdd-archive`, `sdd-explore`, `sdd-init`, `sdd-onboard`, `sdd-verify` appears exactly once. Capture in the verify phase.
5. **Functional test**: invoke `/sdd-apply` in a real Claude Code session and confirm the orchestrator still launches the executor sub-agent, which still reads `~/.claude/skills/sdd-apply/SKILL.md` successfully.

## Risks

1. **Flag semantics may change in future Claude Code versions.** If `user-invocable` or `disable-model-invocation` are deprecated or repurposed, duplication returns. Mitigation: this is reversible in two lines per file; monitor Claude Code release notes; fall back to Option C (path relocation) if the flags are removed.
2. **Linter test currently rejects unknown top-level keys.** The frontmatter linter at `internal/assets/skills_frontmatter_test.go` (lines 30–36) hardcodes an allowlist of `{name, description, license, metadata, version}`. Adding `user-invocable` and `disable-model-invocation` as top-level keys WILL fail the test. The spec must decide: extend the allowlist (preferred — these are first-class Claude Code fields) or nest the two flags under `metadata:` (cheaper test change but moves us off the canonical schema). This is the single most important decision the spec must lock in.
3. **Documentation citation gap.** The exploration didn't pin a URL confirming Claude Code respects `user-invocable: false` AND `disable-model-invocation: true` together. The spec phase MUST verify this via context7 or the official docs before locking the approach. If only one flag is supported, the proposal still works (one is enough to hide from the picker), but we should know which one carries the load.
4. **Golden test churn.** Adding two YAML lines changes file bytes for 10 skills, which cascades into multiple golden files. Risk is purely mechanical (regen + review the diff is exactly +2 lines per affected golden). Low likelihood of going wrong, easy to spot if it does.

## Open questions (for sdd-spec / sdd-design)

1. **Placement: top-level vs nested under `metadata:`.** Top-level matches the canonical Claude Code schema and is more discoverable; nested keeps the existing linter allowlist unchanged. Recommendation: top-level + extend the allowlist, because that matches what Claude Code actually expects to see. Confirm in spec with a docs citation.
2. **Include `_shared/SKILL.md`?** It already declares "Not Invokable" in its prose and the linter exempts it from the `Trigger:` requirement. Adding `user-invocable: false` + `disable-model-invocation: true` is consistent with its self-documented purpose and makes the intent machine-checkable. Recommendation: YES, include `_shared` — same flags, no behavior change in practice (no description with `Trigger:` to auto-load on anyway), gain consistency. Final call in spec.
3. **`judgment-day` confirmed OUT of scope.** It's user-invocable by design (trigger phrases in its description) and is not part of the SDD phase suite. No flags added.
4. **`skill-registry`, `cognitive-doc-design`, `branch-pr`, `chained-pr`, `comment-writer`, `comment-writer`, `go-testing`, `issue-creation`, `skill-creator`, `work-unit-commits`** — all confirmed OUT of scope. They're general-purpose user-invocable skills.
