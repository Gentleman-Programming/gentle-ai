package triageevidence

import (
	"strings"
	"testing"
)

func TestRenderMarkdownEscapesHostileContent(t *testing.T) {
	hostileTitle := `<script>alert("x")</script> | pipe`
	hostileVersion := ParseReportedVersion(`3.4.0</script><script>alert(1)</script>`)
	rel := RelatedChange{
		Kind:  RelatedRelease,
		Title: `release notes ](https://evil.example)`,
		URL:   "https://example.com/releases/3.4.0",
		State: "v3.4.0",
	}
	ev := Evidence{Version: hostileVersion, OS: "<b>macOS</b>"}
	verdict := Classify(ClassificationInput{
		Issue:        Issue{Number: 5, Title: hostileTitle, Body: "harmless"},
		Evidence:     ev,
		LatestStable: ParseReportedVersion("v3.4.0"),
		Related:      []RelatedChange{rel},
	})
	if verdict.Outcome != OutcomeRelatedChangeFound {
		t.Fatalf("wanted related-change-found to drive reason rendering, got %s", verdict.Outcome)
	}

	out := RenderMarkdown(Report{
		Repository:   "A/B",
		GeneratedAt:  "2026-09-19T00:00:00Z",
		LatestStable: ParseReportedVersion("v3.4.0"),
		Items:        []ReportItem{{Issue: Issue{Number: 5, Title: hostileTitle, Body: "x"}, Verdict: verdict, Evidence: ev}},
	})

	for _, forbidden := range []string{
		"<script>",
		"</script>",
		"javascript:",
		"<b>",
		"[x](",
	} {
		if strings.Contains(out, forbidden) {
			t.Errorf("rendered output contains raw %q", forbidden)
		}
	}
	for _, want := range []string{
		"&lt;script&gt;",
		"&#124;",
		"&lt;b&gt;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing escaped form %q", want)
		}
	}
}

func TestRenderMarkdownDoesNotAllowUntrustedStructure(t *testing.T) {
	out := RenderMarkdown(Report{Items: []ReportItem{{
		Issue:   Issue{Number: 1, Title: "safe\n## injected heading\n[x](https://evil.example)"},
		Verdict: Verdict{Outcome: OutcomeInsufficientEvidence, Reasons: []string{"reason\n- injected list\n![image](https://evil.example)"}},
	}}})
	for _, forbidden := range []string{"\n## injected heading", "[x](https://evil.example)", "![image](https://evil.example)", "\n- injected list"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("untrusted markdown introduced structure %q in %q", forbidden, out)
		}
	}
}

func TestRenderMarkdownStructure(t *testing.T) {
	latest := ParseReportedVersion("v3.4.0")
	issue := Issue{Number: 42, Title: "TUI crash", Body: "### Gentle AI Version\n\nv3.4.0\n\nEvery time: panic: boom\n"}
	ev := ExtractEvidence(issue.Body)
	verdict := Classify(ClassificationInput{Issue: issue, Evidence: ev, LatestStable: latest})
	out := RenderMarkdown(Report{
		Repository:   "Gentleman-Programming/gentle-ai",
		GeneratedAt:  "2026-09-19T00:00:00Z",
		LatestStable: latest,
		Items:        []ReportItem{{Issue: issue, Verdict: verdict, Evidence: ev}},
	})

	for _, want := range []string{
		"# Triage evidence report",
		"## Summary",
		"| Outcome | Count |",
		"## #42 — TUI crash",
		"current-evidence",
		"reports the current stable v3.4.0 with reproduction evidence",
		"Reproduction evidence: present",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q", want)
		}
	}
}

func TestRenderMarkdownSummarySkipsZeroCounts(t *testing.T) {
	items := []ReportItem{
		{Issue: Issue{Number: 1, Title: "a"}, Verdict: Verdict{Outcome: OutcomeCurrentEvidence}},
		{Issue: Issue{Number: 2, Title: "b"}, Verdict: Verdict{Outcome: OutcomeInsufficientEvidence}},
	}
	out := RenderMarkdown(Report{Items: items})
	if strings.Contains(out, "related-change-found | 0") {
		t.Error("summary renders a zero-count outcome row")
	}
	if !strings.Contains(out, "current-evidence | 1") {
		t.Error("summary misses a nonzero outcome row")
	}
}

func TestRenderMarkdownDeterministicAndSorted(t *testing.T) {
	items := []ReportItem{
		{Issue: Issue{Number: 3, Title: "c"}, Verdict: Verdict{Outcome: OutcomeInsufficientEvidence}},
		{Issue: Issue{Number: 1, Title: "a"}, Verdict: Verdict{Outcome: OutcomeCurrentEvidence}},
		{Issue: Issue{Number: 2, Title: "b"}, Verdict: Verdict{Outcome: OutcomeRetestRequested}},
	}
	r := Report{GeneratedAt: "2026-09-19T00:00:00Z", Items: items}
	first := RenderMarkdown(r)
	second := RenderMarkdown(r)
	if first != second {
		t.Error("RenderMarkdown is not deterministic for identical input")
	}
	pos1, pos2, pos3 := strings.Index(first, "## #1"), strings.Index(first, "## #2"), strings.Index(first, "## #3")
	if !(pos1 < pos2 && pos2 < pos3) {
		t.Errorf("items not ascending by number: %d %d %d", pos1, pos2, pos3)
	}
}

func TestEscLinkSchemePolicy(t *testing.T) {
	if got := escLink("https://example.com/a_(b)"); got != "[https://example.com/a_\\(b\\)](<https://example.com/a_(b)>)" {
		t.Errorf("https URL rendered %q", got)
	}
	if got := escLink("javascript:alert(1)"); got != "javascript:alert\\(1\\)" {
		t.Errorf("javascript URL rendered %q, want plain escaped text only", got)
	}
}

func TestRenderMarkdownTruncates(t *testing.T) {
	var items []ReportItem
	for i := 1; i <= MaxReportItems+25; i++ {
		items = append(items, ReportItem{Issue: Issue{Number: i, Title: "x"}, Verdict: Verdict{Outcome: OutcomeInsufficientEvidence}})
	}
	out := RenderMarkdown(Report{Items: items})
	if !strings.Contains(out, "truncated") {
		t.Error("truncation note missing for over-cap report")
	}
	if got := strings.Count(out, "\n## #"); got > MaxReportItems+1 {
		t.Errorf("rendered %d items, want at most %d", got, MaxReportItems)
	}
}
