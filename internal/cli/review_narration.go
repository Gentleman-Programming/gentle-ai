package cli

import (
	"fmt"
	"regexp"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// reviewNarrationTier classifies every registered human-facing emission into
// exactly one of the Three-Tier Narration Contract's tiers (spec requirement
// "Three-Tier Narration Contract"). Tier B (machinery) never reaches the
// human surface by construction, so it has no registry entries: its only
// contract is silence, proved by the paired E2E scenario in
// e2e/organicruntime/organic_lifecycle_hardening_test.go rather than by data
// registered here.
type reviewNarrationTier string

const (
	reviewNarrationTierA reviewNarrationTier = "A"
	reviewNarrationTierC reviewNarrationTier = "C"
)

// reviewNarrationEmission is one registered human-facing emission: a stable
// ID independent of its rendered text (so rewording never breaks the
// registry key) plus its tier and the exact statement a human surface may
// show for it.
type reviewNarrationEmission struct {
	ID        string
	Tier      reviewNarrationTier
	Statement string
}

// reviewNarrationRegistry is the closed, growable set of narration sources
// design.md Duty 4 enforces for this change (the "enforced subset"): the
// review_next_transition.go stop reason codes (Tier C, keyed "stop:<code>"),
// the one-time consent-prompt vocabulary from review_mode.go (Tier A, keyed
// "consent:<name>"), and stream 3's escalation accounting statement (Tier A,
// keyed "escalation:accounting"). Phase 5 adds one more Tier C entry for the
// tool-fault defect-report companion clause (keyed "defect_report:<name>").
// Full internal/cli refusal coverage (~200 more errors.New sites) is a
// separate change.
//
// Growth rule: TestReviewNarrationRegistryCoversEveryStopReasonCode fails
// closed, in both directions, on any drift between this map's "stop:" keys
// and the literal reviewStopTransition("...") call sites in
// review_next_transition.go. The consent-prompt and escalation sources are a
// small, closed, already-fully-enumerated set; TestNarrationTierAAndCBanInternalVocabulary
// still runs over every entry here regardless of source.
var reviewNarrationRegistry = buildReviewNarrationRegistry()

func buildReviewNarrationRegistry() map[string]reviewNarrationEmission {
	registry := make(map[string]reviewNarrationEmission, len(reviewStopReasonNarration)+len(reviewConsentPromptNarration)+1)
	for code, statement := range reviewStopReasonNarration {
		id := "stop:" + code
		registry[id] = reviewNarrationEmission{ID: id, Tier: reviewNarrationTierC, Statement: statement}
	}
	for name, statement := range reviewConsentPromptNarration {
		id := "consent:" + name
		registry[id] = reviewNarrationEmission{ID: id, Tier: reviewNarrationTierA, Statement: statement}
	}
	// A representative rendering proves the template itself stays
	// vocabulary-clean; the real call site (sddstatus/review_gate.go) fills
	// in the same template with live numbers at run time.
	escalationSample := fmt.Sprintf(sddstatus.EscalationAccountingReasonTemplate,
		reviewtransactionEscalationCauseSample, 10, 0, 10)
	registry["escalation:accounting"] = reviewNarrationEmission{
		ID: "escalation:accounting", Tier: reviewNarrationTierA, Statement: escalationSample,
	}
	// Phase 5 task 5.6: the tool-fault defect-report companion clause is a
	// Tier C extension (at most one clause, still one statement, one
	// action). The sample uses a placeholder path; the vocabulary check
	// only cares about the fixed English around it.
	registry["defect_report:tool_fault"] = reviewNarrationEmission{
		ID: "defect_report:tool_fault", Tier: reviewNarrationTierC,
		Statement: "This is a tool-internal fault, not something you did." + reviewToolFaultDefectClause("<path>"),
	}
	return registry
}

// reviewtransactionEscalationCauseSample is any one of the three closed
// CompactEscalationAccounting.Cause values; the vocabulary check only cares
// about the template's fixed English, not which cause filled the slot.
const reviewtransactionEscalationCauseSample = "budget_exceeded"

// reviewStopReasonNarration is the Tier C statement for every reason code
// review_next_transition.go's reviewStopTransition(...) can emit (see
// review_stop_invariant_test.go's reviewStopInvariantClassification for the
// terminal/caller-continuable split this text is written to match). Every
// statement names the outcome in domain terms plus the single decision or
// command, per spec "Three-Tier Narration Contract"; commands and flags are
// backtick-quoted so TestNarrationTierAAndCBanInternalVocabulary's code-span
// exemption applies to them. corrected_candidate_unavailable and
// staged_workspace_overlay_recovery_unavailable carry the exact content
// organic-dx tasks.md 3b.10 already recorded as Phase 4 registry input.
// reviewModeDisableCloneCommand is the scoped form of the self-service
// delivery exit named throughout this registry (adversarial finding F6):
// `--scope` defaults to `global` (review_mode.go's own flag default), so
// naming the bare `gentle-ai review mode disable` would let a reader
// silently disable receipt-driven development for every repository on the
// machine instead of just the one they meant. Verified by execution: the
// bare form writes ~/.gentle-ai/state.json; this scoped form writes only
// under the named repository's own .git/gentle-ai directory.
const reviewModeDisableCloneCommand = "gentle-ai review mode disable --scope clone --cwd <repo>"

// reviewModeDisableCloneCaveat is appended everywhere
// reviewModeDisableCloneCommand is named, so a reader of just one narration
// statement -- these are read independently, on stderr, one per stop -- still
// learns that dropping --scope changes the blast radius from "this
// repository" to "every repository on this machine".
const reviewModeDisableCloneCaveat = "(omitting --scope disables it for every repository on the machine)"

var reviewStopReasonNarration = map[string]string{
	"captured_verification_evidence_invalid": "The captured verification record or its immutable bytes failed integrity checks. " +
		"Ask a maintainer to inspect the review record before trusting that evidence, or run `" + reviewModeDisableCloneCommand + "` " +
		reviewModeDisableCloneCaveat + " to deliver under ordinary repository policy instead.",
	"captured_artifacts_unverifiable": "A previously captured review result failed verification, so this review cannot continue on its own. " +
		"Ask a maintainer to inspect the review record directly, or run `" + reviewModeDisableCloneCommand + "` " +
		reviewModeDisableCloneCaveat + " to deliver under ordinary repository policy instead.",
	"captured_result_selection_unavailable": "This run reached a state that should never happen: every review result it expected was already present. " +
		"This is a product defect, not something to retry. If you just want your work delivered, run `" + reviewModeDisableCloneCommand + "` " +
		reviewModeDisableCloneCaveat + " so ordinary repository policy (hooks, tests, CI) decides instead; nothing is silently approved. To get this review itself fixed, report the defect with this run's details.",
	"corrected_candidate_unavailable": "Change the candidate content so it differs from the frozen original, then re-run " +
		"`gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent " + reviewUndeclaredRuntimeIdentitySlot + " --next-transition` " +
		"(or `gentle-ai review finalize --lineage <id>`). " +
		"That is the right path when the review found real defects. If instead the reviewers were given the wrong input " +
		"and their findings describe content that was never the candidate, a maintainer can quarantine those results and " +
		"reopen their lenses over the same frozen content: run `gentle-ai review reopen-results --prepare --cwd <repo> --lineage <id> " +
		"--expected-revision <revision> --target <target> --reason <reason> --actor <actor> --quarantine-lens <lens>` " +
		"(repeat `--quarantine-lens` per affected lens) and follow its output.",
	"empty_base_diff_bootstrap_required": "This selected committed base has no changes to review. " +
		"If you are following the authorized first-publication bootstrap, a maintainer must first insert an empty root below the content commit. " +
		"Then run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent " + reviewUndeclaredRuntimeIdentitySlot + " --next-transition --base-ref <empty-root> --committed-only`.",
	"lens_context_budget_exceeded": "This frozen candidate cannot fit complete reviewer evidence without truncation, so this review stops before an inspection result. " +
		"Reduce the candidate scope or target identity, then run `gentle-ai review start` for that new candidate; or run `" + reviewModeDisableCloneCommand + "` " + reviewModeDisableCloneCaveat + " to deliver under ordinary repository policy instead.",
	"correction_repository_verification_failed": "Repository verification failed for this correction candidate. Change the candidate within the open correction, then re-run " +
		"`gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent " + reviewUndeclaredRuntimeIdentitySlot + " --next-transition` " +
		"to capture evidence for the new candidate.",
	"corrupted_or_unverifiable_authority": "This review's stored record cannot be trusted as-is, and it cannot be repaired automatically. " +
		"Ask a maintainer to inspect it directly, or run `" + reviewModeDisableCloneCommand + "` " +
		reviewModeDisableCloneCaveat + " to deliver under ordinary repository policy instead.",
	"final_verification_retry_unavailable": "This run reached a state that should never happen: it was routed to retry a final verification it was not eligible to retry. " +
		"This is a product defect, not something to retry. If you just want your work delivered, run `" + reviewModeDisableCloneCommand + "` " +
		reviewModeDisableCloneCaveat + " so ordinary repository policy (hooks, tests, CI) decides instead; nothing is silently approved. To get this review itself fixed, report the defect with this run's details.",
	"manual_intervention_required": "This review reached a state Gentle AI does not recognize. " +
		"This is a product defect. If you just want your work delivered, run `" + reviewModeDisableCloneCommand + "` " +
		reviewModeDisableCloneCaveat + " so ordinary repository policy (hooks, tests, CI) decides instead; nothing is silently approved. To get this review itself fixed, ask a maintainer to review it and report the defect.",
	"missing_authority_binding": "This run reached a state that should never happen: it lost track of the record it needs to continue. " +
		"This is a product defect, not something to retry. If you just want your work delivered, run `" + reviewModeDisableCloneCommand + "` " +
		reviewModeDisableCloneCaveat + " so ordinary repository policy (hooks, tests, CI) decides instead; nothing is silently approved. To get this review itself fixed, report the defect with this run's details.",
	"native_stop_required": "This review is stuck at an escalated state that is not yet eligible to continue. " +
		"Ask a maintainer to review it before doing anything else, or run `" + reviewModeDisableCloneCommand + "` " +
		reviewModeDisableCloneCaveat + " to deliver under ordinary repository policy instead.",
	"original_finalize_request_required": "Re-run `gentle-ai review finalize` with the exact same results or evidence you submitted before.",
	"recovery_scope_unchanged": "Change the candidate so it targets something different from what is already on record, then retry the recovery, " +
		"or run `" + reviewModeDisableCloneCommand + "` " + reviewModeDisableCloneCaveat + " to deliver under ordinary repository policy instead.",
	"rdd_disabled": "Review mode is disabled. Run `gentle-ai review mode status --cwd <repo> --json` to inspect the deciding scope; STATUS renders the exact scoped enable command for this request.",
	"staged_workspace_overlay_recovery_unavailable": "Pass `--lineage <id>` to continue the review you already started, " +
		"or drop `--workspace-overlay` and run `gentle-ai review start --projection staged` to start fresh.",
	"unchanged_or_unverified_authority": "This review already used its one correction attempt without a verified change. " +
		"`gentle-ai review start` on this exact unchanged candidate only resumes this same review, not a fresh one -- change the " +
		"candidate content first, then run `gentle-ai review start` to begin a genuinely new one, or run `" +
		reviewModeDisableCloneCommand + "` " + reviewModeDisableCloneCaveat + " to deliver under ordinary repository policy instead.",
}

// reviewConsentPromptNarration registers the one-time RDD consent prompt's
// vocabulary (review_mode.go) as Tier A: it already speaks confidently in
// domain terms and is the one ceremony the happy path shows. Values are the
// existing production constants themselves, not copies, so this registry can
// never drift from what actually prints.
var reviewConsentPromptNarration = map[string]string{
	"headline":          reviewConsentHeadline,
	"value":             reviewConsentValue,
	"answers":           reviewConsentAnswers,
	"off_path":          reviewConsentOffPath,
	"question":          reviewConsentQuestion,
	"medium_reason":     reviewConsentMediumReason,
	"skipped_notice":    reviewConsentSkippedNotice,
	"unreadable_notice": reviewConsentUnreadableNotice,
	"unknown_notice":    reviewConsentUnknownNotice,
	"declined_notice":   reviewConsentDeclinedNotice,
}

// reviewNarrationBannedVocabulary is the internal-architecture vocabulary
// spec "Three-Tier Narration Contract" bans from the human surface: "those
// identifiers appear only in negotiated/JSON envelopes, never on the human
// surface." Matched whole-word, case-insensitive, outside backtick spans.
var reviewNarrationBannedVocabulary = []string{"lineage", "ordinal", "cas", "facade", "receipt", "revision", "digest"}

// reviewNarrationBannedUncertaintyPhrases is the closed uncertainty/
// exploration phrasing ban from the same requirement.
var reviewNarrationBannedUncertaintyPhrases = []string{"i'll figure", "i don't know", "look into", "not sure", "try to"}

// reviewNarrationCodeSpanRegexp matches one backtick-delimited span.
var reviewNarrationCodeSpanRegexp = regexp.MustCompile("`[^`]*`")

// reviewNarrationStripCodeSpans removes every backtick-quoted command/flag
// literal before the vocabulary ban runs. A flag like `--lineage <id>` is an
// unavoidable, literal public CLI contract token a caller must type; the ban
// exists so narration never asks a human to understand gentle-ai's internal
// architecture in prose, not so a copy-pasteable command can never contain
// one of those words as its flag name.
func reviewNarrationStripCodeSpans(text string) string {
	return reviewNarrationCodeSpanRegexp.ReplaceAllString(text, " ")
}

// reviewNarrationWordRegexpCache avoids recompiling the same word-boundary
// regexp on every call across a large registry.
var reviewNarrationWordRegexpCache = map[string]*regexp.Regexp{}

func reviewNarrationContainsWord(lowered, word string) bool {
	pattern, ok := reviewNarrationWordRegexpCache[word]
	if !ok {
		pattern = regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
		reviewNarrationWordRegexpCache[word] = pattern
	}
	return pattern.MatchString(lowered)
}

// There is deliberately no stderr emission machinery here anymore: a
// successful negotiated operation is machine-readable end to end (gentle-pi
// fails closed on any stderr a successful native process writes). The
// registered statements above stay live two ways. The Tier C stop statements
// are contract data cross-validated against the live stop-code emitter in
// review_next_transition.go by review_narration_test.go (the bijection test).
// The Tier A consent prompt remains production-emitted through the
// interactive console ceremony in review_mode.go, proven reachable by
// TestNegotiatedStartUndeclaredInteractiveKeepsConsentCeremony.
