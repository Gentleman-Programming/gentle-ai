package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const emitRevision = "sha256:" + "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

// emitRepo gives the emitter what it needs: a real Git repository, because the
// canonical change root is resolved against one.
func emitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runRuntimeLedgerGit(t, root, "init", "-q")
	seedReadyChange(t, root, "passive-docs", "- [x] 1.1 Work\n")
	return root
}

// emitTree stages the worktree and returns its real tree object, because the
// emitter now derives the report's identity from actual tree content.
func emitTree(t *testing.T, repo string) string {
	t.Helper()
	runRuntimeLedgerGit(t, repo, "add", "-A")
	return strings.TrimSpace(runRuntimeLedgerGit(t, repo, "write-tree"))
}

func emitRequest(t *testing.T, root string) NotApplicableVerifyReportRequest {
	t.Helper()
	return NotApplicableVerifyReportRequest{
		Repo: root, Workspace: root, Change: "passive-docs",
		EvidenceRevision: emitRevision, CandidateTree: emitTree(t, root),
		CoveredPaths: []string{"docs/guide.md", "docs/intro.md"},
	}
}

// TestEmitNotApplicableVerifyReportIsAdmissibleAndCarriesNoExecutionEvidence
// proves the emitted bytes clear the same admission an externally authored
// report faces, and that they record the absence of execution rather than
// inventing it.
func TestEmitNotApplicableVerifyReportIsAdmissibleAndCarriesNoExecutionEvidence(t *testing.T) {
	root := emitRepo(t)

	path, err := EmitNotApplicableVerifyReport(context.Background(), emitRequest(t, root))
	if err != nil {
		t.Fatalf("EmitNotApplicableVerifyReport(): %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	report := string(payload)
	for _, forbidden := range verifyReportExecutionFields {
		if strings.Contains(report, forbidden+":") {
			t.Fatalf("emitted report carries execution evidence field %q", forbidden)
		}
	}
	// seedReadyChange writes one requirement and one scenario.
	counts := SpecCounts{Requirements: 1, Scenarios: 1}
	if admission := ValidateVerifyReportAdmission(report, counts); !admission.Valid {
		t.Fatalf("emitted report is not admissible: %s", admission.Reason)
	}
	evaluation := parseVerifyResult(report, counts)
	if !evaluation.Passing {
		t.Fatalf("emitted report does not satisfy verification: %s", evaluation.Reason)
	}
	if evaluation.EvidenceRevision != emitRevision {
		t.Fatalf("EvidenceRevision = %q, want %q", evaluation.EvidenceRevision, emitRevision)
	}
	if !strings.Contains(report, "does not prove") {
		t.Fatal("emitted report does not state what it leaves unproven")
	}
}

// TestEmitNotApplicableVerifyReportAttestsAfterItIsWritten is the regression
// test for the defect an external review found: the report is stored inside the
// candidate it describes, so identifying that candidate by its tree OID names a
// hash fixed point and no honest emitter could ever satisfy the comparison.
//
// It walks the real sequence — classify, emit, stage the report, settle — and
// requires the attestation to hold across it.
func TestEmitNotApplicableVerifyReportAttestsAfterItIsWritten(t *testing.T) {
	root := emitRepo(t)
	classified := emitTree(t, root)

	path, err := EmitNotApplicableVerifyReport(context.Background(), emitRequest(t, root))
	if err != nil {
		t.Fatal(err)
	}
	settled := emitTree(t, root)
	if settled == classified {
		t.Fatal("writing the report did not change the tree; the regression cannot be observed")
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	admission := ValidateVerifyReportAdmission(string(payload), SpecCounts{Requirements: 1, Scenarios: 1})
	changeRoot := filepath.Join(root, "openspec", "changes", "passive-docs")
	settledDigest, err := canonicalCandidateDigest(context.Background(), root, root, changeRoot, "passive-docs", settled)
	if err != nil {
		t.Fatal(err)
	}
	if !finalVerifyReportAttests(admission, emitRevision, settledDigest) {
		t.Fatalf("the emitted report does not attest the candidate it classified (report digest %q, settled %q)",
			admission.CandidateDigest, settledDigest)
	}

	// Drift must still break it: the exclusion hides the report, nothing else.
	writeApplicabilityContent(t, root, "internal/run.go", "package internal\n")
	driftedDigest, err := canonicalCandidateDigest(context.Background(), root, root, changeRoot, "passive-docs", emitTree(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if finalVerifyReportAttests(admission, emitRevision, driftedDigest) {
		t.Fatal("the report attested a candidate that gained an active path after it was written")
	}
}

func writeApplicabilityContent(t *testing.T, repo, logicalPath, content string) {
	t.Helper()
	full := filepath.Join(repo, filepath.FromSlash(logicalPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEmitNotApplicableVerifyReportRefusesUnprovableRequests keeps the emitter
// fail-closed at its own boundary, so a caller policy regression cannot produce
// a report that claims more than it knows.
func TestEmitNotApplicableVerifyReportRefusesUnprovableRequests(t *testing.T) {
	root := emitRepo(t)
	tests := []struct {
		name   string
		mutate func(*NotApplicableVerifyReportRequest)
	}{
		{name: "without a classified tree", mutate: func(r *NotApplicableVerifyReportRequest) { r.CandidateTree = "" }},
		{name: "without an evidence revision", mutate: func(r *NotApplicableVerifyReportRequest) { r.EvidenceRevision = "" }},
		{name: "with a malformed evidence revision", mutate: func(r *NotApplicableVerifyReportRequest) { r.EvidenceRevision = "sha256:nope" }},
		{name: "with no covered path", mutate: func(r *NotApplicableVerifyReportRequest) { r.CoveredPaths = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := emitRequest(t, root)
			tt.mutate(&request)
			if _, err := EmitNotApplicableVerifyReport(context.Background(), request); err == nil {
				t.Fatal("emitter accepted a request it cannot prove")
			}
			if _, err := os.Stat(root + "/openspec/changes/passive-docs/verify-report.md"); err == nil {
				t.Fatal("a refused request still wrote the canonical report")
			}
		})
	}
}
