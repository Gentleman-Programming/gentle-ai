package filemerge

import (
	"strings"
)

const (
	markerPrefix = "<!-- gentle-ai:"
	markerSuffix = " -->"
	closePrefix  = "<!-- /gentle-ai:"
)

// legacyPersonaFingerprints are substrings that appear in the Gentleman persona
// asset and reliably identify a stale free-text block written by an old installer
// (or manually copied) before the marker-based injection system was in use.
// All fingerprints must be present for the block to be considered a match.
var legacyPersonaFingerprints = []string{
	"## Personality",
	"Senior Architect",
	"## Rules",
}

// StripLegacyPersonaBlock removes a free-text Gentleman persona block that was
// written to a markdown file outside of <!-- gentle-ai: --> markers.
//
// It is safe to call on any file: if no legacy block is detected, the original
// content is returned unchanged. Stripping requires ALL fingerprints to be
// present in the pre-marker zone (the region before the first
// <!-- gentle-ai: --> marker). A fingerprint that exists only inside a marker
// section is ignored — this prevents false positives when a user's own section
// headers happen to match one or two of the fingerprint strings while the
// remaining fingerprints live inside a managed marker block.
func StripLegacyPersonaBlock(content string) string {
	// Quick check: all fingerprints must be present somewhere in the file.
	for _, fp := range legacyPersonaFingerprints {
		if !strings.Contains(content, fp) {
			return content
		}
	}

	// Find the position of the first marker — everything before it is the
	// potential legacy zone. If there are no markers, the whole file is the
	// legacy zone.
	firstMarkerIdx := strings.Index(content, markerPrefix)

	// Determine the candidate zone to inspect.
	zone := content
	if firstMarkerIdx >= 0 {
		zone = content[:firstMarkerIdx]
	}

	// Verify that ALL fingerprints live in the pre-marker zone.
	// Requiring every fingerprint to appear inside the zone prevents a false
	// positive where, for example, "## Rules" is a legitimate user section
	// header before the first marker while the other two fingerprints exist
	// only inside a marker block. Matching on just one fingerprint would
	// incorrectly trigger stripping and destroy user content.
	for _, fp := range legacyPersonaFingerprints {
		if !strings.Contains(zone, fp) {
			return content
		}
	}

	// Strip the legacy zone: remove it entirely and keep the marker content.
	if firstMarkerIdx < 0 {
		// No markers at all — the entire file is legacy persona content.
		// Return empty string so the caller can write a fresh section.
		return ""
	}

	// Keep everything from the first marker onwards.
	remainder := content[firstMarkerIdx:]
	// Trim any leading blank lines between the stripped block and the first marker.
	remainder = strings.TrimLeft(remainder, "\n")
	return remainder
}

// openMarker returns the opening marker for a section ID.
func openMarker(sectionID string) string {
	return markerPrefix + sectionID + markerSuffix
}

// closeMarker returns the closing marker for a section ID.
func closeMarker(sectionID string) string {
	return closePrefix + sectionID + markerSuffix
}

// InjectMarkdownSection replaces or appends a marked section in a markdown file.
// Markers use HTML comments: <!-- gentle-ai:SECTION_ID --> ... <!-- /gentle-ai:SECTION_ID -->
// If the section already exists, its content is replaced.
// If it doesn't exist, it's appended at the end.
// Content outside markers is never touched.
// If content is empty, the section (including markers) is removed.
func InjectMarkdownSection(existing, sectionID, content string) string {
	open := openMarker(sectionID)
	close := closeMarker(sectionID)

	openIdx := strings.Index(existing, open)
	closeIdx := strings.Index(existing, close)

	// If both markers are found and in the correct order, replace the section.
	if openIdx >= 0 && closeIdx >= 0 && closeIdx > openIdx {
		// If content is empty, remove the entire section including markers.
		if content == "" {
			before := existing[:openIdx]
			after := existing[closeIdx+len(close):]

			// Clean up trailing newline after close marker.
			if len(after) > 0 && after[0] == '\n' {
				after = after[1:]
			}
			// Clean up trailing newline before open marker.
			result := strings.TrimRight(before, "\n")
			if after != "" {
				if result != "" {
					result += "\n"
				}
				result += after
			} else if result != "" {
				result += "\n"
			}
			return result
		}

		before := existing[:openIdx]
		after := existing[closeIdx+len(close):]

		var sb strings.Builder
		sb.WriteString(before)
		sb.WriteString(open)
		sb.WriteString("\n")
		sb.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString(close)
		sb.WriteString(after)
		return sb.String()
	}

	// If content is empty and section doesn't exist, return existing unchanged.
	if content == "" {
		return existing
	}

	// Section not found — append at end.
	var sb strings.Builder
	sb.WriteString(existing)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		sb.WriteString("\n")
	}
	if existing != "" {
		sb.WriteString("\n")
	}
	sb.WriteString(open)
	sb.WriteString("\n")
	sb.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString(close)
	sb.WriteString("\n")
	return sb.String()
}
