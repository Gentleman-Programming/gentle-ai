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
	remainder = strings.TrimLeft(remainder, "\r\n")
	return remainder
}

const (
	atlBeginMarker = "<!-- BEGIN:agent-teams-lite -->"
	atlEndMarker   = "<!-- END:agent-teams-lite -->"
)

// findLineStart returns the index of needle in s, but only if it appears
// at the start of a line (position 0 or immediately after '\n').
// Returns -1 if not found at a line boundary.
func findLineStart(s, needle string) int {
	offset := 0
	for {
		idx := strings.Index(s[offset:], needle)
		if idx < 0 {
			return -1
		}
		absIdx := offset + idx
		if absIdx == 0 || s[absIdx-1] == '\n' {
			return absIdx
		}
		// Not at line start — continue searching after this occurrence.
		offset = absIdx + 1
		if offset >= len(s) {
			return -1
		}
	}
}

// removeLineStartMarkers strips all occurrences of marker that appear at line boundaries.
func removeLineStartMarkers(content, marker string) string {
	for {
		idx := findLineStart(content, marker)
		if idx < 0 {
			return content
		}
		end := idx + len(marker)
		// Also consume a trailing line ending (\r\n or \n) if present.
		if end < len(content) && content[end] == '\r' {
			end++
		}
		if end < len(content) && content[end] == '\n' {
			end++
		}
		content = content[:idx] + content[end:]
	}
}

// StripLegacyATLBlock removes the legacy Agent Teams Lite block that was
// written by the standalone ATL installer before gentle-ai superseded it.
// The block is wrapped in <!-- BEGIN:agent-teams-lite --> / <!-- END:agent-teams-lite -->
// HTML comment markers. Its content is now provided by the canonical
// <!-- gentle-ai:sdd-orchestrator --> section, so keeping both wastes ~150
// lines of context per conversation.
//
// Safe to call on any file: returns content unchanged if no ATL block is found.
// All ATL blocks are stripped (not just the first one).
func StripLegacyATLBlock(content string) string {
	for {
		beginIdx := findLineStart(content, atlBeginMarker)
		if beginIdx < 0 {
			// No (more) BEGIN marker — exit the loop and do post-loop cleanup.
			break
		}

		// Search for the END marker starting from after the BEGIN marker so that
		// a stray END marker appearing before BEGIN does not prevent the valid
		// pair from being found.
		searchFrom := beginIdx + len(atlBeginMarker)
		relEndIdx := findLineStart(content[searchFrom:], atlEndMarker)
		if relEndIdx < 0 {
			// Open marker found but no matching close marker — break so that
			// post-loop cleanup still runs (e.g. orphan END markers are removed).
			break
		}
		endIdx := searchFrom + relEndIdx

		// Cut out the entire block including both markers.
		before := content[:beginIdx]
		after := content[endIdx+len(atlEndMarker):]

		// Trim trailing blank lines from the before segment.
		before = strings.TrimRight(before, "\r\n")
		// Trim leading blank lines from the after segment.
		after = strings.TrimLeft(after, "\r\n")

		if before == "" && after == "" {
			content = ""
			continue
		}

		var sb strings.Builder
		if before != "" {
			sb.WriteString(before)
			sb.WriteString("\n")
		}
		if after != "" {
			if before != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(after)
		}

		content = sb.String()
	}

	// Remove any orphan markers left behind. A stray END can appear before a
	// valid BEGIN...END pair; a stray BEGIN can appear without a matching END
	// (e.g. a partial manual edit). The loop only strips complete pairs, so
	// leftover markers must be cleaned up here.
	content = removeLineStartMarkers(content, atlEndMarker)
	content = removeLineStartMarkers(content, atlBeginMarker)

	// Collapse any triple+ newlines into double newlines (done once here,
	// outside the loop, to avoid O(N × content_length) work for N blocks).
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}
	return content
}

// openMarker returns the opening marker for a section ID.
func openMarker(sectionID string) string {
	return markerPrefix + sectionID + markerSuffix
}

// closeMarker returns the closing marker for a section ID.
func closeMarker(sectionID string) string {
	return closePrefix + sectionID + markerSuffix
}

// stripOrphanMarkers removes unpaired opening or closing markers for the given
// sectionID from content before injection logic runs.
//
// An orphan closer is a closing marker that appears with no preceding opening
// marker, OR that appears BEFORE the opening marker in the file. Without this
// cleanup the main injection loop would fall through to append mode, producing
// a duplicate block on every sync call (the root cause of issue #301).
//
// An orphan opener is an opening marker with no subsequent closing marker; it
// is also stripped so the caller can safely append a fresh, well-formed block.
//
// All occurrences of orphan markers are removed — not just the first — so that
// files already corrupted with 20+ duplicate blocks are fully collapsed to a
// clean state after a single sync run.
func stripOrphanMarkers(content, open, close string) string {
	for {
		openIdx := strings.Index(content, open)
		closeIdx := strings.Index(content, close)

		switch {
		case openIdx < 0 && closeIdx < 0:
			// Neither marker present — nothing to strip.
			return content

		case openIdx < 0 && closeIdx >= 0:
			// Orphan closer: closing marker exists but no opener before it.
			// Remove this closer and loop to catch any remaining orphans.
			content = content[:closeIdx] + content[closeIdx+len(close):]

		case openIdx >= 0 && closeIdx < 0:
			// Orphan opener: opening marker exists but no closer follows.
			// Remove the orphan opener so the caller appends a fresh block.
			content = content[:openIdx] + content[openIdx+len(open):]

		case closeIdx < openIdx:
			// Closer appears before opener — the closer is an orphan.
			// Remove it and loop; the opener may then form a valid pair or
			// become an orphan opener that gets cleaned up in the next iteration.
			content = content[:closeIdx] + content[closeIdx+len(close):]

		default:
			// Both markers present and opener precedes closer — valid pair found.
			// Stop; the main injection logic will handle the replacement.
			return content
		}
	}
}

// InjectMarkdownSection replaces or appends a marked section in a markdown file.
// Markers use HTML comments: <!-- gentle-ai:SECTION_ID --> ... <!-- /gentle-ai:SECTION_ID -->
// If the section already exists, its content is replaced.
// If it doesn't exist, it's appended at the end.
// Content outside markers is never touched.
// If content is empty, the section (including markers) is removed.
//
// Before injection, orphan markers (an unpaired closer or opener) are stripped
// so that a file corrupted by a previous buggy sync run is repaired in place
// rather than having another duplicate block appended.
func InjectMarkdownSection(existing, sectionID, content string) string {
	open := openMarker(sectionID)
	close := closeMarker(sectionID)

	// Repair any orphan markers left by previous (buggy) sync runs before
	// attempting replacement. This is the core fix for issue #301.
	existing = stripOrphanMarkers(existing, open, close)

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
