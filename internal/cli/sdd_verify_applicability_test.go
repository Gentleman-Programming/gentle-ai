package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	passive = reviewtransaction.VerificationContentPassive
	active  = reviewtransaction.VerificationContentActive
	unknown = reviewtransaction.VerificationContentUnknown
)

func fact(path string, activity reviewtransaction.VerificationContentActivity) reviewtransaction.CandidateContentFact {
	return reviewtransaction.CandidateContentFact{Path: path, Activity: activity}
}

// TestDecideSDDVerifyApplicabilityFailsClosed pins the SDD-owned policy. Only a
// non-empty, fully passive candidate may reach not_required; every other shape
// keeps the independent verifier mandatory.
func TestDecideSDDVerifyApplicabilityFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		facts    []reviewtransaction.CandidateContentFact
		decision string
		reason   string
		deciding []string
	}{
		{
			name: "empty candidate proves nothing passive", facts: nil,
			decision: SDDVerifyRequired, reason: SDDVerifyReasonEmptyCandidate,
		},
		{
			name:  "fully passive candidate",
			facts: []reviewtransaction.CandidateContentFact{fact("docs/b.md", passive), fact("docs/a.md", passive)},
			// Covered paths are sorted so the same candidate always reports
			// the same substitute proof regardless of diff ordering.
			decision: SDDVerifyNotRequired, reason: SDDVerifyReasonPassiveCandidate,
			deciding: []string{"docs/a.md", "docs/b.md"},
		},
		{
			name:     "single active path",
			facts:    []reviewtransaction.CandidateContentFact{fact("internal/run.go", active)},
			decision: SDDVerifyRequired, reason: SDDVerifyReasonActiveContent,
			deciding: []string{"internal/run.go"},
		},
		{
			name:     "mixed passive and active",
			facts:    []reviewtransaction.CandidateContentFact{fact("docs/a.md", passive), fact("internal/run.go", active)},
			decision: SDDVerifyRequired, reason: SDDVerifyReasonActiveContent,
			deciding: []string{"internal/run.go"},
		},
		{
			name:     "unknown content",
			facts:    []reviewtransaction.CandidateContentFact{fact("docs/a.md", passive), fact("assets/blob.bin", unknown)},
			decision: SDDVerifyRequired, reason: SDDVerifyReasonUnknownContent,
			deciding: []string{"assets/blob.bin"},
		},
		{
			name:  "active outranks unknown in the reported reason",
			facts: []reviewtransaction.CandidateContentFact{fact("assets/blob.bin", unknown), fact("internal/run.go", active)},
			// Active content is the stronger, non-speculative statement, so it
			// is what the decision explains.
			decision: SDDVerifyRequired, reason: SDDVerifyReasonActiveContent,
			deciding: []string{"internal/run.go"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, reason, deciding := decideSDDVerifyApplicability(test.facts)
			if decision != test.decision || reason != test.reason || !reflect.DeepEqual(deciding, test.deciding) {
				t.Fatalf("decideSDDVerifyApplicability() = %q, %q, %#v; want %q, %q, %#v",
					decision, reason, deciding, test.decision, test.reason, test.deciding)
			}
		})
	}
}

// TestSDDVerifyApplicabilityEvidenceGoalAccompaniesEveryRequiredReason keeps a
// required decision explainable: a consumer must never see "verify is required"
// without being told what evidence would satisfy it.
func TestSDDVerifyApplicabilityEvidenceGoalAccompaniesEveryRequiredReason(t *testing.T) {
	for _, reason := range []string{SDDVerifyReasonEmptyCandidate, SDDVerifyReasonActiveContent, SDDVerifyReasonUnknownContent} {
		if evidenceGoalForSDDVerify(reason) == "" {
			t.Fatalf("required reason %q has no evidence goal", reason)
		}
	}
	if evidenceGoalForSDDVerify(SDDVerifyReasonPassiveCandidate) != "" {
		t.Fatal("a not_required decision must not carry an evidence goal")
	}
}

func runApplicability(t *testing.T, args ...string) SDDVerifyApplicability {
	t.Helper()
	var stdout bytes.Buffer
	if err := runSDDVerifyApplicability(context.Background(), args, &stdout); err != nil {
		t.Fatalf("runSDDVerifyApplicability(%v): %v", args, err)
	}
	var result SDDVerifyApplicability
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result %q: %v", stdout.String(), err)
	}
	if result.Schema != SDDVerifyApplicabilitySchema {
		t.Fatalf("schema = %q, want %q", result.Schema, SDDVerifyApplicabilitySchema)
	}
	if result.CandidateTree == "" || result.PathsDigest == "" {
		t.Fatalf("result is not bound to a candidate: %#v", result)
	}
	return result
}

func writeApplicabilityFile(t *testing.T, repo, logicalPath, content string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(logicalPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSDDVerifyApplicabilityClassifiesRealCandidates exercises the command
// against real repository content rather than synthesized facts, so the
// classification wiring is covered end to end.
func TestSDDVerifyApplicabilityClassifiesRealCandidates(t *testing.T) {
	t.Run("passive documentation does not require verification", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeApplicabilityFile(t, repo, "docs/guide.md", "# Guide\n\nPlain prose.\n")
		result := runApplicability(t, "--cwd", repo, "--intended-untracked", "docs/guide.md")
		if result.Decision != SDDVerifyNotRequired || result.Reason != SDDVerifyReasonPassiveCandidate {
			t.Fatalf("decision = %q (%q), want %q", result.Decision, result.Reason, SDDVerifyNotRequired)
		}
		if !reflect.DeepEqual(result.CoveredPaths, []string{"docs/guide.md"}) {
			t.Fatalf("covered paths = %#v", result.CoveredPaths)
		}
		if result.EvidenceGoal != "" {
			t.Fatalf("not_required carried an evidence goal: %q", result.EvidenceGoal)
		}
	})

	t.Run("active source requires verification", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeApplicabilityFile(t, repo, "internal/run.go", "package internal\n\nfunc Run() {}\n")
		result := runApplicability(t, "--cwd", repo, "--intended-untracked", "internal/run.go")
		if result.Decision != SDDVerifyRequired || result.Reason != SDDVerifyReasonActiveContent {
			t.Fatalf("decision = %q (%q), want %q", result.Decision, result.Reason, SDDVerifyRequired)
		}
		if result.EvidenceGoal == "" {
			t.Fatal("required decision carried no evidence goal")
		}
	})

	t.Run("caller operational path overrides passive content", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeApplicabilityFile(t, repo, "docs/guide.md", "# Guide\n\nPlain prose.\n")
		result := runApplicability(t, "--cwd", repo,
			"--intended-untracked", "docs/guide.md", "--operational-path", "docs/guide.md")
		if result.Decision != SDDVerifyRequired || result.Reason != SDDVerifyReasonActiveContent {
			t.Fatalf("decision = %q (%q), want %q", result.Decision, result.Reason, SDDVerifyRequired)
		}
	})

	t.Run("empty candidate fails closed", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		result := runApplicability(t, "--cwd", repo)
		if result.Decision != SDDVerifyRequired || result.Reason != SDDVerifyReasonEmptyCandidate {
			t.Fatalf("decision = %q (%q), want %q", result.Decision, result.Reason, SDDVerifyRequired)
		}
	})
}

// TestSDDVerifyApplicabilityRejectsUnsupportedProjection keeps the projection
// vocabulary closed rather than silently defaulting an unrecognized value.
func TestSDDVerifyApplicabilityRejectsUnsupportedProjection(t *testing.T) {
	repo := initReviewCLIRepo(t)
	var stdout bytes.Buffer
	err := runSDDVerifyApplicability(context.Background(), []string{"--cwd", repo, "--projection", "committed"}, &stdout)
	if err == nil {
		t.Fatal("unsupported projection was accepted")
	}
	if stdout.Len() != 0 {
		t.Fatalf("rejected input still produced output: %q", stdout.String())
	}
}

// TestSDDVerifyApplicabilityClassifiesCommittedWork is the regression test for
// the case the feature exists to serve. SDD commits each work unit, so by the
// time verification is considered the working tree is clean. Without a base the
// candidate is empty and the assessment reads that as an anomaly, leaving the
// verifier to run on every passive change — the exact cost this was built to
// remove.
func TestSDDVerifyApplicabilityClassifiesCommittedWork(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeApplicabilityFile(t, repo, "docs/guide.md", "# Guide\n\nPlain prose.\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "docs: add a guide")

	t.Run("a clean tree with no base reads as an empty candidate", func(t *testing.T) {
		result := runApplicability(t, "--cwd", repo)
		if result.Decision != SDDVerifyRequired || result.Reason != SDDVerifyReasonEmptyCandidate {
			t.Fatalf("decision = %q (%q); the working-tree default should see nothing", result.Decision, result.Reason)
		}
	})

	t.Run("the committed change classifies against its base", func(t *testing.T) {
		result := runApplicability(t, "--cwd", repo, "--base-ref", base)
		if result.Decision != SDDVerifyNotRequired || result.Reason != SDDVerifyReasonPassiveCandidate {
			t.Fatalf("decision = %q (%q), want %q", result.Decision, result.Reason, SDDVerifyNotRequired)
		}
		if !reflect.DeepEqual(result.CoveredPaths, []string{"docs/guide.md"}) {
			t.Fatalf("covered paths = %#v", result.CoveredPaths)
		}
	})

	t.Run("a committed active change still requires verification", func(t *testing.T) {
		writeApplicabilityFile(t, repo, "internal/run.go", "package internal\n\nfunc Run() {}\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "feat: add run")
		result := runApplicability(t, "--cwd", repo, "--base-ref", base)
		if result.Decision != SDDVerifyRequired || result.Reason != SDDVerifyReasonActiveContent {
			t.Fatalf("decision = %q (%q), want %q", result.Decision, result.Reason, SDDVerifyRequired)
		}
	})
}
