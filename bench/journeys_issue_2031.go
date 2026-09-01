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
			Review: reviewOptedIn,
			Title:  "Escalated changed target: expanded scope negotiates recovery authorization",
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
				{Name: "negotiate and execute escalated recovery authorization", Requires: statusCapability, Composite: negotiateIssue2031Recovery},
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
		envelope.NextTransition.Kind != "collect" || envelope.NextTransition.ReasonCode != "recovery_authorization_required" ||
		len(envelope.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("escalated changed-scope status = %+v, want bound recovery authorization collection", envelope)
	}
	input := envelope.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "external.authorize_recovery" ||
		envelope.argument("lineage") != issue2031EscalatedPredecessor || envelope.argument("expected-revision") != expectedRevision ||
		envelope.argument("target") != expectedTarget || envelope.argument("disposition") != "escalated" {
		return fmt.Errorf("escalated recovery collection = %+v, want fully bound escalated authorization", input)
	}
	const successor = "issue-2031-escalated-successor"
	const actor = "bench-maintainer"
	const reason = "authorize corrected escalated target with expanded delivery scope"
	authorization := strings.Join([]string{
		"gentle-ai.review-recovery-authorization/v1",
		"predecessor_lineage=" + issue2031EscalatedPredecessor,
		"predecessor_revision=" + expectedRevision,
		"target_identity=" + expectedTarget,
		"successor_lineage=" + successor,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
	authorized, err := readStatusFor(r, "--lineage", issue2031EscalatedPredecessor,
		"--recovery-successor-lineage", successor, "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)
	if err != nil || authorized.NextTransition.Kind != "execute" || authorized.NextTransition.Execute.Operation != "review.recover" {
		return fmt.Errorf("authorized escalated recovery continuation = %+v, %v", authorized.NextTransition, err)
	}
	if authorized.Authority.Revision != expectedRevision || authorized.TargetIdentity != expectedTarget ||
		authorized.executeArgument("predecessor-lineage") != issue2031EscalatedPredecessor ||
		authorized.executeArgument("expected-predecessor-revision") != expectedRevision ||
		authorized.executeArgument("successor-lineage") != successor || authorized.executeArgument("disposition") != "escalated" ||
		authorized.executeArgument("maintainer-authorization") != authorization {
		return fmt.Errorf("authorized escalated recovery binding = %+v", authorized.NextTransition.Execute)
	}
	recovery, err := runPrintedTransition(r, authorized)
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
