package reviewtransaction

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestClassifyRiskUsesDeterministicFirstMatch(t *testing.T) {
	tests := []struct {
		name  string
		input RiskInput
		want  RiskLevel
	}{
		{name: "auth path is high", input: RiskInput{Stats: []DiffStat{{Path: "internal/auth/token.go", Additions: 1}}}, want: RiskHigh},
		{name: "update signal is high", input: RiskInput{Signals: []RiskSignal{SignalUpdate}}, want: RiskHigh},
		{name: "security signal is high", input: RiskInput{Signals: []RiskSignal{SignalSecurity}}, want: RiskHigh},
		{name: "payments signal is high", input: RiskInput{Signals: []RiskSignal{SignalPayments}}, want: RiskHigh},
		{name: "data exposure signal is high", input: RiskInput{Signals: []RiskSignal{SignalDataExposure}}, want: RiskHigh},
		{name: "data loss signal is high", input: RiskInput{Signals: []RiskSignal{SignalDataLoss}}, want: RiskHigh},
		{name: "permissions signal is high", input: RiskInput{Signals: []RiskSignal{SignalPermissions}}, want: RiskHigh},
		{name: "shell process signal is high", input: RiskInput{Signals: []RiskSignal{SignalShellProcess}}, want: RiskHigh},
		{
			name: "generated golden does not raise authored risk",
			input: RiskInput{
				OnlyNonExecutableChanges: true,
				Stats:                    []DiffStat{{Path: "testdata/golden/rendered.golden", Additions: 401, Generated: true}},
			},
			want: RiskLow,
		},
		{
			name:  "exactly 400 non executable lines is low",
			input: RiskInput{OnlyNonExecutableChanges: true, Stats: []DiffStat{{Path: "docs/guide.md", Additions: 400}}},
			want:  RiskLow,
		},
		{
			name: "large pure documentation is medium",
			input: RiskInput{
				OnlyNonExecutableChanges:     true,
				OnlyPureDocumentationChanges: true,
				Stats:                        []DiffStat{{Path: "docs/guide.md", Additions: 401}},
			},
			want: RiskMedium,
		},
		{
			name: "large operational markdown remains high",
			input: RiskInput{
				OnlyNonExecutableChanges: true,
				Stats:                    []DiffStat{{Path: "prompts/system.md", Additions: 401}},
			},
			want: RiskHigh,
		},
		{
			name: "large pure documentation with a semantic signal remains high",
			input: RiskInput{
				OnlyNonExecutableChanges:     true,
				OnlyPureDocumentationChanges: true,
				Stats:                        []DiffStat{{Path: "docs/security/guide.md", Additions: 401}},
			},
			want: RiskHigh,
		},
		{name: "large code remains high", input: RiskInput{Stats: []DiffStat{{Path: "internal/app.go", Additions: 401}}}, want: RiskHigh},
		{name: "configuration cannot be low", input: RiskInput{OnlyNonExecutableChanges: true, TouchesConfiguration: true}, want: RiskMedium},
		{name: "remaining executable change is medium", input: RiskInput{Stats: []DiffStat{{Path: "internal/ui/view.go", Additions: 1}}}, want: RiskMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ClassifyRisk(tt.input)
			if err != nil {
				t.Fatalf("ClassifyRisk() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ClassifyRisk() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNativeReviewAuthorityPathsEmitCanonicalAuthHotPath(t *testing.T) {
	tests := []struct {
		path      string
		authority bool
	}{
		{path: "internal/reviewtransaction/compact.go", authority: true},
		{path: "internal/reviewtransaction/transaction.go", authority: true},
		{path: "internal/reviewtransaction/compact_store.go", authority: true},
		{path: "internal/reviewtransaction/store.go", authority: true},
		{path: "internal/reviewtransaction/compact_gate.go", authority: true},
		{path: "internal/reviewtransaction/gate.go", authority: true},
		{path: "internal/reviewtransaction/compact_test.go"},
		{path: "docs/internal/reviewtransaction/compact.go"},
		{path: "internal/reviewtransaction/compact_chain.go"},
		{path: "internal/reviewtransaction/snapshot.go"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			signals := hotPathRiskSignals(tt.path)
			if got := len(signals) == 1 && signals[0] == SignalAuth; got != tt.authority {
				t.Fatalf("hotPathRiskSignals(%q) = %v, authority = %t", tt.path, signals, tt.authority)
			}
			if tt.authority {
				want := []RiskReason{{Code: RiskReasonHotPath, Signal: SignalAuth, Path: tt.path}}
				if got := deriveSnapshotRiskReasons([]DiffStat{{Path: tt.path, Additions: 1}}, 1); !reflect.DeepEqual(got, want) {
					t.Fatalf("deriveSnapshotRiskReasons(%q) = %#v, want %#v", tt.path, got, want)
				}
			}
			want := RiskMedium
			if tt.authority {
				want = RiskHigh
			}
			got, err := ClassifyRisk(RiskInput{Stats: []DiffStat{{Path: tt.path, Additions: 1}}})
			if err != nil || got != want {
				t.Fatalf("ClassifyRisk(%q) = %q, %v; want %q", tt.path, got, err, want)
			}
		})
	}
}

func TestPureDocumentationReviewPathExcludesOperationalMarkdown(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "docs/guide.md", want: true},
		{path: "README.md", want: true},
		{path: "book/chapter.mdx", want: true},
		{path: "docs/agents.md", want: false},
		{path: "prompts/system.md", want: false},
		{path: "docs/system-prompt.md", want: false},
		{path: "docs/agent-rules.md", want: false},
		{path: "docs/workflow.md", want: false},
		{path: "docs/runtime.md", want: false},
		{path: "internal/assets/claude/agents/review-risk.md", want: false},
		{path: "AGENTS.md", want: false},
		{path: "claude.md", want: false},
		{path: "Gemini.md", want: false},
		{path: "tools.md", want: false},
		{path: ".github/workflows/release.md", want: false},
		{path: "runtime/README.md", want: false},
		{path: "docs/diagram.svg", want: false},
		{path: "config/settings.yaml", want: false},
		{path: "internal/app.go", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isPureDocumentationReviewPath(tt.path); got != tt.want {
				t.Fatalf("isPureDocumentationReviewPath(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

func TestPureDocumentationReviewStatRequiresRegularNonExecutableFiles(t *testing.T) {
	for _, stat := range []DiffStat{
		{Path: "docs/guide.md", OldMode: "100644", NewMode: "120000"},
		{Path: "docs/guide.md", OldMode: "100755", NewMode: "100755"},
		{Path: "docs/guide.md", OldMode: "160000", NewMode: "160000"},
	} {
		if isPureDocumentationReviewStat(stat) {
			t.Fatalf("isPureDocumentationReviewStat(%#v) accepted an active file mode", stat)
		}
	}
	if !isPureDocumentationReviewStat(DiffStat{Path: "docs/guide.md", OldMode: "100644", NewMode: "100644"}) {
		t.Fatal("isPureDocumentationReviewStat() rejected a regular static document")
	}
}

func TestStaticMDXDocumentRejectsRuntimeSyntaxOutsideCodeExamples(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "plain prose", content: "# Chapter\n\nStatic prose.\n", want: true},
		{name: "fenced example", content: "```tsx\nexport const Example = <Widget />\n```\n", want: true},
		{name: "inline code example", content: "Use `<Widget value={1} />` here.\n", want: true},
		{name: "double-backtick inline code", content: "Use ``<Widget value={1} />`` here.\n", want: true},
		{name: "mismatched code span", content: "Use `<Widget />`` here.\n", want: false},
		{name: "escaped opening backtick", content: "Use \\`<Widget />` here.\n", want: false},
		{name: "escaped closing backtick", content: "Use `<Widget />\\` here.\n", want: false},
		{name: "ESM import", content: "import Widget from './widget'\n", want: false},
		{name: "comment-separated ESM import", content: "import/* runtime */Widget from './widget'\n", want: false},
		{name: "comment-prefixed ESM import", content: "/* runtime */ import Widget from './widget'\n", want: false},
		{name: "BOM-prefixed ESM import", content: "\uFEFFimport Widget from './widget'\n", want: false},
		{name: "multiline ESM import", content: "import\nWidget from './widget'\n", want: false},
		{name: "ESM export", content: "export const value = 1\n", want: false},
		{name: "JSX component", content: "Read <Widget /> next.\n", want: false},
		{name: "expression", content: "Current value: {value}\n", want: false},
		{name: "comment delimiter prose", content: "C comments begin with /* and end with */.\n", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStaticMDXDocument([]byte(tt.content)); got != tt.want {
				t.Fatalf("isStaticMDXDocument() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestLowRiskReviewPathPolicyUsesCanonicalPOSIXOperationalBoundaries(t *testing.T) {
	tests := []struct {
		name string
		stat DiffStat
		want bool
	}{
		{name: "ordinary Markdown", stat: DiffStat{Path: "docs/guide.md", Additions: 1, NewMode: "100644"}, want: true},
		{name: "active SVG", stat: DiffStat{Path: "docs/diagram.svg", Additions: 1, NewMode: "100644"}},
		{name: "AGENTS instructions", stat: DiffStat{Path: "AGENTS.md", Additions: 1, NewMode: "100644"}},
		{name: "nested CLAUDE instructions", stat: DiffStat{Path: "docs/CLAUDE.md", Additions: 1, NewMode: "100644"}},
		{name: "GEMINI instructions", stat: DiffStat{Path: "GEMINI.md", Additions: 1, NewMode: "100644"}},
		{name: "KIMI instructions", stat: DiffStat{Path: "KIMI.md", Additions: 1, NewMode: "100644"}},
		{name: "SKILL instructions", stat: DiffStat{Path: "skills/go/SKILL.md", Additions: 1, NewMode: "100644"}},
		{name: "copilot instructions", stat: DiffStat{Path: ".github/copilot-instructions.md", Additions: 1, NewMode: "100644"}},
		{name: "agent name", stat: DiffStat{Path: "docs/review-agent.md", Additions: 1, NewMode: "100644"}},
		{name: "skill path", stat: DiffStat{Path: "skills/review/guide.md", Additions: 1, NewMode: "100644"}},
		{name: "prompt path", stat: DiffStat{Path: "prompts/review.md", Additions: 1, NewMode: "100644"}},
		{name: "instruction name", stat: DiffStat{Path: "docs/review-instructions.md", Additions: 1, NewMode: "100644"}},
		{name: "orchestrator name", stat: DiffStat{Path: "docs/review-orchestrator.md", Additions: 1, NewMode: "100644"}},
		{name: "workflow path", stat: DiffStat{Path: "workflows/release.md", Additions: 1, NewMode: "100644"}},
		{name: "dot agent", stat: DiffStat{Path: ".agent/rules.md", Additions: 1, NewMode: "100644"}},
		{name: "dot agents", stat: DiffStat{Path: ".agents/reviewer.md", Additions: 1, NewMode: "100644"}},
		{name: "dot codex", stat: DiffStat{Path: ".codex/instructions.md", Additions: 1, NewMode: "100644"}},
		{name: "dot cursor", stat: DiffStat{Path: ".cursor/rules.md", Additions: 1, NewMode: "100644"}},
		{name: "Claude command", stat: DiffStat{Path: ".claude/commands/deploy.md", Additions: 1, NewMode: "100644"}},
		{name: "dot opencode", stat: DiffStat{Path: ".opencode/agents.md", Additions: 1, NewMode: "100644"}},
		{name: "GitHub agents", stat: DiffStat{Path: ".github/agents/reviewer.md", Additions: 1, NewMode: "100644"}},
		{name: "Windsurf workflows", stat: DiffStat{Path: ".windsurf/workflows/release.md", Additions: 1, NewMode: "100644"}},
		{name: "internal runtime", stat: DiffStat{Path: "internal/runtime/prompt.md", Additions: 1, NewMode: "100644"}},
		{name: "internal assets", stat: DiffStat{Path: "internal/assets/agent.md", Additions: 1, NewMode: "100644"}},
		{name: "internal templates", stat: DiffStat{Path: "internal/templates/review.md", Additions: 1, NewMode: "100644"}},
		{name: "runtime policy", stat: DiffStat{Path: "runtime/policy.md", Additions: 1, NewMode: "100644"}},
		{name: "OpenSpec", stat: DiffStat{Path: "openspec/changes/example/proposal.md", Additions: 1, NewMode: "100644"}},
		{name: "MDX", stat: DiffStat{Path: "docs/guide.mdx", Additions: 1, NewMode: "100644"}},
		{name: "source comment", stat: DiffStat{Path: "internal/view.go", Additions: 1, NewMode: "100644"}},
		{name: "binary Markdown", stat: DiffStat{Path: "docs/guide.md", Binary: true, NewMode: "100644"}},
		{name: "symlink Markdown", stat: DiffStat{Path: "docs/guide.md", Additions: 1, NewMode: "120000"}},
		{name: "gitlink Markdown", stat: DiffStat{Path: "docs/guide.md", Additions: 1, NewMode: "160000"}},
		{name: "executable Markdown", stat: DiffStat{Path: "docs/guide.md", Additions: 1, NewMode: "100755"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLowRiskNonExecutableStat(tt.stat); got != tt.want {
				t.Fatalf("isLowRiskNonExecutableStat(%#v) = %t, want %t", tt.stat, got, tt.want)
			}
		})
	}
}

func TestClassifyRiskPreservesFourHundredLineBoundary(t *testing.T) {
	for _, tt := range []struct {
		lines int
		want  RiskLevel
	}{{lines: 400, want: RiskLow}, {lines: 401, want: RiskHigh}} {
		got, err := ClassifyRisk(RiskInput{
			Stats:                    []DiffStat{{Path: "docs/ordinary-guide.md", Additions: tt.lines}},
			OnlyNonExecutableChanges: true,
		})
		if err != nil || got != tt.want {
			t.Fatalf("ClassifyRisk(%d Markdown lines) = %q, %v; want %q", tt.lines, got, err, tt.want)
		}
	}
}

func TestFallbackRiskReasonUsesTheSameLowRiskStatPolicy(t *testing.T) {
	for _, tt := range []struct {
		name string
		stat DiffStat
		want RiskReasonCode
	}{
		{name: "ordinary Markdown", stat: DiffStat{Path: "docs/guide.md", Additions: 1, NewMode: "100644"}, want: RiskReasonNonExecutableOnly},
		{name: "active SVG", stat: DiffStat{Path: "docs/diagram.svg", Additions: 1, NewMode: "100644"}, want: RiskReasonExecutableChange},
		{name: "binary Markdown", stat: DiffStat{Path: "docs/guide.md", Binary: true, NewMode: "100644"}, want: RiskReasonExecutableChange},
		{name: "mode-only Markdown", stat: DiffStat{Path: "docs/guide.md", ModeOnly: true, OldMode: "100644", NewMode: "100644"}, want: RiskReasonExecutableChange},
		{name: "operational Markdown", stat: DiffStat{Path: "AGENTS.md", Additions: 1, NewMode: "100644"}, want: RiskReasonExecutableChange},
		{name: "Claude command", stat: DiffStat{Path: ".claude/commands/deploy.md", Additions: 1, NewMode: "100644"}, want: RiskReasonExecutableChange},
		{name: "runtime policy", stat: DiffStat{Path: "runtime/policy.md", Additions: 1, NewMode: "100644"}, want: RiskReasonExecutableChange},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reasons := deriveSnapshotRiskReasons([]DiffStat{tt.stat}, tt.stat.Additions+tt.stat.Deletions)
			if len(reasons) != 1 || reasons[0].Code != tt.want {
				t.Fatalf("risk reasons = %#v, want %q", reasons, tt.want)
			}
		})
	}
}

func TestCountChangedLinesHasOneCrossAdapterRule(t *testing.T) {
	stats := []DiffStat{
		{Path: "generated/client.go", Additions: 250, Deletions: 50, Generated: true},
		{Path: "internal/x.go", Additions: 100, Deletions: 1},
		{Path: "image.bin", Additions: 999, Deletions: 999, Binary: true},
		{Path: "script.sh", ModeOnly: true},
		{Path: "renamed.txt"},
	}

	got, err := CountChangedLines(stats)
	if err != nil {
		t.Fatalf("CountChangedLines() error = %v", err)
	}
	if got != 401 {
		t.Fatalf("CountChangedLines() = %d, want 401", got)
	}
	if _, err := CountChangedLines([]DiffStat{{Path: "same.go", Additions: 1}, {Path: "same.go", Deletions: 1}}); err == nil {
		t.Fatal("CountChangedLines() accepted duplicate logical paths")
	}
}

func TestConfigurationReviewPathRecognizesDotEnvVariants(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: ".env", want: true},
		{path: "config/.env.production", want: true},
		{path: "config/env.example", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isConfigurationReviewPath(tt.path); got != tt.want {
				t.Fatalf("isConfigurationReviewPath(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

func TestDeriveSemanticRiskSignalsRecognizesEligibleServiceTokenPaths(t *testing.T) {
	tests := []struct {
		name  string
		stats []DiffStat
		want  []RiskSignal
	}{
		{name: "underscore Go source", stats: []DiffStat{{Path: "internal/identity/service_token.go", Additions: 1}}, want: []RiskSignal{SignalAuth}},
		{name: "hyphen TypeScript source", stats: []DiffStat{{Path: "internal/identity/service-token.ts", Additions: 1}}, want: []RiskSignal{SignalAuth}},
		{name: "configuration path", stats: []DiffStat{{Path: "config/service-token.yaml", Additions: 1}}, want: []RiskSignal{SignalAuth}},
		{name: "deletion-only source", stats: []DiffStat{{Path: "internal/identity/service-token.ts", Deletions: 1}}, want: []RiskSignal{SignalAuth}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveSemanticRiskSignals(tt.stats); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("deriveSemanticRiskSignals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeriveSemanticRiskSignalsRejectsIneligibleAndAmbiguousPaths(t *testing.T) {
	tests := []struct {
		name string
		stat DiffStat
	}{
		{name: "joined token", stat: DiffStat{Path: "internal/identity/servicetoken.go", Additions: 1}},
		{name: "cross segment token", stat: DiffStat{Path: "internal/service/token.go", Additions: 1}},
		{name: "zero change", stat: DiffStat{Path: "internal/identity/service-token.ts"}},
		{name: "binary", stat: DiffStat{Path: "internal/identity/service-token.ts", Additions: 1, Binary: true}},
		{name: "mode only", stat: DiffStat{Path: "internal/identity/service-token.ts", Additions: 1, ModeOnly: true}},
		{name: "generated golden", stat: DiffStat{Path: "testdata/golden/service-token.golden", Additions: 1, Generated: true}},
		{name: "fixture", stat: DiffStat{Path: "fixtures/service-token.ts", Additions: 1}},
		{name: "testdata", stat: DiffStat{Path: "testdata/service-token.ts", Additions: 1}},
		{name: "requirements prose", stat: DiffStat{Path: "service-token-requirements.txt", Additions: 1}},
		{name: "CMake prose", stat: DiffStat{Path: "service-token-CMakeLists.txt", Additions: 1}},
		{name: "executable MDX", stat: DiffStat{Path: "service-token.mdx", Additions: 1}},
		{name: "README shell", stat: DiffStat{Path: "README-service-token.sh", Additions: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveSemanticRiskSignals([]DiffStat{tt.stat}); len(got) != 0 {
				t.Fatalf("deriveSemanticRiskSignals() = %v, want no signals", got)
			}
		})
	}
}

func TestClassifySnapshotRiskDerivesAuthAfterCountingCanonicalStats(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "neutral/service-token.ts", "export const token = 'candidate'\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"neutral/service-token.ts"}})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil || risk != RiskHigh || lines >= LargeChangeLines {
		t.Fatalf("ClassifySnapshotRisk() = %q, %d, %v; want high below %d lines", risk, lines, err, LargeChangeLines)
	}
}

func TestAssessSnapshotRiskDerivesProvableShellAndExecutableModeReasons(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, repo string) Target
		want  RiskReason
		lines int
	}{
		{
			name: "eligible shell source",
			setup: func(t *testing.T, repo string) Target {
				writeSnapshotFile(t, repo, "tools/run.sh", "printf '%s\\n' safe\n")
				return Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"tools/run.sh"}}
			},
			want:  RiskReason{Code: RiskReasonShellSource, Signal: SignalShellProcess, Path: "tools/run.sh"},
			lines: 1,
		},
		{
			name: "GitHub workflow YAML",
			setup: func(t *testing.T, repo string) Target {
				writeSnapshotFile(t, repo, ".github/workflows/ci.yml", "jobs: {}\n")
				return Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{".github/workflows/ci.yml"}}
			},
			want:  RiskReason{Code: RiskReasonShellSource, Signal: SignalShellProcess, Path: ".github/workflows/ci.yml"},
			lines: 1,
		},
		{
			name: "GitHub workflow YAML long extension",
			setup: func(t *testing.T, repo string) Target {
				writeSnapshotFile(t, repo, ".github/workflows/ci.yaml", "jobs: {}\n")
				return Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{".github/workflows/ci.yaml"}}
			},
			want:  RiskReason{Code: RiskReasonShellSource, Signal: SignalShellProcess, Path: ".github/workflows/ci.yaml"},
			lines: 1,
		},
		{
			name: "executable mode change",
			setup: func(t *testing.T, repo string) Target {
				gitSnapshot(t, repo, "config", "core.filemode", "true")
				if err := os.Chmod(filepath.Join(repo, "tracked.txt"), 0o755); err != nil {
					t.Fatal(err)
				}
				return Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}
			},
			want: RiskReason{
				Code: RiskReasonExecutableMode, Signal: SignalPermissions, Path: "tracked.txt",
				OldMode: "100644", NewMode: "100755",
			},
			lines: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tt.name == "executable mode change" {
				t.Skip("Git worktree executable-bit transitions are POSIX-only")
			}
			repo := initSnapshotRepo(t)
			snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), tt.setup(t, repo))
			if err != nil {
				t.Fatal(err)
			}
			assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if assessment.Level != RiskHigh || assessment.ChangedLines != tt.lines || !reflect.DeepEqual(assessment.Reasons, []RiskReason{tt.want}) {
				t.Fatalf("AssessSnapshotRisk() = %#v, want high/%d/%#v", assessment, tt.lines, []RiskReason{tt.want})
			}
		})
	}
}

func TestProvableRiskReasonsRejectNearMissesAndFilenameGuesses(t *testing.T) {
	nearMisses := []DiffStat{
		{Path: "docs/run.sh", Additions: 1, OldMode: "000000", NewMode: "100644"},
		{Path: "docs/workflow.yml", Additions: 1, OldMode: "000000", NewMode: "100644"},
		{Path: "config/app.yaml", Additions: 1, OldMode: "000000", NewMode: "100644"},
		{Path: "README-run.sh", Additions: 1, OldMode: "000000", NewMode: "100644"},
		{Path: "fixtures/run.sh", Additions: 1, OldMode: "000000", NewMode: "100644"},
		{Path: "tools/run.sh.txt", Additions: 1, OldMode: "000000", NewMode: "100644"},
		{Path: "tools/already-executable.txt", Additions: 1, OldMode: "100755", NewMode: "100755"},
		{Path: "internal/data-exposure.go", Additions: 1, OldMode: "000000", NewMode: "100644"},
		{Path: "internal/data-loss.go", Additions: 1, OldMode: "000000", NewMode: "100644"},
	}
	for _, stat := range nearMisses {
		t.Run(stat.Path, func(t *testing.T) {
			for _, reason := range deriveSnapshotRiskReasons([]DiffStat{stat}, 1) {
				if reason.Signal == SignalShellProcess || reason.Signal == SignalPermissions || reason.Signal == SignalDataExposure || reason.Signal == SignalDataLoss {
					t.Fatalf("deriveSnapshotRiskReasons(%#v) guessed unsafe reason %#v", stat, reason)
				}
			}
		})
	}
}

func TestClassifySnapshotRiskRejectsMalformedStatsBeforeSemanticDerivation(t *testing.T) {
	if _, err := CountChangedLines([]DiffStat{{Path: "neutral/../service-token.ts", Additions: 1}}); err == nil {
		t.Fatal("CountChangedLines() accepted noncanonical path")
	}
}

func TestCorrectionBudgetBoundaries(t *testing.T) {
	tests := []struct {
		original int
		want     int
	}{
		{original: 0, want: 0}, {original: 1, want: 1}, {original: 2, want: 1},
		{original: 196, want: 98}, {original: 399, want: 200}, {original: 400, want: 200},
		{original: 401, want: 200}, {original: 867, want: 200}, {original: math.MaxInt, want: 200},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d original lines", tt.original), func(t *testing.T) {
			got, err := CorrectionBudget(tt.original)
			if err != nil || got != tt.want {
				t.Fatalf("CorrectionBudget(%d) = %d, %v; want %d", tt.original, got, err, tt.want)
			}
		})
	}
	if _, err := CorrectionBudget(-1); err == nil {
		t.Fatal("CorrectionBudget() accepted negative original lines")
	}
}
