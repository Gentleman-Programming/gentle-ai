package recoverytrace

// This file is the recovery's audit record. Every path named by the File Actions
// table of openspec/changes/organic-rdd-recovery/design.md carries exactly one
// row here, together with the evidence that authorizes its disposition.
//
// Three conventions keep the record honest and are never relaxed:
//
//   - Publication evidence is measured, not asserted. Each row's refs were
//     produced by `git cat-file -e origin/main:<path>`, `git cat-file -e
//     main:<path>`, and the same query against `git describe --tags --abbrev=0
//     origin/main`, which resolved to v2.1.11.
//   - Every Proof and DestinationProof names a test file that exists in this
//     working tree today. A missing receiving test is recorded as a gap — an
//     unwritten destination is never named as if it already proved something.
//   - A disposition the evidence cannot yet authorize is DEFER, never DELETE.
//     DEFER states plainly that the outcome is still open; a DELETE row would
//     claim an authorization nobody has produced.
//
// Dispositions map onto the design's action vocabulary as follows: an action of
// CREATE that has already landed, MODIFY, or KEEP is a KEEP row, because the
// path and its invariant survive the recovery; a CREATE that is still unwritten
// is DEFER, because nothing exists to own an invariant yet.

// recoveryContributor is the sole credit this recovery carries. The design
// credits no other author for any listed path, and inventing a second name would
// put an unverifiable claim into the contribution ledger.
const recoveryContributor = "@Alan-TheGentleman"

// latestReleaseTag is the tag `git describe --tags --abbrev=0 origin/main`
// resolved to when this evidence was captured. It is named once so a re-capture
// against a newer release changes one constant instead of every row.
const latestReleaseTag = "v2.1.11"

// published records a path that exists on both branches and in the latest
// release. Such a path shipped to users, so it may never take the early-removal
// deviation and must retain systemic migration order.
func published() []PublicationRef {
	return []PublicationRef{
		{Ref: remoteMainRef, Present: true},
		{Ref: localMainRef, Present: true},
		{Ref: latestReleaseTag, Present: true},
	}
}

// unpublished records a path that exists on no authoritative ref. This is the
// only evidence that authorizes an early removal, because it proves no user ever
// received the behavior being removed.
func unpublished() []PublicationRef {
	return []PublicationRef{
		{Ref: remoteMainRef, Present: false},
		{Ref: localMainRef, Present: false},
		{Ref: latestReleaseTag, Present: false},
	}
}

// unreleased records a path that reached both branches but not the latest tag.
// It is kept distinct from unpublished on purpose: the path is public on main,
// so it is barred from the early-removal deviation even though no release
// carries it yet.
func unreleased() []PublicationRef {
	return []PublicationRef{
		{Ref: remoteMainRef, Present: true},
		{Ref: localMainRef, Present: true},
		{Ref: latestReleaseTag, Present: false},
	}
}

// RecoveryOverlaps carries the three reconciliation figures Appendix B does not
// record. They come from the systemic audit rather than from a count taken here,
// because a generator that derived them would only ever agree with itself.
func RecoveryOverlaps() OverlapCounts {
	return OverlapCounts{
		CollisionPRs:   74,
		Overlaps:       499,
		Decompositions: 16,
	}
}

// RecoveryRows returns the declared disposition of every path the design's File
// Actions table names. The slice is rebuilt on each call so a caller mutating a
// row cannot retroactively change evidence another caller already validated.
func RecoveryRows() []Row {
	rows := make([]Row, 0, 128)
	rows = append(rows, agentGuidanceRows()...)
	rows = append(rows, routingWiringRows()...)
	rows = append(rows, reviewAuthorityRows()...)
	rows = append(rows, sddLifecycleRows()...)
	rows = append(rows, transplantSourceRows()...)
	rows = append(rows, alreadyImplementedDeletionRows()...)
	rows = append(rows, retiredPackageRows()...)
	rows = append(rows, retiredCommandRows()...)
	rows = append(rows, unauthorizedDeletionRows()...)
	rows = append(rows, regenerationRows()...)
	return rows
}

// agentGuidanceRows covers the ACI surface that replaces the retired remote
// control plane: the landed baseline projection, the manifest that describes
// each adapter, and the read-only inspection that is still unwritten.
func agentGuidanceRows() []Row {
	return []Row{
		{
			Path:        "internal/components/agentguidance/inject.go",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "baseline organic guidance is injected transactionally for every configured agent",
			Proof:       []string{"internal/components/agentguidance/inject_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/components/agentguidance/routing.go",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "routing guidance renders semantically equal across adapters and omits retired control-plane vocabulary",
			Proof:       []string{"internal/components/agentguidance/routing_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/components/agentguidance/inject_test.go",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "injection failure leaves no partially written agent configuration",
			Proof:       []string{"internal/components/agentguidance/inject_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/components/agentguidance/routing_test.go",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "an unregistered agent is rejected rather than rendered with guessed guidance",
			Proof:       []string{"internal/components/agentguidance/routing_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		// Read-only digest inspection is still unwritten, so it owns no invariant
		// yet. Recording it as KEEP would credit the recovery with behavior that
		// does not exist in any tree.
		{
			Path:        "internal/components/agentguidance/status.go",
			Disposition: DispositionDefer,
			Context:     ContextACI,
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/components/agentguidance/status_test.go",
			Disposition: DispositionDefer,
			Context:     ContextACI,
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/agents/capabilitymanifest/manifest.go",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "per-adapter facts stay truthful and unknown agents are rejected instead of assumed",
			Proof:       []string{"internal/agents/capabilitymanifest/manifest_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/agents/capabilitymanifest/manifest_test.go",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "every declared adapter capability is asserted against the manifest, not against prose",
			Proof:       []string{"internal/agents/capabilitymanifest/manifest_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
	}
}

// routingWiringRows covers the install, sync, and diagnostic surfaces that
// schedule the guidance projection and report when it has drifted.
func routingWiringRows() []Row {
	return []Row{
		{
			Path:        "internal/cli/run.go",
			Disposition: DispositionKeep,
			Context:     ContextHCR,
			Invariant:   "the install plan schedules routing injection for every agent outside the SDD component",
			Proof:       []string{"internal/cli/run_component_paths_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/cli/sync.go",
			Disposition: DispositionKeep,
			Context:     ContextHCR,
			Invariant:   "an explicit sync replaces managed blocks, preserves user content, and converges",
			Proof:       []string{"internal/cli/sync_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/cli/run_component_paths_test.go",
			Disposition: DispositionKeep,
			Context:     ContextHCR,
			Invariant:   "the staged component paths are asserted per agent rather than assumed uniform",
			Proof:       []string{"internal/cli/run_component_paths_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/cli/sync_test.go",
			Disposition: DispositionKeep,
			Context:     ContextHCR,
			Invariant:   "repeated sync is idempotent and never destroys user-authored content",
			Proof:       []string{"internal/cli/sync_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/cli/doctor.go",
			Disposition: DispositionKeep,
			Context:     ContextEPD,
			Invariant:   "Doctor reports routing drift as an actionable diagnostic instead of silently converging",
			Proof:       []string{"internal/cli/doctor_test.go"},
			Contributor: recoveryContributor,
			Publication: unreleased(),
		},
		{
			Path:        "internal/cli/doctor_test.go",
			Disposition: DispositionKeep,
			Context:     ContextEPD,
			Invariant:   "the diagnostic surface is asserted by code, not by its help text",
			Proof:       []string{"internal/cli/doctor_test.go"},
			Contributor: recoveryContributor,
			Publication: unreleased(),
		},
		{
			Path:        "internal/doctor/doctor.go",
			Disposition: DispositionKeep,
			Context:     ContextEPD,
			Invariant:   "routing:sync-required is emitted as a typed diagnostic with a bounded remedy",
			Proof:       []string{"internal/doctor/doctor_test.go"},
			Contributor: recoveryContributor,
			Publication: unreleased(),
		},
		{
			Path:        "internal/doctor/doctor_test.go",
			Disposition: DispositionKeep,
			Context:     ContextEPD,
			Invariant:   "every diagnostic code the doctor can emit is covered by an assertion",
			Proof:       []string{"internal/doctor/doctor_test.go"},
			Contributor: recoveryContributor,
			Publication: unreleased(),
		},
	}
}

// reviewAuthorityRows covers the RAR surface: the kill switch, the risk tier, the
// verification contract, and the command surface that projects them.
func reviewAuthorityRows() []Row {
	return []Row{
		{
			Path:        "internal/state/state.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "global review mode persists in uncommitted user state; any off wins over a repository preference",
			Proof:       []string{"internal/state/state_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/state/state_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "a repository cannot force review mode on or share policy with another clone",
			Proof:       []string{"internal/state/state_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/reviewtransaction/risk.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "review tier follows authorization, data-loss, mutation, release, and security risk; size alone never escalates",
			Proof:       []string{"internal/reviewtransaction/risk_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		// risk_test.go is named by the design as deliberately absent from the
		// branch diff. It is recorded so the ledger states that its coverage was
		// left untouched rather than leaving a named path unaccounted for.
		{
			Path:        "internal/reviewtransaction/risk_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "the risk classifier's tier boundaries stay asserted by an untouched test",
			Proof:       []string{"internal/reviewtransaction/risk_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/cli/review.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "the review command surface is the only route to review authority",
			Proof:       []string{"internal/cli/review_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/cli/review_facade.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "the compact facade appends and reads back native authority before materializing artifacts",
			Proof:       []string{"internal/cli/review_facade_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/cli/review_status_contract.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "status is read-only and never authors a transition",
			Proof:       []string{"internal/cli/review_status_contract_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/cli/review_next_transition.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "at most one authorized transition is offered, and a stale authorization is refused",
			Proof:       []string{"internal/cli/review_next_transition_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/app/app.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "retired remote control-plane commands no longer dispatch",
			Proof:       []string{"internal/app/app_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/app/app_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "the dispatch table is asserted command by command rather than by count",
			Proof:       []string{"internal/app/app_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/app/help.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "help advertises only commands that still exist",
			Proof:       []string{"internal/app/help_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/app/help_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "help text and dispatch cannot drift apart unnoticed",
			Proof:       []string{"internal/app/help_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/reviewtransaction/verification_contract.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "applicability, cost, mutation effect, and permission effect are independent dimensions",
			Proof:       []string{"internal/reviewtransaction/verification_contract_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/reviewtransaction/verification_contract_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "every cross-product of the verification dimensions is asserted, including unknown",
			Proof:       []string{"internal/reviewtransaction/verification_contract_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/reviewtransaction/rar_plan_authority.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "the native plan authority binds every verification dimension; adapters never select lenses",
			Proof:       []string{"internal/reviewtransaction/rar_plan_authority_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/reviewtransaction/rar_plan_authority_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "a sensitive effect requires immediate authorization and cannot be pre-authorized",
			Proof:       []string{"internal/reviewtransaction/rar_plan_authority_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/reviewtransaction/compact_reviewer_capture.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "reviewer results are canonicalized and hashed natively; models never construct canonical bytes",
			Proof:       []string{"internal/reviewtransaction/compact_reviewer_capture_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/reviewtransaction/compact_reviewer_capture_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "a capture that does not echo the frozen subject hash is refused",
			Proof:       []string{"internal/reviewtransaction/compact_reviewer_capture_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		// The kill switch is written now: rdd_mode.go carries the global plus
		// clone-local off-only review mode, and review_mode.go projects it as the
		// command surface. Its rows are KEEP so the deletion rows that name
		// rdd_mode_test.go as a destination proof cite a path the ledger itself
		// acknowledges exists.
		{
			Path:        "internal/reviewtransaction/rdd_mode.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "review mode is user-owned and off-only per clone: any off wins, and re-enabling applies to future candidates only",
			Proof:       []string{"internal/reviewtransaction/rdd_mode_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/reviewtransaction/rdd_mode_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "disabled review authority stays read-only frozen and delivery reports disabled/unmanaged",
			Proof:       []string{"internal/reviewtransaction/rdd_mode_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		// The dedicated consent surface remains designed but unwritten; the
		// frozen-plan consent obligation is owned by rar_plan_authority.go today.
		{
			Path:        "internal/reviewtransaction/verification_consent.go",
			Disposition: DispositionDefer,
			Context:     ContextRAR,
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/reviewtransaction/verification_consent_test.go",
			Disposition: DispositionDefer,
			Context:     ContextRAR,
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/cli/review_mode.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "the review mode command is the only route to the kill switch and status never mutates",
			Proof:       []string{"internal/cli/review_mode_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/cli/review_mode_test.go",
			Disposition: DispositionKeep,
			Context:     ContextRAR,
			Invariant:   "every mode transition and its source/effective diagnostics are asserted by code",
			Proof:       []string{"internal/cli/review_mode_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/cli/review_verification_decide.go",
			Disposition: DispositionDefer,
			Context:     ContextRAR,
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/cli/review_verification_decide_test.go",
			Disposition: DispositionDefer,
			Context:     ContextRAR,
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
	}
}

// sddLifecycleRows covers the SDD component and status surfaces: what survives
// the removal of the remote control plane, and what dies with it.
func sddLifecycleRows() []Row {
	return []Row{
		{
			Path:        "internal/components/sdd/inject.go",
			Disposition: DispositionKeep,
			Context:     ContextSDD,
			Invariant:   "SDD injection carries optional SDD assets alone once routing coupling is removed",
			Proof:       []string{"internal/components/sdd/inject_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/components/sdd/boundedreview.go",
			Disposition: DispositionKeep,
			Context:     ContextSDD,
			Invariant:   "the bounded review contract is delivered to agents without remote work vocabulary",
			Proof:       []string{"internal/components/sdd/bounded_review_contract_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/components/sdd/inject_test.go",
			Disposition: DispositionKeep,
			Context:     ContextSDD,
			Invariant:   "the injected SDD surface is asserted section by section, so a removed step cannot pass silently",
			Proof:       []string{"internal/components/sdd/inject_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/components/sdd/bounded_review_contract_test.go",
			Disposition: DispositionKeep,
			Context:     ContextSDD,
			Invariant:   "the bounded review contract text is asserted rather than trusted",
			Proof:       []string{"internal/components/sdd/bounded_review_contract_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		// The trigger-rules injector is published, so it is removed in systemic
		// order rather than early. Its retained obligation — that organic routing
		// guidance reaches every configured agent — is carried by the ACI routing
		// projection, which is proven today.
		{
			Path:             "internal/components/sdd/triggerrules.go",
			Disposition:      DispositionDelete,
			Context:          ContextACI,
			Invariant:        "organic routing guidance reaches every configured agent",
			Contributor:      recoveryContributor,
			Publication:      published(),
			DestinationPath:  "internal/components/agentguidance/routing.go",
			DestinationProof: []string{"internal/components/agentguidance/routing_test.go"},
		},
		{
			Path:             "internal/components/sdd/triggerrules_test.go",
			Disposition:      DispositionDelete,
			Context:          ContextACI,
			Invariant:        "the rendered routing guidance is asserted for every supported adapter",
			Contributor:      recoveryContributor,
			Publication:      published(),
			DestinationPath:  "internal/components/agentguidance/routing_test.go",
			DestinationProof: []string{"internal/components/agentguidance/routing_test.go"},
		},
		{
			Path:        "internal/sddstatus/runtime_ledger.go",
			Disposition: DispositionKeep,
			Context:     ContextSDD,
			Invariant:   "attempt authority survives the removal of the WorkRun binding branches",
			Proof:       []string{"internal/sddstatus/runtime_ledger_test.go"},
			Contributor: recoveryContributor,
			Publication: unreleased(),
		},
		{
			Path:        "internal/sddstatus/runtime_ledger_test.go",
			Disposition: DispositionKeep,
			Context:     ContextSDD,
			Invariant:   "attempt transitions stay asserted after the reservation branches are gone",
			Proof:       []string{"internal/sddstatus/runtime_ledger_test.go"},
			Contributor: recoveryContributor,
			Publication: unreleased(),
		},
		{
			Path:        "internal/cli/sdd_attempt.go",
			Disposition: DispositionKeep,
			Context:     ContextSDD,
			Invariant:   "the SDD attempt command keeps its own authority and never depends on a WorkRun",
			Proof:       []string{"internal/cli/sdd_attempt_test.go"},
			Contributor: recoveryContributor,
			Publication: unreleased(),
		},
		// The WorkRun binding exists only to reserve a run against the retired
		// control plane. Nothing outside that plane consumes it, so it retains no
		// invariant to transplant.
		{
			Path:                "internal/sddstatus/workrun_binding.go",
			Disposition:         DispositionDelete,
			Context:             ContextSDD,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		{
			Path:                "internal/sddstatus/workrun_binding_test.go",
			Disposition:         DispositionDelete,
			Context:             ContextSDD,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
	}
}

// transplantSourceRows once carried the highest-loss sources whose invariant
// was implemented nowhere else in the surviving tree. Every one of them has
// since been resolved: the RAR plan authority (rar_plan_authority.go with
// verification_contract.go) now owns frozen-plan consent end to end, and the
// kill switch (rdd_mode.go) landed with its recovery classes. Each former
// TRANSPLANT row is therefore a DELETE whose destination names the surviving
// test that proves the invariant today.
func transplantSourceRows() []Row {
	return []Row{
		// NewVerificationFrozenPlanConsent manufactures consent only from a plan
		// whose gate asks for it; ResumeFrozenVerificationPlan replays the exact
		// frozen plan once, never asks again, and rejects stale, cross-bound, or
		// forged consent (TestFrozenPlanConsentResumeReusesTheIdenticalPlan).
		{
			Path:             "internal/workrun/verification_consent.go",
			Disposition:      DispositionDelete,
			Context:          ContextRAR,
			Invariant:        "verification consent is frozen once, replayed exactly, and never re-asked",
			Contributor:      recoveryContributor,
			Publication:      unpublished(),
			EarlyDeviation:   true,
			DestinationPath:  "internal/reviewtransaction/rar_plan_authority.go",
			DestinationProof: []string{"internal/reviewtransaction/rar_plan_authority_test.go"},
		},
		// The consent record binds AuthorityRef and PlanDigest of the exact
		// owner-authored frozen plan; nothing else can manufacture one, and a
		// plan that does not require consent refuses to issue it.
		{
			Path:             "internal/workprovider/productive_verification_consent.go",
			Disposition:      DispositionDelete,
			Context:          ContextRAR,
			Invariant:        "the consent prompt is owner-authored and offers only the choices the owner declared",
			Contributor:      recoveryContributor,
			Publication:      unpublished(),
			EarlyDeviation:   true,
			DestinationPath:  "internal/reviewtransaction/rar_plan_authority.go",
			DestinationProof: []string{"internal/reviewtransaction/rar_plan_authority_test.go"},
		},
		// One consent resumes one identical plan; the resumed gate never asks
		// twice, and consent cannot cross to another frozen plan.
		{
			Path:             "internal/cli/work_verification_decide.go",
			Disposition:      DispositionDelete,
			Context:          ContextRAR,
			Invariant:        "a consent decision is taken exactly once against the owner's prompt reference",
			Contributor:      recoveryContributor,
			Publication:      unpublished(),
			EarlyDeviation:   true,
			DestinationPath:  "internal/reviewtransaction/rar_plan_authority.go",
			DestinationProof: []string{"internal/reviewtransaction/rar_plan_authority_test.go"},
		},
		// The failure classes this test carried — stale, cross-bound, and forged
		// consent answers that must mutate nothing — are asserted against the
		// surviving plan authority rather than the retired CLI surface.
		{
			Path:             "internal/cli/work_verification_decide_test.go",
			Disposition:      DispositionDelete,
			Context:          ContextRAR,
			Invariant:        "an invalid, ambiguous, or unoffered consent answer mutates nothing",
			Contributor:      recoveryContributor,
			Publication:      unpublished(),
			EarlyDeviation:   true,
			DestinationPath:  "internal/reviewtransaction/rar_plan_authority.go",
			DestinationProof: []string{"internal/reviewtransaction/rar_plan_authority_test.go"},
		},
		// The kill switch this row waited for exists now (rdd_mode.go), and the
		// recovery classes live in the surviving review transaction: disabled
		// frozen authority stays read-only (rdd_mode_test.go), interrupted
		// authority is repaired without a second effect (authority_repair_test.go),
		// and the prepared reconcile journal admits exactly one replay
		// (compact_batch_reconcile_journal_test.go).
		{
			Path:            "internal/workprovider/productive_advance_recovery_test.go",
			Disposition:     DispositionDelete,
			Context:         ContextHCR,
			Invariant:       "crash and kill-switch failure classes leave recoverable state without a second effect",
			Contributor:     recoveryContributor,
			Publication:     unpublished(),
			EarlyDeviation:  true,
			DestinationPath: "internal/reviewtransaction/",
			DestinationProof: []string{
				"internal/reviewtransaction/rdd_mode_test.go",
				"internal/reviewtransaction/authority_repair_test.go",
				"internal/reviewtransaction/compact_batch_reconcile_journal_test.go",
			},
		},
	}
}

// alreadyImplementedDeletionRows covers the sources the design listed as
// transplants that the re-check found already implemented, independently, in the
// surviving internal/reviewtransaction. They are DELETE with no retained
// invariant rather than TRANSPLANT, because a transplant row would claim an
// obligation is still in flight when the receiving code is written, tested, and
// reachable today.
//
// Each row's comment names the exact surviving file and symbol, so the claim is
// auditable from the ledger instead of taken on trust. All of them are
// unpublished on every ref, so the early-removal deviation is authorized.
func alreadyImplementedDeletionRows() []Row {
	return []Row{
		// store.go carries all three halves: the CAS revision guard rejects a
		// mismatched predecessor with ErrConcurrentUpdate, publication is
		// content-addressed and reads the existing event back before accepting it
		// rather than clobbering, and HEAD is replaced through writeAtomic.
		{
			Path:                "internal/workrun/store.go",
			Disposition:         DispositionDelete,
			Context:             ContextMMI,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// snapshot.go runGitCaptured scrubs the unsafe GIT_* variables, forces
		// LC_ALL=C, pins LANG=C for the frozen candidate, bounds every launch with
		// localGitCommandTimeout/remoteGitCommandTimeout, and caps output through
		// boundedGitOutput/GitOutputLimitError. Full-OID-only results come from
		// gate.go resolving revisions with rev-parse --verify <rev>^{commit} against
		// the object format frozen_candidate_context.go validated. Process-tree kill
		// is git_process_unix.go and git_process_windows.go.
		{
			Path:                "internal/workprovider/pad_git_object_authority.go",
			Disposition:         DispositionDelete,
			Context:             ContextHCR,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// rar_path_safety.go detects directory identity replacement across the whole
		// window with six os.SameFile checks spanning lock acquisition
		// (acquireRARAuthorityLock, acquirePrivateRARLocalStoreLock) through
		// publishPrivateRARImmutable, and repository_locator.go adds eleven more.
		{
			Path:                "internal/workrun/path_safety.go",
			Disposition:         DispositionDelete,
			Context:             ContextMMI,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// rar_path_safety_unix.go refuses traversal at the syscall: rarPathUnsafe
		// rejects a symlink outright, and openRARPathNoFollow opens relative to a
		// secured parent descriptor with O_NOFOLLOW|O_CLOEXEC.
		{
			Path:                "internal/workrun/path_safety_unix.go",
			Disposition:         DispositionDelete,
			Context:             ContextMMI,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// rar_path_safety_windows.go validates reparse points before publication
		// (rarPathUnsafe, openWindowsRARObject, openWindowsRARFileUnsafe) and
		// enforces the owner-only DACL through privateRARSecurityDescriptorSafe,
		// ownerOnlyRARSecurityDescriptor, and ownerOnlyRARWindowsAccessMask.
		{
			Path:                "internal/workrun/path_safety_windows.go",
			Disposition:         DispositionDelete,
			Context:             ContextMMI,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// compact_reviewer_capture_unix.go implements the hardlink-count guard as
		// compactReviewerFileSingleLink (stat.Nlink == 1), and
		// compact_reviewer_capture.go applies it three times: before opening, on the
		// opened descriptor, and again after the write.
		{
			Path:                "internal/workprovider/productive_advance_path_unix.go",
			Disposition:         DispositionDelete,
			Context:             ContextMMI,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// compact_reviewer_capture_windows.go is the Windows half of the same guard,
		// refusing a shared-handle target when NumberOfLinks != 1.
		{
			Path:                "internal/workprovider/productive_advance_path_windows.go",
			Disposition:         DispositionDelete,
			Context:             ContextMMI,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// compact_batch_reconcile_journal.go is the same invariant: a prepared
		// journal is persisted as a global fail-closed marker before any entry
		// moves, the whole plan is bound by request identity, and a crash resumes
		// only through replayCompactBatchReconcileJournalLocked, which either
		// completes the identical batch or refuses it. compact_batch_reconcile_plan.go
		// derives the all-or-nothing plan itself.
		{
			Path:                "internal/workprovider/runtime_policy_provisioner.go",
			Disposition:         DispositionDelete,
			Context:             ContextMMI,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// artifact_subject.go makes candidate identity immutable: NewArtifactSubject
		// derives one hashed slot identity from the frozen candidate and refuses a
		// subject that binds no lens slot. compact_reviewer_capture.go keeps the slot
		// append-only, and requireCompactReviewerSlotCompatible refuses to rebind a
		// bound slot to different canonical bytes; compact_result_disposition.go
		// keeps the order+lens slot exclusive.
		{
			Path:                "internal/workprovider/pad_candidate_catalog.go",
			Disposition:         DispositionDelete,
			Context:             ContextPAD,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
	}
}

// retiredPackageRows covers the six packages that die with the remote control
// plane. Each is unpublished on every ref, so the early-removal deviation is
// authorized.
//
// The first four are delete-clean, not transplant sources. Their obligations are
// already implemented independently in the surviving internal/reviewtransaction,
// which imports no other internal package at all; the surviving review flow
// (internal/cli/review.go, review_facade.go, review_status_contract.go,
// review_next_transition.go) reaches only internal/reviewtransaction and
// internal/sddstatus. Outside the retired plane the four packages have no
// consumer but e2e/organicruntime/organic_runtime_test.go, which this recovery
// rewrites. Naming a DestinationPath would therefore overstate the record: no
// obligation moves, because none was ever exclusively theirs. Each row's comment
// names the exact surviving file that already carries the behavior, so a reader
// can audit the claim without rerunning the search that produced it.
func retiredPackageRows() []Row {
	return []Row{
		// Already implemented in the surviving review transaction:
		//   - Windows use-handle token-owner rebind and the canonical
		//     private-directory ACE split: rar_path_safety_windows.go
		//     (privateRARSecurityDescriptorSafe, ownerOnlyRARSecurityDescriptor,
		//     rarSharedSecurityDescriptorOwnedByCurrentProcess), with
		//     publish_immutable_windows.go and compact_reviewer_capture_windows.go.
		//   - Advisory-lock inode truth: rar_path_safety.go, whose
		//     acquireRARAuthorityLock and acquirePrivateRARLocalStoreLock re-check
		//     os.SameFile on the held descriptor instead of trusting the path.
		//   - Read-only delivery replay: compact_batch_reconcile_journal.go, whose
		//     prepared marker admits exactly one replay and no second effect.
		//   - Trusted-repository identity lease: repository_locator.go, which pins
		//     directory identity across eleven os.SameFile checks.
		//
		// One invariant was retained rather than found pre-implemented: the "PR
		// commands" threat-matrix boundary (validateDeliveryRefToken) moved to
		// gate.go validateBoundaryRefSelector, which holds every pre-PR/pre-push
		// boundary selector and gate-request base ref to the same structured-ref
		// rule before it can become a Git argv element.
		{
			Path:             "internal/deliveryadmission/",
			Disposition:      DispositionDelete,
			Context:          ContextMMI,
			Invariant:        "PR head and destination refs are structured bindings, never command strings",
			Contributor:      recoveryContributor,
			Publication:      unpublished(),
			EarlyDeviation:   true,
			DestinationPath:  "internal/reviewtransaction/gate.go",
			DestinationProof: []string{"internal/reviewtransaction/pr_command_boundary_test.go"},
		},
		// Already implemented in the surviving review transaction:
		//   - Immutable append-only, replay-safe execution admission:
		//     artifact_admission.go (AdmitArtifact validates the subject echo and a
		//     completed full-manifest inspection before a result becomes canonical)
		//     over the content-addressed append-only chain in store.go.
		//   - Owner action tickets that require execution proof:
		//     final_verification_retry.go, which refuses a retry whose captured
		//     evidence bytes do not bind the terminal state and the FINALIZE
		//     journal, and incident.go for the terminal record.
		//   - Windows evidence path safety: rar_path_safety_windows.go and
		//     compact_reviewer_capture_windows.go.
		{
			Path:                "internal/evidence/",
			Disposition:         DispositionDelete,
			Context:             ContextEPD,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// Already implemented in the surviving review transaction:
		//   - Bounded execution with process-tree kill on both platforms:
		//     git_process_unix.go and git_process_windows.go (startGitProcessTree),
		//     driven by snapshot.go runGitCaptured, which binds one request to one
		//     launched argv under localGitCommandTimeout/remoteGitCommandTimeout and
		//     caps output through boundedGitOutput.
		//
		// The generic host executor's remaining pieces — request-to-launch evidence
		// binding and toolchain content identity — are not transplanted, because the
		// surviving flow launches only git through that one bounded path and nothing
		// outside the retired plane ever asked for an arbitrary host command.
		{
			Path:                "internal/hostruntime/",
			Disposition:         DispositionDelete,
			Context:             ContextHCR,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// Already implemented in the surviving review transaction:
		//   - Cross-platform directory fsync before a mutation counts as complete:
		//     store.go SyncReviewDirectory, called by writeAtomic after every
		//     rename and degrading only where Windows rejects a directory handle.
		//   - Structural completion evidence: compact_reviewer_capture.go observes
		//     the target before opening, after opening, and after writing, failing
		//     closed on any identity or link-count change, and
		//     frozen_candidate_context.go freezes the candidate it read.
		{
			Path:                "internal/mutationintegrity/",
			Disposition:         DispositionDelete,
			Context:             ContextMMI,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		},
		// The two control-plane packages keep their own rows for the highest-loss
		// sources above. These directory rows carry only the residual generic
		// invariants, which already have surviving equivalents.
		{
			Path:            "internal/workrun/",
			Disposition:     DispositionDelete,
			Context:         ContextMMI,
			Invariant:       "residual atomic-publication and path-containment invariants outside the individually transplanted sources",
			Contributor:     recoveryContributor,
			Publication:     unpublished(),
			EarlyDeviation:  true,
			DestinationPath: "internal/reviewtransaction/",
			DestinationProof: []string{
				"internal/reviewtransaction/store_test.go",
				"internal/reviewtransaction/rar_path_safety_windows_test.go",
			},
		},
		{
			Path:            "internal/workprovider/",
			Disposition:     DispositionDelete,
			Context:         ContextPAD,
			Invariant:       "residual delivery-authorization and terminal-outcome invariants outside the individually transplanted sources",
			Contributor:     recoveryContributor,
			Publication:     unpublished(),
			EarlyDeviation:  true,
			DestinationPath: "internal/reviewtransaction/",
			DestinationProof: []string{
				"internal/reviewtransaction/gate_test.go",
				"internal/reviewtransaction/receipt_test.go",
			},
		},
	}
}

// retiredCommandRows covers the command surface, its tests, and the wire
// contracts of the remote control plane. None of them reached any authoritative
// ref, and none owns an invariant that outlives the plane they belong to.
func retiredCommandRows() []Row {
	paths := []string{
		"internal/cli/work_advance.go",
		"internal/cli/work_capabilities.go",
		"internal/cli/work_reconcile.go",
		"internal/cli/work_route.go",
		"internal/cli/work_start.go",
		"internal/cli/work_status.go",
		"internal/cli/work_transition.go",
		// work_flags.go holds validateExactWorkFlags and encodeWorkJSON, consumed
		// only by the eight retired commands, so it cannot outlive them.
		"internal/cli/work_flags.go",
		"internal/cli/work_advance_test.go",
		"internal/cli/work_command_test.go",
		"internal/cli/work_reconcile_test.go",
		"internal/cli/work_route_test.go",
		"internal/cli/work_runtime_test.go",
		"internal/cli/work_status_test.go",
		"internal/cli/work_transition_test.go",
		// The only importer of internal/workprovider outside internal/cli and e2e.
		"internal/app/work_provider_test.go",
		"contracts/work-routing/v1/",
	}

	rows := make([]Row, 0, len(paths))
	for _, path := range paths {
		rows = append(rows, Row{
			Path:                path,
			Disposition:         DispositionDelete,
			Context:             ContextPAD,
			Contributor:         recoveryContributor,
			Publication:         unpublished(),
			EarlyDeviation:      true,
			NoRetainedInvariant: true,
		})
	}
	return rows
}

// unauthorizedDeletionRows records the published paths the branch already
// removed without authorization from any design.
//
// They are DEFER, not DELETE, and that is the whole point of the row. A DELETE
// row would have to name either a destination that received the trigger rule set
// or a proof that nothing was retained, and neither exists: the default
// lifecycle bindings these paths defined survive nowhere in the tree. The
// deviation is therefore recorded as unresolved, with publication evidence
// showing the paths shipped to users.
func unauthorizedDeletionRows() []Row {
	paths := []string{
		"internal/catalog/triggers.go",
		"internal/catalog/triggers_test.go",
		"internal/catalog/bounded_review_triggers_test.go",
		"internal/catalog/release_event_test.go",
		"internal/testdata/golden/trigger-rules-default.golden",
		// The file survives; the Trigger* types it declared do not. The removal is
		// recorded against the path because the ledger's unit is a path, and a
		// whole-path DELETE would misstate what happened.
		"internal/model/types.go",
	}

	rows := make([]Row, 0, len(paths))
	for _, path := range paths {
		rows = append(rows, Row{
			Path:        path,
			Disposition: DispositionDefer,
			Context:     ContextACI,
			Contributor: recoveryContributor,
			Publication: published(),
		})
	}
	return rows
}

// regenerationRows covers the rewritten runtime journey, the regenerated agent
// assets, and the published guidance they project.
func regenerationRows() []Row {
	rows := []Row{
		{
			Path:        "e2e/organicruntime/organic_runtime_test.go",
			Disposition: DispositionRewrite,
			Context:     ContextRAR,
			Invariant:   "real configured-agent journeys cover direct, delegated, optional-SDD, every review tier, one bounded correction, and flexible delivery",
			Proof:       []string{"e2e/organicruntime/organic_runtime_test.go"},
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
		{
			Path:        "internal/assets/assets_test.go",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "the embedded asset assertions match the organic surface, and are rewritten before regeneration rather than after",
			Proof:       []string{"internal/assets/assets_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/assets/skills/_shared/review-ledger-contract.md",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "the embedded key skills/_shared/review-ledger-contract.md is unchanged so existing installs still resolve it",
			Proof:       []string{"internal/components/sdd/review_ledger_contract_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		{
			Path:        "internal/components/golden_test.go",
			Disposition: DispositionKeep,
			Context:     ContextACI,
			Invariant:   "golden projections are compared byte for byte, so a regenerated asset cannot drift unnoticed",
			Proof:       []string{"internal/components/golden_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		},
		// The published guidance is regenerated, but no test reads it. Leaving its
		// invariant empty states that gap instead of naming an unrelated test as
		// though it proved the document.
		{
			Path:        "docs/trigger-rules.md",
			Disposition: DispositionRegenerate,
			Context:     ContextACI,
			Contributor: recoveryContributor,
			Publication: published(),
		},
		// The golden fixture for the regenerated projection is still unwritten.
		{
			Path:        "testdata/golden/organic-routing-manifest.json",
			Disposition: DispositionDefer,
			Context:     ContextACI,
			Contributor: recoveryContributor,
			Publication: unpublished(),
		},
	}

	// Every adapter receives the same regenerated orchestrator prompt, and the
	// same test asserts all of them, so the rows are generated from the adapter
	// list rather than repeated twelve times.
	adapters := []string{
		"antigravity", "claude", "codex", "cursor", "gemini", "generic",
		"hermes", "kimi", "kiro", "opencode", "qwen", "windsurf",
	}
	for _, adapter := range adapters {
		rows = append(rows, Row{
			Path:        "internal/assets/" + adapter + "/sdd-orchestrator.md",
			Disposition: DispositionRegenerate,
			Context:     ContextACI,
			Invariant:   "the orchestrator prompt carries organic routing under systemic order and no remote work vocabulary",
			Proof:       []string{"internal/assets/assets_test.go"},
			Contributor: recoveryContributor,
			Publication: published(),
		})
	}
	return rows
}
