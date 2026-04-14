package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

// categoryOrder defines the display order for grouped output.
var categoryOrder = []string{"deps", "platform", "agents", "env"}

// categoryLabel maps category IDs to human-readable section headers.
var categoryLabel = map[string]string{
	"deps":     "Dependency Health",
	"platform": "Platform Recommendations",
	"agents":   "Agent Config Health",
	"env":      "System Environment",
}

// statusIcon maps check statuses to terminal icons.
// Matches the icons already used in verify.RenderReport.
var statusIcon = map[verify.CheckStatus]string{
	verify.CheckStatusPassed:  "[ok]",
	verify.CheckStatusFailed:  "[!!]",
	verify.CheckStatusWarning: "[??]",
	verify.CheckStatusSkipped: "[--]",
}

// RenderReport formats a doctor report for human consumption.
func RenderReport(report Report) string {
	var b strings.Builder

	b.WriteString("gentle-ai doctor — system health check\n")
	b.WriteString(strings.Repeat("=", 39))
	b.WriteString("\n\n")

	grouped := groupByCategory(report.Checks)

	for _, cat := range categoryOrder {
		results, ok := grouped[cat]
		if !ok {
			continue
		}

		label := categoryLabel[cat]
		fmt.Fprintf(&b, "[%s] %s\n", cat, label)

		for _, r := range results {
			icon := statusIcon[r.Status]
			fmt.Fprintf(&b, "  %s %s - %s\n", icon, r.ID, r.Message)

			for _, d := range r.Details {
				fmt.Fprintf(&b, "        %s\n", d)
			}
		}

		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Summary: %d passed, %d failed, %d warnings, %d skipped\n",
		report.Passed, report.Failed, report.Warnings, report.Skipped)

	if !report.Healthy {
		b.WriteString("Status: unhealthy\n")
	} else if report.Warnings > 0 {
		b.WriteString("Status: healthy (with warnings)\n")
	} else {
		b.WriteString("Status: healthy\n")
	}

	return b.String()
}

// RenderJSON writes the report as machine-readable JSON.
// Used with --json for CI pipelines.
func RenderJSON(w io.Writer, report Report) error {
	type jsonReport struct {
		Checks   []CheckResult `json:"checks"`
		Passed   int           `json:"passed"`
		Failed   int           `json:"failed"`
		Warnings int           `json:"warnings"`
		Skipped  int           `json:"skipped"`
		Healthy  bool          `json:"healthy"`
		Duration string        `json:"duration"`
	}

	jr := jsonReport{
		Checks:   report.Checks,
		Passed:   report.Passed,
		Failed:   report.Failed,
		Warnings: report.Warnings,
		Skipped:  report.Skipped,
		Healthy:  report.Healthy,
		Duration: report.Duration.String(),
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jr)
}

// groupByCategory organizes results by their category for section rendering.
func groupByCategory(results []CheckResult) map[string][]CheckResult {
	grouped := make(map[string][]CheckResult)
	for _, r := range results {
		grouped[r.Category] = append(grouped[r.Category], r)
	}
	return grouped
}
