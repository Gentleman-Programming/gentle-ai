package filemerge

import (
	"strings"
	"testing"
)

// --- Adversarial edge cases ---

// Test: TrimRight("\n") eats spaces and tabs too? No — TrimRight with "\n" cutset
// only trims newlines. But what about \r\n (Windows)?
func TestAdversarial_WindowsLineEndings(t *testing.T) {
	// Windows-style CRLF around the ATL block
	input := "before\r\n\r\n<!-- BEGIN:agent-teams-lite -->\r\ncontent\r\n<!-- END:agent-teams-lite -->\r\n\r\nafter"
	result := StripLegacyATLBlock(input)
	t.Logf("Windows CRLF result: %q", result)
	if strings.Contains(result, "BEGIN:agent-teams-lite") {
		t.Fatal("BEGIN marker not stripped with CRLF")
	}
	// Check that \r characters don't pile up
	if strings.Contains(result, "\r\n\r\n\r\n") {
		t.Fatal("triple CRLF not collapsed")
	}
}

// Test: only END marker, no BEGIN at all
func TestAdversarial_OrphanEndOnly(t *testing.T) {
	input := "some content\n<!-- END:agent-teams-lite -->\nmore content"
	result := StripLegacyATLBlock(input)
	if strings.Contains(result, "<!-- END:agent-teams-lite -->") {
		t.Fatal("orphan END marker should be stripped by post-loop cleanup")
	}
	if !strings.Contains(result, "some content") {
		t.Fatal("content before orphan END should be preserved")
	}
	if !strings.Contains(result, "more content") {
		t.Fatal("content after orphan END should be preserved")
	}
}

// Test: TrimRight(before, "\n") strips ALL trailing newlines from "before",
// but what if "before" is entirely newlines? e.g. content starts with \n\n\nBEGIN
func TestAdversarial_LeadingNewlinesBeforeBlock(t *testing.T) {
	input := "\n\n\n<!-- BEGIN:agent-teams-lite -->\ncontent\n<!-- END:agent-teams-lite -->\n\nafter"
	result := StripLegacyATLBlock(input)
	t.Logf("Leading newlines result: %q", result)
	// before = "\n\n\n", TrimRight("\n") = "", so before == ""
	// after = "\n\nafter", TrimLeft("\n") = "after"
	// before=="" && after!="" → sb writes after only → "after"
	if !strings.Contains(result, "after") {
		t.Fatal("content after block should be preserved")
	}
}

// Test: nested markers (BEGIN inside BEGIN...END)
func TestAdversarial_NestedBeginMarkers(t *testing.T) {
	input := "<!-- BEGIN:agent-teams-lite -->\n<!-- BEGIN:agent-teams-lite -->\nnested\n<!-- END:agent-teams-lite -->\nouter tail\n<!-- END:agent-teams-lite -->"
	result := StripLegacyATLBlock(input)
	t.Logf("Nested result: %q", result)
	// First iteration: beginIdx=0, searches for END from after first BEGIN
	// finds the FIRST END. Strips [0 .. first_END+len]. "after" = "\nouter tail\n<!-- END:agent-teams-lite -->"
	// Second iteration: finds "<!-- BEGIN:agent-teams-lite -->" inside remaining? No — first BEGIN was cut.
	// Actually "after" contains "outer tail\n<!-- END:agent-teams-lite -->", no BEGIN.
	// Loop breaks. Orphan END cleanup removes the trailing END.
	if strings.Contains(result, "BEGIN:agent-teams-lite") {
		t.Fatal("nested: BEGIN marker should be gone")
	}
	if strings.Contains(result, "END:agent-teams-lite") {
		t.Fatal("nested: END marker should be gone (orphan cleanup)")
	}
}

// Test: content that contains the string "BEGIN:agent-teams-lite" but not as HTML comment
func TestAdversarial_PartialMarkerString(t *testing.T) {
	input := "The marker is BEGIN:agent-teams-lite but not in comment form\n"
	result := StripLegacyATLBlock(input)
	if result != input {
		t.Fatal("partial marker string should not trigger stripping")
	}
}

// Test: triple newline collapse doesn't eat content — make sure it only replaces \n\n\n
func TestAdversarial_TripleNewlineCollapsePreservesContent(t *testing.T) {
	input := "before\n\n<!-- BEGIN:agent-teams-lite -->\ncontent\n<!-- END:agent-teams-lite -->\n\nafter"
	result := StripLegacyATLBlock(input)
	t.Logf("Triple newline result: %q", result)
	if strings.Contains(result, "\n\n\n") {
		t.Fatal("triple newlines should be collapsed")
	}
	if !strings.Contains(result, "before") || !strings.Contains(result, "after") {
		t.Fatal("surrounding content should be preserved")
	}
}

// Test: orphan END cleanup + collapse interaction — does removing END create triple newlines?
func TestAdversarial_OrphanEndCleanupCreatesTripleNewlines(t *testing.T) {
	// Orphan END sits between two blocks of content with blank lines around it
	input := "content A\n\n<!-- END:agent-teams-lite -->\n\ncontent B"
	result := StripLegacyATLBlock(input)
	t.Logf("Orphan + collapse result: %q", result)
	// After ReplaceAll(END, ""): "content A\n\n\n\ncontent B"
	// Triple-newline collapse should fix it
	if strings.Contains(result, "\n\n\n") {
		t.Fatal("triple newlines after orphan END removal should be collapsed")
	}
}

// Test: multiple orphan END markers
func TestAdversarial_MultipleOrphanEnds(t *testing.T) {
	input := "<!-- END:agent-teams-lite -->\n<!-- END:agent-teams-lite -->\ncontent"
	result := StripLegacyATLBlock(input)
	if strings.Contains(result, "END:agent-teams-lite") {
		t.Fatal("all orphan END markers should be removed")
	}
	if !strings.Contains(result, "content") {
		t.Fatal("non-ATL content should be preserved")
	}
}
