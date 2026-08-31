package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NotApplicableVerifyReportRequest carries everything needed to write the
// canonical report for a candidate that owed no execution.
//
// The classification itself is not performed here. The classifier lives in a
// package this one deliberately does not import, so the caller that spans both
// supplies the already-decided facts and this emitter renders and admits them.
type NotApplicableVerifyReportRequest struct {
	// Repo is the Git repository root; Workspace is the planning root, which
	// may be a subdirectory of it.
	Repo, Workspace, Change string
	// EvidenceRevision ties the report to the runtime attempt it settles. It
	// is supplied rather than derived, exactly as a sub-agent report supplies
	// it, so this path introduces no second source of attempt identity.
	EvidenceRevision string
	// CandidateTree is the Git tree whose bytes were classified. The identity
	// stored in the report is derived from it here, not supplied, so one rule
	// governs both writing and later verifying it.
	CandidateTree string
	// CoveredPaths are the passive paths the assessment accounted for. They
	// are reported as a count, because the report records why no execution was
	// owed rather than restating the diff.
	CoveredPaths []string
}

// EmitNotApplicableVerifyReport writes the canonical verify report for a
// candidate that carried no runtime obligation, and returns its path.
//
// It admits its own output before writing. Rendering a report and persisting
// it are separated by the same validation an externally authored report faces,
// so a defect here is refused rather than written to the canonical path where
// it would be read as verification evidence.
func EmitNotApplicableVerifyReport(ctx context.Context, request NotApplicableVerifyReportRequest) (string, error) {
	if strings.TrimSpace(request.CandidateTree) == "" {
		return "", errors.New("a not-applicable report requires the classified candidate tree") // refusal:by-design world-action: the request is assembled by the calling code from its own assessment, so a missing tree is a defect at that call site and no invocation can supply it here
	}
	if !sha256IdentityPattern.MatchString(request.EvidenceRevision) {
		return "", errors.New("a not-applicable report requires the settling attempt's evidence revision") // refusal:by-design world-action: the revision belongs to the attempt the caller is settling and is passed in, so only that call site can correct it
	}
	if len(request.CoveredPaths) == 0 {
		// An empty candidate proves nothing passive, so it never reaches this
		// state. Refusing here keeps that fail-closed rule true even if a
		// caller's own policy were to regress.
		return "", errors.New("a not-applicable report requires at least one covered path") // refusal:by-design world-action: an empty candidate never reaches this state under the caller's own policy, so reaching it is a defect at that call site
	}
	changeRoot, err := resolveBindingChangeRoot(ctx, request.Repo, request.Workspace, request.Change)
	if err != nil {
		return "", err
	}
	artifactPaths, err := resolveArtifactPaths(changeRoot)
	if err != nil {
		return "", err
	}
	counts, err := readSpecCounts(artifactPaths.Specs)
	if err != nil {
		return "", err
	}
	digest, err := canonicalCandidateDigest(ctx, request.Repo, request.Workspace, changeRoot, request.Change, request.CandidateTree)
	if err != nil {
		return "", err
	}
	payload := renderNotApplicableVerifyReport(request, counts, digest)
	if admission := ValidateVerifyReportAdmission(payload, counts); !admission.Valid {
		return "", fmt.Errorf("rendered not-applicable report is not admissible: %s", admission.Reason) // refusal:by-design world-action: the emitter authored these bytes itself, so a rejection here is a defect in this renderer and requires a code fix, never an operator action
	}
	path := filepath.Join(changeRoot, "verify-report.md")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// renderNotApplicableVerifyReport builds the exact report bytes.
//
// The six execution fields are absent rather than zeroed: the contract forbids
// them in this state, and their absence is the report's honesty.
func renderNotApplicableVerifyReport(request NotApplicableVerifyReportRequest, counts SpecCounts, digest string) string {
	covered := len(request.CoveredPaths)
	noun := "paths"
	if covered == 1 {
		noun = "path"
	}
	reason := fmt.Sprintf(
		"no test or build was owed because all %d changed %s classify as passive content under the native verification classifier",
		covered, noun)
	lines := []string{
		"```yaml",
		"schema: " + VerifyResultSchemaV2,
		"evidence_revision: " + request.EvidenceRevision,
		"verdict: " + VerifyVerdictNotApplicable,
		"blockers: 0",
		"critical_findings: 0",
		// None assessed: applicability, not coverage, is what this report
		// establishes. The totals stay bound to the change's own specs.
		fmt.Sprintf("requirements: 0/%d", counts.Requirements),
		fmt.Sprintf("scenarios: 0/%d", counts.Scenarios),
		"candidate_digest: " + digest,
		"na_reason: " + reason,
		"```",
		"",
		"## Verification Report",
		"",
		fmt.Sprintf("Change: %s", request.Change),
		"",
		"No independent test or build execution was owed for this candidate. Every",
		"changed path classifies as passive content, so there is no runtime behaviour",
		"to exercise and no execution evidence to record.",
		"",
		"### What this proves",
		"",
		fmt.Sprintf("- Structural readback of %d changed %s identified by `%s`.", covered, noun, digest),
		"- Each of those paths classifies as passive under the native classifier.",
		"",
		"### What this does not prove",
		"",
		fmt.Sprintf("- Conformance of the written content to any of the %d requirements or %d scenarios;", counts.Requirements, counts.Scenarios),
		"  none were assessed, because no runtime obligation existed to assess them against.",
		"- Any runtime behaviour, because none was exercised.",
		"",
		"### Covered paths",
		"",
	}
	for _, path := range request.CoveredPaths {
		lines = append(lines, "- `"+path+"`")
	}
	return strings.Join(append(lines, ""), "\n")
}
