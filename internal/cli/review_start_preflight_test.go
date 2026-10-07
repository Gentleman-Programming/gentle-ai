package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// rdd-risk-gated S14/S17: STATUS preflights the START options a caller
// wants to freeze (--request-context, --escalate-item/--escalate-reason) and
// renders them into the provider-owned review.start vector, so a host relay
// never appends tokens to a START it did not author.

func runStartPreflightStatus(t *testing.T, contract, repo string, extra ...string) (ReviewTargetStatusResult, []byte, error) {
	t.Helper()
	var output bytes.Buffer
	args := append([]string{"status", "--contract", contract, "--cwd", repo, "--next-transition"}, extra...)
	err := RunReview(args, &output)
	if err != nil {
		return ReviewTargetStatusResult{}, output.Bytes(), err
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status, output.Bytes(), nil
}

func startPreflightTransitionNames(t *testing.T, status ReviewTargetStatusResult) []string {
	t.Helper()
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "fresh_target_ready" || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("next_transition = %#v, want an executable fresh_target_ready START", status.NextTransition)
	}
	names := make([]string, 0, len(status.NextTransition.Execute.Arguments))
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Token != reviewTransitionArgumentToken(argument) {
			t.Fatalf("START argument %#v carries a token that does not match it", argument)
		}
		names = append(names, argument.Name)
	}
	return names
}

func startPreflightTokens(status ReviewTargetStatusResult) []string {
	tokens := []string{"start"}
	for _, argument := range status.NextTransition.Execute.Arguments {
		tokens = append(tokens, argument.Token)
	}
	return tokens
}

// reviewStartPreflightSideEffects fingerprints every file STATUS could touch:
// the shared review store under the Git common directory and the review home.
func reviewStartPreflightSideEffects(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if entry.IsDir() {
				files[path] = "dir"
				return nil
			}
			payload, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			sum := sha256.Sum256(payload)
			files[path] = hex.EncodeToString(sum[:])
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// TestReviewStatusPreflightsStartOptionsIntoTheFreshStart is S17: with the
// START options, STATUS validates them without creating authority and its
// fresh review.start vector carries them; running that vector verbatim makes
// START freeze the request and the escalation itself.
func TestReviewStatusPreflightsStartOptionsIntoTheFreshStart(t *testing.T) {
	for _, contract := range []string{ReviewIntegrationContractV1, ReviewIntegrationContractV2} {
		t.Run(contract, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
			requestPath := writeRequestContextFile(t, requestContextFixture)

			plain, _, err := runStartPreflightStatus(t, contract, repo)
			if err != nil {
				t.Fatal(err)
			}
			plainNames := startPreflightTransitionNames(t, plain)
			status, payload, err := runStartPreflightStatus(t, contract, repo,
				"--request-context", requestPath, "--escalate-item", "2", "--escalate-reason", escalationReasonFixture)
			if err != nil {
				t.Fatalf("STATUS preflight refused valid START options: %v\n%s", err, payload)
			}
			names := startPreflightTransitionNames(t, status)
			wantNames := append(append([]string{}, plainNames...), "request-context", "escalate-item", "escalate-reason")
			if !reflect.DeepEqual(names, wantNames) {
				t.Fatalf("START arguments = %v, want the plain vector followed by the options %v", names, wantNames)
			}
			if !reflect.DeepEqual(status.NextTransition.Execute.Arguments[:len(plainNames)], plain.NextTransition.Execute.Arguments) {
				t.Fatalf("options changed the plain START arguments:\n%#v\n%#v", status.NextTransition.Execute.Arguments, plain.NextTransition.Execute.Arguments)
			}
			options := status.NextTransition.Execute.Arguments[len(plainNames):]
			wantOptions := []ReviewTransitionArgument{
				{Name: "request-context", Value: requestPath},
				{Name: "escalate-item", Value: "2"},
				{Name: "escalate-reason", Value: escalationReasonFixture},
			}
			for index := range wantOptions {
				wantOptions[index].Token = reviewTransitionArgumentToken(wantOptions[index])
			}
			if !reflect.DeepEqual(options, wantOptions) {
				t.Fatalf("START options = %#v, want %#v", options, wantOptions)
			}
			if status.NextTransition.Execute.Binding != plain.NextTransition.Execute.Binding ||
				!reflect.DeepEqual(status.NextTransition.Execute.Preconditions, plain.NextTransition.Execute.Preconditions) {
				t.Fatalf("options changed the START binding: %#v vs %#v", status.NextTransition.Execute, plain.NextTransition.Execute)
			}
			if contract == ReviewIntegrationContractV2 {
				validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), payload)
			}
			lineage := status.NextTransition.Execute.Binding.LineageID
			occupied, err := reviewtransaction.ExactReviewLineageOccupied(context.Background(), repo, lineage)
			if err != nil || occupied {
				t.Fatalf("STATUS preflight created authority for %q: occupied=%v err=%v", lineage, occupied, err)
			}

			var output bytes.Buffer
			if err := RunReview(startPreflightTokens(status), &output); err != nil {
				t.Fatal(negotiatedReviewStartFailure(err, output.String()))
			}
			if contract == ReviewIntegrationContractV2 {
				// The v2 vector relays consent, and the escalation makes the
				// candidate high: answer through the granted follow-up START
				// itself printed, which repeats the options.
				var question ReviewIntegrationConsentResult
				decodeStrictReviewJSON(t, output.Bytes(), &question)
				if question.Action != "consent_required" || question.RiskLevel != reviewtransaction.RiskHigh || len(question.Choices) == 0 {
					t.Fatalf("relayed START = %s", output.String())
				}
				words := reviewShellWords(t, question.Choices[0].Invocation)
				if len(words) < 3 || words[0] != "gentle-ai" || words[1] != "review" {
					t.Fatalf("granted follow-up = %q", question.Choices[0].Invocation)
				}
				output.Reset()
				if err := RunReview(words[2:], &output); err != nil {
					t.Fatal(negotiatedReviewStartFailure(err, output.String()))
				}
			}
			if started := decodeNegotiatedReviewStart(t, output.Bytes()); started.RiskLevel != reviewtransaction.RiskHigh || started.LineageID != lineage {
				t.Fatalf("START from the preflighted vector = risk %q lineage %q, want high %q", started.RiskLevel, started.LineageID, lineage)
			}
			record := loadRequestContextRecord(t, repo, lineage)
			if reviewFrozenRequestContext(record.State) != requestContextFixture {
				t.Fatalf("START did not freeze the preflighted request: %q", reviewFrozenRequestContext(record.State))
			}
			want := reviewtransaction.CompactAgentEscalation{Item: 2, Reason: escalationReasonFixture}
			if record.State.AgentEscalation == nil || *record.State.AgentEscalation != want ||
				record.State.InitialAtomicStart == nil || record.State.InitialAtomicStart.AgentEscalation == nil ||
				*record.State.InitialAtomicStart.AgentEscalation != want {
				t.Fatalf("START did not freeze the preflighted escalation: %#v", record.State.AgentEscalation)
			}
		})
	}
}

// TestReviewStatusPreflightRendersEachStartOptionAlone triangulates the
// rendering: each option rides the vector alone, a relative request path is
// rendered absolute so the vector does not depend on the caller's cwd, and
// an item spelled with spaces renders the integer START parses.
func TestReviewStatusPreflightRendersEachStartOptionAlone(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	requestPath := writeRequestContextFile(t, requestContextFixture)
	t.Chdir(filepath.Dir(requestPath))

	for _, tt := range []struct {
		name string
		args []string
		want []ReviewTransitionArgument
	}{
		{"request context only", []string{"--request-context", filepath.Base(requestPath)}, []ReviewTransitionArgument{{Name: "request-context", Value: requestPath}}},
		{"escalation only", []string{"--escalate-item= 4", "--escalate-reason", "shares a lock with the gate"}, []ReviewTransitionArgument{
			{Name: "escalate-item", Value: "4"}, {Name: "escalate-reason", Value: "shares a lock with the gate"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plain, _, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo)
			if err != nil {
				t.Fatal(err)
			}
			status, payload, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo, tt.args...)
			if err != nil {
				t.Fatalf("STATUS preflight refused %v: %v\n%s", tt.args, err, payload)
			}
			startPreflightTransitionNames(t, status)
			arguments := status.NextTransition.Execute.Arguments
			plainCount := len(plain.NextTransition.Execute.Arguments)
			for index := range tt.want {
				tt.want[index].Token = reviewTransitionArgumentToken(tt.want[index])
			}
			if len(arguments) != plainCount+len(tt.want) || !reflect.DeepEqual(arguments[plainCount:], tt.want) {
				t.Fatalf("START options = %#v, want %#v", arguments[plainCount:], tt.want)
			}
		})
	}
}

// TestReviewStatusWithoutStartOptionsIsUnchanged is the PRESERVE half: without
// the options the fresh START vector names exactly the arguments it always
// did, and the published bytes are stable across identical calls.
func TestReviewStatusWithoutStartOptionsIsUnchanged(t *testing.T) {
	reviewEnabledHome(t)
	// The Pi host relay must declare its contract, as the installed launcher does.
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	first, firstPayload, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo, "--agent", "pi")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cwd", "contract", "target", "target-evidence", "projection", "lineage", "agent", "consent"}
	if got := startPreflightTransitionNames(t, first); !reflect.DeepEqual(got, want) {
		t.Fatalf("plain START arguments = %v, want %v", got, want)
	}
	_, secondPayload, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo, "--agent", "pi")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstPayload, secondPayload) {
		t.Fatalf("plain STATUS changed across identical calls:\n%s\n%s", firstPayload, secondPayload)
	}
}

// TestReviewStatusValidatesStartOptionsLikeStart applies START's own rules
// in STATUS: every refusal is a not_started preflight failure that writes
// nothing, so an option is never ignored with success.
func TestReviewStatusValidatesStartOptionsLikeStart(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	requestPath := writeRequestContextFile(t, requestContextFixture)
	emptyPath := writeRequestContextFile(t, " \n")
	invalidPath := filepath.Join(t.TempDir(), "request.bin")
	if err := os.WriteFile(invalidPath, []byte{0xff, 0xfe, 'x'}, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"item without reason", []string{"--escalate-item", "2"}, "review status --escalate-item and --escalate-reason must be passed together"},
		{"reason without item", []string{"--escalate-reason", "why"}, "review status --escalate-item and --escalate-reason must be passed together"},
		{"item out of range", []string{"--escalate-item", "7", "--escalate-reason", "why"}, `review status --escalate-item "7" must be an integer from 1 to 6`},
		{"item not a number", []string{"--escalate-item", "two", "--escalate-reason", "why"}, `review status --escalate-item "two" must be an integer from 1 to 6`},
		{"blank reason", []string{"--escalate-item", "2", "--escalate-reason", " "}, "review status --escalate-reason must be non-empty and at most 500 characters"},
		{"reason too long", []string{"--escalate-item", "2", "--escalate-reason", strings.Repeat("x", 501)}, "review status --escalate-reason must be non-empty and at most 500 characters"},
		{"repeated item", []string{"--escalate-item", "2", "--escalate-item", "2", "--escalate-reason", "why"}, "review status repeats --escalate-item"},
		{"repeated reason", []string{"--escalate-item", "2", "--escalate-reason", "why", "--escalate-reason", "why"}, "review status repeats --escalate-reason"},
		{"repeated request", []string{"--request-context", requestPath, "--request-context", requestPath}, "review status repeats --request-context"},
		{"blank request path", []string{"--request-context="}, "review status --request-context requires the path of a request file"},
		{"missing request file", []string{"--request-context", filepath.Join(t.TempDir(), "missing.md")}, "read review request context"},
		{"empty request file", []string{"--request-context", emptyPath}, "names an empty file"},
		{"non-UTF-8 request file", []string{"--request-context", invalidPath}, "must be UTF-8 text"},
		{"without next transition", []string{"--next-transition=false", "--request-context", requestPath}, "require --next-transition"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := reviewStartPreflightSideEffects(t, filepath.Join(repo, ".git", "gentle-ai"), home)
			var output bytes.Buffer
			err := RunReview(append([]string{"status", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--next-transition"}, tt.args...), &output)
			if err == nil {
				t.Fatalf("STATUS preflight accepted %v:\n%s", tt.args, output.String())
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Operation != "review.status" || failure.Phase != "preflight" || failure.Code != "invalid_request" ||
				failure.MutationOutcome != ReviewMutationNotStarted || !strings.Contains(failure.Cause, tt.want) {
				t.Fatalf("STATUS preflight failure = %#v, want cause %q", failure, tt.want)
			}
			if after := reviewStartPreflightSideEffects(t, filepath.Join(repo, ".git", "gentle-ai"), home); !reflect.DeepEqual(after, before) {
				t.Fatalf("refused STATUS preflight wrote state:\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
	t.Run("without contract", func(t *testing.T) {
		var output bytes.Buffer
		err := RunReview([]string{"status", "--cwd", repo, "--request-context", requestPath}, &output)
		if err == nil || !strings.Contains(err.Error(), "require --contract") {
			t.Fatalf("uncontracted STATUS accepted START options: %v\n%s", err, output.String())
		}
	})
}

// TestReviewStatusRefusesStartOptionsWithoutAFreshStart covers the routes
// that offer no fresh START: an existing reviewing lineage (whose options
// START already froze or refused) and a candidate that first needs a base.
// Ignoring the options there would report success for options nothing
// applies, so STATUS refuses before it writes anything.
func TestReviewStatusRefusesStartOptionsWithoutAFreshStart(t *testing.T) {
	t.Run("existing lineage", func(t *testing.T) {
		home := reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
		var output bytes.Buffer
		if err := RunReview(boundNegotiatedStartArgs(t, []string{
			"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "preflight-existing",
		}), &output); err != nil {
			t.Fatal(negotiatedReviewStartFailure(err, output.String()))
		}
		record := loadRequestContextRecord(t, repo, "preflight-existing")
		before := reviewStartPreflightSideEffects(t, filepath.Join(repo, ".git", "gentle-ai"), home)
		_, payload, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo, "--lineage", "preflight-existing",
			"--escalate-item", "2", "--escalate-reason", escalationReasonFixture)
		if err == nil {
			t.Fatalf("STATUS accepted START options for an existing lineage:\n%s", payload)
		}
		failure := decodeReviewIntegrationFailure(t, payload)
		if failure.Operation != "review.status" || failure.Code != "invalid_request" || failure.MutationOutcome != ReviewMutationNotStarted ||
			!strings.Contains(failure.Cause, "only preflight a fresh START") {
			t.Fatalf("existing-lineage preflight failure = %#v", failure)
		}
		if after := loadRequestContextRecord(t, repo, "preflight-existing"); after.Revision != record.Revision {
			t.Fatal("a refused STATUS preflight rewrote the lineage")
		}
		if after := reviewStartPreflightSideEffects(t, filepath.Join(repo, ".git", "gentle-ai"), home); !reflect.DeepEqual(after, before) {
			t.Fatalf("refused STATUS preflight wrote state:\nbefore=%v\nafter=%v", before, after)
		}
		if _, _, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo, "--lineage", "preflight-existing"); err != nil {
			t.Fatalf("the named continuation without options failed: %v", err)
		}
	})
	t.Run("candidate needs a base", func(t *testing.T) {
		reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		plain, _, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo)
		if err != nil {
			t.Fatal(err)
		}
		if plain.NextTransition == nil || plain.NextTransition.Execute != nil && plain.NextTransition.Execute.Operation == "review.start" {
			t.Fatalf("clean repository unexpectedly offers a fresh START: %#v", plain.NextTransition)
		}
		_, payload, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo, "--request-context", writeRequestContextFile(t, requestContextFixture))
		if err == nil {
			t.Fatalf("STATUS accepted START options without a fresh START:\n%s", payload)
		}
		if failure := decodeReviewIntegrationFailure(t, payload); failure.MutationOutcome != ReviewMutationNotStarted ||
			!strings.Contains(failure.Cause, "only preflight a fresh START") || !strings.Contains(failure.Cause, plain.NextTransition.ReasonCode) {
			t.Fatalf("no-START preflight failure = %#v", failure)
		}
	})
}

// TestReviewStatusStartValidatorBindsThePreflightedOptions keeps the fresh
// START validator exact: the vector must carry precisely the options STATUS
// preflighted, so a vector that drops, changes, or invents one is refused.
func TestReviewStatusStartValidatorBindsThePreflightedOptions(t *testing.T) {
	escalation := &reviewtransaction.CompactAgentEscalation{Item: 2, Reason: escalationReasonFixture}
	preflighted := reviewStartPreflightOptions{RequestContextPath: "/requests/request.md", Escalation: escalation}
	fresh := func(options reviewStartPreflightOptions) ReviewTargetStatusResult {
		status := emptyWorkspaceCandidateStatus()
		status.Projection.Paths = []string{"tracked.txt"}
		status.Projection.CurrentCandidateTree = strings.Repeat("d", 40)
		status.startOptions = options
		transition := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-start-options"})
		status.NextTransition = &transition
		return status
	}
	for _, options := range []reviewStartPreflightOptions{{}, preflighted, {RequestContextPath: "/requests/request.md"}, {Escalation: escalation}} {
		if err := fresh(options).validateStartNextTransition(); err != nil {
			t.Fatalf("START vector for %#v failed its own validator: %v", options, err)
		}
	}
	for _, tt := range []struct {
		name    string
		emitted reviewStartPreflightOptions
		bound   reviewStartPreflightOptions
	}{
		{"options dropped", reviewStartPreflightOptions{}, preflighted},
		{"options invented", preflighted, reviewStartPreflightOptions{}},
		{"request changed", reviewStartPreflightOptions{RequestContextPath: "/requests/other.md", Escalation: escalation}, preflighted},
		{"escalation changed", reviewStartPreflightOptions{RequestContextPath: "/requests/request.md", Escalation: &reviewtransaction.CompactAgentEscalation{Item: 3, Reason: escalationReasonFixture}}, preflighted},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status := fresh(tt.emitted)
			status.startOptions = tt.bound
			if err := status.validateStartNextTransition(); err == nil {
				t.Fatal("fresh START validator accepted a vector that does not carry the preflighted options")
			}
		})
	}
}
