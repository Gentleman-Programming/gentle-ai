# Proposal: dev-orchestrator as Installable, Change-Owning Orchestrator Mode

## Intent

`internal/devorchestrator` is dead code — nothing in `internal/cli`, `internal/tui`, or `internal/sddstatus` imports it, despite Slices 1-4 being on main and 12 `dev-*` subagents already shipping in `internal/assets/opencode/sdd-overlay-single.json`. Make it a real, **opt-in**, installable orchestrator for OpenCode (a third `mode: primary` tab beside build/plan, alongside `gentle-orchestrator`), and make change **ownership** explicit so the two engines can never stomp each other.

Decision already made by the user: **ownership-per-change, stamped at change-creation time** — "una firma que hace uno y el otro no puede tocar". NOT a live mid-change engine switch.

## Scope

### In Scope
- **Ownership marker**: `engine: gentle-orchestrator | dev-orchestrator` in the change's first artifact frontmatter (`openspec/changes/<id>/proposal.md` | `explore.md`). Absent marker ⇒ `gentle-orchestrator`.
- **Enforcement (closes a live bug)**: `internal/devorchestrator/intent/router.go:57-86` `RouteIntent` currently `MkdirAll` + `WriteFile`s `proposal.md` **unconditionally** — no existence check, no lock — over the exact path `internal/sddstatus/status.go:1706` reads. Add read-before-write: if the change dir exists and its marker is absent or foreign, **refuse** (typed error, no write). Otherwise stamp the marker.
- **Reciprocal refusal**: `internal/sddstatus` parses `engine:` (`artifact_states.go`, `status.go`) and surfaces it as an additive `Engine` field (`omitempty`, same pattern as `RepoProgress`/`ReviewOffer`), so gentle-orchestrator paths refuse/flag dev-owned changes.
- **Installation**: add `dev-orchestrator` as a second `mode: primary` agent via an **additive overlay fragment**, gated by a new opt-in, never written unless requested.
- Fix mid-flight bug: greenfield branch (`router.go:65-75`) reassigns `artifactName` to `explore.md` **after** `artifactPath` was computed at line 63 — it writes `proposal.md` while returning `explore.md`. *(Status: already fixed and committed independently — see design doc's Provenance Notes.)*

### Out of Scope
- Live mid-change engine switching / runtime tab toggle (explicitly rejected).
- Any behavior change to gentle-orchestrator for existing/unmarked changes.
- Changing `SDDModeSingle`/`SDDModeMulti` semantics, the `sdd_mode.go` radio picker, or the ratified `openspec/specs/installer-picker-navigation/spec.md` screen order.
- Generic file locking for `openspec/changes/**` (marker-based refusal only; `reviewtransaction/store_lock.go` is anchored to `review-transactions` and not reusable).
- Convergence of devorchestrator onto `sddstatus` as shared state manager (per `orchestrator_summary_report.md`) — separate change.

## Capabilities

### New Capabilities
- `change-engine-ownership`: marker format, stamping rules, refusal semantics, back-compat default.
- `dev-orchestrator-install-mode`: opt-in installation of `dev-orchestrator` as an OpenCode primary agent.

### Modified Capabilities
- None. `installer-picker-navigation` MUST remain byte-compatible — the new opt-in must not alter its predicates.

## Approach

**Ownership.** Marker lives in frontmatter (not a sidecar file) so it travels with the artifact `sddstatus` already parses and git already tracks. Write path: `RouteIntent` resolves `openspec/changes/<id>/`; if it exists, read the existing marker and refuse on mismatch/absent; if new, create and stamp `engine: dev-orchestrator`. Read path: `sddstatus.Resolve` reads the marker into `Status.Engine`; unmarked ⇒ `gentle-orchestrator`, zero projection change.

**Installation.** OpenCode surfaces `mode: primary` agents as switchable tabs, so a `dev-orchestrator` primary entry in the overlay *is* the "third tab" the user asked for. Two candidate mechanisms:

| Option | Mechanism | Verdict |
|---|---|---|
| A | New `SDDModeID` value (`model/types.go:180-184`, `normalizeSDDMode` `internal/cli/validate.go:168-179`, `sdd_mode.go` radio, `overlayAssetPath` `inject.go:223`) | **Rejected** — third value perturbs the `SDDMode == Multi` predicate gating `ScreenModelPicker`, breaking a ratified spec and installer round-trip tests |
| B | Orthogonal opt-in (flag/component) that merges an additive `sdd-overlay-devorchestrator.json` fragment on top of the chosen single/multi overlay | **Recommended** — single/multi picker untouched; default off; overlay merge is idempotent |

Design phase decides flag-vs-component and fragment shape. *(Resolved in design: flag/sync-override, no new TUI screen for v1.)*

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/devorchestrator/intent/router.go` | Modified | Read-before-write guard, marker stamping, greenfield artifactPath fix |
| `internal/devorchestrator/orchestrator.go` | Modified *(added during design)* | `GenerateContextForAgent` ownership check — defense in depth beyond creation time |
| `internal/sddstatus/{status.go,artifact_states.go,status_v1.go}` | Modified | Parse `engine:` (via new `internal/changeowner` package, NOT `artifact_states.go`), additive `Engine` field, v1 shape frozen |
| `internal/assets/opencode/` | New | Additive dev-orchestrator primary-agent overlay fragment |
| `internal/components/sdd/inject.go` | Modified | Conditional fragment merge; `overlayAssetPath` untouched |
| `internal/cli/{install.go,validate.go}` | Modified | New opt-in flag; `normalizeSDDMode` unchanged |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| **Install flow corrupts existing installs** (user's explicit caution: "ojo con la instalación") | Med | Additive fragment only, default OFF; reuse `filemerge.WriteFileAtomic`; idempotency test (second inject ⇒ `filesChanged=0`); e2e assert single/multi output byte-identical when opt-in absent, verified against a golden fixture captured from `main` before any code lands |
| Breaking ratified installer navigation spec | Med | Option B by construction; regression suite `TestInstallNavigationRoundTrips` must stay green |
| Silently enabling dev-orchestrator for users who didn't ask | Med | Opt-in defaults false; no auto-detect; explicit flag/sync-override consent; dry-run must print the choice |
| Existing changes lack a marker | High | Absent ⇒ `gentle-orchestrator`; no migration, no rewrite of historical artifacts |
| Cross-engine overwrite (today reproducible) | High | Refusal guard in `RouteIntent` AND `GenerateContextForAgent`; regression test proving a gentle-authored `proposal.md` is never overwritten |
| Marker drift / hand-edited frontmatter | Low | Marker is advisory-but-enforced; refuse on unrecognized value rather than guessing |
| Two primary agents confuse users | Low | Distinct descriptions; docs state ownership is per-change and permanent |
| True write-refusal symmetry is impossible in pure Go (gentle-orchestrator writes via LLM/skill tools, not a Go path) | Known limitation | Symmetry enforced at the `sdd-status` gate contract instead — same guarantee level as every other SDD invariant in the system |

## Rollback Plan

Per-layer revert:
1. **Install**: delete the overlay fragment + the opt-in flag. Users who never opted in are unaffected (config byte-identical).
2. **sddstatus**: drop the `Engine` field + parser (`omitempty`, additive ⇒ no wire downgrade owed).
3. **RouteIntent / GenerateContextForAgent guards**: reverting restores today's unguarded write — acceptable only because devorchestrator is still not externally invoked.

## Dependencies

- Slices 1-4 on main (`sdd/dev-orchestrator-p1-foundations/archive-report` #2914).
- `dev-*` subagents present in the OpenCode overlay (`sdd/dev-orchestrator-opencode-parity/archive-report` #2894).
- Uncommitted work in progress on `intent/router.go` + `sddstatus/*` must land or be reconciled first — *(reconciled: those files are now committed on main as of `be3909ab`/`b1b3f931`)*.

## Success Criteria

- [ ] `RouteIntent` refuses (no write) against a change dir owned by gentle-orchestrator; regression test proves the file is untouched.
- [ ] `GenerateContextForAgent` refuses to build context for a foreign-owned change at any phase-advance.
- [ ] New changes created by dev-orchestrator carry `engine: dev-orchestrator`; `sddstatus` reports it.
- [ ] Unmarked/legacy changes behave byte-for-byte as today (`TestProjectStatusV1FreezesExactLegacyShape` and single-repo byte-identical tests green).
- [ ] Install without the opt-in produces byte-identical OpenCode config vs. current `main` (verified against a pre-change golden fixture); installer navigation round-trip tests green.
- [ ] With the opt-in, `dev-orchestrator` appears as a selectable OpenCode primary agent (tab) beside build/plan/gentle-orchestrator.
- [ ] Greenfield `RouteIntent` writes and returns the same path.

---
*This is a read-only review copy exported from Engram (obs #2922) for human approval. The source of truth remains Engram — edits here do not sync back automatically.*
