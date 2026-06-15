# Tasks: SDD Role Profile Generator

## Review Workload Forecast

Estimated changed lines: ~1250 additions (16 new files + tests)
400-line budget risk: High
Chained PRs recommended: Yes
Chain strategy: pending

Decision needed before apply: Yes

| Unit | Goal | PR | ~Lines |
|------|------|----|--------|
| 1 | Foundation types + discovery engine | PR #1 | ~370 |
| 2 | Generator + loader + registry | PR #2 | ~510 |
| 3 | CLI + default profiles + templates | PR #3 | ~370 |

---

## Phase 1: Foundation — Model Types

- [x] T01: Create `internal/model/role_profile.go` — RoleEnum, PersonaOverride, SkillRef, MCPServerRef, GateRule, SDDAdaptations, QualityScore, ProfileMetadata, RoleProfile (~180 lines)
- [x] T02: Create `internal/model/role_profile_test.go` — RoleEnum.Valid(), JSON round-trip, zero-value (~60 lines)

## Phase 2: Discovery — Search Engine

- [x] T03: Create `internal/components/discovery/sources.go` — SourceReader interface, DiscoveryQuery, DiscoveryResult, RecommendationSet (~80 lines) [after T01]
- [x] T04: Create `internal/components/discovery/scorer.go` — QualityScorer: relevance×0.4, safety×0.3, popularity×0.3 (~60 lines) [after T03]
- [x] T05: Create `internal/components/discovery/dedup.go` — merge by name+URL, keep highest score (~40 lines) [after T04]
- [x] T06: Create `internal/components/discovery/engine.go` — DiscoveryEngine.Search() with graceful source degradation (~100 lines) [after T05]
- [x] T07: Create `internal/components/discovery/discovery_test.go` — scorer, dedup, engine with mock sources (~90 lines) [after T06]

## Phase 3: Generator — Templates & Pipeline

- [x] T08: Create `internal/components/profile/skill_bundle.go` — role→skill mapping with priority (~50 lines) [after T01] *(integrated into generator.go selectSkills)*
- [x] T09: Create `internal/components/profile/templates.go` — TemplateEngine: base + role templates, variable substitution (~70 lines) [after T01]
- [x] T10: Create `internal/components/profile/validator.go` — completeness check, quality ≥ 80% gate (~60 lines) [after T01]
- [x] T11: Create `internal/components/profile/generator.go` — Generate(): discover → select → template → validate (~80 lines) [after T06,T08,T09,T10]
- [x] T12: Create `internal/components/profile/generator_test.go` — pipeline tests with mock discovery (~80 lines) [after T11]

## Phase 4: Loader — Runtime Activation

- [x] T13: Create `internal/components/profile/loader.go` — Load, Activate, List, ActivationPlan, ToSelection (~100 lines) [after T01]
- [x] T14: Create `internal/components/profile/loader_test.go` — file-based Load/Activate with mock profiles dir (~70 lines) [after T13]

## Phase 5: Registry — Community Sharing

- [x] T15: Create `internal/components/registry/client.go` — RegistryClient: List, Install, Update, Search (~80 lines) [after T01,T13]
- [x] T16: Create `internal/components/registry/registry_test.go` — mock HTTP server for registry + download (~70 lines) [after T15]

## Phase 6: CLI

- [x] T17: Create `internal/cli/profile.go` — ProfileCommand + handlers: list, generate, install, activate, remove, info, validate, search (~120 lines) [after T11,T13,T15]
- [x] T18: Create `internal/cli/profile_test.go` — flag parsing and command routing (~60 lines) [after T17]

## Phase 7: Default Profiles & Templates

- [x] T19: Create `internal/components/profile/defaults.go` — hardcoded defaults for 6 roles (~40 lines) [after T01]
- [x] T20–T25: Create 6 default profile JSONs under `internal/assets/profiles/defaults/` — developer, cybersecurity, marketing, education, design, data-science (~6×50 lines)
- [x] T26–T28: Create base templates under `internal/assets/templates/base/` — persona.md.tmpl, sdd-adaptations.md.tmpl, trigger-rules.md.tmpl (~3×20 lines)
- [x] T29: Create role-specific templates under `internal/assets/templates/roles/` — 6 roles × 2 templates (~6×2×15 lines)
