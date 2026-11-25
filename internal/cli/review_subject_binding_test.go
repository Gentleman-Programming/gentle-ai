package cli

import (
	"bytes"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func writeReviewerOutputWithOptionalEcho(t *testing.T, result facadeReviewerResult, echo string) string {
	t.Helper()
	fields := map[string]any{"inspection": result.Inspection, "findings": result.Findings, "evidence": result.Evidence}
	if echo != "" {
		fields["subject_hash"] = echo
	}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, fields)
	return input
}

func TestReviewCaptureResultPiBindsHostSubjectNotEcho(t *testing.T) {
	for _, name := range []string{"omitted echo", "mistyped echo"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
			binding := piHostRelayCaptureBinding(t, repo, args, record)
			if err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", string(model.AgentPi), "--materialize=true"), &bytes.Buffer{}); err != nil {
				t.Fatalf("materialize for Pi: %v", err)
			}
			result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
			var echo string
			if name == "mistyped echo" {
				hash := []byte(result.SubjectHash)
				for _, i := range []int{len(hash) - 2, len(hash) - 1} {
					if hash[i] == '0' {
						hash[i] = '1'
					} else {
						hash[i] = '0'
					}
				}
				echo = string(hash)
			}
			input := writeReviewerOutputWithOptionalEcho(t, result, echo)
			var output bytes.Buffer
			argv := append([]string{"capture-result"}, binding...)
			if err := RunReview(append(argv, "--agent", string(model.AgentPi), "--input", input), &output); err != nil {
				t.Fatalf("Pi submission refused: %v\nstdout: %s", err, &output)
			}
			var terminal reviewLastEventClosureResult
			decodeStrictReviewJSON(t, output.Bytes(), &terminal)
			if terminal.Operation != "review/capture-result" || terminal.State != reviewtransaction.StateApproved || terminal.LineageID != record.State.LineageID {
				t.Fatalf("Pi submission terminal result = %#v", terminal)
			}
		})
	}
}

func TestReviewCaptureResultSubjectVersionSelection(t *testing.T) {
	reviewEnabledHome(t)
	for _, test := range []struct {
		name string
		host string
		echo string
		want string
	}{
		{name: "manual without echo", want: "current"},
		{name: "legacy echo fallback", echo: "legacy", want: "legacy"},
		{name: "legacy host without echo", host: "legacy", want: "legacy"},
		{name: "legacy host beats current echo", host: "legacy", echo: "current", want: "legacy"},
		{name: "current host beats legacy echo", host: "current", echo: "legacy", want: "current"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, started, _, record := newArtifactReview(t, true)
			lens := record.State.SelectedLenses[0]
			result := admittedReviewerResultForTest(t, repo, record, lens, 0)
			builder := reviewtransaction.SnapshotBuilder{Repo: repo}
			frozen, err := builder.FrozenCandidateContext(t.Context(), record.State.InitialSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			legacyFrozen, err := builder.WithLegacyCandidateDiff(t.Context(), record.State.InitialSnapshot, frozen)
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := reviewtransaction.NewLegacyArtifactSubject(record.State, record.State.CapturePhaseRevision, legacyFrozen, lens, 0, "")
			if err != nil {
				t.Fatal(err)
			}
			hashes := map[string]string{"current": result.SubjectHash, "legacy": legacy.SubjectHash}
			input := writeReviewerOutputWithOptionalEcho(t, result, hashes[test.echo])
			argv := []string{
				"capture-result", "--cwd", repo, "--lineage", started.LineageID,
				"--target", started.TargetIdentity, "--lens", lens, "--order", "0", "--input", input,
			}
			if test.host != "" {
				argv = append(argv, "--subject-hash", hashes[test.host])
			}
			var output bytes.Buffer
			if err := RunReview(argv, &output); err != nil {
				t.Fatalf("capture refused: %v\nstdout: %s", err, &output)
			}
			var artifact reviewResultArtifact
			decodeStrictReviewJSON(t, output.Bytes(), &artifact)
			if artifact.Schema != reviewResultArtifactSchema || artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted ||
				artifact.LineageID != started.LineageID || artifact.Lens != lens || artifact.SelectedOrder != 0 || artifact.SubjectHash != hashes[test.want] {
				t.Fatalf("capture = %#v, want completed capture for %s (%s)", artifact, test.want, hashes[test.want])
			}
			status := explicitFrozenReviewingStatus(t, repo, started.LineageID)
			if status.NextTransition == nil || status.NextTransition.Collect == nil ||
				len(status.NextTransition.Collect.Inputs) != len(record.State.SelectedLenses)-1 {
				t.Fatalf("STATUS did not retain partial capture: %#v", status.NextTransition)
			}
		})
	}
}
