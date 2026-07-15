package reviewtransaction

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
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
