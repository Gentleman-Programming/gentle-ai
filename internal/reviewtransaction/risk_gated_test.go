package reviewtransaction

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

var reviewAuthorityPaths = []string{
	"internal/reviewtransaction/compact.go", "internal/reviewtransaction/transaction.go",
	"internal/reviewtransaction/compact_store.go", "internal/reviewtransaction/store.go",
	"internal/reviewtransaction/compact_gate.go", "internal/reviewtransaction/gate.go",
}

// assertNameOnlyMedium proves a path carries no high-risk reason and that the
// classifier keeps it at the consolidated one-lens review.
func assertNameOnlyMedium(t *testing.T, stats ...DiffStat) {
	t.Helper()
	for _, reason := range deriveSnapshotRiskReasons(stats, nil) {
		if reason.Signal != "" {
			t.Fatalf("deriveSnapshotRiskReasons(%#v) = high reason %#v, want none", stats, reason)
		}
	}
	got, err := ClassifyRisk(RiskInput{Stats: stats})
	if err != nil || got != RiskMedium {
		t.Fatalf("ClassifyRisk(%#v) = %q, %v; want %q", stats, got, err, RiskMedium)
	}
}

// TestUpdatePathTokenIsNotHighRisk is S1: the update token alone named no
// finding in the mined lineages, so it no longer selects focused 4R.
func TestUpdatePathTokenIsNotHighRisk(t *testing.T) {
	t.Parallel()
	for _, logicalPath := range []string{"internal/update/update.go", "cmd/self-update.go", "internal/app/update_check.go"} {
		t.Run(logicalPath, func(t *testing.T) {
			t.Parallel()
			if signals := hotPathRiskSignals(logicalPath); len(signals) != 0 {
				t.Fatalf("hotPathRiskSignals(%q) = %v, want none", logicalPath, signals)
			}
			assertNameOnlyMedium(t, DiffStat{Path: logicalPath, Additions: 1})
		})
	}
}

// TestReviewAuthorityFilesAreNotHighByName is S2: touching an RDD authority
// file is a consolidated review unless the change itself is large.
func TestReviewAuthorityFilesAreNotHighByName(t *testing.T) {
	t.Parallel()
	for _, logicalPath := range reviewAuthorityPaths {
		t.Run(logicalPath, func(t *testing.T) {
			t.Parallel()
			if signals := hotPathRiskSignals(logicalPath); len(signals) != 0 {
				t.Fatalf("hotPathRiskSignals(%q) = %v, want none", logicalPath, signals)
			}
			assertNameOnlyMedium(t, DiffStat{Path: logicalPath, Additions: 1})
			assertNameOnlyMedium(t, DiffStat{Path: logicalPath, Additions: LargeChangeLines - 1})
		})
	}
}

// TestLargeReviewAuthorityChangeIsHighBySize is S3: only lines changed inside
// the authority files count toward the boundary, and reaching it selects the
// full lens set under the hot-path/auth reason negotiated START already
// accepts. Canonical reasons keep one entry per code and signal, so several
// authority files surface as the first one in path order.
func TestLargeReviewAuthorityChangeIsHighBySize(t *testing.T) {
	t.Parallel()
	want := RiskReason{Code: RiskReasonHotPath, Signal: SignalAuth, Path: "internal/reviewtransaction/gate.go"}
	tests := []struct {
		name  string
		stats []DiffStat
		high  bool
	}{
		{name: "one file at the boundary", stats: []DiffStat{{Path: "internal/reviewtransaction/gate.go", Additions: 200, Deletions: 200}}, high: true},
		{name: "spread across authority files", stats: []DiffStat{
			{Path: "internal/reviewtransaction/gate.go", Additions: 150},
			{Path: "internal/reviewtransaction/store.go", Additions: 100, Deletions: 50},
			{Path: "internal/reviewtransaction/transaction.go", Deletions: 100},
		}, high: true},
		{name: "one line below the boundary", stats: []DiffStat{
			{Path: "internal/reviewtransaction/gate.go", Additions: 200},
			{Path: "internal/reviewtransaction/store.go", Deletions: LargeChangeLines - 201},
		}},
		{name: "non-authority lines do not count", stats: []DiffStat{
			{Path: "internal/reviewtransaction/gate.go", Additions: 1},
			{Path: "internal/reviewtransaction/snapshot.go", Additions: 5_000},
		}},
		{name: "binary and mode-only entries do not count", stats: []DiffStat{
			{Path: "internal/reviewtransaction/gate.go", Additions: 500, Binary: true},
			{Path: "internal/reviewtransaction/store.go", Additions: 500, ModeOnly: true},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !tt.high {
				assertNameOnlyMedium(t, tt.stats...)
				return
			}
			if got := deriveSnapshotRiskReasons(tt.stats, nil); !reflect.DeepEqual(got, []RiskReason{want}) {
				t.Fatalf("deriveSnapshotRiskReasons() = %#v, want %#v", got, []RiskReason{want})
			}
			level, err := ClassifyRisk(RiskInput{Stats: tt.stats})
			if err != nil || level != RiskHigh {
				t.Fatalf("ClassifyRisk() = %q, %v; want %q", level, err, RiskHigh)
			}
			lenses, err := SelectReviewLenses(RiskAssessment{Level: level}, "risk")
			if err != nil || !reflect.DeepEqual(lenses, supportedLenses) {
				t.Fatalf("SelectReviewLenses() = %v, %v; want %v", lenses, err, supportedLenses)
			}
		})
	}
}

// TestAssessSnapshotRiskGatesAuthorityFilesBySize runs S2/S3 through the
// immutable snapshot path so the published reasons explain the tier.
func TestAssessSnapshotRiskGatesAuthorityFilesBySize(t *testing.T) {
	t.Parallel()
	authorityFile := func(lines int) string {
		rendered := []string{"package reviewtransaction"}
		for index := len(rendered); index < lines; index++ {
			rendered = append(rendered, fmt.Sprintf("var authorityValue%03d = %d", index, index))
		}
		return strings.Join(rendered, "\n") + "\n"
	}
	t.Run("below the boundary", func(t *testing.T) {
		t.Parallel()
		assessment := assessUntrackedCandidate(t, candidateFile{path: "internal/reviewtransaction/gate.go", content: authorityFile(LargeChangeLines - 1)})
		if assessment.Level != RiskMedium {
			t.Fatalf("AssessSnapshotRisk() = %#v, want medium", assessment)
		}
	})
	t.Run("at the boundary", func(t *testing.T) {
		t.Parallel()
		assessment := assessUntrackedCandidate(t, candidateFile{path: "internal/reviewtransaction/gate.go", content: authorityFile(LargeChangeLines)})
		want := []RiskReason{{Code: RiskReasonHotPath, Signal: SignalAuth, Path: "internal/reviewtransaction/gate.go"}}
		if assessment.Level != RiskHigh || !reflect.DeepEqual(assessment.Reasons, want) {
			t.Fatalf("AssessSnapshotRisk() = %#v, want high with %#v", assessment, want)
		}
	})
}

// TestProcessSpawnSkipsCommentOnlyAddedLines is S4: a spawn word inside an
// added comment is a mention, not a process boundary.
func TestProcessSpawnSkipsCommentOnlyAddedLines(t *testing.T) {
	tests := []struct {
		name, path, content string
		high                bool
	}{
		{name: "Python hash comment", path: "tools/notes.py", content: "# wrap subprocess.run later\nVALUE = 1\n"},
		{name: "Go line comment", path: "tools/notes.go", content: "package tools\n\n// exec.Command is wired later\nvar value = 1\n"},
		{name: "block comment body", path: "tools/notes.ts", content: "/*\n * child_process stays out of this module\n */\nconst value = 1;\n"},
		{name: "real spawn after a comment", path: "tools/run.py", content: "# spawn the tool\nimport subprocess\nsubprocess.run(argv)\n", high: true},
		{name: "trailing comment on code", path: "tools/run.go", content: "package tools\n\nvar run = exec // spawns\n", high: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := assessUntrackedCandidate(t, candidateFile{path: tt.path, content: tt.content})
			want := RiskMedium
			if tt.high {
				want = RiskHigh
			}
			if assessment.Level != want {
				t.Fatalf("AssessSnapshotRisk() = %q with reasons %#v, want %q", assessment.Level, assessment.Reasons, want)
			}
		})
	}
}

// TestInterpreterDirectiveCountsOnlyInTheCandidateTree is S5: a shebang that
// exists only on the base side is not a process boundary of the candidate.
func TestInterpreterDirectiveCountsOnlyInTheCandidateTree(t *testing.T) {
	commitBase := func(t *testing.T, logicalPath, content string) string {
		t.Helper()
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, logicalPath, content)
		gitSnapshot(t, repo, "add", "--", logicalPath)
		gitSnapshot(t, repo, "commit", "-m", "base")
		return repo
	}
	t.Run("shebang removed from the candidate", func(t *testing.T) {
		repo := commitBase(t, "tools/runner.py", "#!/usr/bin/env python3\nx = 1\n")
		writeSnapshotFile(t, repo, "tools/runner.py", "x = 1\n")
		assertProcessBoundaryMedium(t, repo)
	})
	t.Run("script with a shebang deleted", func(t *testing.T) {
		repo := commitBase(t, "tools/runner.py", "#!/usr/bin/env python3\nx = 1\n")
		gitSnapshot(t, repo, "rm", "--", "tools/runner.py")
		assertProcessBoundaryMedium(t, repo)
	})
	t.Run("shebang added to the candidate", func(t *testing.T) {
		repo := commitBase(t, "tools/runner.py", "x = 1\n")
		writeSnapshotFile(t, repo, "tools/runner.py", "#!/usr/bin/env python3\nx = 1\n")
		assertProcessBoundaryHigh(t, repo, "tools/runner.py")
	})
}

// TestPathTokensInTestFilesAreNotHighRisk is S6: a sensitive token that only
// names test files no longer selects focused 4R, while the same token on any
// non-test path still does.
func TestPathTokensInTestFilesAreNotHighRisk(t *testing.T) {
	t.Parallel()
	for _, logicalPath := range []string{
		"internal/auth/token_test.go",
		"internal/security/check_test.go",
		"src/webhook.test.ts",
		"tests/payments/charge.py",
		"internal/identity/service_token_test.go",
		"spec/auth/login.spec.ts",
	} {
		t.Run(logicalPath, func(t *testing.T) {
			t.Parallel()
			assertNameOnlyMedium(t, DiffStat{Path: logicalPath, Additions: 1})
		})
	}
	t.Run("a non-test path still escalates", func(t *testing.T) {
		t.Parallel()
		stats := []DiffStat{{Path: "internal/auth/token.go", Additions: 1}, {Path: "internal/auth/token_test.go", Additions: 1}}
		want := []RiskReason{{Code: RiskReasonHotPath, Signal: SignalAuth, Path: "internal/auth/token.go"}}
		if got := deriveSnapshotRiskReasons(stats, nil); !reflect.DeepEqual(got, want) {
			t.Fatalf("deriveSnapshotRiskReasons() = %#v, want %#v", got, want)
		}
		if level, err := ClassifyRisk(RiskInput{Stats: stats}); err != nil || level != RiskHigh {
			t.Fatalf("ClassifyRisk() = %q, %v; want %q", level, err, RiskHigh)
		}
	})
}

// TestUnchangedNameAndModeSignalsStayHigh is the S7 guard for the
// name- and mode-derived evidence this narrowing must not touch.
func TestUnchangedNameAndModeSignalsStayHigh(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		stat DiffStat
		want RiskReason
	}{
		{name: "auth", stat: DiffStat{Path: "internal/auth/token.go", Additions: 1}, want: RiskReason{Code: RiskReasonHotPath, Signal: SignalAuth, Path: "internal/auth/token.go"}},
		{name: "security", stat: DiffStat{Path: "internal/security/check.go", Additions: 1}, want: RiskReason{Code: RiskReasonHotPath, Signal: SignalSecurity, Path: "internal/security/check.go"}},
		{name: "webhook", stat: DiffStat{Path: "internal/webhook/handler.go", Additions: 1}, want: RiskReason{Code: RiskReasonHotPath, Signal: SignalSecurity, Path: "internal/webhook/handler.go"}},
		{name: "payments", stat: DiffStat{Path: "internal/payments/charge.go", Additions: 1}, want: RiskReason{Code: RiskReasonHotPath, Signal: SignalPayments, Path: "internal/payments/charge.go"}},
		{name: "service token", stat: DiffStat{Path: "internal/identity/service_token.go", Additions: 1}, want: RiskReason{Code: RiskReasonServiceToken, Signal: SignalAuth, Path: "internal/identity/service_token.go"}},
		{name: "workflow", stat: DiffStat{Path: ".github/workflows/ci.yml", Additions: 1}, want: RiskReason{Code: RiskReasonShellSource, Signal: SignalShellProcess, Path: ".github/workflows/ci.yml"}},
		{name: "shell script", stat: DiffStat{Path: "tools/run.sh", Additions: 1}, want: RiskReason{Code: RiskReasonShellSource, Signal: SignalShellProcess, Path: "tools/run.sh"}},
		{name: "executable mode", stat: DiffStat{Path: "tools/run.txt", ModeOnly: true, OldMode: "100644", NewMode: "100755"}, want: RiskReason{Code: RiskReasonExecutableMode, Signal: SignalPermissions, Path: "tools/run.txt", OldMode: "100644", NewMode: "100755"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reasons := deriveSnapshotRiskReasons([]DiffStat{tt.stat}, nil)
			if !reflect.DeepEqual(reasons, []RiskReason{tt.want}) {
				t.Fatalf("deriveSnapshotRiskReasons() = %#v, want %#v", reasons, []RiskReason{tt.want})
			}
			level, err := ClassifyRisk(RiskInput{Stats: []DiffStat{tt.stat}, Signals: riskSignalsFromReasons(reasons)})
			if err != nil || level != RiskHigh {
				t.Fatalf("ClassifyRisk() = %q, %v; want %q", level, err, RiskHigh)
			}
		})
	}
}

// TestProcessSpawnCommentSkippingIsLanguageAware is the S4 correction: a
// comment marker only counts in a language that uses it, and code after a
// closed block comment is still code. Each reproduction was a real spawn that
// prefix-only skipping dropped to medium.
func TestProcessSpawnCommentSkippingIsLanguageAware(t *testing.T) {
	t.Run("Go dereference of a spawned command", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tools/runner.go", "package tools\n\nfunc f(cmd *exec.Cmd) {\n}\n")
		gitSnapshot(t, repo, "add", "--", "tools/runner.go")
		gitSnapshot(t, repo, "commit", "-m", "base")
		writeSnapshotFile(t, repo, "tools/runner.go", "package tools\n\nfunc f(cmd *exec.Cmd) {\n\t*cmd = *exec.Command(\"sh\")\n}\n")
		assertProcessBoundaryHigh(t, repo, "tools/runner.go")
	})
	tests := []struct {
		name, path, content string
		high                bool
	}{
		{name: "Go code after a closed block comment", path: "tools/runner.go", content: "package tools\n\n/* run it */ var cmd = exec.Command(\"sh\")\n", high: true},
		{name: "C preprocessor macro", path: "src/open.c", content: "#define OPEN(c) popen(c, \"r\")\n", high: true},
		{name: "TypeScript private field", path: "src/runner.ts", content: "class Runner {\n  #run = () => require('child_process')\n}\n", high: true},
		{name: "Rust attribute", path: "src/run.rs", content: "#[allow(unused)] fn g() { unsafe { libc::popen(cmd, mode) } }\n", high: true},
		{name: "JavaScript decrement", path: "src/run.js", content: "let i = 1;\n--i; execSync('ls')\n", high: true},
		{name: "Go line comment", path: "tools/a.go", content: "package tools\n\n// exec.Command(\"sh\")\n"},
		{name: "TypeScript line comment", path: "src/a.ts", content: "// exec(command)\nconst value = 1;\n"},
		{name: "JavaScript line comment", path: "src/a.js", content: "// execSync('ls')\nconst value = 1;\n"},
		{name: "Python hash comment", path: "tools/a.py", content: "# subprocess.run(argv)\nVALUE = 1\n"},
		{name: "YAML hash comment", path: "config/app.yaml", content: "# subprocess.run(argv)\nkey: value\n"},
		{name: "requirements hash comment", path: "requirements-dev.txt", content: "# subprocess.run(argv)\nrequests==2.31.0\n"},
		{name: "Dockerfile hash comment", path: "Dockerfile", content: "# subprocess.run(argv)\nFROM scratch\n"},
		{name: "Go block comment only", path: "tools/b.go", content: "package tools\n\n/* exec.Command(\"sh\") */\n"},
		{name: "Go block comment continuation", path: "tools/c.go", content: "package tools\n\n/*\n * exec.Command(\"sh\")\n */\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := assessUntrackedCandidate(t, candidateFile{path: tt.path, content: tt.content})
			want := RiskMedium
			if tt.high {
				want = RiskHigh
			}
			if assessment.Level != want {
				t.Fatalf("AssessSnapshotRisk() = %q with reasons %#v, want %q", assessment.Level, assessment.Reasons, want)
			}
		})
	}
}

// TestCommentOnlySourceLineUsesTheFileLanguage pins the per-language comment
// syntax, including languages the spawn scan does not read today.
func TestCommentOnlySourceLineUsesTheFileLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path, line string
		want       bool
	}{
		{"tools/a.go", `// exec.Command("sh")`, true},
		{"tools/a.go", `/* exec.Command("sh") */`, true},
		{"tools/a.go", `/* exec.Command("sh")`, true},
		{"tools/a.go", ` * exec.Command("sh")`, true},
		{"tools/a.go", ` *`, true},
		{"tools/a.go", ` */`, true},
		{"tools/a.go", `*cmd = *exec.Command("sh")`, false},
		{"tools/a.go", `*/ exec.Command("sh")`, false},
		{"tools/a.go", `/* run it */ var cmd = exec.Command("sh")`, false},
		{"tools/a.go", `# exec.Command("sh")`, false},
		{"src/open.c", `#define OPEN(c) popen(c, "r")`, false},
		{"src/runner.ts", `#run = () => require('child_process')`, false},
		{"src/run.rs", `#[allow(unused)] fn g() {}`, false},
		{"src/run.js", `--i; execSync('ls')`, false},
		{"tools/a.py", `# subprocess.run(argv)`, true},
		{"tools/a.sh", `# exec "$@"`, true},
		{"tools/a.rb", `# exec(command)`, true},
		{"config/app.yml", `# subprocess.run(argv)`, true},
		{"requirements.txt", `# subprocess.run(argv)`, true},
		{"Dockerfile", `# subprocess.run(argv)`, true},
		{"Makefile", `# exec`, true},
		{"db/query.sql", `-- exec`, true},
		{"scripts/a.lua", `-- os.execute`, true},
		{"tools/a.py", `-- exec`, false},
		{"notes/a.txt", `# subprocess.run(argv)`, false},
		{"notes/unknown", `// exec`, false},
	}
	for _, tt := range tests {
		if got := isCommentOnlySourceLine(tt.path, tt.line); got != tt.want {
			t.Errorf("isCommentOnlySourceLine(%q, %q) = %t, want %t", tt.path, tt.line, got, tt.want)
		}
	}
}
