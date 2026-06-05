# Design: fix-skill-duplication-and-migrate

## TL;DR

Hide the 11 SDD `SKILL.md` files from Claude Code v2.x's `/` picker by adding two **top-level** YAML frontmatter keys (`user-invocable: false`, `disable-model-invocation: true`) and widening the embedded-asset frontmatter linter allowlist by exactly two keys. No path changes, no migration, no cross-adapter ripple.

---

## Architecture Overview

This is a content-only change against embedded assets. There is no runtime architecture to redesign — the `Read`-based access path agents and commands use today is preserved verbatim. The only behavioral surface that changes is **how Claude Code v2.x indexes the file at startup**, controlled by two declarative frontmatter flags.

Components touched:

```
internal/assets/skills/{_shared,sdd-*}/SKILL.md   <- content edit (11 files)
internal/assets/skills_frontmatter_test.go        <- allowlist widen (1 file)
testdata/golden/...                               <- mechanical regen
```

Components NOT touched: `internal/components/sdd/inject.go`, the Claude adapter, the uninstall service, agent files under `~/.claude/agents/`, command files under `~/.claude/commands/`, and every non-SDD skill's frontmatter.

Data flow stays identical:

```
gentle-ai install/sync
   -> embedded FS (//go:embed)
   -> Claude adapter writer
   -> ~/.claude/skills/sdd-*/SKILL.md  (path unchanged)
                                      \
                                       -> Claude Code v2.x indexer
                                          reads frontmatter
                                          honors user-invocable: false       (no /-picker entry)
                                          honors disable-model-invocation:   (no autoload by description)
                                          true
```

`Read` from sub-agents and commands continues to work because the file still exists at the same path with the same body. The two flags are **indexer-only** signals.

---

## ADRs

### ADR 1 — Frontmatter placement: TOP-LEVEL, not nested under `metadata:`

**Context.** The two flags can technically be placed at the top level of the YAML map (matching official Claude Code docs at https://code.claude.com/docs/en/skills) or nested inside the existing `metadata:` block already used by gentle-ai skills. The current frontmatter linter at `internal/assets/skills_frontmatter_test.go:30-36` enforces a strict top-level allowlist of `{name, description, license, metadata, version}` — top-level placement requires widening this allowlist; nested placement does not.

**Decision.** Top-level placement.

**Alternatives considered.**
- *Nest under `metadata:`* — cheaper for the linter (no allowlist change), but does not match Claude Code's documented schema. Whether Claude Code's indexer would honor nested flags is undocumented and untested by upstream — at best implementation-defined, at worst silently ignored, which is the failure mode we are trying to fix.

**Rationale.** Top-level is what every official Claude Code example shows. It is explicit, future-proof, and aligns with how upstream documents the feature. The linter cost is minimal: two new strings added to a map literal. Choosing the canonical placement also keeps the change reviewable against a public spec rather than against an internal convention.

---

### ADR 2 — Use BOTH flags, not just `user-invocable: false`

**Context.** Two independent indexer signals exist:
- `user-invocable: false` removes the skill from the `/` picker (the directly visible duplication bug we are fixing).
- `disable-model-invocation: true` prevents Claude from auto-loading the skill into context based on description matching (a separate, less-visible behavior).

**Decision.** Set both flags on all 11 SDD SKILL.md files.

**Alternatives considered.**
- *Only `user-invocable: false`* — fixes the visible duplication and is the minimum required to satisfy the user-facing acceptance criteria. Cheaper, less defensive.

**Rationale.** SDD SKILL.md files are NOT supposed to be loaded autonomously by Claude based on a description match — they are libraries of phase-specific instructions that the orchestrator delegates to explicitly via sub-agents. If Claude auto-injects (for example) `sdd-tasks/SKILL.md` into a casual conversation because the user mentioned "task breakdown", the result is wasted context and potentially confused behavior. `disable-model-invocation: true` enforces the invariant that **SDD skills are explicit-invoke only** — the orchestrator delegates, sub-agents `Read` deliberately, and Claude never autoloads. Defense in depth at zero additional cost (one extra line per file).

**Doc citations.** Both flags are documented at https://code.claude.com/docs/en/skills:
- `user-invocable: false` — under the "Allow Only Claude to Invoke Skill" section: *"Set `user-invocable: false` to ensure only Claude can invoke a skill. This is suitable for background knowledge or context-providing skills that are not intended as direct user commands."*
- `disable-model-invocation: true` — appears in the canonical YAML frontmatter example at https://code.claude.com/docs/en/slash-commands as a top-level kebab-case key alongside `name`, `description`, and `allowed-tools`.

---

### ADR 3 — Linter strategy: explicit allowlist expansion, not permissive scheme

**Context.** Top-level placement (ADR 1) means `internal/assets/skills_frontmatter_test.go` must accept the two new keys. Three strategies:

1. **Expand the explicit allowlist by exactly two keys** — preserve strictness.
2. **Switch to a permissive scheme** (e.g. allow any key, only forbid known-bad ones).
3. **Nest under `metadata:`** — sidesteps the linter (rejected in ADR 1).

**Decision.** Expand the explicit allowlist by exactly two keys: `user-invocable` and `disable-model-invocation`.

**Alternatives considered.**
- *Permissive scheme.* Would let typos like `user-invokable: false` (note the misspelling) silently slip past CI. The whole point of the existing strict allowlist is to catch authoring mistakes at PR time rather than in production.

**Rationale.** The strict allowlist is doing real work today — it caught `allowed-tools:` and similar non-standard fields in the past (per the comment at line 22). Adding two known, intentional keys is the minimum-blast-radius change that preserves the property the linter exists to enforce. Future contributors still get a hard CI failure if they fat-finger a frontmatter key. The cost is a two-line diff in a map literal.

---

## Implementation Flow

The apply phase will execute these steps in order; each step is verifiable before the next begins:

1. **Edit 11 embedded SKILL.md files** — add `user-invocable: false` and `disable-model-invocation: true` immediately after the existing top-level keys, before the closing `---`. Files: `internal/assets/skills/_shared/SKILL.md` plus the 10 `internal/assets/skills/sdd-{apply,archive,design,explore,init,onboard,propose,spec,tasks,verify}/SKILL.md`.
2. **Widen linter allowlist** — extend the map literal at `internal/assets/skills_frontmatter_test.go:30-36` to include the two new keys.
3. **Run frontmatter linter alone** — `go test ./internal/assets/...`. MUST pass before continuing. This is the cheapest, fastest signal that placement and allowlist are aligned.
4. **Regenerate goldens** — run the project's golden-update flow (typically `go test ./... -update` on the relevant packages). The exact set of golden files affected will be the union of `testdata/golden/skills-claude-*.golden` and `testdata/golden/sdd-*-skill-*.golden` that embed any of the 11 SKILL.md files. The list is mechanically determined by the regen step, not by guessing.
5. **Run full test suite** — `go test ./...`. MUST pass without `-update`.
6. **Manual verification** — build the binary, run `gentle-ai install` (or `sync`) against a clean `~/.claude/`, open Claude Code v2.1.131+, confirm the `/` picker shows each `sdd-*` exactly once and that all entries originate from `~/.claude/commands/sdd-*.md`. Smoke test one delegation chain (e.g. `/sdd-explore <topic>`) to confirm `Read` of the hidden SKILL.md still works from a sub-agent.

Steps 1–5 are mechanical and CI-verifiable. Step 6 is the human-in-the-loop confirmation that the upstream indexer actually honors the flags as documented.

---

## Risk Register

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|------------|--------|------------|
| 1 | Future Claude Code versions change the semantics of `user-invocable` / `disable-model-invocation` and duplication returns | Low | Medium | Change is reversible in 2 lines per file. Monitor Claude Code release notes. Fallback is the rejected Option C (path relocation) which can be revisited if upstream removes these flags. |
| 2 | A user manually edits an installed `~/.claude/skills/sdd-*/SKILL.md`, then runs `gentle-ai sync` and the frontmatter is overwritten | Low | Low | Out of scope for this change — sync's overwrite policy for embedded assets is deliberate and pre-existing. Document elsewhere if it becomes a real complaint. |
| 3 | Golden test churn produces noisy diffs | Certainty | Trivial | Diffs are purely mechanical (+2 lines per affected golden). Reviewers spot-check one or two and trust the rest. |
| 4 | Top-level placement is canonical per docs but the running v2.1.131 indexer has a bug honoring it | Low | Medium | Step 6 manual verification catches this before merge. If the bug is real, file upstream and consider nesting under `metadata` as a temporary workaround in a follow-up PR. |

---

## File-Level Change Inventory

### Modified — embedded SKILL.md files (11)

- `internal/assets/skills/_shared/SKILL.md`
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

Each gains exactly two lines inside the existing frontmatter block:
```yaml
user-invocable: false
disable-model-invocation: true
```

### Modified — frontmatter linter (1)

- `internal/assets/skills_frontmatter_test.go` — extend `allowedKeys` map literal at lines 30–36 to include `"user-invocable": true` and `"disable-model-invocation": true`.

### Regenerated — golden test files (mechanical, exact set TBD at apply time)

- `testdata/golden/skills-claude-*.golden` — Claude adapter skill goldens that embed any affected SKILL.md.
- `testdata/golden/sdd-claude-skill-*.golden` — SDD-specific Claude goldens.
- `testdata/golden/sdd-{cursor,kiro,opencode,gemini,vscode,antigravity,codex,windsurf}-skill-sdd-*.golden` — same SKILL.md content propagates to other adapters via the SDD injector. Even though those adapters don't have the duplication bug, the embedded SKILL.md is the single source of truth and the goldens reflect its content verbatim.

The exact list resolves itself when running the golden-update step; it is not a design-time decision.

---

## Out of Scope (locked)

- Path relocation to `~/.claude/sdd-lib/` (Option C from the proposal).
- Migration logic for existing installs — same path, content overwrite handles it.
- Changes to `internal/components/sdd/inject.go` write paths or write policy.
- Cross-adapter behavioral changes for Cursor, Kiro, OpenCode, Windsurf — those adapters do not exhibit the duplicate-picker bug. Their goldens regenerate mechanically because content is embedded centrally; that is not a behavioral change.
- Frontmatter changes to non-SDD skills (`judgment-day`, `branch-pr`, `chained-pr`, `cognitive-doc-design`, `skill-creator`, `skill-registry`, `comment-writer`, `go-testing`, `issue-creation`, `work-unit-commits`) — these are intentionally user-invocable.
- `CONTRIBUTING.md` doc updates and inline comments in `inject.go` — explicitly trimmed by the orchestrator's scope; can land in a follow-up PR if maintainers want them.
- `internal/components/uninstall/service.go` — unaffected.

---

## References

- Proposal (engram): `sdd/fix-skill-duplication-and-migrate/proposal`
- Proposal (file): `openspec/changes/fix-skill-duplication-and-migrate/proposal.md`
- Spec (engram): `sdd/fix-skill-duplication-and-migrate/spec`
- Spec (file): `openspec/changes/fix-skill-duplication-and-migrate/spec.md`
- Frontmatter linter: `internal/assets/skills_frontmatter_test.go` (allowlist at lines 30–36)
- Claude Code skill schema: https://code.claude.com/docs/en/skills
- Claude Code slash commands: https://code.claude.com/docs/en/slash-commands
