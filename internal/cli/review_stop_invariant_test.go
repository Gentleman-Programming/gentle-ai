package cli

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// reviewStopReasonDocsRowTextRegexp extracts both the reason code and its
// full continuation text from one row of the "Continue after a stop reason
// code" table in docs/review-integration.md (reviewStopReasonDocsSection
// already scopes the search to that table only). The docs table's own prose
// defines the operative signal this file cross-checks against: "the table
// below names ... the exact continuation for consumers that hold --cwd
// access, and `terminal` where no flag-driven continuation exists ...
// `terminal` never means 'contact support with no further detail'; each
// terminal row states the concrete precondition that would unblock it." A
// row is only ever written with that literal leading word "Terminal" when no
// flag-driven continuation exists — every other row leads with the concrete
// command or condition itself.
var reviewStopReasonDocsRowTextRegexp = regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\| (.+) \\|$")

// reviewStopReasonDocsTerminalPrefix is the exact literal marker
// docs/review-integration.md uses to open a row that has no caller-side,
// flag-driven continuation. See reviewStopReasonDocsRowTextRegexp.
const reviewStopReasonDocsTerminalPrefix = "Terminal"

// reviewStopInvariantClassification is the table-driven binding Duty 3 of
// design.md requires: every reason code newReviewNextTransition (and its
// helpers) can emit is bound here to exactly one disposition —
// reviewStopTerminal ("no caller-side continuation exists; a human/maintainer
// action outside this negotiation is required") or
// reviewStopCallerContinuable ("the stop is intentional and still reason-coded
// as `stop`, but a caller who holds --cwd access can resolve it without a
// maintainer, by acting on the docs-table continuation — usually different
// flags/selectors, or in-repository work such as writing a corrected
// candidate"). TestReviewStopInvariantReasonCodesAreClassified fails closed if
// a code is emitted without an entry here or an entry here is no longer
// emitted; TestReviewStopInvariantTerminalClassificationAgreesWithDocs fails
// if any entry's disposition here disagrees with whether its docs row opens
// with the literal "Terminal" marker — the structural proof for the
// "In-Band Recovery Discoverability" requirement's negative scenario ("a code
// whose docs row names a concrete command CANNOT be classified terminal").
//
// A SUSPECT resolved by adding a routing branch (so review_next_transition.go
// no longer emits that literal reviewStopTransition("...") call at all) is
// removed from this table entirely, not marked with either disposition — see
// the organic-dx Phase 3 investigation note on escalated_recovery_requires_changed_target.
var reviewStopInvariantClassification = map[string]reviewStopDisposition{
	"captured_artifacts_unverifiable": {
		Terminal:      true,
		Justification: "inspection failure of an already-captured artifact, not a routable state; requires a maintainer to inspect the review authority store directly",
		// User-decision, not tool-fault: this is a data-integrity finding
		// (tampering or hash mismatch on a previously captured artifact),
		// not proof the code reached a state it shouldn't. Filing it as a
		// public defect report could also leak that a store was tampered
		// with — a maintainer decision, not an automatic one.
		ToolFault: reviewStopToolFault(false),
	},
	"captured_result_selection_unavailable": {
		Terminal:      true,
		Justification: "internal invariant violation — every selected lens already reports a captured artifact yet the caller was routed here anyway; no caller-side retry exists",
		// Tool-fault: the justification is a literal internal invariant
		// violation — this state should be structurally unreachable, so
		// reaching it proves a genuine code defect.
		ToolFault: reviewStopToolFault(true),
	},
	"corrected_candidate_unavailable": {
		Terminal:      false,
		Justification: "caller-continuable: change the candidate content so it differs from the frozen original, then re-run `review status --next-transition` (or `review finalize`) — a concrete, flag-driven command, not a maintainer-only action; the docs row does not open with \"Terminal\", so pinning this terminal would contradict it (discoverability sweep finding beyond the three the audit named explicitly)",
	},
	"corrupted_or_unverifiable_authority": {
		Terminal:      true,
		Justification: "review repair --preflight already classified this authority unsupported/ambiguous/conflicting/truncated; requires a maintainer to inspect the review authority store directly",
		// User-decision: a corrupted/unsupported store is a data-integrity
		// finding requiring a maintainer's eyes, not proof of a code defect.
		ToolFault: reviewStopToolFault(false),
	},
	"final_verification_retry_unavailable": {
		Terminal:      true,
		Justification: "internal invariant violation — routed to final-verification-retry collection without a retry-eligible disposition; no caller-side retry exists",
		// Tool-fault: literal internal invariant violation.
		ToolFault: reviewStopToolFault(true),
	},
	"manual_intervention_required": {
		Terminal:      true,
		Justification: "default branch of the Authority.State switch: the authority state is not one of this negotiated protocol's known states. Every state the compact authority (or a legacy chain routed through TargetStatusActionStop) can actually reach is handled by an earlier, explicit branch, so this default is structurally unreachable in practice and exists as a defensive fallback for an unmodeled future state, exactly like Go's convention for an exhaustive string-typed switch — genuinely terminal if it ever fires",
		// Tool-fault: an unmodeled state was reached, the clearest possible
		// invariant-violation-class stop this table has.
		ToolFault: reviewStopToolFault(true),
	},
	"missing_authority_binding": {
		Terminal:      true,
		Justification: "internal invariant violation — applicability was current_target but no authority binding resolved; no caller-side retry exists",
		// Tool-fault: literal internal invariant violation.
		ToolFault: reviewStopToolFault(true),
	},
	"native_stop_required": {
		Terminal:      true,
		Justification: "only reachable for StateEscalated when neither the target changed, the escalation is accounting-only-eligible, nor a final-verification-retry is eligible (target_status.go:176-199) — a genuinely stuck escalated lineage; requires maintainer review",
		// User-decision: this is the state machine correctly reporting a
		// legitimate business outcome (an escalated lineage genuinely stuck
		// pending a human's review), not an unreachable/unmodeled state.
		ToolFault: reviewStopToolFault(false),
	},
	"original_finalize_request_required": {
		Terminal:      false,
		Justification: "caller-continuable: re-run `review finalize --lineage <id>` with the exact original content-bound payload — a concrete, flag-driven command; design.md previously misclassified this as terminal (Phase 3 task 3.10 contradiction fix)",
	},
	"pre_pr_selector_unrepresentable": {
		Terminal:      false,
		Justification: "caller-continuable: pass a symbolic ref name for --base-ref instead of a raw commit SHA — a concrete, flag-driven fix; no in-process routing substitutes a different gate, because the caller explicitly chose the pre-pr gate and silently validating against a different gate would answer a different question than the one asked",
	},
	"recovery_scope_unchanged": {
		Terminal:      false,
		Justification: "caller-continuable: change the candidate so its target identity differs from the current authority's, then retry the same selector-scoped review.recover — a concrete, flag-driven command; design.md previously misclassified this as terminal (Phase 3 task 3.10 contradiction fix)",
	},
	"recovery_target_unrepresentable": {
		Terminal:      false,
		Justification: "caller-continuable: use one of the three representable recovery selector shapes — a concrete, flag-driven fix; design.md previously misclassified this as terminal (Phase 3 task 3.10 contradiction fix)",
	},
	"staged_workspace_overlay_recovery_unavailable": {
		Terminal:      true,
		Justification: "terminal only for a fresh target with no existing lineage to recover; the row still names --lineage <id> or dropping --workspace-overlay as the way out of that terminal condition, matching the docs convention that a terminal row states its unblocking precondition",
		// User-decision: a caller-input-shape mismatch (this exact flag
		// combination is recovery-only), not an unreachable/unmodeled state.
		ToolFault: reviewStopToolFault(false),
	},
	"unchanged_or_unverified_authority": {
		Terminal:      true,
		Justification: "the single correction attempt for this lineage is already consumed without a verified candidate change (classifyCompactCorrectionTarget's Blocked claim); an ordinary lineage admits exactly one correction attempt, so no further in-lineage action exists — further work requires a new lineage",
		// User-decision: a legitimate, by-design business-rule terminal (one
		// correction attempt is the designed limit), not a code defect.
		ToolFault: reviewStopToolFault(false),
	},
}

type reviewStopDisposition struct {
	// Terminal is true when no caller-side, flag-driven continuation exists
	// for this reason code — only a maintainer acting outside this
	// negotiation can unblock it.
	Terminal bool
	// Justification is the human-authored proof backing Terminal's value. It
	// must be non-empty; TestReviewStopInvariantReasonCodesAreClassified
	// enforces that structurally rather than trusting review comments alone.
	Justification string
	// ToolFault is organic-dx Phase 5 task 5.1's classification column: only
	// meaningful when Terminal is true. true means the justification proves
	// this is an invariant-violation-class stop — a state the code's own
	// comments say should be structurally unreachable, so reaching it proves
	// a genuine code defect rather than a legitimate business-rule terminal
	// (an exhausted correction attempt, a genuinely stuck escalated lineage,
	// or a store the code correctly refuses to trust without a maintainer's
	// eyes). Only tool-fault terminals are candidates for automatic defect
	// reporting; nil for every caller-continuable (Terminal: false) entry,
	// where the question does not apply. This column is documentation only —
	// the two reason codes organic-dx Phase 5 actually wires a runtime
	// defect-report trigger for (receipt_publication_conflict and the
	// captured-final-EVIDENCE byte conflict, both confirmed genuine
	// deadlocks per the maintainer decision on tasks.md 3b.8/3b.9) are not
	// review_next_transition.go stop reason codes at all — they live in
	// review_facade.go and review_artifact.go respectively and are wired
	// directly in internal/cli/review_defect_report.go, not derived from
	// this table.
	ToolFault *bool
}

func reviewStopToolFault(value bool) *bool { return &value }

// TestReviewStopInvariantReasonCodesAreClassified is the RED-first structural
// proof for design.md Duty 3 / spec "No-Stop-With-Successor Invariant": every
// reason code review_next_transition.go's reviewStopTransition(...) call
// sites can literally emit must have exactly one entry in
// reviewStopInvariantClassification, and every entry must still be emitted.
// It fails closed in both directions, reusing the same literal-call
// extraction TestEveryReviewStopReasonCodeHasADocsContinuation already proves
// covers every reviewStopTransition("...") call site in this package
// (review_next_transition_docs_test.go). A newly discovered unrouted stop
// must be resolved by adding a routing branch (removing it from the source
// entirely) or by adding a classified, docs-agreeing entry here — never by
// exempting it from this test.
func TestReviewStopInvariantReasonCodesAreClassified(t *testing.T) {
	source, err := os.ReadFile("review_next_transition.go")
	if err != nil {
		t.Fatal(err)
	}
	matches := reviewStopTransitionCallRegexp.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatal("found no reviewStopTransition(\"...\") call sites in review_next_transition.go; the extraction regexp is stale")
	}
	sourceCodes := map[string]bool{}
	for _, match := range matches {
		sourceCodes[match[1]] = true
	}

	for code := range sourceCodes {
		disposition, classified := reviewStopInvariantClassification[code]
		if !classified {
			t.Errorf("reason code %q is emitted by review_next_transition.go but has no entry in reviewStopInvariantClassification — classify it terminal (with a justification agreeing with the docs table) or fix it via a routing branch", code)
			continue
		}
		if disposition.Justification == "" {
			t.Errorf("reason code %q has an empty Justification in reviewStopInvariantClassification", code)
		}
	}
	for code := range reviewStopInvariantClassification {
		if !sourceCodes[code] {
			t.Errorf("reviewStopInvariantClassification classifies reason code %q, which review_next_transition.go no longer emits — remove its entry (it was fixed by routing, not by exemption)", code)
		}
	}
}

// TestReviewStopInvariantTerminalClassificationAgreesWithDocs is the
// structural proof for spec requirement "In-Band Recovery Discoverability"'s
// negative scenario: "GIVEN a stop reason code whose documented continuation
// table row names a concrete command, WHEN the stop-invariant classification
// is built, THEN classifying that code as terminal fails the invariant
// test." It derives, from docs/review-integration.md's own row text, whether
// each code's row opens with the literal "Terminal" marker (the docs' own
// convention for "no flag-driven continuation exists" — see
// reviewStopReasonDocsRowTextRegexp) and fails if that disagrees with
// reviewStopInvariantClassification's Terminal value in either direction:
// a code marked Terminal here whose docs row is not "Terminal"-prefixed, or a
// code marked caller-continuable here whose docs row IS "Terminal"-prefixed.
func TestReviewStopInvariantTerminalClassificationAgreesWithDocs(t *testing.T) {
	docs, err := os.ReadFile("../../docs/review-integration.md")
	if err != nil {
		t.Fatal(err)
	}
	section := reviewStopReasonDocsSection(t, string(docs))
	rows := reviewStopReasonDocsRowTextRegexp.FindAllStringSubmatch(section, -1)
	if len(rows) == 0 {
		t.Fatal("found no rows in the \"Continue after a stop reason code\" table in docs/review-integration.md; the table heading or row shape moved")
	}
	docsTerminal := map[string]bool{}
	for _, row := range rows {
		code, text := row[1], row[2]
		docsTerminal[code] = strings.HasPrefix(text, reviewStopReasonDocsTerminalPrefix)
	}

	for code, disposition := range reviewStopInvariantClassification {
		docsSaysTerminal, present := docsTerminal[code]
		if !present {
			t.Errorf("reason code %q has no docs row to cross-check (TestEveryReviewStopReasonCodeHasADocsContinuation should already have caught this)", code)
			continue
		}
		if disposition.Terminal != docsSaysTerminal {
			t.Errorf("reason code %q classified Terminal=%v here, but its docs/review-integration.md row %s open with the literal %q marker — a code whose docs row names a concrete command must not be classified terminal, and a code whose docs row is genuinely terminal must not be classified caller-continuable",
				code, disposition.Terminal, map[bool]string{true: "does", false: "does not"}[docsSaysTerminal], reviewStopReasonDocsTerminalPrefix)
		}
	}
}

// TestReviewStopInvariantToolFaultColumnIsWellFormed is the structural proof
// for organic-dx Phase 5 task 5.1's "tool-fault vs user-decision" column:
// every terminal-proof entry (Terminal: true) must classify ToolFault one
// way or the other, and the question must not apply to any caller-continuable
// entry (Terminal: false), where ToolFault must stay nil.
func TestReviewStopInvariantToolFaultColumnIsWellFormed(t *testing.T) {
	for code, disposition := range reviewStopInvariantClassification {
		if disposition.Terminal && disposition.ToolFault == nil {
			t.Errorf("reason code %q is terminal but has no ToolFault classification (task 5.1 requires every terminal-proof row to get one of the two values)", code)
		}
		if !disposition.Terminal && disposition.ToolFault != nil {
			t.Errorf("reason code %q is caller-continuable, so ToolFault does not apply and must stay nil, got %v", code, *disposition.ToolFault)
		}
	}
}
