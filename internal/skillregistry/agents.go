package skillregistry

import (
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
)

// AGENTS.md managed-section contract (spec §3.2). The canonical pair is what
// filemerge.InjectMarkdownSection emits for sectionID "skills-index"; the
// legacy pair is recognized and elevated by that same function.
const (
	agentsRelPath          = "AGENTS.md"
	skillsIndexSectionID   = "skills-index"
	skillsIndexOpen        = "<!-- axiom:skills-index -->"
	skillsIndexClose       = "<!-- /axiom:skills-index -->"
	skillsIndexLegacyOpen  = "<!-- gentle-ai:skills-index -->"
	skillsIndexLegacyClose = "<!-- /gentle-ai:skills-index -->"
)

// AgentsRelPath is the workspace-relative path of the agent instructions file
// whose `## Skills` section this engine manages (spec §3.3).
const AgentsRelPath = agentsRelPath

// AdoptSkillsIndexMarkers rewrites an AGENTS.md body so that its `## Skills`
// region is wrapped in the canonical marker pair, preparing it for
// filemerge.InjectMarkdownSection. It exists because InjectMarkdownSection
// APPENDS at end of file when it finds no marker pair
// (internal/components/filemerge/section.go), which would duplicate the
// section on the current AGENTS.md (REQ-22.12). The filemerge engine is reused
// unmodified (D-10).
//
// Contract (spec §3.5):
//
//   - canonical OR legacy pair already present -> returned unchanged
//     (InjectMarkdownSection replaces the managed body and elevates legacy)
//   - no pair + `## Skills` at line start -> that region (from the header up to
//     the line before the next `## ` at line start, or EOF) is replaced in situ
//     by an empty canonical pair
//   - no pair + no `## Skills` header -> returned unchanged
//     (InjectMarkdownSection then appends)
//
// This is a shape normalization, not a merge: it neither repairs orphan
// markers nor elevates legacy markers (InjectMarkdownSection already does
// both).
func AdoptSkillsIndexMarkers(existing string) string {
	if hasMarkerPair(existing, skillsIndexOpen, skillsIndexClose) ||
		hasMarkerPair(existing, skillsIndexLegacyOpen, skillsIndexLegacyClose) {
		return existing
	}
	start, end, ok := skillsHeadingRegion(existing)
	if !ok {
		return existing
	}
	// The region's final line terminator stays outside the replacement, so
	// every byte outside the marker lines is preserved as-is (spec §3.6).
	regionEnd := end
	if regionEnd > start && existing[regionEnd-1] == '\n' {
		regionEnd--
		if regionEnd > start && existing[regionEnd-1] == '\r' {
			regionEnd--
		}
	}
	return existing[:start] + skillsIndexOpen + "\n" + skillsIndexClose + existing[regionEnd:]
}

// injectSkillsIndex is the unmodified filemerge injection step (D-10): the
// engine here never reimplements marker repair or legacy elevation.
func injectSkillsIndex(adopted, body string) string {
	return filemerge.InjectMarkdownSection(adopted, skillsIndexSectionID, body)
}

// hasMarkerPair reports whether open is followed by close somewhere later.
func hasMarkerPair(s, open, close string) bool {
	i := strings.Index(s, open)
	if i < 0 {
		return false
	}
	return strings.Index(s[i+len(open):], close) >= 0
}

// skillsHeadingRegion returns the byte range of the first `## Skills` region:
// from the heading line start to the line start of the next `## ` heading, or
// EOF. A `## Skills` that is not at line start or that continues past the
// heading text (e.g. `## Skills Index`) is not a region start (T-4). With two
// `## Skills` headings only the first region is returned.
func skillsHeadingRegion(s string) (start, end int, ok bool) {
	offset := 0
	found := false
	for offset <= len(s) {
		lineEnd := strings.IndexByte(s[offset:], '\n')
		line := s[offset:]
		if lineEnd >= 0 {
			line = s[offset : offset+lineEnd]
		}
		if found {
			if strings.HasPrefix(line, "## ") {
				return start, offset, true
			}
		} else if isSkillsHeading(line) {
			start = offset
			found = true
		}
		if lineEnd < 0 {
			break
		}
		offset += lineEnd + 1
	}
	if found {
		return start, len(s), true
	}
	return 0, 0, false
}

// isSkillsHeading reports whether line is exactly the `## Skills` H2 heading,
// allowing trailing spaces and a trailing carriage return.
func isSkillsHeading(line string) bool {
	trimmed := strings.TrimRight(line, " \t\r")
	return trimmed == "## Skills"
}

// skillsIndexBody is the managed content between the skills-index markers
// (spec §3.3): the `## Skills` title line, one blank line, the four-column
// table and one final newline. The title lives INSIDE the managed content so
// title and table are replaced as one block. The Path column uses decision O-2
// (repository-relative for project skills).
func skillsIndexBody(cwd string, entries []SkillEntry) string {
	return "## Skills\n\n" + renderSkillsTable(cwd, entries, PathRepoRelative) + "\n"
}

// mirrorContent is the Engram mirror body (D-11): the managed `## Skills` body
// preceded by one provenance line. The file-oriented sections of
// .atl/skill-registry.md are deliberately not mirrored: one recognizable
// format must stay recognizable in every destination.
func mirrorContent(cwd string, entries []SkillEntry) string {
	return "<!-- Mirrored from the unified skills index (axiom skill index refresh). -->\n\n" +
		skillsIndexBody(cwd, entries)
}

// writeAgentsIndex rewrites the managed `## Skills` section of AGENTS.md and
// reports the destination outcome. A missing AGENTS.md is omitted and never
// created (spec §3.6). Any other failure is fatal and wrapped with the
// destination ("partial already emitted" ordering is the caller's concern).
func writeAgentsIndex(agentsPath, cwd string, entries []SkillEntry) (DestinationOutcome, error) {
	existingBytes, err := os.ReadFile(agentsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DestinationOutcome{Status: DestOmitted, Path: agentsPath, Reason: "AGENTS.md absent"}, nil
		}
		return DestinationOutcome{Status: DestOmitted, Path: agentsPath}, wrapAgentsError("read", err)
	}
	adopted := AdoptSkillsIndexMarkers(string(existingBytes))
	merged := injectSkillsIndex(adopted, skillsIndexBody(cwd, entries))
	if _, err := filemerge.WriteFileAtomic(agentsPath, []byte(merged), 0o644); err != nil {
		return DestinationOutcome{Status: DestOmitted, Path: agentsPath}, wrapAgentsError("write", err)
	}
	return DestinationOutcome{Status: DestUpdated, Path: agentsPath}, nil
}

// agentsCacheOutcome describes AGENTS.md when the fingerprint cache hit and no
// destination is written: unchanged when the file exists, omitted when it does
// not (nothing is created either way).
func agentsCacheOutcome(agentsPath string) DestinationOutcome {
	if fileExists(agentsPath) {
		return DestinationOutcome{Status: DestUnchanged, Path: agentsPath, Reason: "cache-hit"}
	}
	return DestinationOutcome{Status: DestOmitted, Path: agentsPath, Reason: "AGENTS.md absent"}
}

// wrapAgentsError names the destination in every AGENTS.md failure so a caller
// can report which destination broke after earlier ones already wrote.
func wrapAgentsError(verb string, err error) error {
	return fmt.Errorf("AGENTS.md %s: %w", verb, err)
}
