package triage

import (
	"context"
	"sort"
	"strings"
)

type Outcome string

const (
	OutcomeCurrentEvidence      Outcome = "current-evidence"
	OutcomeRetestRequested      Outcome = "retest-requested"
	OutcomeRelatedChangeFound   Outcome = "related-change-found"
	OutcomeInsufficientEvidence Outcome = "insufficient-evidence"
)

type Origin string

const (
	OriginManual       Origin = "manual"
	OriginAIAssisted   Origin = "AI-assisted"
	OriginGentleReport Origin = "gentle-report"
	OriginUnknown      Origin = "unknown"
)

type Provenance string

const (
	ProvenanceOfficial    Provenance = "official"
	ProvenanceCustomDirty Provenance = "custom-dirty"
	ProvenanceUnknown     Provenance = "unknown"

	AttributionObserved  = "observed"
	AttributionAbstained = "abstained"
)

type OriginEvidence struct {
	Origin Origin
	Kind   string
	URL    string
}

type IssueEvidence struct{ Number int }

type ProvenanceObservation struct{ Kind Provenance }

type ProvenanceProvider interface {
	Observe(context.Context, IssueEvidence) ProvenanceObservation
}

type Fixture struct {
	Name               string
	Number             int
	Official           bool
	CustomDirty        bool
	Origin             OriginEvidence
	ExpectedOrigin     Origin
	ExpectedKnown      bool
	ExpectedAbstention bool
}

type Record struct {
	Number      int
	Cohort      string
	Outcome     Outcome
	Origin      OriginEvidence
	Attribution string
	Partial     []string
}

type Ratio struct {
	Numerator   int
	Denominator int
	Value       *float64
}

type Metrics struct {
	FalseConfirmation Ratio
	FalseAttribution  Ratio
	Abstention        Ratio
}

type Bounds struct {
	IssuesPerCohort     int
	CommentsPerIssue    int
	TimelinePerIssue    int
	CandidatesPerSource int
	Requests            int
	PagesPerEndpoint    int
	JSONBytes           int
	MarkdownBytes       int
}

type Issue struct {
	Number int
	Open   bool
	Labels []string
}

type Cohorts struct {
	Review           []Issue
	MissingLifecycle []Issue
	Partial          []string
}

func DefaultBounds() Bounds {
	return Bounds{5, 10, 20, 5, 60, 2, 256 * 1024, 64 * 1024}
}

func Classify(fixture Fixture, provider ProvenanceProvider) Record {
	observation := ProvenanceObservation{Kind: ProvenanceUnknown}
	if provider != nil {
		observation = provider.Observe(context.Background(), IssueEvidence{Number: fixture.Number})
	}
	record := Record{Number: fixture.Number, Origin: validOrigin(fixture.Origin), Attribution: AttributionObserved}
	if observation.Kind != ProvenanceOfficial && !fixture.Official {
		record.Outcome = OutcomeInsufficientEvidence
		record.Attribution = AttributionAbstained
		return record
	}
	record.Outcome = OutcomeCurrentEvidence
	return record
}

func ComputeMetrics(fixtures []Fixture, records []Record) Metrics {
	byNumber := make(map[int]Record, len(records))
	for _, record := range records {
		byNumber[record.Number] = record
	}
	var falseConfirmation, falseAttribution, abstention Ratio
	for _, fixture := range fixtures {
		record := byNumber[fixture.Number]
		if fixture.ExpectedAbstention {
			falseConfirmation.Denominator++
			abstention.Denominator++
			if record.Outcome != OutcomeInsufficientEvidence {
				falseConfirmation.Numerator++
			}
			if record.Attribution == AttributionAbstained {
				abstention.Numerator++
			}
		}
		if fixture.ExpectedKnown {
			falseAttribution.Denominator++
			if record.Origin.Origin != fixture.ExpectedOrigin {
				falseAttribution.Numerator++
			}
		}
	}
	falseConfirmation.Value = ratioValue(falseConfirmation)
	falseAttribution.Value = ratioValue(falseAttribution)
	abstention.Value = ratioValue(abstention)
	return Metrics{falseConfirmation, falseAttribution, abstention}
}

func StableRecords(records []Record) []Record {
	ordered := append([]Record(nil), records...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Number < ordered[j].Number || ordered[i].Number == ordered[j].Number && ordered[i].Cohort < ordered[j].Cohort
	})
	return ordered
}

func ClassifyCohorts(issues []Issue, taxonomy []string, available bool) Cohorts {
	cohorts := Cohorts{}
	if !available {
		cohorts.Partial = []string{"taxonomy-unavailable"}
	}
	for _, issue := range issues {
		if !issue.Open {
			continue
		}
		review, lifecycle := false, false
		for _, label := range issue.Labels {
			if label == "status:needs-review" {
				review = true
			}
			for _, known := range taxonomy {
				if label == known && strings.HasPrefix(known, "status:") {
					lifecycle = true
				}
			}
		}
		if review {
			cohorts.Review = append(cohorts.Review, issue)
		}
		if available && !lifecycle {
			cohorts.MissingLifecycle = append(cohorts.MissingLifecycle, issue)
		}
	}
	sort.Slice(cohorts.Review, func(i, j int) bool { return cohorts.Review[i].Number < cohorts.Review[j].Number })
	sort.Slice(cohorts.MissingLifecycle, func(i, j int) bool { return cohorts.MissingLifecycle[i].Number < cohorts.MissingLifecycle[j].Number })
	return cohorts
}

func IsBug(issue Issue) bool {
	for _, label := range issue.Labels {
		if label == "bug" || label == "type:bug" {
			return true
		}
	}
	return false
}

func ratioValue(ratio Ratio) *float64 {
	if ratio.Denominator == 0 {
		return nil
	}
	value := float64(ratio.Numerator) / float64(ratio.Denominator)
	return &value
}

func validOrigin(origin OriginEvidence) OriginEvidence {
	if origin.Origin == OriginManual || origin.Origin == OriginAIAssisted || origin.Origin == OriginGentleReport {
		if origin.Kind != "" && strings.HasPrefix(origin.URL, "https://") {
			return origin
		}
	}
	return OriginEvidence{Origin: OriginUnknown}
}
