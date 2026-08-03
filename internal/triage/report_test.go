package triage

import (
	"context"
	"testing"
)

type fixtureProvider struct{ observation ProvenanceObservation }

func (p fixtureProvider) Observe(context.Context, IssueEvidence) ProvenanceObservation {
	return p.observation
}

func TestClassifyProvenanceFixturesAndMetrics(t *testing.T) {
	t.Parallel()

	fixtures := []Fixture{
		{Name: "official reproduction", Number: 1, Origin: OriginEvidence{Origin: OriginManual, Kind: "event", URL: "https://example.com/1"}, ExpectedOrigin: OriginManual, ExpectedKnown: true},
		{Name: "custom dirty only", Number: 2, CustomDirty: true, ExpectedAbstention: true},
		{Name: "same defect official and custom", Number: 3, Official: true, CustomDirty: true, Origin: OriginEvidence{Origin: OriginAIAssisted, Kind: "event", URL: "https://example.com/3"}, ExpectedOrigin: OriginAIAssisted, ExpectedKnown: true},
		{Name: "unknown provenance", Number: 4, ExpectedAbstention: true},
	}
	providers := []ProvenanceProvider{
		fixtureProvider{ProvenanceObservation{Kind: ProvenanceOfficial}},
		fixtureProvider{ProvenanceObservation{Kind: ProvenanceCustomDirty}},
		fixtureProvider{ProvenanceObservation{Kind: ProvenanceOfficial}},
		fixtureProvider{ProvenanceObservation{Kind: ProvenanceUnknown}},
	}
	wantOutcomes := []Outcome{OutcomeCurrentEvidence, OutcomeInsufficientEvidence, OutcomeCurrentEvidence, OutcomeInsufficientEvidence}

	records := make([]Record, 0, len(fixtures))
	for i, fixture := range fixtures {
		record := Classify(fixture, providers[i])
		if record.Outcome != wantOutcomes[i] {
			t.Errorf("%s outcome = %q, want %q", fixture.Name, record.Outcome, wantOutcomes[i])
		}
		if i == 1 || i == 3 {
			if record.Attribution != AttributionAbstained {
				t.Errorf("%s attribution = %q, want abstained when #1884 is not official", fixture.Name, record.Attribution)
			}
		} else if record.Origin.Origin != fixture.ExpectedOrigin {
			t.Errorf("%s origin = %q, want %q", fixture.Name, record.Origin.Origin, fixture.ExpectedOrigin)
		}
		records = append(records, record)
	}

	metrics := ComputeMetrics(fixtures, records)
	assertRatio(t, "false confirmation", metrics.FalseConfirmation, 0, 2, 0)
	assertRatio(t, "false attribution", metrics.FalseAttribution, 0, 2, 0)
	assertRatio(t, "abstention", metrics.Abstention, 2, 2, 1)
}

func TestBoundsAndStableRecords(t *testing.T) {
	t.Parallel()

	bounds := DefaultBounds()
	if bounds.IssuesPerCohort != 5 || bounds.Requests != 60 || bounds.JSONBytes != 256*1024 {
		t.Fatalf("DefaultBounds() = %+v, want approved maxima", bounds)
	}
	records := StableRecords([]Record{{Number: 9, Cohort: "b"}, {Number: 2}, {Number: 9, Cohort: "a"}})
	if records[0].Number != 2 || records[1].Cohort != "a" || records[2].Cohort != "b" {
		t.Fatalf("StableRecords() = %+v, want number-ordered deterministic records", records)
	}
}

func TestClassifyCohortsSeparatesLifecycleLabels(t *testing.T) {
	t.Parallel()

	issues := []Issue{
		{Number: 9, Open: true, Labels: []string{"bug", "status:needs-review"}},
		{Number: 8, Open: true, Labels: []string{"type:bug", "priority:high"}},
		{Number: 7, Open: true, Labels: []string{"status:approved", "type:bug"}},
		{Number: 2234, Open: true, Labels: []string{"status:needs-review"}},
		{Number: 9100, Open: true, Labels: []string{"priority:high"}},
		{Number: 9101, Open: true, Labels: []string{"bug"}},
		{Number: 6, Open: false, Labels: []string{"status:needs-review"}},
	}

	cohorts := ClassifyCohorts(issues, []string{"status:needs-review", "status:approved"}, true)
	if got := issueNumbers(cohorts.Review); !equalNumbers(got, []int{9, 2234}) {
		t.Errorf("review cohort = %v, want [9 2234]", got)
	}
	if got := issueNumbers(cohorts.MissingLifecycle); !equalNumbers(got, []int{8, 9100, 9101}) {
		t.Errorf("missing lifecycle cohort = %v, want [8 9100 9101] with unrelated labels retained", got)
	}
	if !IsBug(issues[0]) || !IsBug(issues[1]) || IsBug(issues[3]) {
		t.Error("bug and type:bug must classify equivalently without treating unrelated labels as bugs")
	}

	unavailable := ClassifyCohorts(issues, nil, false)
	if got := issueNumbers(unavailable.Review); !equalNumbers(got, []int{9, 2234}) {
		t.Errorf("review cohort with unavailable taxonomy = %v, want independent [9 2234]", got)
	}
	if len(unavailable.MissingLifecycle) != 0 || !hasMarker(unavailable.Partial, "taxonomy-unavailable") {
		t.Errorf("unavailable taxonomy = %+v, want explicit unavailable marker", unavailable)
	}
}

func assertRatio(t *testing.T, name string, ratio Ratio, numerator, denominator int, value float64) {
	t.Helper()
	if ratio.Numerator != numerator || ratio.Denominator != denominator || ratio.Value == nil || *ratio.Value != value {
		t.Errorf("%s = %+v, want %d/%d = %v", name, ratio, numerator, denominator, value)
	}
}

func issueNumbers(issues []Issue) []int {
	numbers := make([]int, len(issues))
	for i, issue := range issues {
		numbers[i] = issue.Number
	}
	return numbers
}

func equalNumbers(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasMarker(markers []string, want string) bool {
	for _, marker := range markers {
		if marker == want {
			return true
		}
	}
	return false
}
