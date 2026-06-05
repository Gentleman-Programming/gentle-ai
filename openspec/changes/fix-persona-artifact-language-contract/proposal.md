# Proposal: Fix persona/artifact language contract

## Intent

Separate language behavior across three domains so persona style no longer leaks into technical artifacts or public comments. Gentleman remains a Rioplatense teaching voice for direct conversation, while generated artifacts default to English and comments react to their target context.

## Product language contract

| Domain | Default contract | Regional Spanish behavior |
|--------|------------------|---------------------------|
| Direct user/orchestrator conversation | Governed by the active persona. | `gentleman` uses the expected Rioplatense senior-architect teaching voice: voseo, concepts before code, warm/direct tone. `neutral` keeps the same teaching core without regional tone. |
| Generated technical artifacts | Default to English regardless of persona or conversation language. Examples: OpenSpec artifacts, specs, designs, tasks, generated code comments, UI copy, prompt-generated technical files, and SDD phase artifacts. | If Spanish artifacts are explicitly requested, or project convention requires Spanish, use neutral/professional Spanish unless the user explicitly asks for a regional variant. |
| Public/contextual comments | `comment-writer` writes in the target context language by default: Spanish issue/thread -> Spanish comment, English thread -> English comment, mixed -> target message language. Explicit user override wins. | Spanish comments default to neutral/professional unless the user or surrounding context clearly calls for regional tone. |

## Scope

### In scope

- Remove hardcoded Rioplatense/voseo wording from persona-agnostic SDD orchestrator assets, especially the known OpenCode leak: `elegí`, `Respondé`, and `¿Querés ajustar algo o continuamos?`.
- Apply the same contract across all currently supported SDD assets, not only OpenCode:
  - OpenCode and Kilocode via `internal/assets/opencode/sdd-orchestrator.md`
  - Claude via `internal/assets/claude/sdd-orchestrator.md`
  - Kimi via `internal/assets/kimi/sdd-orchestrator.md`
  - Codex via `internal/assets/codex/sdd-orchestrator.md`
  - Gemini via `internal/assets/gemini/sdd-orchestrator.md`
  - Qwen via `internal/assets/qwen/sdd-orchestrator.md`
  - Cursor via `internal/assets/cursor/sdd-orchestrator.md`
  - Windsurf via `internal/assets/windsurf/sdd-orchestrator.md`
  - Antigravity via `internal/assets/antigravity/sdd-orchestrator.md`
  - generic fallback via `internal/assets/generic/sdd-orchestrator.md`
  - discovered agent-specific assets, including Kiro via `internal/assets/kiro/sdd-orchestrator.md`
- Fix `comment-writer` source consistency so root and embedded skills enforce context-reactive comments without forcing Rioplatense Spanish.
- Preserve Gentleman persona assets that intentionally govern direct conversation, while making their artifact-boundary rules clearer if needed.
- Update install/sync behavior tests so refreshed assets cannot regenerate the old language leak.
- Update golden fixtures and language drift guards affected by asset changes.

### Out of scope

- Removing or weakening the Gentleman direct conversation voice.
- Adding a new language preference UI or changing persona selection semantics.
- Changing SDD phase order, delegation mechanics, or artifact storage semantics beyond language-contract wording.
- Implementing this proposal in this phase. This artifact defines the planned change only.

## Proposed changes

1. **Codify the three-domain contract in prompt assets.**
   - Persona governs direct user/orchestrator conversation only.
   - Persona-agnostic SDD assets must instruct generated technical artifacts to default to English.
   - Spanish artifact output must be neutral/professional unless regional Spanish is explicitly requested.

2. **Normalize SDD orchestrator assets.**
   - Replace hardcoded Rioplatense Spanish examples with neutral/professional Spanish or language-neutral instructions.
   - Keep localized user-facing preflight support, but avoid voseo in persona-agnostic prompts.
   - Ensure delegated phase prompts inherit artifact-language rules instead of conversation persona style.

3. **Fix `comment-writer`.**
   - Make the root `skills/comment-writer/SKILL.md` match the embedded `internal/assets/skills/comment-writer/SKILL.md` contract.
   - Require target-context language by default.
   - Require neutral/professional Spanish unless explicit user/context signal calls for regional tone.

4. **Guard install and sync.**
   - Verify SDD and skills components install the updated embedded assets.
   - Verify sync refreshes do not reintroduce old OpenCode/Kilocode SDD wording or stale root skill wording.

5. **Add regression tests.**
   - Asset-level language guards with allowlists for Gentleman direct-conversation persona files.
   - Source consistency checks for root and embedded `comment-writer` assets.
   - Install/sync checks for neutral persona plus SDD plus skills, with OpenCode/Kilocode coverage for the known leak path.

## Affected internal packages and areas

| Area | Impact |
|------|--------|
| `internal/assets` | Primary prompt and skill asset changes, including SDD orchestrators, persona boundary wording, embedded `comment-writer`, and golden fixtures. |
| `internal/components/sdd` | Tests around SDD asset selection, OpenCode/Kilocode overlay inlining, shared prompt writing, and sync/install propagation. |
| `internal/components/persona` | Possible tests or wording updates to preserve direct Gentleman conversation while preventing artifact leakage. |
| `internal/components/skills` | Tests ensuring installed skills receive the corrected embedded `comment-writer` behavior. |
| `internal/cli` | Sync/install regression tests to ensure refreshed assets do not regenerate stale language rules. |
| `internal/agents` / `internal/model` | Coverage awareness for all supported agent IDs and fallback behavior; no model/API changes expected. |
| `testdata/golden` | Golden updates for affected installed prompt/skill outputs. |
| root `skills/` | Source skill update for in-repo agent usage and consistency with embedded assets. |

## Risks and mitigations

| Risk | Mitigation |
|------|------------|
| Broad duplicated prompt surface leaves one agent unfixed. | Enumerate SDD orchestrator assets from known asset paths and add tests that include OpenCode, Kilocode via OpenCode, Claude, Kimi, Codex, Gemini, Qwen, Cursor, Windsurf, Antigravity, Kiro, and generic fallback. |
| Regression accidentally removes Gentleman direct voseo. | Allow regional wording only in Gentleman direct-conversation persona/output-style assets and add/keep assertions that Gentleman Spanish replies remain Rioplatense. |
| Tests over-ban legitimate explanatory mentions of `voseo` or `Rioplatense`. | Use scoped assertions and allowlists: ban regional imperatives in persona-agnostic artifacts, but permit explicit boundary/prohibition text where it is needed. |
| Root and embedded skill sources drift again. | Add a consistency test or shared required-rule assertion for both `skills/comment-writer/SKILL.md` and `internal/assets/skills/comment-writer/SKILL.md`. |
| Sync/install regenerates stale content from a hidden source. | Cover embedded assets, generated OpenCode overlays, shared prompt files, and sync paths in tests. |
| Changed-line count exceeds CI's practical 400-line budget despite the session budget of 1000. | Forecast implementation in tasks/design; if asset/golden churn exceeds 400 changed lines, split by asset family or request/record `size:exception` before apply. |

## Rollback plan

If the language-contract change causes regressions:

1. Revert the asset-normalization commit(s) for SDD orchestrators and `comment-writer`.
2. Revert associated golden fixture updates and language guard tests from the same work unit.
3. Keep any non-invasive source-discovery tests only if they still pass with the prior behavior; otherwise revert them too.
4. No data migration is expected, so rollback is file-level only.
5. Re-run targeted install/sync tests after rollback to confirm generated prompts match the restored asset baseline.

## Success criteria

- [ ] Direct conversation remains persona-governed: Gentleman keeps Rioplatense teaching voice, neutral keeps the same teaching core without regional tone.
- [ ] Generated technical artifacts default to English across SDD phase instructions and generated OpenSpec artifacts.
- [ ] Spanish artifacts are neutral/professional unless explicitly requested otherwise.
- [ ] `comment-writer` writes in the target context language and no longer forces Rioplatense Spanish by default.
- [ ] No persona-agnostic SDD orchestrator contains the known leak terms `elegí`, `Respondé`, or `¿Querés ajustar algo o continuamos?`.
- [ ] All supported SDD asset paths are covered, including OpenCode, Kilocode, Claude, Kimi, Codex, Gemini, Qwen, Cursor, Windsurf, Antigravity, Kiro, and generic fallback.
- [ ] Install and sync tests prove refreshed assets do not regenerate the old problem.
- [ ] `go test ./...` and `go vet ./...` pass, or any narrower verification in apply is explicitly justified before final verify.
