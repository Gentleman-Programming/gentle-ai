package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
)

func TestPiReviewerCollectStatusMatchesPublishedSchema(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, _, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lineage := record.State.LineageID
	var output bytes.Buffer
	stderr := captureReviewProcessStderr(t)
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent=pi", "--next-transition",
	}, &output); err != nil {
		t.Fatalf("Pi STATUS: %v\n%s", err, output.Bytes())
	}
	if got := stderr(); got != "" {
		t.Fatalf("Pi STATUS wrote to stderr: %q", got)
	}

	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	transition := status.NextTransition
	if status.Schema != ReviewIntegrationStatusSchemaV9 || transition == nil || transition.Kind != "collect" ||
		transition.ReasonCode != "reviewer_results_required" || transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		t.Fatalf("want current reviewer collect STATUS, got %s", output.Bytes())
	}
	input := transition.Collect.Inputs[0]
	subject := input.ArtifactSubject
	if input.CaptureOperation != "review.capture-result" || subject == nil ||
		subject.TargetIdentity != status.TargetIdentity || status.RepositoryContext == nil {
		t.Fatalf("want a reviewer input bound to the STATUS target and repository, got %+v", input)
	}
	wantTokens := []string{
		"--lineage=" + lineage,
		"--expected-revision=" + subject.AuthorityRevision,
		"--target=" + status.TargetIdentity,
		"--repository-context=" + status.RepositoryContext.Handle,
		"--lens=" + subject.Lens,
		fmt.Sprintf("--order=%d", subject.SelectedOrder),
		"--subject-hash=" + subject.SubjectHash,
		"--agent=pi",
		"--input={{value}}",
	}
	wantValue := ReviewTransitionSubmissionValue{
		Slot: "reviewer_result", Domain: "artifact_path_or_stdin", Schema: reviewReviewerSchemaID, SubstitutionLocation: 8,
	}
	if submission := input.Submission; submission == nil || submission.OperationToken != "capture-result" ||
		!slices.Equal(submission.ArgumentTokens, wantTokens) || len(submission.Values) != 0 || submission.Value == nil ||
		submission.Value.Slot != wantValue.Slot || submission.Value.Domain != wantValue.Domain ||
		submission.Value.Schema != wantValue.Schema || submission.Value.SubstitutionLocation != wantValue.SubstitutionLocation {
		t.Fatalf("unexpected Pi submission: %+v; want tokens %q and value %+v", submission, wantTokens, wantValue)
	}

	current := compileWholeNativeStatusSchema(t, "status-v9.schema.json")
	t.Run("emitted Pi submission", func(t *testing.T) {
		validatePublishedReviewSchema(t, current, output.Bytes())
	})

	for _, tc := range []struct {
		name   string
		mutate func(*ReviewTransitionSubmission)
	}{
		{"missing agent with Pi substitution index", func(s *ReviewTransitionSubmission) {
			s.ArgumentTokens = slices.Delete(s.ArgumentTokens, 7, 8)
		}},
		{"wrong runtime", func(s *ReviewTransitionSubmission) { s.ArgumentTokens[7] = "--agent=codex" }},
		{"agent after input", func(s *ReviewTransitionSubmission) {
			s.ArgumentTokens[7], s.ArgumentTokens[8] = s.ArgumentTokens[8], s.ArgumentTokens[7]
		}},
		{"extra materialize token", func(s *ReviewTransitionSubmission) {
			s.ArgumentTokens = slices.Insert(s.ArgumentTokens, 8, "--materialize=true")
			s.Value.SubstitutionLocation = 9
		}},
		{"legacy substitution index with Pi agent", func(s *ReviewTransitionSubmission) { s.Value.SubstitutionLocation = 7 }},
		{"input is not a placeholder", func(s *ReviewTransitionSubmission) { s.ArgumentTokens[8] = "--input=result.json" }},
		{"malformed target binding", func(s *ReviewTransitionSubmission) { s.ArgumentTokens[2] = "--target=unbound" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var document ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &document)
			tc.mutate(document.NextTransition.Collect.Inputs[0].Submission)
			payload, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			decodeStrictReviewJSON(t, payload, &value)
			if err := current.Validate(value); err == nil {
				t.Fatalf("current published schema accepted %s", tc.name)
			}
		})
	}

	for _, tc := range []struct {
		name         string
		index        int
		value, token string
	}{
		{"malformed materialize revision", 1, "unbound", "--expected-revision=unbound"},
		{"wrong materialize lens token", 4, subject.Lens, "--agent=codex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value map[string]any
			decodeStrictReviewJSON(t, output.Bytes(), &value)
			input := value["next_transition"].(map[string]any)["collect"].(map[string]any)["inputs"].([]any)[0].(map[string]any)
			row := input["arguments"].([]any)[tc.index].(map[string]any)
			row["value"], row["token"] = tc.value, tc.token
			if current.Validate(value) == nil {
				t.Fatal("accepted malformed materialization row with a valid submission")
			}
		})
	}

	t.Run("legacy eight-token submission", func(t *testing.T) {
		var document ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &document)
		input := &document.NextTransition.Collect.Inputs[0]
		older := compileWholeNativeStatusSchema(t, "status-v8.schema.json")
		document.Schema = ReviewIntegrationStatusSchemaV8
		payload, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		decodeStrictReviewJSON(t, payload, &value)
		if err := older.Validate(value); err == nil {
			t.Fatal("older published schema accepted the new Pi form")
		}

		input.Arguments = input.Arguments[:7] // the surrounding collect also predates host materialization
		input.Submission.ArgumentTokens = slices.Delete(input.Submission.ArgumentTokens, 7, 8)
		input.Submission.Value.SubstitutionLocation = 7
		payload, err = json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		validatePublishedReviewSchema(t, older, payload)

		document.Schema = ReviewIntegrationStatusSchemaV9
		payload, err = json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		validatePublishedReviewSchema(t, current, payload)
	})
}
