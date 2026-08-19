# Delta Spec: dev-orchestrator-installable-owner

## Capability: change-engine-ownership (NEW)

### Purpose
Stamp each SDD change with the engine that created it (`gentle-orchestrator` or `dev-orchestrator`) at creation time, and enforce that only the owning engine may write further artifacts into that change directory, at BOTH creation time and every subsequent phase-advance. Ownership is per-change, permanent for the life of the change — no mid-change engine switching, no locking mechanism.

### Requirement: SPEC-001 Ownership marker format and location
The first artifact written for a change (`proposal.md` or `explore.md` under `openspec/changes/<id>/`) MUST carry an `engine:` field in its frontmatter with value `gentle-orchestrator` or `dev-orchestrator`.

#### Scenario: New change created by dev-orchestrator
- GIVEN no `openspec/changes/<id>/` directory exists
- WHEN `internal/devorchestrator/intent/router.go` `RouteIntent` creates the first artifact for `<id>`
- THEN the written artifact frontmatter MUST contain `engine: dev-orchestrator`

#### Scenario: Unmarked legacy change
- GIVEN a change directory created before this feature exists, with no `engine:` field in any artifact frontmatter
- WHEN any engine or `internal/sddstatus` reads the change
- THEN ownership MUST be treated as `gentle-orchestrator`, with zero change to existing behavior or output shape

### Requirement: SPEC-002 sddstatus exposes ownership
`internal/sddstatus` MUST parse the `engine:` frontmatter field via `internal/changeowner` (NOT via `artifact_states.go`, which remains solely the artifact-readiness key registry) and surface it as an additive, `omitempty` `Engine` field on `Status`, following the same pattern as `RepoProgress`/`ReviewOffer`.

#### Scenario: Status read for marked change
- GIVEN a change whose first artifact has `engine: dev-orchestrator`
- WHEN `sddstatus.Resolve` computes status for that change
- THEN the returned `Status.Engine` MUST equal `"dev-orchestrator"`

#### Scenario: Status read for unmarked change preserves legacy shape
- GIVEN a change with no `engine:` marker
- WHEN `sddstatus.Resolve` computes status
- THEN the JSON/struct output MUST be byte-identical to pre-change output (per `TestProjectStatusV1FreezesExactLegacyShape`), i.e. `Engine` omitted or empty per `omitempty`

### Requirement: SPEC-003 Write refusal on cross-engine writes (creation time)
`RouteIntent` MUST perform a read-before-write check: if the target change directory already exists, it MUST read the existing ownership marker before writing any new artifact. If the marker is foreign (belongs to the other engine) or unrecognized, the write MUST be refused via a typed error and no file MUST be modified or created.

#### Scenario: dev-orchestrator refused against a gentle-owned change
- GIVEN a change directory exists with `engine: gentle-orchestrator` (or unmarked, defaulting to gentle-orchestrator)
- WHEN `dev-orchestrator`'s `RouteIntent` attempts to write an artifact into that change
- THEN the write MUST be refused with a typed ownership error
- AND the existing `proposal.md`/`explore.md` content MUST remain byte-unchanged

#### Scenario: gentle-orchestrator refused against a dev-owned change
- GIVEN a change directory exists with `engine: dev-orchestrator`
- WHEN gentle-orchestrator's write path attempts to write an artifact into that change
- THEN the write MUST be refused, and the dev-orchestrator-authored content MUST remain untouched

#### Scenario: Same-engine write proceeds normally
- GIVEN a change directory owned by `dev-orchestrator`
- WHEN `dev-orchestrator`'s `RouteIntent` writes a subsequent artifact into the same change
- THEN the write MUST succeed exactly as before this feature existed

### Requirement: SPEC-004 Greenfield path consistency (regression guard)
For a brand-new change, the artifact path returned by `RouteIntent` MUST match the artifact actually written to disk, including when the greenfield branch reassigns the artifact name (e.g. to `explore.md`).

#### Scenario: Greenfield returns matching path
- GIVEN no change directory exists for `<id>`
- WHEN `RouteIntent` classifies it as greenfield and reassigns the artifact name
- THEN the returned artifact path MUST reference the same file that was written (not the pre-reassignment name)

### Requirement: SPEC-007 Phase-advance ownership check (defense in depth)
`GenerateContextForAgent` MUST check change ownership before generating any agent context, in addition to the creation-time check in SPEC-003, so ownership is enforced at every phase advance (explore→propose→spec→design→tasks→apply→verify), not only at change creation. This check MUST reuse the `internal/changeowner` predicate and MUST follow the same strict-enforcement error style already introduced for unregistered agents (`agent.Registry` lookup).

#### Scenario: Phase advance refused on foreign-owned change
- GIVEN a change directory owned by `gentle-orchestrator` (or unmarked, defaulting to gentle-orchestrator)
- WHEN `dev-orchestrator`'s `GenerateContextForAgent` is invoked for that change
- THEN context generation MUST be refused with a strict-enforcement error
- AND no context package MUST be returned

#### Scenario: Phase advance proceeds for own change
- GIVEN a change directory owned by `dev-orchestrator`
- WHEN `GenerateContextForAgent` is invoked for that change
- THEN context generation MUST proceed exactly as before this feature existed

## Capability: dev-orchestrator-install-mode (NEW)

### Purpose
Allow `dev-orchestrator` to be installed as a second, equal `mode: primary` OpenCode agent alongside `gentle-orchestrator`, opt-in and off by default, without altering the existing single/multi overlay picker.

### Requirement: SPEC-005 Opt-in, default-off installation
Installing `dev-orchestrator` as a primary OpenCode agent MUST require an explicit opt-in (flag or component selection in `internal/cli/install.go`). Without this opt-in, install output MUST be byte-identical to current `main` behavior. This byte-identical claim MUST be verified against a golden fixture captured from `main` BEFORE any code in this change touches `inject.go` or the overlay JSON assets.

#### Scenario: Install without opt-in
- GIVEN a user runs install without selecting the dev-orchestrator opt-in
- WHEN the OpenCode config/overlay is generated
- THEN the resulting files MUST be byte-identical to the pre-change golden fixture
- AND no dev-orchestrator overlay fragment MUST be present

#### Scenario: Install with opt-in
- GIVEN a user explicitly selects the dev-orchestrator opt-in
- WHEN install runs
- THEN an additive `mode: primary` `dev-orchestrator` agent entry MUST appear in the generated OpenCode config, selectable as its own tab beside `gentle-orchestrator`/build/plan

### Requirement: SPEC-006 Additive overlay fragment, no new SDDMode value, no new TUI screen
The dev-orchestrator opt-in MUST be implemented as an overlay fragment merged on top of the existing single/multi overlay (`internal/components/sdd/inject.go`), explicitly overriding `dev-orchestrator`'s existing `hidden: true` entry (it already exists as a hidden subagent in `sdd-overlay-single.json`/`sdd-overlay-multi.json`) to `hidden: false, mode: primary`. It MUST NOT introduce a third `SDDModeID` value, MUST NOT alter `normalizeSDDMode` (`internal/cli/validate.go`), MUST NOT alter the `SDDMode == Multi` predicate gating `ScreenModelPicker`, and MUST NOT add a new TUI picker screen (v1 ships flag/sync-override opt-in only; an interactive picker is deferred to a separate future change).

#### Scenario: Idempotent merge
- GIVEN the dev-orchestrator opt-in is selected and inject runs twice
- WHEN the second inject run occurs
- THEN `filesChanged` MUST be `0` on the second run

#### Scenario: Ratified installer-picker-navigation spec unaffected
- GIVEN the dev-orchestrator opt-in is enabled or disabled
- WHEN `ScreenModelPicker`'s single/multi navigation flow runs
- THEN behavior MUST match `openspec/specs/installer-picker-navigation/spec.md` exactly, with `TestInstallNavigationRoundTrips` remaining green in both cases

## Out of Scope (explicitly not specified here)
- Live mid-change engine switching or runtime tab toggle.
- Generic locking for `openspec/changes/**` beyond marker-based refusal.
- Convergence of `internal/devorchestrator` onto `internal/sddstatus` as shared state manager.
- Interactive TUI screen for the dev-orchestrator opt-in (deferred to a future change).

---
*This is a read-only review copy exported from Engram (obs #2930) for human approval. The source of truth remains Engram — edits here do not sync back automatically.*
