package filemerge

import (
	"testing"
)

func TestInjectMarkdownSection_EmptyFile(t *testing.T) {
	result := InjectMarkdownSection("", "sdd", "## SDD Config\nSome content here.\n")

	want := "<!-- gentle-ai:sdd -->\n## SDD Config\nSome content here.\n<!-- /gentle-ai:sdd -->\n"
	if result != want {
		t.Fatalf("empty file inject:\ngot:  %q\nwant: %q", result, want)
	}
}

func TestInjectMarkdownSection_AppendToExistingContent(t *testing.T) {
	existing := "# My Config\n\nSome existing content.\n"
	result := InjectMarkdownSection(existing, "persona", "You are a senior architect.\n")

	want := "# My Config\n\nSome existing content.\n\n<!-- gentle-ai:persona -->\nYou are a senior architect.\n<!-- /gentle-ai:persona -->\n"
	if result != want {
		t.Fatalf("append to existing:\ngot:  %q\nwant: %q", result, want)
	}
}

func TestInjectMarkdownSection_UpdateExistingSection(t *testing.T) {
	existing := "# Config\n\n<!-- gentle-ai:sdd -->\nOld SDD content.\n<!-- /gentle-ai:sdd -->\n\nOther stuff.\n"
	result := InjectMarkdownSection(existing, "sdd", "New SDD content.\n")

	want := "# Config\n\n<!-- gentle-ai:sdd -->\nNew SDD content.\n<!-- /gentle-ai:sdd -->\n\nOther stuff.\n"
	if result != want {
		t.Fatalf("update existing section:\ngot:  %q\nwant: %q", result, want)
	}
}

func TestInjectMarkdownSection_MultipleSectionsOnlyTargetedOneUpdated(t *testing.T) {
	existing := "# Config\n\n<!-- gentle-ai:persona -->\nPersona content.\n<!-- /gentle-ai:persona -->\n\n<!-- gentle-ai:sdd -->\nOld SDD.\n<!-- /gentle-ai:sdd -->\n\n<!-- gentle-ai:skills -->\nSkills content.\n<!-- /gentle-ai:skills -->\n"

	result := InjectMarkdownSection(existing, "sdd", "Updated SDD.\n")

	// persona and skills should be unchanged
	want := "# Config\n\n<!-- gentle-ai:persona -->\nPersona content.\n<!-- /gentle-ai:persona -->\n\n<!-- gentle-ai:sdd -->\nUpdated SDD.\n<!-- /gentle-ai:sdd -->\n\n<!-- gentle-ai:skills -->\nSkills content.\n<!-- /gentle-ai:skills -->\n"
	if result != want {
		t.Fatalf("multiple sections:\ngot:  %q\nwant: %q", result, want)
	}
}

func TestInjectMarkdownSection_PreserveUserContentBeforeAndAfter(t *testing.T) {
	existing := "# User's custom intro\n\nHand-written notes.\n\n<!-- gentle-ai:persona -->\nAuto persona.\n<!-- /gentle-ai:persona -->\n\n# User's custom footer\n\nMore hand-written content.\n"

	result := InjectMarkdownSection(existing, "persona", "Updated persona.\n")

	want := "# User's custom intro\n\nHand-written notes.\n\n<!-- gentle-ai:persona -->\nUpdated persona.\n<!-- /gentle-ai:persona -->\n\n# User's custom footer\n\nMore hand-written content.\n"
	if result != want {
		t.Fatalf("preserve user content:\ngot:  %q\nwant: %q", result, want)
	}
}

func TestInjectMarkdownSection_MalformedMarkersTreatedAsNotFound(t *testing.T) {
	// Only opening marker, no closing marker — treat as not found, append.
	existing := "# Config\n\n<!-- gentle-ai:sdd -->\nOrphaned content.\n"
	result := InjectMarkdownSection(existing, "sdd", "New SDD content.\n")

	// Should append since closing marker is missing.
	if result == existing {
		t.Fatalf("malformed markers: expected content to be appended, but got unchanged result")
	}

	// Result should contain the new properly-formed section.
	wantOpen := "<!-- gentle-ai:sdd -->\nNew SDD content.\n<!-- /gentle-ai:sdd -->\n"
	if !containsStr(result, wantOpen) {
		t.Fatalf("malformed markers: result should contain proper section:\ngot: %q", result)
	}
}

func TestInjectMarkdownSection_CloseBeforeOpenTreatedAsNotFound(t *testing.T) {
	// Closing marker appears before opening — treat as not found.
	existing := "<!-- /gentle-ai:sdd -->\nSome content.\n<!-- gentle-ai:sdd -->\n"
	result := InjectMarkdownSection(existing, "sdd", "New content.\n")

	// Should append the section, not replace.
	wantSuffix := "<!-- gentle-ai:sdd -->\nNew content.\n<!-- /gentle-ai:sdd -->\n"
	if !hasSuffix(result, wantSuffix) {
		t.Fatalf("close-before-open: expected appended section:\ngot: %q\nwant suffix: %q", result, wantSuffix)
	}
}

func TestInjectMarkdownSection_EmptyContentRemovesSection(t *testing.T) {
	existing := "# Config\n\n<!-- gentle-ai:sdd -->\nSDD content here.\n<!-- /gentle-ai:sdd -->\n\nOther stuff.\n"
	result := InjectMarkdownSection(existing, "sdd", "")

	want := "# Config\n\nOther stuff.\n"
	if result != want {
		t.Fatalf("empty content removes section:\ngot:  %q\nwant: %q", result, want)
	}
}

func TestInjectMarkdownSection_EmptyContentOnMissingSectionNoOp(t *testing.T) {
	existing := "# Config\n\nSome content.\n"
	result := InjectMarkdownSection(existing, "sdd", "")

	if result != existing {
		t.Fatalf("empty content on missing section should be no-op:\ngot:  %q\nwant: %q", result, existing)
	}
}

func TestInjectMarkdownSection_ContentWithoutTrailingNewline(t *testing.T) {
	result := InjectMarkdownSection("", "test", "no trailing newline")

	want := "<!-- gentle-ai:test -->\nno trailing newline\n<!-- /gentle-ai:test -->\n"
	if result != want {
		t.Fatalf("content without trailing newline:\ngot:  %q\nwant: %q", result, want)
	}
}

func TestInjectMarkdownSection_ExistingWithoutTrailingNewline(t *testing.T) {
	existing := "# Title"
	result := InjectMarkdownSection(existing, "test", "Content.\n")

	want := "# Title\n\n<!-- gentle-ai:test -->\nContent.\n<!-- /gentle-ai:test -->\n"
	if result != want {
		t.Fatalf("existing without trailing newline:\ngot:  %q\nwant: %q", result, want)
	}
}

// --- StripLegacyPersonaBlock tests ---

const legacyPersonaBlock = `## Rules

- NEVER add "Co-Authored-By" or any AI attribution to commits.

## Personality

Senior Architect, 15+ years experience, GDE & MVP.

## Language

- Spanish input → Rioplatense Spanish.

`

const gentleAiMarkerSection = `<!-- gentle-ai:persona -->
## Personality

Senior Architect, 15+ years experience, GDE & MVP.
<!-- /gentle-ai:persona -->
`

func TestStripLegacyPersonaBlock_NoFingerprintReturnsSame(t *testing.T) {
	input := "# My Config\n\nSome unrelated user content.\n"
	result := StripLegacyPersonaBlock(input)
	if result != input {
		t.Fatalf("no fingerprint: expected unchanged result:\ngot:  %q\nwant: %q", result, input)
	}
}

func TestStripLegacyPersonaBlock_FingerprintInsideMarkerReturnsSame(t *testing.T) {
	// Fingerprints only exist inside gentle-ai markers — should NOT be stripped.
	input := "# My Config\n\n" + gentleAiMarkerSection
	result := StripLegacyPersonaBlock(input)
	if result != input {
		t.Fatalf("fingerprint inside marker: expected unchanged result:\ngot:  %q\nwant: %q", result, input)
	}
}

func TestStripLegacyPersonaBlock_LegacyBlockOnlyReturnsEmpty(t *testing.T) {
	// File contains only the legacy persona block with no markers.
	result := StripLegacyPersonaBlock(legacyPersonaBlock)
	if result != "" {
		t.Fatalf("legacy-only: expected empty string:\ngot: %q", result)
	}
}

func TestStripLegacyPersonaBlock_LegacyBlockBeforeMarkersStripped(t *testing.T) {
	// Stale free-text persona block sits before a properly-marked section.
	input := legacyPersonaBlock + "\n" + gentleAiMarkerSection
	result := StripLegacyPersonaBlock(input)

	// The legacy block should be gone.
	if containsStr(result, "## Rules") {
		t.Fatal("stripped result should not contain legacy '## Rules' header")
	}
	// The marked section must survive.
	if !containsStr(result, "<!-- gentle-ai:persona -->") {
		t.Fatal("stripped result missing gentle-ai marker section")
	}
}

func TestStripLegacyPersonaBlock_MarkerSectionContentPreserved(t *testing.T) {
	// Markers and their content must be fully preserved after stripping.
	input := legacyPersonaBlock + "\n" + gentleAiMarkerSection + "\n# User Notes\n\nSome user text.\n"
	result := StripLegacyPersonaBlock(input)

	if !containsStr(result, "<!-- gentle-ai:persona -->") {
		t.Fatal("marker open not preserved")
	}
	if !containsStr(result, "<!-- /gentle-ai:persona -->") {
		t.Fatal("marker close not preserved")
	}
	if !containsStr(result, "# User Notes") {
		t.Fatal("user content after markers not preserved")
	}
}

func TestStripLegacyPersonaBlock_OnlyTwoOfThreeFingerprints(t *testing.T) {
	// File has "## Personality" and "Senior Architect" but NOT "## Rules" —
	// only two of three fingerprints, so it should NOT be stripped.
	input := "## Personality\n\nSenior Architect, 15+ years experience.\n\n" + gentleAiMarkerSection
	result := StripLegacyPersonaBlock(input)
	// With only 2/3 fingerprints, stripping should NOT occur.
	if result != input {
		t.Fatalf("partial fingerprint: expected unchanged result:\ngot:  %q\nwant: %q", result, input)
	}
}

func TestStripLegacyPersonaBlock_MixedZone_OnlyOneFingerprint_PreMarker(t *testing.T) {
	// Edge case: "## Rules" appears in user content before the first marker,
	// but the other two fingerprints ("## Personality" and "Senior Architect")
	// exist only inside a gentle-ai marker block.
	//
	// Old behaviour (bug): one fingerprint in the pre-marker zone was enough to
	// trigger stripping, destroying the user's "## Rules" section.
	// New behaviour (fixed): ALL fingerprints must appear in the pre-marker zone;
	// since only one does, the file is returned unchanged.
	userRulesSection := "## Rules\n\n- Never do X.\n- Always do Y.\n\n"
	markerWithOtherFingerprints := "<!-- gentle-ai:persona -->\n## Personality\n\nSenior Architect, 15+ years experience.\n<!-- /gentle-ai:persona -->\n"

	input := userRulesSection + markerWithOtherFingerprints
	result := StripLegacyPersonaBlock(input)

	if result != input {
		t.Fatalf(
			"mixed-zone edge case: only one fingerprint in pre-marker zone, expected unchanged result:\ngot:  %q\nwant: %q",
			result, input,
		)
	}
}

func TestStripLegacyPersonaBlock_MixedZone_TwoFingerprints_PreMarker(t *testing.T) {
	// Two of the three fingerprints appear before the first marker, but only the
	// third ("## Rules") exists inside the marker block. Stripping must NOT fire
	// because not all fingerprints are in the pre-marker zone.
	preMarker := "## Personality\n\nSenior Architect, 15+ years experience.\n\n"
	markerWithRule := "<!-- gentle-ai:persona -->\n## Rules\n\n- Rule inside marker.\n<!-- /gentle-ai:persona -->\n"

	input := preMarker + markerWithRule
	result := StripLegacyPersonaBlock(input)

	if result != input {
		t.Fatalf(
			"mixed-zone (2 of 3 in pre-marker): expected unchanged result:\ngot:  %q\nwant: %q",
			result, input,
		)
	}
}

func TestStripLegacyPersonaBlock_AllFingerprintsPreMarker_Strips(t *testing.T) {
	// Positive case: ALL three fingerprints appear before the first marker.
	// Stripping MUST fire, removing the pre-marker legacy block.
	preMarker := "## Rules\n\n- Some rule.\n\n## Personality\n\nSenior Architect, veteran.\n\n"
	markerSection := "<!-- gentle-ai:persona -->\nUpdated persona.\n<!-- /gentle-ai:persona -->\n"

	input := preMarker + markerSection
	result := StripLegacyPersonaBlock(input)

	if result == input {
		t.Fatal("all-fingerprints-pre-marker: expected stripping to occur, but got unchanged result")
	}
	if containsStr(result, "## Rules") {
		t.Fatal("all-fingerprints-pre-marker: legacy '## Rules' should have been stripped")
	}
	if !containsStr(result, "<!-- gentle-ai:persona -->") {
		t.Fatal("all-fingerprints-pre-marker: marker section must be preserved")
	}
}

func TestStripLegacyPersonaBlock_EmptyFileReturnsSame(t *testing.T) {
	result := StripLegacyPersonaBlock("")
	if result != "" {
		t.Fatalf("empty file: expected empty result, got %q", result)
	}
}

func TestStripLegacyPersonaBlock_UserContentBeforeAndAfterMarkersPreserved(t *testing.T) {
	// User has hand-written notes before the legacy block — these should survive
	// IF they are not part of the legacy block.  Since the legacy detection works
	// by looking for fingerprints before the first marker, user content that
	// predates the legacy block would also be stripped.  This is an accepted
	// tradeoff documented in the function comment.
	input := legacyPersonaBlock + "\n" + gentleAiMarkerSection + "\n# Custom section\n\nUser stuff.\n"
	result := StripLegacyPersonaBlock(input)

	if !containsStr(result, "# Custom section") {
		t.Fatal("content after gentle-ai markers must be preserved")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
