package triageevidence

import (
	"html"
	"sort"
	"strconv"
	"strings"
)

// MaxReportItems is the hard cap the renderer enforces on top of whatever bound
// the caller applied: an artifact never grows without limit even if a caller
// misconfigures its own bound.
const MaxReportItems = 500

// Report is the whole artifact: fixed metadata plus one verdict per issue.
type Report struct {
	// Repository is the OWNER/NAME pair, used only in the header.
	Repository string
	// GeneratedAt is pinned by the caller (workflow run start) so output stays
	// deterministic for unchanged evidence between the report and its reruns.
	GeneratedAt  string
	LatestStable ReportedVersion
	Items        []ReportItem
}

// ReportItem pairs an issue with its verdict and the evidence that drove it.
type ReportItem struct {
	Issue    Issue
	Verdict  Verdict
	Evidence Evidence
}

// OutcomeCount aggregates the report by outcome, always in the fixed display
// order so unchanged evidence renders byte-identical.
type OutcomeCount struct {
	Outcome Outcome
	Count   int
}

// Summarize returns one count row per outcome in fixed precedence order.
func Summarize(items []ReportItem) []OutcomeCount {
	counts := map[Outcome]int{}
	for _, it := range items {
		counts[it.Verdict.Outcome]++
	}
	return []OutcomeCount{
		{OutcomeRelatedChangeFound, counts[OutcomeRelatedChangeFound]},
		{OutcomeRetestRequested, counts[OutcomeRetestRequested]},
		{OutcomeCurrentEvidence, counts[OutcomeCurrentEvidence]},
		{OutcomeInsufficientEvidence, counts[OutcomeInsufficientEvidence]},
	}
}

// RenderMarkdown renders the artifact. Every string that originates in issue
// content, related-change metadata, or a reason line is HTML-escaped before it
// can reach the markdown stream; table cells additionally escape the pipe
// character. The function never reads the network, the filesystem, or the
// clock, and renders items in ascending issue-number order regardless of input
// order. Rendered output is hard-capped at MaxReportItems.
func RenderMarkdown(r Report) string {
	items := make([]ReportItem, len(r.Items))
	copy(items, r.Items)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Issue.Number < items[j].Issue.Number })

	var b strings.Builder
	b.WriteString("# Triage evidence report\n\n")
	b.WriteString("- Repository: " + esc(r.Repository) + "\n")
	b.WriteString("- Generated at: " + esc(r.GeneratedAt) + "\n")
	if r.LatestStable.IsVersion() {
		b.WriteString("- Latest stable release: " + esc(r.LatestStable.Raw) + " (" + esc(r.LatestStable.Channel.String()) + ")\n")
	} else {
		b.WriteString("- Latest stable release: unavailable (no stable release found)\n")
	}
	b.WriteString("- Analyzed issues: " + strconv.Itoa(len(items)))
	if len(items) > MaxReportItems {
		b.WriteString(" (rendered " + strconv.Itoa(MaxReportItems) + "; truncated)")
	}
	b.WriteString("\n\n")

	b.WriteString("## Summary\n\n")
	b.WriteString("| Outcome | Count |\n|---|---|\n")
	for _, row := range Summarize(items) {
		if row.Count == 0 {
			continue
		}
		b.WriteString("| " + esc(string(row.Outcome)) + " | " + strconv.Itoa(row.Count) + " |\n")
	}
	b.WriteString("\n")

	rendered := 0
	for _, it := range items {
		if rendered >= MaxReportItems {
			break
		}
		rendered++
		b.WriteString(renderItem(it))
	}
	return b.String()
}

func renderItem(it ReportItem) string {
	var b strings.Builder
	title := it.Issue.Title
	if title == "" {
		title = "(untitled)"
	}
	b.WriteString("## #" + strconv.Itoa(it.Issue.Number) + " — " + esc(title) + "\n\n")
	b.WriteString("- Outcome: `" + esc(string(it.Verdict.Outcome)) + "`\n")
	b.WriteString("- Reported version channel: `" + esc(it.Evidence.Version.Channel.String()) + "`")
	if it.Evidence.Version.IsVersion() {
		b.WriteString(" (" + esc(it.Evidence.Version.Raw) + ")")
	}
	b.WriteString("\n")
	env := []string{}
	if it.Evidence.OS != "" {
		env = append(env, "OS: "+it.Evidence.OS)
	}
	if it.Evidence.Agent != "" {
		env = append(env, "agent: "+it.Evidence.Agent)
	}
	if it.Evidence.AffectedArea != "" {
		env = append(env, "area: "+it.Evidence.AffectedArea)
	}
	if len(env) > 0 {
		b.WriteString("- Environment: " + esc(strings.Join(env, "; ")) + "\n")
	}
	b.WriteString("- Reproduction evidence: ")
	if it.Evidence.HasReproductionEvidence() {
		b.WriteString("present\n")
	} else {
		b.WriteString("none\n")
	}
	if len(it.Verdict.Reasons) == 0 {
		b.WriteString("- Rationale: unavailable\n")
	} else {
		b.WriteString("- Rationale:\n")
		for _, reason := range it.Verdict.Reasons {
			b.WriteString("  - " + esc(reason) + "\n")
		}
	}
	if len(it.Verdict.Support) == 0 {
		b.WriteString("- Supporting evidence: unavailable\n")
	} else {
		b.WriteString("- Supporting evidence:\n")
		for _, u := range it.Verdict.Support {
			b.WriteString("  - " + escLink(u) + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// esc escapes untrusted text for HTML contexts (the markdown stream GitHub
// renders) and neutralizes the table-cell pipe.
func esc(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "|", "&#124;")
}

// escLink renders a supporting URL as a clickable markdown link after escaping
// both halves. Only http(s) URLs become links; anything else renders as escaped
// plain text so a hostile "javascript:..." value can never become clickable.
func escLink(u string) string {
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return esc(u)
	}
	return "[" + esc(u) + "](" + esc(u) + ")"
}
