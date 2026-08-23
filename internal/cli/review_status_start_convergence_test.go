package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// reviewNegotiatedTransition runs the negotiated STATUS route exactly as a
// consumer does and returns the parsed next_transition. Narration lines
// precede the JSON on the same stream, so the payload is located rather than
// assumed to start the output.
func reviewNegotiatedTransition(t *testing.T, repo string, extra ...string) (kind, reason, operation string, tokens []string) {
	t.Helper()
	args := append([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", "claude-code", "--next-transition"}, extra...)
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("negotiated status failed: %v\n%s", err, output.String())
	}
	payload := regexp.MustCompile(`(?s)\{.*\}`).FindString(output.String())
	if payload == "" {
		t.Fatalf("negotiated status produced no JSON payload:\n%s", output.String())
	}
	var status struct {
		NextTransition struct {
			Kind       string `json:"kind"`
			ReasonCode string `json:"reason_code"`
			Execute    *struct {
				Operation string `json:"operation"`
				Arguments []struct {
					Token string `json:"token"`
				} `json:"arguments"`
			} `json:"execute"`
		} `json:"next_transition"`
	}
	if err := json.Unmarshal([]byte(payload), &status); err != nil {
		t.Fatalf("parse negotiated status: %v", err)
	}
	nt := status.NextTransition
	if nt.Execute != nil {
		for _, argument := range nt.Execute.Arguments {
			tokens = append(tokens, argument.Token)
		}
		operation = nt.Execute.Operation
	}
	return nt.Kind, nt.ReasonCode, operation, tokens
}

// TestNegotiatedStatusNeverOffersAStartItsOwnPreflightRefuses is the
// convergence property behind a whole family of field reports: #3102
// (committed-only base resolving to the candidate's own tree, START refused
// empty_candidate_scope), #2584 before it (clean workspace), #2196 and #2017
// as open siblings. Every member reads the same way: STATUS classifies a
// target as ready and emits an executable review.start, the consumer runs
// those exact tokens as the contract requires, and START's preflight refuses
// or resolves a different scope. Two surfaces answering the same question
// differently is root 13 of #2471; this pins the review pair the way
// TestStatusAndAcquireAgreeOnEveryRepositoryBlocker pinned the SDD pair after
// #2114.
//
// The property, not the instances: for every repository shape staged here,
// if STATUS answers kind=execute operation=review.start, then executing the
// returned tokens verbatim must not fail preflight. Shapes whose truthful
// answer is a stop or collect assert that STATUS says so up front — the
// #3102 shape was RED on this exact assertion the day it was reported and
// turned GREEN when #3106 landed, which is the demonstration that the
// property has teeth.
func TestNegotiatedStatusNeverOffersAStartItsOwnPreflightRefuses(t *testing.T) {
	shapes := []struct {
		name    string
		arrange func(t *testing.T) (repo string, extra []string)
		// wantExecutable: STATUS must offer review.start and it must run.
		// Otherwise: STATUS must NOT offer review.start for this shape, and
		// wantReason names the truthful classification it must give instead.
		wantExecutable bool
		wantReason     string
	}{
		{
			name: "dirty workspace offers a start that runs",
			arrange: func(t *testing.T) (string, []string) {
				repo := initReviewCLIRepo(t)
				// Modify the TRACKED file: an untracked path correctly routes
				// to intended-untracked selection first, which is a collect,
				// not this shape's subject.
				if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\nchanged\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return repo, nil
			},
			wantExecutable: true,
		},
		{
			name: "committed-only with a distinct base offers a start that runs",
			arrange: func(t *testing.T) (string, []string) {
				repo := initReviewCLIRepo(t)
				base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
				if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\nnext\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				runReviewCLIGit(t, repo, "add", "-A")
				runReviewCLIGit(t, repo, "commit", "-m", "next change")
				return repo, []string{"--base-ref", base, "--committed-only"}
			},
			wantExecutable: true,
		},
		{
			// #3102's exact shape: one root commit, clean tree, the selected
			// base resolves to the candidate's own tree. START's preflight
			// refuses this as empty_candidate_scope while asking for the
			// base_ref the transition already carried, so a STATUS that
			// offers START here has manufactured an unroutable loop.
			name: "committed-only base equal to candidate stops up front",
			arrange: func(t *testing.T) (string, []string) {
				repo := initReviewCLIRepo(t)
				head := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
				return repo, []string{"--base-ref", head, "--committed-only"}
			},
			wantReason: "empty_base_diff_bootstrap_required",
		},
		{
			// #2584's shape: a clean workspace freezes zero paths, so START
			// cannot succeed; the truthful answer is collecting a base.
			name: "clean workspace collects a base instead of offering a start",
			arrange: func(t *testing.T) (string, []string) {
				return initReviewCLIRepo(t), nil
			},
			wantReason: "empty_candidate_base_ref_required",
		},
	}

	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo, extra := shape.arrange(t)
			kind, reason, operation, tokens := reviewNegotiatedTransition(t, repo, extra...)

			if !shape.wantExecutable {
				if kind == "execute" && operation == "review.start" {
					t.Fatalf("STATUS offered review.start for a shape whose preflight refuses it (reason %q); this is the exact divergence behind #3102", reason)
				}
				if reason != shape.wantReason {
					t.Fatalf("STATUS classified the shape as %s/%s, want the truthful %q", kind, reason, shape.wantReason)
				}
				return
			}

			if kind != "execute" || operation != "review.start" {
				t.Fatalf("STATUS did not offer review.start for a reviewable shape: kind=%s reason=%s", kind, reason)
			}
			advertised := ""
			for _, token := range tokens {
				if strings.HasPrefix(token, "--target=") {
					advertised = strings.TrimPrefix(token, "--target=")
				}
			}
			// The contract requires executing the returned tokens unchanged,
			// so that is exactly what the property does. A medium-risk START
			// answers with the consent/v3 envelope first; the property then
			// runs the granted choice's exact invocation, which is precisely
			// the flow #2017 reports diverging: consent consumed, then START
			// resolving a different scope than the one consented to.
			text := runReviewStartFollowingConsent(t, tokens)
			if strings.Contains(text, `"phase": "preflight"`) || strings.Contains(text, "empty_candidate_scope") {
				t.Fatalf("the exact START that STATUS offered was refused in preflight:\n%s", text)
			}
			if !strings.Contains(text, `"lineage_id"`) {
				t.Fatalf("START produced no lineage for the offered target:\n%s", text)
			}
			if advertised != "" && !strings.Contains(text, advertised) {
				t.Fatalf("START bound a different target than STATUS advertised (%s); this is #2017's divergence:\n%s", advertised, text)
			}
		})
	}
}

// runReviewStartFollowingConsent executes the offered START and, when the
// candidate-scoped consent question comes back, answers it the way the
// contract requires: by running the granted choice's exact invocation, once.
func runReviewStartFollowingConsent(t *testing.T, tokens []string) string {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview(append([]string{"start"}, tokens...), &output); err != nil {
		t.Fatalf("the exact START that STATUS offered failed: %v\n%s", err, output.String())
	}
	text := output.String()
	if !strings.Contains(text, "gentle-ai.review-integration.consent/v3") {
		return text
	}
	payload := regexp.MustCompile(`(?s)\{.*\}`).FindString(text)
	var envelope struct {
		Choices []struct {
			Answer     string `json:"answer"`
			Invocation string `json:"invocation"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatalf("parse consent envelope: %v\n%s", err, text)
	}
	for _, choice := range envelope.Choices {
		if choice.Answer != "granted" {
			continue
		}
		// Issue #3181: the rendered invocation quotes paths (Windows --cwd),
		// so it must be tokenized by the shipped splitter, never by
		// whitespace alone.
		var granted bytes.Buffer
		if err := RunReview(invocationArgs(t, choice.Invocation), &granted); err != nil {
			t.Fatalf("the granted invocation the consent envelope named failed: %v\n%s", err, granted.String())
		}
		return granted.String()
	}
	t.Fatalf("consent envelope carries no granted choice:\n%s", text)
	return ""
}
