package triageevidence

import (
	"regexp"
	"strings"
)

// Evidence is the structured reading of a bug report body. Every field is best
// effort: a report that skips a template section simply leaves the field empty.
type Evidence struct {
	Version      ReportedVersion
	OS           string
	Agent        string
	AffectedArea string
	// HasDetailedSteps is true when the steps-to-reproduce section carries real
	// content, not the template's empty "1. / 2. / 3." placeholder list.
	HasDetailedSteps bool
	// HasLogsOrCommands is true when the body contains a fenced code block,
	// a shell command, or an error message — the cheapest independent signal
	// that the reporter actually ran something.
	HasLogsOrCommands bool
	// StatesReproducibility is true when the reporter claims repeatable
	// reproduction in words ("reproducib", "consistently", "every time").
	StatesReproducibility bool
}

// HasReproductionEvidence is the aggregate signal the classification uses: at
// least one of detailed steps, logs/commands, or an explicit reproducibility
// claim. Absence here never proves anything negative about the report; it only
// leaves the issue short of the evidence threshold.
func (e Evidence) HasReproductionEvidence() bool {
	return e.HasDetailedSteps || e.HasLogsOrCommands || e.StatesReproducibility
}

// ExtractEvidence parses the template sections out of a bug report body. The
// parsing is purely structural: headings the template defines, values in
// between. Unknown or mistyped sections are ignored, never guessed.
func ExtractEvidence(body string) Evidence {
	sections := splitSections(body)
	e := Evidence{
		Version:      ParseReportedVersion(firstLine(sectionValue(sections, "gentle ai version"))),
		OS:           sectionValue(sections, "operating system"),
		Agent:        sectionValue(sections, "ai agent"),
		AffectedArea: sectionValue(sections, "affected area"),
	}
	e.HasDetailedSteps = hasMeaningfulSteps(sectionValue(sections, "steps to reproduce"))
	e.HasLogsOrCommands = hasLogsOrCommands(body)
	e.StatesReproducibility = reproducibilityRe.MatchString(body)
	return e
}

// firstLine takes the first non-empty line of a section value: the template's
// version field is a single-line input, and trailing prose (e.g. a missing
// later heading in a hand-edited report) must not leak into the parsed version.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// headingRe matches the template's markdown headings: "### Gentle AI Version",
// "## Operating System", or "## 🖥️ Environment" style.
var headingRe = regexp.MustCompile(`(?m)^#{2,3}\s*(.+?)\s*$`)

func splitSections(body string) map[string]string {
	out := map[string]string{}
	var current string
	var lines []string
	flush := func() {
		if current != "" {
			out[current] = strings.TrimSpace(strings.Join(lines, "\n"))
		}
		lines = nil
	}
	for _, line := range strings.Split(body, "\n") {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			flush()
			current = normalizeHeading(m[1])
			continue
		}
		lines = append(lines, line)
	}
	flush()
	return out
}

// normalizeHeading lowercases and strips the emoji and numbering a heading may
// carry so section lookups are stable ("📋 Affected Area" == "affected area").
func normalizeHeading(h string) string {
	noEmoji := strings.Map(func(r rune) rune {
		if r >= 0x1F000 && r <= 0x1FAFF || r >= 0x2600 && r <= 0x27BF {
			return -1
		}
		return r
	}, h)
	return strings.ToLower(strings.TrimSpace(noEmoji))
}

func sectionValue(sections map[string]string, key string) string {
	for k, v := range sections {
		if strings.Contains(k, key) {
			return v
		}
	}
	return ""
}

// hasMeaningfulSteps discounts the template placeholder list ("1. ", "2. ",
// "3. ") and any line whose only content is list markers or punctuation.
func hasMeaningfulSteps(steps string) bool {
	meaningful := false
	for _, line := range strings.Split(steps, "\n") {
		l := strings.TrimSpace(line)
		l = strings.TrimLeft(l, "0123456789.)-* ")
		if len(l) >= 10 {
			meaningful = true
		}
	}
	return meaningful
}

var (
	// logsRe finds copies of terminal output: fenced code blocks, "$ command"
	// lines (including the backtick-quoted form reporters paste), and the common
	// error/exit shapes. Built as interpreted strings because raw strings cannot
	// safely carry backticks or the \$ escape.
	logsRe = regexp.MustCompile(
		"(?m)(^```|" +
			"`\\$ |" + // literal "`$ " (backtick-quoted command)
			"\\$ [a-z0-9_./-]+ |" + // literal "$ cmd "
			"error[:：]|failed(?: to)? |exit code|exited with code|panic: )",
	)
	// reproducibility claims.
	reproducibilityRe = regexp.MustCompile("(?i)(reproducib|consistently|every time|100% of the time|happens (always|every time))")
)

func hasLogsOrCommands(body string) bool {
	return logsRe.MatchString(body)
}
