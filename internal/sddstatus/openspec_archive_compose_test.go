package sddstatus

import (
	"errors"
	"strings"
	"testing"
)

const composeCanonicalFixture = `# Widgets Specification

## Requirements

### Requirement: Unrelated Listing

The system MUST list widgets.

#### Scenario: List widgets

- GIVEN a widget store
- WHEN a client lists widgets
- THEN the system returns every widget

### Requirement: Widget Expiration

The system MUST expire widgets after 30 days.

#### Scenario: Expire after 30 days

- GIVEN a widget older than 30 days
- WHEN expiration runs
- THEN the widget is removed
`

// #4119: composition previously lived only in model Read/Edit instructions,
// which could drop unrelated requirements or skip a delta while reporting
// success. This pins the Go merge now backing the sdd-archive skill.
func TestComposeOpenSpecCanonicalSpecAppliesAddedAndModifiedPreservingUnrelated(t *testing.T) {
	delta := `## ADDED Requirements

### Requirement: Widget Tagging

The system MUST support tagging widgets.

#### Scenario: Tag a widget

- GIVEN a widget
- WHEN a client adds a tag
- THEN the tag is stored

## MODIFIED Requirements

### Requirement: Widget Expiration

The system MUST expire widgets after 90 days.
(Previously: 30 days)
`

	composed, err := ComposeOpenSpecCanonicalSpec(composeCanonicalFixture, delta)
	if err != nil {
		t.Fatalf("ComposeOpenSpecCanonicalSpec() error = %v", err)
	}

	// Dropped-requirement bug from #4119: the unrelated requirement must survive.
	if !strings.Contains(composed, "### Requirement: Unrelated Listing") || !strings.Contains(composed, "GIVEN a widget store") {
		t.Fatalf("composed spec dropped the unrelated requirement:\n%s", composed)
	}
	// Unapplied-delta bug from #4119: the MODIFIED delta must land.
	if !strings.Contains(composed, "expire widgets after 90 days") || strings.Contains(composed, "expire widgets after 30 days") {
		t.Fatalf("composed spec did not apply the MODIFIED delta:\n%s", composed)
	}
	if count := strings.Count(composed, "### Requirement:"); count != 3 {
		t.Fatalf("composed spec has %d requirements, want 3 (2 original + 1 added):\n%s", count, composed)
	}
}

func TestComposeOpenSpecCanonicalSpecAppliesRemovedAndRenamed(t *testing.T) {
	delta := `## REMOVED Requirements

### Requirement: Unrelated Listing

(Reason: no longer needed)

## RENAMED Requirements

### Requirement: Widget Expiration → Widget Retention

(Reason: clarify naming)
`
	composed, err := ComposeOpenSpecCanonicalSpec(composeCanonicalFixture, delta)
	if err != nil {
		t.Fatalf("ComposeOpenSpecCanonicalSpec() error = %v", err)
	}
	if strings.Contains(composed, "### Requirement: Unrelated Listing") {
		t.Fatalf("composed spec kept the removed requirement:\n%s", composed)
	}
	if !strings.Contains(composed, "### Requirement: Widget Retention") || strings.Contains(composed, "### Requirement: Widget Expiration") {
		t.Fatalf("composed spec did not apply the rename:\n%s", composed)
	}
}

// #4119: archive must refuse (typed error naming the section+requirement)
// rather than compose partially or report success.
func TestComposeOpenSpecCanonicalSpecRefusesUnapplicableDeltas(t *testing.T) {
	tests := []struct {
		name        string
		canonical   string
		delta       string
		wantSection string
		wantReq     string
	}{
		{
			name:        "MODIFIED references unknown requirement",
			canonical:   composeCanonicalFixture,
			delta:       "## MODIFIED Requirements\n\n### Requirement: Missing\n\nBody.\n",
			wantSection: "MODIFIED",
			wantReq:     "Missing",
		},
		{
			name:        "REMOVED without a Reason note",
			canonical:   composeCanonicalFixture,
			delta:       "## REMOVED Requirements\n\n### Requirement: Widget Expiration\n\nNo reason given.\n",
			wantSection: "REMOVED",
			wantReq:     "Widget Expiration",
		},
		{
			name:        "ADDED duplicates an existing requirement",
			canonical:   composeCanonicalFixture,
			delta:       "## ADDED Requirements\n\n### Requirement: Unrelated Listing\n\nDuplicate.\n",
			wantSection: "ADDED",
			wantReq:     "Unrelated Listing",
		},
		{
			name:        "RENAMED target name already exists",
			canonical:   composeCanonicalFixture,
			delta:       "## RENAMED Requirements\n\n### Requirement: Widget Expiration → Unrelated Listing\n\n(Reason: collide)\n",
			wantSection: "RENAMED",
			wantReq:     "Unrelated Listing",
		},
		{
			name:        "empty delta declares no sections",
			canonical:   composeCanonicalFixture,
			delta:       "No sections here.\n",
			wantSection: "DELTA",
		},
		{
			name:        "empty canonical has no requirements",
			canonical:   "# Widgets Specification\n\nNo requirements yet.\n",
			delta:       "## ADDED Requirements\n\n### Requirement: New One\n\nBody.\n",
			wantSection: "CANONICAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ComposeOpenSpecCanonicalSpec(tt.canonical, tt.delta)
			var unapplied *UnappliedDeltaError
			if !errors.As(err, &unapplied) {
				t.Fatalf("error = %v, want *UnappliedDeltaError", err)
			}
			if unapplied.Section != tt.wantSection || (tt.wantReq != "" && unapplied.Requirement != tt.wantReq) {
				t.Fatalf("unapplied = %+v, want Section=%q Requirement=%q", unapplied, tt.wantSection, tt.wantReq)
			}
		})
	}
}

// #4119 follow-up (R3, CRITICAL): a "## Section" sitting between two
// requirement blocks belonged to no block under the old preamble/
// requirements/trailing model and was silently dropped. This pins the
// segment model that preserves it byte-for-byte while still applying a
// MODIFIED delta to the requirement that follows it.
func TestComposeOpenSpecCanonicalSpecPreservesInterstitialSectionBetweenRequirements(t *testing.T) {
	const interstitial = "## Notes\n\nSome interstitial notes that are not a requirement.\n\n"
	canonical := "# Widgets Specification\n\n## Requirements\n\n" +
		"### Requirement: Req A\n\nOriginal req A sentinel.\n\n" +
		interstitial +
		"### Requirement: Req B\n\nOriginal req B sentinel.\n"
	delta := "## MODIFIED Requirements\n\n### Requirement: Req B\n\nReplaced req B sentinel.\n"

	composed, err := ComposeOpenSpecCanonicalSpec(canonical, delta)
	if err != nil {
		t.Fatalf("ComposeOpenSpecCanonicalSpec() error = %v", err)
	}
	if !strings.Contains(composed, interstitial) {
		t.Fatalf("composed spec dropped the interstitial section:\n%s", composed)
	}
	if !strings.Contains(composed, "Original req A sentinel.") {
		t.Fatalf("composed spec altered the unrelated requirement:\n%s", composed)
	}
	if !strings.Contains(composed, "Replaced req B sentinel.") {
		t.Fatalf("composed spec did not apply the MODIFIED delta after the interstitial section:\n%s", composed)
	}
	if strings.Contains(composed, "Original req B sentinel.") {
		t.Fatalf("composed spec kept the stale pre-delta requirement text:\n%s", composed)
	}
}

// #4119 follow-up (regression): the ADDED insertion anchor was recomputed as
// "last requirement index + 1", which becomes 0 (before the preamble) once
// REMOVED deletes the only requirement. A REMOVE-then-ADD on a
// single-requirement spec must still land the replacement after the
// preamble, not ahead of it.
func TestComposeOpenSpecCanonicalSpecReplacesTheOnlyRequirementKeepingPreambleFirst(t *testing.T) {
	const preamble = "# Widgets Specification\n\n## Requirements\n\n"
	canonical := preamble + "### Requirement: Only\n\nOriginal only body.\n"
	delta := "## REMOVED Requirements\n\n### Requirement: Only\n\n(Reason: replaced)\n\n" +
		"## ADDED Requirements\n\n### Requirement: Replacement\n\nReplacement body.\n"

	composed, err := ComposeOpenSpecCanonicalSpec(canonical, delta)
	if err != nil {
		t.Fatalf("ComposeOpenSpecCanonicalSpec() error = %v", err)
	}
	want := preamble + "### Requirement: Replacement\n\nReplacement body.\n"
	if composed != want {
		t.Fatalf("composed = %q, want %q (preamble first, replacement after it)", composed, want)
	}
}

// #4119 follow-up (regression): the emit loop wrote requirement segments
// verbatim, so a canonical whose final requirement lacked a trailing newline
// got an ADDED requirement concatenated onto its last line.
func TestComposeOpenSpecCanonicalSpecAddsMissingNewlineBeforeAddedRequirement(t *testing.T) {
	canonical := "## Requirements\n\n### Requirement: Only\n\nBody without trailing newline."
	delta := "## ADDED Requirements\n\n### Requirement: New One\n\nNew body.\n"

	composed, err := ComposeOpenSpecCanonicalSpec(canonical, delta)
	if err != nil {
		t.Fatalf("ComposeOpenSpecCanonicalSpec() error = %v", err)
	}
	if strings.Contains(composed, "trailing newline.### Requirement: New One") {
		t.Fatalf("new requirement was concatenated onto the previous requirement's last line:\n%s", composed)
	}
	if !strings.Contains(composed, "trailing newline.\n### Requirement: New One") {
		t.Fatalf("new requirement does not start on its own line:\n%s", composed)
	}
}

// #4119 follow-up: an empty-effect delta (a declared section with no actual
// requirement blocks under it) must never silently lose canonical bytes. The
// contract refuses rather than compose a no-op, so verify the refusal and
// that the canonical text itself was never touched to produce it.
func TestComposeOpenSpecCanonicalSpecRefusesEmptyEffectDeltaWithoutLosingBytes(t *testing.T) {
	composed, err := ComposeOpenSpecCanonicalSpec(composeCanonicalFixture, "## ADDED Requirements\n\nNo requirement heading follows.\n")
	var unapplied *UnappliedDeltaError
	if !errors.As(err, &unapplied) || unapplied.Section != "DELTA" {
		t.Fatalf("error = %v, want *UnappliedDeltaError{Section: DELTA}", err)
	}
	if composed != "" {
		t.Fatalf("refused compose returned non-empty output: %q", composed)
	}

	// The refusal must not have left any mutated/shared state behind: the
	// same canonical text must still compose correctly on a real delta,
	// with every original requirement intact in the returned document.
	recomposed, err := ComposeOpenSpecCanonicalSpec(composeCanonicalFixture, "## ADDED Requirements\n\n### Requirement: Widget Tagging\n\nBody.\n")
	if err != nil {
		t.Fatalf("ComposeOpenSpecCanonicalSpec() after a refusal error = %v", err)
	}
	if !strings.Contains(recomposed, "### Requirement: Unrelated Listing") || !strings.Contains(recomposed, "### Requirement: Widget Expiration") {
		t.Fatalf("canonical content lost after an earlier refusal:\n%s", recomposed)
	}
}

// #4119 follow-up (regression): a "## " line inside a fenced code block was
// still treated as a hard segment boundary, splitting a requirement body in
// two. MODIFIED then replaced only the truncated head, leaving the fenced
// sample's tail as an orphan segment with an unterminated fence.
func TestComposeOpenSpecCanonicalSpecKeepsFencedHeadingInsideRequirementBody(t *testing.T) {
	const fencedBlock = "```text\n## Not a heading\n```\n\n"
	canonical := "## Requirements\n\n" +
		"### Requirement: Req A\n\n" + fencedBlock + "More body text.\n\n" +
		"### Requirement: Req B\n\nBody B.\n"
	delta := "## MODIFIED Requirements\n\n### Requirement: Req A\n\nReplaced req A body.\n"

	composed, err := ComposeOpenSpecCanonicalSpec(canonical, delta)
	if err != nil {
		t.Fatalf("ComposeOpenSpecCanonicalSpec() error = %v", err)
	}
	if strings.Contains(composed, "## Not a heading") || strings.Contains(composed, "More body text.") {
		t.Fatalf("fenced content leaked past the MODIFIED replacement as an orphan tail:\n%s", composed)
	}
	if !strings.Contains(composed, "Replaced req A body.") {
		t.Fatalf("MODIFIED delta was not applied:\n%s", composed)
	}
	if !strings.Contains(composed, "### Requirement: Req B\n\nBody B.\n") {
		t.Fatalf("unrelated requirement B was altered or dropped:\n%s", composed)
	}
	if strings.Count(composed, "```")%2 != 0 {
		t.Fatalf("composed spec has an unterminated fence:\n%s", composed)
	}
}
