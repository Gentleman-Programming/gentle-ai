package verify

import (
	"fmt"
	"strings"
)

// readyMessage is the generic fallback used when the caller has no specific
// runnable agent commands to name (e.g. BuildReport itself, which has no
// knowledge of which agents were selected this run). Callers that DO know
// which runnable agent commands were installed should use
// ReadyMessageForCommands instead of relying on this generic text, so the
// completion line never claims an agent the user did not install.
const readyMessage = "You're ready. Start building."

// ReadyMessage is the exported form of readyMessage, so callers outside this
// package can detect the exact generic ready text BuildReport produces
// before deciding whether to replace it with an agent-specific note.
const ReadyMessage = readyMessage

// ReadyMessageForCommands renders the "you're ready" completion line naming
// exactly the given runnable agent commands, in the order given (e.g.
// []string{"claude", "opencode"} -> "Run `claude` or `opencode` and start
// building."). It falls back to the generic readyMessage when there are no
// commands to name, so an install with no known-runnable CLI agent never
// claims one it did not install.
func ReadyMessageForCommands(commands []string) string {
	if len(commands) == 0 {
		return readyMessage
	}
	quoted := make([]string, len(commands))
	for index, command := range commands {
		quoted[index] = "`" + command + "`"
	}
	var run string
	if len(quoted) == 1 {
		run = quoted[0]
	} else {
		run = strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
	}
	return fmt.Sprintf("You're ready. Run %s and start building.", run)
}

// verificationIssuesMessage is the generic fallback used when the caller has
// no specific remediation command to name (e.g. BuildReport itself, which has
// no knowledge of which CLI verb produced this report). Callers that DO know
// the concrete command that re-runs the failed work (install vs sync) should
// use VerificationIssuesMessageForCommand instead, so the failure line never
// names a command that does not exist -- there is no `repair` case in the CLI
// dispatcher (internal/app/app.go), and naming one is a false continuation
// strictly worse than silence.
const verificationIssuesMessage = "Installation completed with verification issues."

// VerificationIssuesMessage is the exported form of verificationIssuesMessage,
// so callers outside this package can detect the exact generic failure text
// BuildReport produces before deciding whether to replace it with a
// command-specific note.
const VerificationIssuesMessage = verificationIssuesMessage

// VerificationIssuesMessageForCommand renders the failure completion line
// naming exactly the given command that re-runs the failed checks (e.g.
// "gentle-ai sync" or "gentle-ai install --agent claude-code"). It falls back
// to the generic verificationIssuesMessage when no command is given, so a
// caller that does not know a concrete retry command never invents one.
func VerificationIssuesMessageForCommand(command string) string {
	if strings.TrimSpace(command) == "" {
		return verificationIssuesMessage
	}
	return fmt.Sprintf("%s Run `%s` to retry the failed checks.", verificationIssuesMessage, command)
}

type Report struct {
	Checks    []CheckResult
	Passed    int
	Failed    int
	Skipped   int
	Warnings  int
	Ready     bool
	FinalNote string
}

func BuildReport(results []CheckResult) Report {
	report := Report{Checks: append([]CheckResult(nil), results...)}
	for _, result := range results {
		switch result.Status {
		case CheckStatusPassed:
			report.Passed++
		case CheckStatusFailed:
			report.Failed++
		case CheckStatusSkipped:
			report.Skipped++
		case CheckStatusWarning:
			report.Warnings++
		}
	}

	report.Ready = report.Failed == 0
	if report.Ready {
		report.FinalNote = readyMessage
	} else {
		report.FinalNote = verificationIssuesMessage
	}

	return report
}

func RenderReport(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Verification checks: %d passed, %d failed, %d warnings, %d skipped\n", report.Passed, report.Failed, report.Warnings, report.Skipped)

	for _, check := range report.Checks {
		line := "[ ]"
		switch check.Status {
		case CheckStatusPassed:
			line = "[ok]"
		case CheckStatusFailed:
			line = "[!!]"
		case CheckStatusWarning:
			line = "[??]"
		case CheckStatusSkipped:
			line = "[--]"
		}

		fmt.Fprintf(&b, "%s %s", line, check.ID)
		if check.Description != "" {
			fmt.Fprintf(&b, " - %s", check.Description)
		}
		if check.Error != "" {
			fmt.Fprintf(&b, " (%s)", check.Error)
		}
		b.WriteString("\n")
	}

	b.WriteString(report.FinalNote)
	if !strings.HasSuffix(report.FinalNote, "\n") {
		b.WriteString("\n")
	}

	return b.String()
}
