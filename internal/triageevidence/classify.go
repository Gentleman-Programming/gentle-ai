package triageevidence

import (
	"strconv"
	"strings"
)

// Outcome is the provisional, evidence-oriented disposition a report assigns to
// one issue. Outcomes are report data: they are never applied as GitHub labels,
// never decide a close, and never assign product responsibility.
type Outcome string

const (
	// OutcomeRelatedChangeFound: a released change or in-flight PR already
	// touches this report's area; the maintainer should evaluate the change
	// before re-testing the report.
	OutcomeRelatedChangeFound Outcome = "related-change-found"
	// OutcomeRetestRequested: the report names a version older than the current
	// release (or says an upgrade fixed it); retesting on current is the
	// evidence-gathering step, not a conclusion.
	OutcomeRetestRequested Outcome = "retest-requested"
	// OutcomeCurrentEvidence: the report names the current release or an
	// unreleased main build and carries reproduction evidence; this is a live
	// candidate, not a proof.
	OutcomeCurrentEvidence Outcome = "current-evidence"
	// OutcomeInsufficientEvidence: the report cannot meet the evidence
	// threshold (no parseable version, no reproduction evidence, no related
	// change); the report says what is missing. Absence of evidence is never
	// treated as absence of a bug.
	OutcomeInsufficientEvidence Outcome = "insufficient-evidence"
)

// Verdict is the deterministic classification of one issue. Reasons are
// human-readable but stable; Support holds the exact URLs backing the verdict
// (empty means "no supporting URL found", which the report renders as
// evidence unavailable, never as proof).
type Verdict struct {
	Outcome Outcome
	Reasons []string
	Support []string
}

func (v Verdict) addReason(r string) Verdict { v.Reasons = append(v.Reasons, r); return v }
func (v Verdict) addSupport(u string) Verdict {
	if u != "" {
		v.Support = append(v.Support, u)
	}
	return v
}

// ClassificationInput is everything Classify needs. The caller guarantees the
// Related slice is already bounded and each entry's URL is a real URL.
type ClassificationInput struct {
	Issue        Issue
	Evidence     Evidence
	LatestStable ReportedVersion // parsed newest stable release; IsVersion()==false when the repo has none
	Related      []RelatedChange
}

// Classify returns exactly one Outcome per issue, chosen by fixed precedence:
// related-change-found > retest-requested > current-evidence >
// insufficient-evidence. The same input always produces the same verdict, and
// no outcome can ever recommend closing, labeling, editing, or commenting.
func Classify(in ClassificationInput) Verdict {
	issue := in.Issue
	ev := in.Evidence

	// 1. A corroborated change already exists (released or in flight) and is
	// related to this report's area or explicitly references it.
	for _, rel := range in.Related {
		if rel.IsReleasedChange() {
			v := Verdict{Outcome: OutcomeRelatedChangeFound}
			label := describeRelated(rel)
			v = v.addReason(label + " already landed; evaluate it before re-testing this report")
			v = v.addSupport(rel.URL)
			if issue.MatchesReference(rel.Number) {
				v = v.addReason("the report explicitly references " + linkRef(issue.Number, rel.Number))
			}
			return v
		}
	}
	for _, rel := range in.Related {
		if rel.IsInFlight() && rel.Kind == RelatedPR {
			return Verdict{Outcome: OutcomeRelatedChangeFound}.
				addReason(describeRelated(rel) + " is open; the report is under active change").
				addSupport(rel.URL)
		}
	}

	// 2. The report names a version but it is not the current one: retest on
	// current before drawing any conclusion. "Upgrading fixed it" is evidence
	// for retesting, never a reason to close.
	if v, ok := classifyRetest(in); ok {
		return v
	}

	// 3. Current version (or unreleased main) with reproduction evidence.
	if v, ok := classifyCurrent(in); ok {
		return v
	}

	// 4. Fallback: state precisely what is missing.
	reasons := []string{}
	if !ev.Version.IsVersion() && ev.Version.Channel != ChannelMain {
		reasons = append(reasons, "no parseable version: the report cannot be matched to a release")
	}
	if !ev.HasReproductionEvidence() {
		reasons = append(reasons, "no reproduction evidence: no detailed steps, logs/commands, or reproducibility claim")
	}
	if len(in.Related) == 0 {
		reasons = append(reasons, "no related issue, PR, release, or known change found in the bounded search")
	}
	return Verdict{Outcome: OutcomeInsufficientEvidence, Reasons: reasons}
}

func classifyRetest(in ClassificationInput) (Verdict, bool) {
	ev := in.Evidence
	if !ev.Version.IsVersion() {
		return Verdict{}, false
	}
	body := strings.ToLower(in.Issue.Body)
	upgradeClaim := strings.Contains(body, "upgrading fixed") ||
		strings.Contains(body, "fixed in newer") ||
		strings.Contains(body, "works on the latest") ||
		strings.Contains(body, "works in newer")

	v := Verdict{}
	switch {
	case in.LatestStable.IsVersion() && ev.Version.Compare(in.LatestStable) < 0:
		v = Verdict{Outcome: OutcomeRetestRequested}.
			addReason("reports " + ev.Version.Raw + " (older than current stable " + in.LatestStable.Raw + "); retest on current to gather evidence").
			addReason("an old reported version is not evidence of a current defect")
		return v, true
	case upgradeClaim:
		v = Verdict{Outcome: OutcomeRetestRequested}.
			addReason("the reporter says an upgrade changed the behavior; retest on the current release").
			addReason("'upgrading fixed it' is evidence for retesting, not a reason to close")
		return v, true
	}
	return v, false
}

func classifyCurrent(in ClassificationInput) (Verdict, bool) {
	ev := in.Evidence
	if !ev.HasReproductionEvidence() {
		return Verdict{}, false
	}
	switch ev.Version.Channel {
	case ChannelMain:
		return Verdict{Outcome: OutcomeCurrentEvidence}.
			addReason("reports an unreleased main build with reproduction evidence; treat as current, not released").
			addReason("version channel: main"), true
	case ChannelStable, ChannelPrerelease:
		if !in.LatestStable.IsVersion() {
			return Verdict{Outcome: OutcomeCurrentEvidence}.
				addReason("reports " + ev.Version.Raw + " with reproduction evidence; current channel could not be established (no stable release found)").
				addReason("a missing latest-release fact is reported, never assumed"), true
		}
		cmp := ev.Version.Compare(in.LatestStable)
		switch {
		case cmp == 0:
			return Verdict{Outcome: OutcomeCurrentEvidence}.
				addReason("reports the current stable " + ev.Version.Raw + " with reproduction evidence"), true
		case cmp > 0:
			return Verdict{Outcome: OutcomeCurrentEvidence}.
				addReason("reports " + ev.Version.Raw + " (newer than current stable " + in.LatestStable.Raw + "); verify it resolves to a real release"), true
		}
	}
	return Verdict{}, false
}

func describeRelated(r RelatedChange) string {
	if r.Title != "" {
		return string(r.Kind) + " " + r.Title
	}
	return "related " + string(r.Kind)
}

func linkRef(issueNumber, relatedNumber int) string {
	if relatedNumber == 0 {
		return "#" + strconv.Itoa(issueNumber)
	}
	return "#" + strconv.Itoa(relatedNumber)
}
