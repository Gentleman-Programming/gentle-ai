package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewDefectReportIssuesURL is the single destination every tool-fault
// Tier C statement points at, per organic-dx tasks.md 5.6.
const reviewDefectReportIssuesURL = "https://github.com/Gentleman-Programming/gentle-ai/issues/new/choose"

// reviewDefectReportDirName is the subdirectory under the repository's
// Git-common-dir "gentle-ai" root (the same convention repository_locator.go
// and store.go already use) that holds generated reports. It is never
// inside the working tree and is never committed — see the maintainer
// decision recorded in organic-dx tasks.md against 3b.8/3b.9.
const reviewDefectReportDirName = "defect-reports"

// reviewDefectReportInput is every raw field a tool-fault Tier C site can
// supply. Only opaque, non-content-bearing values belong here in principle —
// reviewDefectReport itself is still the last privacy gate before these
// bytes could reach a public issue tracker, so every field is scrubbed
// regardless of how disciplined the caller is (organic-dx tasks.md 5.3).
type reviewDefectReportInput struct {
	// Operation is the command plus flag shape, never raw argv (task 5.2).
	Operation string
	// ReasonCode is the verbatim reason/error code identifying the fault.
	ReasonCode string
	// ErrorMessage is the verbatim %v of the returned error.
	ErrorMessage string
	// TerminalPrecondition is the docs-table terminal precondition sentence.
	TerminalPrecondition string
	// StateIdentifiers are opaque state identifiers only: lineage state
	// name, gate, denial code, revision/target hashes — never raw content.
	StateIdentifiers map[string]string
}

// reviewDefectReport is the fully-resolved, already-scrubbed report ready to
// render. Constructing one (newReviewDefectReport) is the privacy boundary:
// nothing downstream of it ever sees the caller's raw strings again.
type reviewDefectReport struct {
	Version   string
	Commit    string
	OS        string
	Arch      string
	Input     reviewDefectReportInput
	Generated time.Time
}

// reviewGentleAIVersionAndCommit resolves the build's own version/commit the
// same way internal/app.ResolveVersion does, without importing internal/app
// (this package must not depend on the top-level command dispatcher).
// AppVersion is the build's stamped version, handed to this package by
// internal/app at startup. It is the same string `gentle-ai --version` prints.
//
// Reading build info alone was wrong and a tester caught it: for any stamped
// build the module pseudo-version disagrees with what --version reports, so
// doctor announced "version 1.49.1-0.2026..." for a binary that calls itself
// 2.2.0-rc.1. Doctor exists precisely to say which build is running, and the
// defect report carries this value to whoever has to reproduce the fault, so
// two answers for one binary is a defect in both.
func reviewGentleAIVersionAndCommit() (string, string) {
	version, commit := "dev", "unknown"
	if stamped := strings.TrimSpace(AppVersion); stamped != "" && stamped != "dev" {
		version = stamped
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		// Build info still answers when nothing stamped the binary, and always
		// supplies the commit, which the stamped version does not carry.
		if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = strings.TrimPrefix(info.Main.Version, "v")
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				commit = setting.Value
			}
		}
	}
	return version, commit
}

// newReviewDefectReport scrubs input and resolves the environment fields.
// This is the one privacy boundary every tool-fault site must cross before
// any caller-supplied string reaches a rendered report.
func newReviewDefectReport(input reviewDefectReportInput) reviewDefectReport {
	version, commit := reviewGentleAIVersionAndCommit()
	return reviewDefectReport{
		Version: version, Commit: commit, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Input:     reviewScrubDefectReportInput(input),
		Generated: time.Now().UTC(),
	}
}

const reviewDefectReportRedactionMarker = "<redacted>"

var (
	// reviewDefectReportAbsolutePathRegexp matches a POSIX or Windows
	// absolute path token. It is intentionally broad (any run of
	// non-whitespace, non-quote characters following a path root) because a
	// public issue tracker must never see a local filesystem layout,
	// including a $HOME-rooted path.
	reviewDefectReportAbsolutePathRegexp = regexp.MustCompile(`(?:[A-Za-z]:)?[\\/][^\s"'` + "`" + `]+`)
	// reviewDefectReportEmailRegexp matches any email-shaped substring.
	// Every email is redacted unconditionally: a defect report has no
	// legitimate use for one, "repo-identity" or not.
	reviewDefectReportEmailRegexp = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// reviewDefectReportEnvAssignmentRegexp matches a KEY=VALUE token shaped
	// like an environment variable assignment.
	reviewDefectReportEnvAssignmentRegexp = regexp.MustCompile(`\b[A-Z][A-Z0-9_]{2,}=\S+`)
)

// reviewScrubDefectReportField is the field-level privacy gate: every string
// that reaches a rendered report passes through this exact function first.
// Multi-line input (the shape of a diff, file contents, or a stack dump —
// exactly what this report must never carry) is truncated to its first line
// before the remaining redactions run.
func reviewScrubDefectReportField(value string) string {
	if value == "" {
		return value
	}
	if newline := strings.IndexByte(value, '\n'); newline >= 0 {
		value = value[:newline]
	}
	value = reviewDefectReportEnvAssignmentRegexp.ReplaceAllString(value, reviewDefectReportRedactionMarker)
	value = reviewDefectReportEmailRegexp.ReplaceAllString(value, reviewDefectReportRedactionMarker)
	value = reviewDefectReportAbsolutePathRegexp.ReplaceAllString(value, reviewDefectReportRedactionMarker)
	return strings.TrimSpace(value)
}

func reviewScrubDefectReportInput(input reviewDefectReportInput) reviewDefectReportInput {
	scrubbed := reviewDefectReportInput{
		Operation:            reviewScrubDefectReportField(input.Operation),
		ReasonCode:           reviewScrubDefectReportField(input.ReasonCode),
		ErrorMessage:         reviewScrubDefectReportField(input.ErrorMessage),
		TerminalPrecondition: reviewScrubDefectReportField(input.TerminalPrecondition),
	}
	if len(input.StateIdentifiers) > 0 {
		scrubbed.StateIdentifiers = make(map[string]string, len(input.StateIdentifiers))
		for key, value := range input.StateIdentifiers {
			scrubbed.StateIdentifiers[reviewScrubDefectReportField(key)] = reviewScrubDefectReportIdentifierValue(value)
		}
	}
	return scrubbed
}

// reviewScrubDefectReportIdentifierValue is the stricter gate for
// StateIdentifiers values specifically: those are documented as opaque
// identifiers only (lineage state name, gate, denial code, revision/target
// hashes) and none of those legitimately contain a space. Anything that
// still contains a space after the general field scrub is presumptively
// narrative content (a sentence, file contents, a diff line) smuggled into
// an identifier slot, so the entire value is redacted rather than trusted.
func reviewScrubDefectReportIdentifierValue(value string) string {
	scrubbed := reviewScrubDefectReportField(value)
	if strings.ContainsRune(scrubbed, ' ') {
		return reviewDefectReportRedactionMarker
	}
	return scrubbed
}

// render renders the Markdown body. Section headers match
// .github/ISSUE_TEMPLATE/bug_report.yml's field labels (task 5.2) so a
// maintainer can paste this directly into a new issue.
func (report reviewDefectReport) render() string {
	var body strings.Builder
	fmt.Fprintf(&body, "# Bug Description\n\n")
	fmt.Fprintf(&body, "Gentle AI reached a tool-internal fault state (`%s`) that should never happen. %s\n\n",
		report.Input.ReasonCode, report.Input.TerminalPrecondition)
	fmt.Fprintf(&body, "## Steps to Reproduce\n\n")
	fmt.Fprintf(&body, "Not automatically captured. Re-run the operation that surfaced this:\n\n```\n%s\n```\n\n", report.Input.Operation)
	fmt.Fprintf(&body, "## Expected Behavior\n\nThe operation completes, or reports a caller-actionable stop.\n\n")
	fmt.Fprintf(&body, "## Actual Behavior\n\n%s\n\n", report.Input.ErrorMessage)
	fmt.Fprintf(&body, "## Gentle AI Version\n\n%s (%s)\n\n", report.Version, report.Commit)
	fmt.Fprintf(&body, "## Operating System\n\n%s/%s\n\n", report.OS, report.Arch)
	fmt.Fprintf(&body, "## AI Agent / Client\n\nUnspecified (filled automatically; edit if known)\n\n")
	fmt.Fprintf(&body, "## Affected Area\n\nCLI (commands, flags)\n\n")
	fmt.Fprintf(&body, "## Logs / Error Output\n\n```\n")
	fmt.Fprintf(&body, "reason_code: %s\n", report.Input.ReasonCode)
	keys := make([]string, 0, len(report.Input.StateIdentifiers))
	for key := range report.Input.StateIdentifiers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&body, "%s: %s\n", key, report.Input.StateIdentifiers[key])
	}
	fmt.Fprintf(&body, "generated_at: %s\n```\n", report.Generated.Format(time.RFC3339))
	fmt.Fprintf(&body, "\nFile this report at %s.\n", reviewDefectReportIssuesURL)
	return body.String()
}

// reviewDefectReportFileName derives a deterministic, non-leaking file name:
// the reason code (already opaque) plus a content hash, so two reports for
// the same fault with the same content collapse to the same file instead of
// accumulating duplicates, while still being unique per distinct occurrence.
func reviewDefectReportFileName(reasonCode, content string) string {
	sum := sha256.Sum256([]byte(content))
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, reasonCode)
	if slug == "" {
		slug = "tool-fault"
	}
	return fmt.Sprintf("%s-%s.md", slug, hex.EncodeToString(sum[:])[:12])
}

// writeReviewDefectReport writes the rendered report under
// <GitCommonDir>/gentle-ai/defect-reports/ and returns the absolute path.
// GitCommonDir keeps this outside the working tree (repository_locator.go's
// existing convention), so it never pollutes `git status` and is never
// committed.
func writeReviewDefectReport(ctx context.Context, root string, report reviewDefectReport) (string, error) {
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(ctx, root)
	if err != nil {
		return "", fmt.Errorf("resolve repository identity for defect report: %w", err)
	}
	dir := filepath.Join(lease.Identity().GitCommonDir, "gentle-ai", reviewDefectReportDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create defect report directory: %w", err)
	}
	content := report.render()
	path := filepath.Join(dir, reviewDefectReportFileName(report.Input.ReasonCode, content))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write defect report: %w", err)
	}
	return path, nil
}

// reviewToolFaultDefectClausePrefix is the stable marker every defect clause
// starts with. The preflight parity guard uses it to recognize the clause as a
// human-only decoration rather than refusal information.
const reviewToolFaultDefectClausePrefix = " A defect report was saved at "

// reviewToolFaultDefectClause is the "at most one clause" Tier C extension
// task 5.6 requires: the report path plus the issues URL, appended to the
// base fault message so the whole thing stays one statement, one action.
func reviewToolFaultDefectClause(reportPath string) string {
	return fmt.Sprintf("%s%s -- file it at %s.", reviewToolFaultDefectClausePrefix, reportPath, reviewDefectReportIssuesURL)
}

// reviewGenerateToolFaultDefectReport is the one call every tool-fault site
// makes: build, scrub, and persist the report, then return the Tier C
// clause to append to that site's own error message. Persistence failure
// degrades to an empty clause rather than masking the original fault with a
// second, unrelated error -- the original tool-fault error is always what
// the caller sees.
func reviewGenerateToolFaultDefectReport(ctx context.Context, root string, input reviewDefectReportInput) string {
	report := newReviewDefectReport(input)
	path, err := writeReviewDefectReport(ctx, root, report)
	if err != nil {
		return ""
	}
	return reviewToolFaultDefectClause(path)
}

// reviewUnexpectedFaultDefectReportClause extends the defect-report treatment
// from the two hand-picked tool-fault sites to the whole unanticipated-residue
// class at the negotiated failure choke point (issue #1881 follow-up). The
// class boundary is enforced by the classifier itself, not by a list of call
// sites: every anticipated condition carries its own typed failure code, so
// only a failure that still reads operation_outcome_unknown after the full
// typed cascade is the "this should not happen" residue. Two explicit fences
// keep the boundary honest:
//
//   - a typed refusal is NOT a defect: policy denials, validation errors, and
//     correct refusals all leave the residue code behind and never report;
//   - read-only operations never write, the same rule that keeps gates and
//     STATUS report-free — only operations the registry declares mutating
//     qualify.
//
// Persistence failure degrades to an empty clause inside
// reviewGenerateToolFaultDefectReport, so the reporter can never mask or
// replace the original error path.
func reviewUnexpectedFaultDefectReportClause(ctx context.Context, operation string, args []string, failure ReviewIntegrationFailure) string {
	if failure.Code != "operation_outcome_unknown" {
		return ""
	}
	metadata, known := reviewIntegrationOperationByName(operation)
	if !known || !reviewIntegrationOperationMutates(metadata, args) {
		return ""
	}
	values, _ := safeReviewIntegrationArguments(operation, args)
	root := strings.TrimSpace(values["cwd"])
	if root == "" {
		root = "."
	}
	message := failure.Cause
	if message == "" {
		message = failure.Message
	}
	identifiers := map[string]string{"operation": operation, "phase": failure.Phase}
	if failure.LineageID != "" {
		identifiers["lineage"] = failure.LineageID
	}
	return reviewGenerateToolFaultDefectReport(ctx, root, reviewDefectReportInput{
		Operation:            reviewDefectReportOperationShape(metadata.Command, operation, args),
		ReasonCode:           failure.Code,
		ErrorMessage:         message,
		TerminalPrecondition: "a mutating review operation failed in a way no typed classification anticipated, so its true outcome is unknown to the tool",
		StateIdentifiers:     identifiers,
	})
}

// reviewDefectReportOperationShape renders "review <command>" plus the sorted
// flag NAMES the caller provided — the command's shape, never raw argv, so no
// flag value can ride into a report destined for a public issue tracker.
func reviewDefectReportOperationShape(command, operation string, args []string) string {
	shape := []string{"review", command}
	if values, valid := safeReviewIntegrationArguments(operation, args); valid {
		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, "--"+name)
		}
		sort.Strings(names)
		shape = append(shape, names...)
	}
	return strings.Join(shape, " ")
}

// reviewAppendUnexpectedFaultDefectClause gives the plain (non-negotiated)
// form the same residue treatment: it classifies the failure with the exact
// negotiated cascade — purely, nothing is emitted — and, when the result is
// unanticipated residue on a mutating operation, appends the saved-report
// clause to the error the operator is about to read. Named refusals and
// read-only operations fall out of the same two fences above.
func reviewAppendUnexpectedFaultDefectClause(ctx context.Context, operation string, args []string, err error) error {
	if err == nil || operation == "" {
		return err
	}
	clause := reviewUnexpectedFaultDefectReportClause(ctx, operation, args, newReviewIntegrationFailure(operation, args, err))
	if clause == "" {
		return err
	}
	return fmt.Errorf("%w%s", err, clause)
}
