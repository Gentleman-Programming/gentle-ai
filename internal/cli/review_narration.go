package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

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
var reviewStopReasonNarration = map[string]string{
	"captured_artifacts_unverifiable": "A previously captured review result failed verification, so this review cannot continue on its own. " +
		"Ask a maintainer to inspect the review record directly.",
	"captured_result_selection_unavailable": "This run reached a state that should never happen: every review result it expected was already present. " +
		"This is a defect; there is nothing more to do from here.",
	"corrected_candidate_unavailable": "Change the candidate content so it differs from the frozen original, then re-run " +
		"`gentle-ai review status --next-transition` (or `review finalize`). " +
		"That is the right path when the review found real defects. If instead the reviewers were given the wrong input " +
		"and their findings describe content that was never the candidate, a maintainer can quarantine those results and " +
		"reopen their lenses over the same frozen content: run `gentle-ai review reopen-results --prepare --quarantine-lens <lens>` " +
		"(repeat `--quarantine-lens` per affected lens) and follow its output.",
	"corrupted_or_unverifiable_authority": "This review's stored record cannot be trusted as-is, and it cannot be repaired automatically. " +
		"Ask a maintainer to inspect it directly.",
	"final_verification_retry_unavailable": "This run reached a state that should never happen: it was routed to retry a final verification it was not eligible to retry. " +
		"This is a defect; there is nothing more to do from here.",
	"manual_intervention_required": "This review reached a state Gentle AI does not recognize. " +
		"This is a defect; there is nothing more to do from here.",
	"missing_authority_binding": "This run reached a state that should never happen: it lost track of the record it needs to continue. " +
		"This is a defect; there is nothing more to do from here.",
	"native_stop_required": "This review is stuck at an escalated state that is not yet eligible to continue. " +
		"Ask a maintainer to review it before doing anything else.",
	"original_finalize_request_required": "Re-run `gentle-ai review finalize` with the exact same results or evidence you submitted before.",
	"pre_pr_selector_unrepresentable":    "Pass a branch or tag name to `--base-ref` instead of a raw commit id when selecting the pre-pr gate.",
	"recovery_scope_unchanged":           "Change the candidate so it targets something different from what is already on record, then retry the recovery.",
	"recovery_target_unrepresentable": "Use one of the supported ways to select what to recover: no base selector for current changes, " +
		"`--base-ref <ref> --committed-only` for a base diff, or `--workspace-overlay --base-ref <ref>` for a workspace overlay.",
	"staged_workspace_overlay_recovery_unavailable": "Pass `--lineage <id>` to continue the review you already started, " +
		"or drop `--workspace-overlay` and run `gentle-ai review start --projection staged` to start fresh.",
	"unchanged_or_unverified_authority": "This review already used its one correction attempt without a verified change. Start a new review to continue.",
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

// reviewNarrationOutput is the human-surface stream Tier C statements print
// to. It is a var, like reviewConsole, so tests can capture it without a
// real terminal; production always writes to stderr, never stdout, because
// stdout carries the machine-readable JSON envelope and a narration line
// mixed into it would corrupt every caller that parses it.
var reviewNarrationOutput io.Writer = os.Stderr

// reviewStopReasonStatement looks up the registered Tier C statement for a
// review_next_transition.go stop reason code. The second result is false for
// a code with no registry entry — TestReviewNarrationRegistryCoversEveryStopReasonCode
// is what keeps that from happening for any code the source actually emits.
func reviewStopReasonStatement(reason string) (string, bool) {
	emission, ok := reviewNarrationRegistry["stop:"+strings.TrimSpace(reason)]
	if !ok {
		return "", false
	}
	return emission.Statement, true
}

// reviewNarrateStopReason writes exactly one Tier C statement to the human
// surface for a stop-kind next transition. It never touches stdout, so the
// negotiated JSON envelope (the machine surface) is byte-for-byte unchanged;
// it is the additive half of spec "Three-Tier Narration Contract"'s "Tier C
// terminal state emits exactly one statement" scenario. A reason with no
// registry entry prints nothing: TestReviewNarrationRegistryCoversEveryStopReasonCode
// is the fail-closed proof that should never happen for a code the source emits.
func reviewNarrateStopReason(reason string) {
	statement, ok := reviewStopReasonStatement(reason)
	if !ok {
		return
	}
	_, _ = fmt.Fprintln(reviewNarrationOutput, statement)
}
