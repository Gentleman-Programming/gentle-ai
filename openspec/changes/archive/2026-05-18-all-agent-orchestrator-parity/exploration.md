## Exploration: all-agent-orchestrator-parity

### Current State
The repo now has Claude↔OpenCode parity for SDD chain strategy and Claude-native delegation wording (already shipped in `f13949d`). Other agent orchestrator assets still diverge.

`internal/assets/opencode/sdd-orchestrator.md` is the canonical reference: it includes explicit `### Chain Strategy`, forwards both `delivery_strategy` + `chain_strategy` into `sdd-tasks` and `sdd-apply`, and defines workload-guard behavior.

`internal/assets/claude/sdd-orchestrator.md` already mirrors that chain guidance and uses Claude-specific delegation wording. The remaining assets are mixed:
- Some agent assets still have **delivery strategy only** (no explicit chain strategy section/forwarding).
- Some assets still use `delegate/task` wording that is OpenCode-specific, not platform-accurate.
- Solo-inline assets (`windsurf`, `antigravity`) still contain a few stale “delegate to sub-agent / pass to sub-agent launch” phrases inconsistent with their own inline model.

### Affected Areas
- `internal/assets/codex/sdd-orchestrator.md` — has `delivery_strategy` section but no explicit chain strategy propagation.
- `internal/assets/gemini/sdd-orchestrator.md` — same gap as codex.
- `internal/assets/qwen/sdd-orchestrator.md` — same gap as codex.
- `internal/assets/generic/sdd-orchestrator.md` — same gap; affects generic-based hosts (including VS Code and Claude fallback mapping paths).
- `internal/assets/kimi/sdd-orchestrator.md` — platform-specific Task wording exists, but no explicit chain strategy section/forwarding parity.
- `internal/assets/kiro/sdd-orchestrator.md` — Kiro-native wording exists, but chain strategy propagation needs parity check/update.
- `internal/assets/windsurf/sdd-orchestrator.md` — solo-inline model; has delivery strategy only and stale “delegate to sub-agent” phrasing in init/artifact-store text.
- `internal/assets/antigravity/sdd-orchestrator.md` — solo-inline model; same stale delegation phrasing plus missing chain section.
- `internal/components/golden_test.go` — golden fixtures that encode orchestrator output for codex/gemini/cursor/vscode/windsurf/kiro/antigravity must be updated if assets change.
- `internal/components/sdd/inject_test.go` — platform wording assertions (notably Kimi/Qwen) may need extension for new parity guarantees.
- `internal/assets/assets_test.go` — currently has Claude-specific chain/delegation assertions only; likely needs broadened multi-agent assertions.
- `testdata/golden/sdd-codex-agentsmd.golden`
- `testdata/golden/sdd-gemini-geminimd.golden`
- `testdata/golden/sdd-cursor-rules.golden`
- `testdata/golden/sdd-vscode-instructions.golden`
- `testdata/golden/sdd-windsurf-global-rules.golden`
- `testdata/golden/sdd-kiro-instructions.golden`
- `testdata/golden/sdd-antigravity-rulesmd.golden`

### Approaches
1. **Targeted Asset Parity (recommended)** — update only remaining non-Claude orchestrator assets + directly coupled tests/goldens.
   - Pros: Controlled scope; aligns with request; low risk of runtime regression.
   - Cons: Repeated wording updates across multiple files.
   - Effort: Medium.

2. **Template/Generator refactor first** — centralize orchestration text then regenerate all assets.
   - Pros: Reduces long-term drift.
   - Cons: Much larger blast radius; violates “controlled parity” scope; likely introduces unrelated churn.
   - Effort: High.

### Recommendation
Use **Targeted Asset Parity**. Treat OpenCode/Claude as references, patch only remaining agent orchestrator assets, then update/expand static assertions and regenerate only impacted goldens. Keep platform semantics intact per host (do not force OpenCode `delegate/task` language into Kiro/Windsurf/Antigravity/Kimi).

### Risks
- Over-normalizing delegation wording could break platform-native semantics (especially Kiro/Windsurf/Antigravity/Kimi).
- Generic asset edits can have wider impact because multiple adapters consume it.
- Golden churn can hide semantic misses unless assertions are strengthened in `assets_test.go`.

### Ready for Proposal
Yes — scope is clear and constrained to non-Claude orchestrator assets plus directly-coupled tests/goldens.
