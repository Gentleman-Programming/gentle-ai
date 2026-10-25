# docs-wave-c-sdd-sweep

Issue: #5168 (tracker #4556). Branch: `docs/5168-sdd-sweep` from upstream/main 47409837.
Goal: remove retired SDD references from living docs; ODD is the only workflow on main.
Keep: review-integration.md:220, telemetry legacy `sdd-*` values, negative controls, docs/audits, docs/releases, rdd-* design docs. Out of scope: engram.md:159.

## Tasks

- [x] T1 User-facing docs: components.md, trigger-rules.md, kiro.md (ODD rewrite, drop `.kiro/specs`, keep steering, fix dead link, "Since v4 (unreleased)" notes)
- [x] T2 Architecture/testing: organic-rdd.md (FINALIZE -> `acknowledge-approved` burn; "keeps ODD routing"), testing-agents-deterministically.md (`sdd-orchestrator.md` -> `opencode/orchestrator.md`)
- [x] T3 One-line sweeps: platforms.md, architecture.md, docker-e2e-testing.md, community-roadmap.md, review-authority-threat-model.md, gga-powershell-shim.md, skill-registry.md, telemetry.md (496 sdd -> engram; drop `sdd-attempt` at 542/584)
- [ ] T4 Verify residual grep + links, open PR, comment on #4556

## Evidence
- T1 ad0dfa8b docs(sdd): describe ODD in components, trigger rules and Kiro guides
- T2 3c82b1d1 docs(sdd): replace FINALIZE and stale orchestrator prompt claims (testing guide: organic e2e agent has no prompt, so the claim was corrected rather than repathed)
- T3 b963e4b5 docs(sdd): sweep remaining SDD references from living docs
- Rebased onto upstream/main 9dfe17d8 after the v4.0.0 release (clean, no conflicts); ff0834e7 docs(sdd): mark the SDD retirement as shipped in v4.0.0 (verified: e219644b is an ancestor of v4.0.0, no sdd-* assets in the v4.0.0 tree).
- Checks: git diff --check clean; residual grep hits are banners, legacy paths/values, or negative controls.
- Follow-ups (code, out of scope): internal/tui/screens/preset.go:21 "Memory + SDD + skills"; internal/catalog/skills.go still lists sdd-* skills.
