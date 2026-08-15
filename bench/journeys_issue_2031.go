package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

const issue2031EscalatedPredecessor = "issue-2031-escalated-predecessor"

// issue2031Journeys proves that a materially changed escalated target remains
// recoverable when its delivery scope also expands (#2031).
func issue2031Journeys() []Journey {
	return []Journey{
		{
			ID:     "j94-escalated-changed-scope-negotiates-recovery",
			Title:  "Escalated changed target: expanded scope executes native recovery",
			Source: "issue #2031: changed-target recovery must precede delivery-scope matching",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: stage high-risk candidate", Fixture: stageAuthCode},
				{Name: "start high-risk review", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", issue2031EscalatedPredecessor)},
				{Name: "capture every review lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize captured review results", Requires: finalizeResultsCapability,
					Args: productArgs("review", "finalize", "--lineage", issue2031EscalatedPredecessor, "--captured-results=true")},
				{Name: "finalize failed verification", Requires: finalizeFailedCapability, Composite: finalizeAsFailedVerification},
				{Name: "fixture: change candidate bytes and delivery scope", Fixture: stageIssue2031RecoveryTarget},
				{Name: "negotiate and execute self-derived escalated recovery", Requires: statusCapability, Composite: negotiateIssue2031Recovery},
			},
		},
	}
}

func stageIssue2031RecoveryTarget(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "internal", "auth", "session.go"), "package auth\n\nfunc CheckToken(token string) bool {\n\treturn len(token) >= 16\n}\n"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "recovery.md"), "# Recovery scope\n"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "internal/auth/session.go", "docs/recovery.md")
}

func negotiateIssue2031Recovery(r *journeyRun) error {
	expectedCandidateTree, err := gitOut(r.sandbox, r.sandbox.Repo, "write-tree")
	if err != nil {
		return fmt.Errorf("derive staged issue #2031 recovery tree: %w", err)
	}
	envelope, err := readStatusFor(r, "--lineage", issue2031EscalatedPredecessor)
	if err != nil {
		return err
	}
	expectedRevision := envelope.Authority.Revision
	expectedTarget := envelope.TargetIdentity
	if expectedCandidateTree == "" || expectedRevision == "" || expectedTarget == "" {
		return fmt.Errorf("escalated changed-scope recovery anchors = tree=%q revision=%q target=%q, want non-empty", expectedCandidateTree, expectedRevision, expectedTarget)
	}
	if envelope.Authority.LineageID != issue2031EscalatedPredecessor || envelope.Authority.State != "escalated" ||
		envelope.Projection.CurrentCandidateTree != expectedCandidateTree ||
		strings.Join(envelope.Projection.Paths, "\x00") != "docs/recovery.md\x00internal/auth/session.go" ||
		envelope.NextTransition.Kind != "execute" || envelope.NextTransition.Execute.Operation != "review.recover" {
		return fmt.Errorf("escalated changed-scope status = %+v, want executable native recovery", envelope)
	}
	successor := envelope.executeArgument("successor-lineage")
	if envelope.Authority.Revision != expectedRevision || envelope.TargetIdentity != expectedTarget ||
		envelope.executeArgument("predecessor-lineage") != issue2031EscalatedPredecessor ||
		envelope.executeArgument("expected-predecessor-revision") != expectedRevision || successor == "" ||
		envelope.executeArgument("disposition") != "escalated" || envelope.executeArgument("actor") != "" ||
		envelope.executeArgument("reason") != "" || envelope.executeArgument("maintainer-authorization") != "" {
		return fmt.Errorf("self-derived escalated recovery binding = %+v", envelope.NextTransition.Execute)
	}
	recovery, err := runPrintedTransition(r, envelope)
	if err != nil {
		return err
	}
	recovered, err := decodeWaveOperation(recovery, "escalated changed-scope recovery")
	if err != nil || recovered.LineageID != successor || recovered.State != "reviewing" ||
		recovered.TargetIdentity != expectedTarget {
		return fmt.Errorf("escalated recovery successor = %+v, %v", recovered, err)
	}
	successorStatus, err := readStatusFor(r, "--lineage", successor)
	if err != nil || successorStatus.Authority.LineageID != successor || successorStatus.Authority.State != "reviewing" ||
		successorStatus.TargetIdentity != expectedTarget {
		return fmt.Errorf("escalated recovery successor status = %+v, %v", successorStatus, err)
	}
	return nil
}
