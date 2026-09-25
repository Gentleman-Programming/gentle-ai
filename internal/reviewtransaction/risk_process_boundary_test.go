package reviewtransaction

import (
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestProcessBoundaryInspectionIsBatchedAndBounded(t *testing.T) {
	repo := initSnapshotRepo(t)
	paths := []string{"tools/a.py", "tools/b.py"}
	for _, path := range paths {
		writeSnapshotFile(t, repo, path, "import subprocess\nsubprocess.run(argv)\n")
	}
	snapshot, _ := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: paths})
	original, inspections := gitCommandContext, 0
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if len(args) > 3 && (args[3] == "ls-tree" || args[3] == "grep" || args[3] == "show") {
			inspections++
		}
		return original(ctx, name, args...)
	}
	_, _ = (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if inspections > 4 {
		t.Fatalf("source inspection used %d Git subprocesses, want at most 4", inspections)
	}
	processBoundaryScanByteLimit = 16
	t.Cleanup(func() { gitCommandContext, processBoundaryScanByteLimit = original, 8<<20 })
	assessment, _ := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if assessment.Level != RiskHigh || len(assessment.Reasons) == 0 || assessment.Reasons[0].Code != RiskReasonCode("process_scan_limit") {
		t.Fatalf("cap exhaustion assessment = %#v, want explicit high-risk process_scan_limit", assessment)
	}
}

// buildIssue1438Wrapper renders the issue #1438 reproduction module: a new
// executable Python module wrapping subprocess.run with one network-capable
// read-only Git command and an argv/cwd/timeout/environment policy surface.
func buildIssue1438Wrapper(lines int) string {
	header := []string{
		`"""Executable module wrapping subprocess.run with argv/cwd/timeout/env policy."""`,
		"import subprocess",
		"",
		"def run_command(argv, cwd, timeout, env):",
		"    completed = subprocess.run(argv, cwd=cwd, timeout=timeout, env=env, capture_output=True, check=True)",
		"    return completed.stdout, completed.stderr",
		"",
		"def ls_remote(url):",
		`    return run_command(["git", "ls-remote", url], cwd=None, timeout=30, env={})`,
	}
	return paddedPolicyModule(header, lines)
}

// paddedPolicyModule pads a module to an exact authored line count with inert
// policy comments that never contain a process-spawn construct.
func paddedPolicyModule(header []string, lines int) string {
	rendered := append([]string{}, header...)
	for index := len(rendered); index < lines; index++ {
		rendered = append(rendered, fmt.Sprintf("# policy line %03d", index+1))
	}
	return strings.Join(rendered[:lines], "\n") + "\n"
}

// TestAssessSnapshotRiskClassifiesUntrackedSubprocessWrapperHigh captures the
// issue #1438 reproduction verbatim: 318 authored changed lines across four
// untracked paths, one of them a subprocess.run wrapper, must classify high
// from the content-derived process-boundary signal, not the size rule.
func TestAssessSnapshotRiskClassifiesUntrackedSubprocessWrapperHigh(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tools/procwrap/runner.py", buildIssue1438Wrapper(200))
	writeSnapshotFile(t, repo, "tools/procwrap/policy.py", paddedPolicyModule([]string{`"""Argv, cwd, timeout, and environment policy tables."""`}, 80))
	writeSnapshotFile(t, repo, "tools/procwrap/errors.py", paddedPolicyModule([]string{`"""Process output and error policy."""`}, 30))
	writeSnapshotFile(t, repo, "tools/procwrap/__init__.py", paddedPolicyModule([]string{`"""Package marker."""`}, 8))
	untracked := []string{
		"tools/procwrap/__init__.py", "tools/procwrap/errors.py",
		"tools/procwrap/policy.py", "tools/procwrap/runner.py",
	}
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: untracked})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.ChangedLines != 318 {
		t.Fatalf("ChangedLines = %d, want the 318-line reproduction", assessment.ChangedLines)
	}
	if assessment.Level != RiskHigh {
		t.Fatalf("AssessSnapshotRisk() level = %q with reasons %#v, want %q from the process-boundary content signal", assessment.Level, assessment.Reasons, RiskHigh)
	}
	want := RiskReason{Code: RiskReasonCode("process_boundary"), Signal: SignalShellProcess, Path: "tools/procwrap/runner.py"}
	if !reflect.DeepEqual(assessment.Reasons, []RiskReason{want}) {
		t.Fatalf("Reasons = %#v, want %#v", assessment.Reasons, []RiskReason{want})
	}
	if assessment.DominantLens != "" {
		t.Fatalf("DominantLens = %q, want empty for high risk", assessment.DominantLens)
	}
}

func TestAssessSnapshotRiskDetectsProcessBoundaryAcrossSupportedLanguages(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
	}{
		{name: "Go exec", path: "tools/runner.go", content: "package tools\n\nfunc run() { exec(command) }\n"},
		{name: "Python subprocess", path: "tools/runner.py", content: "import subprocess\n\nsubprocess.run(argv)\n"},
		{name: "Ruby exec", path: "tools/runner.rb", content: "exec(command)\n"},
		{name: "Rust exec", path: "tools/runner.rs", content: "exec(command);\n"},
		{name: "C sharp Exec", path: "tools/runner.cs", content: "Exec(command);\n"},
		{name: "TypeScript exec", path: "tools/runner.ts", content: "exec(command);\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, tt.path, tt.content)
			snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{tt.path}})
			if err != nil {
				t.Fatal(err)
			}
			assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			want := RiskReason{Code: RiskReasonCode("process_boundary"), Signal: SignalShellProcess, Path: tt.path}
			if assessment.Level != RiskHigh || !reflect.DeepEqual(assessment.Reasons, []RiskReason{want}) {
				t.Fatalf("AssessSnapshotRisk(%s) = %#v, want high with %#v", tt.name, assessment, want)
			}
		})
	}
}

func TestAssessSnapshotRiskDetectsProcessBoundaryInTrackedEdits(t *testing.T) {
	t.Run("modification adds a spawn construct", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tools/runner.go", "package tools\n\nfunc Run() error {\n\treturn nil\n}\n")
		gitSnapshot(t, repo, "add", "--", "tools/runner.go")
		gitSnapshot(t, repo, "commit", "-m", "neutral runner")
		writeSnapshotFile(t, repo, "tools/runner.go", "package tools\n\nimport \"os/exec\"\n\nfunc Run() error {\n\treturn exec.Command(\"git\", \"status\").Run()\n}\n")
		assertProcessBoundaryHigh(t, repo, "tools/runner.go")
	})
	t.Run("deletion of a spawning module does not add a boundary", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tools/runner.py", "import subprocess\n\ndef run(argv):\n    return subprocess.run(argv)\n")
		gitSnapshot(t, repo, "add", "--", "tools/runner.py")
		gitSnapshot(t, repo, "commit", "-m", "spawning runner")
		gitSnapshot(t, repo, "rm", "--", "tools/runner.py")
		assertProcessBoundaryMedium(t, repo)
	})
}

func TestProcessBoundaryChangedLines(t *testing.T) {
	cases := []struct {
		name, path, before, after string
		high                      bool
	}{
		{"unchanged spawn", "tools/runner.go", "package tools\nfunc run() { exec.Command(\"git\") }\nvar x = 1\n", "package tools\nfunc run() { exec.Command(\"git\") }\nvar x = 2\n", false},
		{"added shell spawn", "tools/runner.ts", "const x = 1;\n", "const x = 1;\nexecSync(cmd, { shell: true });\n", true},
		{"edited spawn", "tools/runner.go", "package tools\nfunc run() { exec.Command(\"git\") }\n", "package tools\nfunc run() { exec.Command(\"sh\") }\n", true},
		{"removed spawn", "tools/runner.go", "package tools\nfunc run() { exec.Command(\"git\") }\n", "package tools\n", false},
		{"Go test", "tools/foo_test.go", "package tools\n", "package tools\nfunc run() { exec.Command(\"git\") }\n", false},
		{"TS test", "tools/runner.test.ts", "const x = 1;\n", "const x = 1;\nexecSync(cmd, { shell: true });\n", false},
		{"shebang retained", "tools/runner.py", "#!/usr/bin/env python3\nx = 1\n", "#!/usr/bin/env python3\nx = 2\n", true},
		// Regression for the "+++" misparse: the diff line for this added
		// content is "+++counter++; exec.Command(...)" (a '+' diff marker
		// followed by text that itself starts with "++"). A parser that treats
		// any line beginning with the three bytes "+++" as a file header,
		// instead of tracking the first "@@" hunk marker per section, would
		// silently skip this added spawn construct.
		{"added line whose text starts with ++", "tools/runner.go", "package tools\n", "package tools\n++counter++; exec.Command(\"sh\")\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, tc.path, tc.before)
			gitSnapshot(t, repo, "add", "--", tc.path)
			gitSnapshot(t, repo, "commit", "-m", "base")
			writeSnapshotFile(t, repo, tc.path, tc.after)
			if tc.high {
				assertProcessBoundaryHigh(t, repo, tc.path)
			} else {
				assertProcessBoundaryMedium(t, repo)
			}
		})
	}
}

// TestProcessBoundaryDiffScanIgnoresUserDiffPresentation keeps the added-line
// scan independent of repository diff presentation config: custom prefixes
// must not turn every change into a scan-limit high, and a textconv driver
// must not hide the frozen bytes that are actually committed, and neither
// may a binary or -diff attribute collapse the patch to "Binary files differ".
func TestProcessBoundaryDiffScanIgnoresUserDiffPresentation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		attributes string
		config     [][2]string
	}{
		{"noprefix", "*.go diff=hide\n", [][2]string{{"diff.noprefix", "true"}}},
		{"mnemonic prefix", "*.go diff=hide\n", [][2]string{{"diff.mnemonicPrefix", "true"}}},
		{"custom prefixes", "*.go diff=hide\n", [][2]string{{"diff.srcPrefix", "old/"}, {"diff.dstPrefix", "new/"}}},
		{"textconv hides content", "*.go diff=hide\n", [][2]string{{"diff.hide.textconv", "true"}}},
		{"binary attribute", "*.go binary\n", nil},
		{"no-diff attribute", "*.go -diff\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, ".gitattributes", tc.attributes)
			writeSnapshotFile(t, repo, "tools/runner.go", "package tools\n")
			gitSnapshot(t, repo, "add", "--", ".gitattributes", "tools/runner.go")
			gitSnapshot(t, repo, "commit", "-m", "base")
			for _, entry := range tc.config {
				gitSnapshot(t, repo, "config", entry[0], entry[1])
			}
			writeSnapshotFile(t, repo, "tools/runner.go", "package tools\nfunc run() { exec.Command(\"sh\") }\n")
			assertProcessBoundaryHigh(t, repo, "tools/runner.go")
		})
	}
}

// TestProcessBoundaryDiffScanFailsClosedOnOutputLimit exercises a case the
// blob-size scan alone cannot catch: the base and candidate blobs are tiny
// (well under the limit), but the diff *output* -- headers plus per-line
// +/- markers -- exceeds it. The diff scan must fail closed into the same
// process_scan_limit reason the blob scan uses, not return a bare error.
func TestProcessBoundaryDiffScanFailsClosedOnOutputLimit(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tools/f.go", "x=1\n")
	gitSnapshot(t, repo, "add", "--", "tools/f.go")
	gitSnapshot(t, repo, "commit", "-m", "base")
	writeSnapshotFile(t, repo, "tools/f.go", "x=2\n")
	original := processBoundaryScanByteLimit
	processBoundaryScanByteLimit = 20
	t.Cleanup(func() { processBoundaryScanByteLimit = original })
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("AssessSnapshotRisk() error = %v, want fail-closed process_scan_limit instead of an error", err)
	}
	if assessment.Level != RiskHigh || len(assessment.Reasons) != 1 || assessment.Reasons[0].Code != RiskReasonProcessScanLimit {
		t.Fatalf("assessment = %#v, want high with a single process_scan_limit reason", assessment)
	}
}

func assertProcessBoundaryMedium(t *testing.T, repo string) {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Level != RiskMedium {
		t.Fatalf("assessment = %#v, want medium", assessment)
	}
	for _, reason := range assessment.Reasons {
		if reason.Code == RiskReasonProcessBoundary {
			t.Fatalf("unexpected process boundary: %#v", assessment)
		}
	}
}

func assertProcessBoundaryHigh(t *testing.T, repo, path string) {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	want := RiskReason{Code: RiskReasonCode("process_boundary"), Signal: SignalShellProcess, Path: path}
	if assessment.Level != RiskHigh || !reflect.DeepEqual(assessment.Reasons, []RiskReason{want}) {
		t.Fatalf("AssessSnapshotRisk() = %#v, want high with %#v", assessment, want)
	}
}

func TestAssessSnapshotRiskProcessBoundaryNegativesAndDocumentedConservatism(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		want    RiskLevel
	}{
		{
			name:    "neutral module without spawn constructs stays medium",
			path:    "tools/helper.py",
			content: "def subprocess_helper(value):\n    return value\n",
			want:    RiskMedium,
		},
		{
			// Near-miss: "subprocess" as a prefix of a longer identifier must
			// not satisfy the standalone-token boundary and never escalates.
			name:    "subprocess prefix near-miss stays medium",
			path:    "tools/runner.py",
			content: "def run(argv):\n    return subprocessing.run(argv)\n",
			want:    RiskMedium,
		},
		{
			// Near-miss: "exec" as a prefix of a longer identifier must not
			// satisfy the standalone-token boundary and never escalates.
			name:    "exec prefix near-miss stays medium",
			path:    "tools/runner.ts",
			content: "function run(command) {\n  return executor.run(command);\n}\n",
			want:    RiskMedium,
		},
		{
			name:    "fixture directories stay outside semantic content scanning",
			path:    "fixtures/runner.py",
			content: "import subprocess\n\ndef run(argv):\n    return subprocess.run(argv)\n",
			want:    RiskMedium,
		},
		{
			name:    "documentation mentioning subprocess stays low",
			path:    "docs/spawning.md",
			content: "Call subprocess.run with an explicit argv list.\n",
			want:    RiskLow,
		},
		{
			// Documented conservative contract: pattern presence triggers the
			// signal even inside comments or strings. False positives only
			// widen review; false negatives would skip the mandated 4R set.
			name:    "comment-only mention still triggers the conservative signal",
			path:    "tools/notes.py",
			content: "# This module will wrap subprocess.run in a follow-up.\nVALUE = 1\n",
			want:    RiskHigh,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, tt.path, tt.content)
			snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{tt.path}})
			if err != nil {
				t.Fatal(err)
			}
			assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if assessment.Level != tt.want {
				t.Fatalf("AssessSnapshotRisk() = %q with reasons %#v, want %q", assessment.Level, assessment.Reasons, tt.want)
			}
		})
	}
}

func TestAssessSnapshotRiskProcessBoundaryIsDeterministic(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tools/procwrap/runner.py", buildIssue1438Wrapper(120))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"tools/procwrap/runner.py"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same immutable snapshot produced different assessments:\n%#v\n%#v", first, second)
	}
}
