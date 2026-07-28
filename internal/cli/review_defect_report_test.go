package cli

import (
	"strings"
	"testing"
)

// TestReviewDefectReportRendersTemplateShapedFields is the RED-first proof
// for organic-dx Phase 5 task 5.2: reviewDefectReport renders Markdown whose
// section headers match .github/ISSUE_TEMPLATE/bug_report.yml's field labels,
// populated from gentle-ai version + commit, OS/arch, the failing operation
// shape (not raw argv), the verbatim reason code, opaque state identifiers,
// and the stop-code row's terminal precondition.
func TestReviewDefectReportRendersTemplateShapedFields(t *testing.T) {
	report := newReviewDefectReport(reviewDefectReportInput{
		Operation:            "review capture-evidence --lineage --target --expected-revision --input",
		ReasonCode:           "captured_final_evidence_conflict",
		ErrorMessage:         "captured final evidence already exists with different bytes",
		TerminalPrecondition: "final verification evidence was already captured for this validating target with different bytes",
		StateIdentifiers: map[string]string{
			"state":  "validating",
			"target": "sha256:" + strings.Repeat("a", 64),
		},
	})
	report.Version, report.Commit, report.OS, report.Arch = "1.2.3", "deadbeef", "linux", "amd64"
	rendered := report.render()

	for _, header := range []string{
		"# Bug Description", "## Steps to Reproduce", "## Expected Behavior", "## Actual Behavior",
		"## Gentle AI Version", "## Operating System", "## AI Agent / Client", "## Affected Area", "## Logs / Error Output",
	} {
		if !strings.Contains(rendered, header) {
			t.Errorf("rendered report missing template-shaped header %q:\n%s", header, rendered)
		}
	}
	for _, want := range []string{
		"captured_final_evidence_conflict",
		"captured final evidence already exists with different bytes",
		"final verification evidence was already captured for this validating target with different bytes",
		"1.2.3", "deadbeef", "linux/amd64",
		"review capture-evidence --lineage --target --expected-revision --input",
		"state: validating",
		"target: sha256:" + strings.Repeat("a", 64),
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered report missing expected content %q:\n%s", want, rendered)
		}
	}
}

// TestReviewDefectReportScrubsPoisonedInput is the RED-first, hard-privacy
// requirement of task 5.3: driving the builder with content-bearing strings,
// diffs, file contents, absolute paths under $HOME, env-var-shaped values,
// and email addresses must leave none of that raw material in the rendered
// output. A leak here would land on a public issue tracker.
func TestReviewDefectReportScrubsPoisonedInput(t *testing.T) {
	home := "/home/definitely-a-real-user"
	poisoned := reviewDefectReportInput{
		Operation: "review capture-evidence --input " + home + "/secret-project/plan.txt",
		ReasonCode: "captured_final_evidence_conflict\n" +
			"-func leaked() { return \"" + home + "/.ssh/id_rsa\" }",
		ErrorMessage: "open " + home + "/.ssh/id_rsa: permission denied\n" +
			"@@ -1,3 +1,4 @@\n+AWS_SECRET_ACCESS_KEY=super-secret-value-should-never-leak\n" +
			"contact definitely-a-real-user@example.com for access",
		TerminalPrecondition: "diff:\n--- a/file\n+++ b/file\n@@\n-old line with secret token XYZZY-DO-NOT-LEAK\n+new line",
		StateIdentifiers: map[string]string{
			"path":    home + "/.config/gentle-ai/credentials.json",
			"env":     "GITHUB_TOKEN=ghp_shouldneverleakshouldneverleakshouldneverleak",
			"email":   "definitely-a-real-user@example.com",
			"content": "the quick brown fox file contents should never appear\nsecond line of file content",
		},
	}
	rendered := newReviewDefectReport(poisoned).render()

	poisonedSubstrings := []string{
		home,
		"id_rsa",
		"AWS_SECRET_ACCESS_KEY=super-secret-value-should-never-leak",
		"definitely-a-real-user@example.com",
		"XYZZY-DO-NOT-LEAK",
		"ghp_shouldneverleakshouldneverleakshouldneverleak",
		"the quick brown fox file contents should never appear",
		"second line of file content",
		"credentials.json",
	}
	for _, poison := range poisonedSubstrings {
		if strings.Contains(rendered, poison) {
			t.Fatalf("rendered defect report leaked poisoned input %q:\n%s", poison, rendered)
		}
	}
	// A multi-line reason code must never smuggle a second line (e.g. a diff
	// hunk) into the report; only its first, scrubbed line may survive.
	if strings.Contains(rendered, "-func leaked()") {
		t.Fatalf("rendered defect report leaked a multi-line reason code payload:\n%s", rendered)
	}
}

// TestReviewDefectReportScrubbedFieldIsSingleLineAndBounded pins the exact
// scrubbing contract reviewScrubDefectReportField implements, independent of
// the full-report integration test above.
func TestReviewDefectReportScrubbedFieldIsSingleLineAndBounded(t *testing.T) {
	cases := map[string]struct{ input, wantNotContain string }{
		"multiline":     {"first line\nsecond line", "second line"},
		"absolute-path": {"see /home/someone/secret.txt for detail", "/home/someone/secret.txt"},
		"email":         {"reach out to someone@example.com", "someone@example.com"},
		"env-shaped":    {"TOKEN=abc123shouldnotleak", "abc123shouldnotleak"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			scrubbed := reviewScrubDefectReportField(testCase.input)
			if strings.Contains(scrubbed, testCase.wantNotContain) {
				t.Fatalf("scrubbed field still contains %q: %q", testCase.wantNotContain, scrubbed)
			}
		})
	}
}
