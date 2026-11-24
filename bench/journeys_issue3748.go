package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const issue3748CodexLineage = "issue-3748-codex-committed-continuation"

var issue3748CodexCaptureCapability = &Capability{Verb: []string{"review", "capture-result"}, Flags: []string{
	"--agent", "--repository-context", "--expected-revision", "--lineage", "--target", "--lens", "--order",
}}

func issue3748Journeys() []Journey {
	return []Journey{{
		ID:     "j116-codex-committed-correction-runs-returned-status-continuation",
		Review: reviewOptedIn,
		Title:  "Codex committed correction keeps its frozen exclusion when the returned STATUS continuation runs beside an unrelated untracked file",
		Source: "issues #3748/#6058: Codex parity for committed correction re-entry; an unrelated untracked file must not reopen the frozen empty intended-untracked exclusion",
		Steps: []Step{
			{Name: "fixture: committed medium-risk candidate and isolated Codex runtime", Fixture: issue3748CodexFixture},
			{Name: "negotiate and start the committed Codex review", Requires: statusCapability, Composite: issue3748StartCodexReview},
			{Name: "Codex reports a candidate-caused blocker and returns correction_required", Requires: issue3748CodexCaptureCapability, Composite: issue3748CaptureCodexBlocker},
			{Name: "the returned ordered STATUS tokens re-enter the frozen committed correction beside an unrelated untracked file", Requires: statusCapability, Composite: issue3748ExecuteCodexContinuation},
		},
	}}
}

func issue3748CodexFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	baseTree, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	sandbox.Scratch["issue-3748-base-tree"] = baseTree
	const candidate = "package candidate\n\nfunc value() int {\n\treturn 1\n}\n"
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), candidate); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "feat: committed Codex candidate"); err != nil {
		return err
	}

	bin := filepath.Join(sandbox.Root, "codex-bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	const codex = `#!/bin/sh
set -eu
prompt="$0.prompt"
cat > "$prompt"
output=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output-last-message) output=$2; shift 2 ;;
    *) shift ;;
  esac
done
if grep -q 'gentle-ai.review-provider-refuter-request/v1' "$prompt"; then
  hash=$(grep -o '"request_hash":"[^"]*"' "$prompt" | head -n 1 | sed 's/.*:"\(.*\)"/\1/')
  test -n "$hash" && test -n "$output"
  results=
  for id in $(grep -o '"finding_id":"[^"]*"' "$prompt" | sed 's/.*:"\(.*\)"/\1/' | sort -u); do
    results="$results${results:+,}{\"finding_id\":\"$id\",\"outcome\":\"corroborated\",\"proof_refs\":[\"candidate.go:4 returns the wrong value in the frozen candidate\"]}"
  done
  printf '{"refuter_request_hash":"%s","results":[%s]}\n' "$hash" "$results" > "$output"
  exit 0
fi
subject=$(sed -n 's/.*"subject_hash"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$prompt" | head -n 1)
test -n "$subject" && test -n "$output"
printf '{"subject_hash":"%s","inspection":{"status":"completed","paths":["candidate.go"]},"lens":"review-reliability","findings":[{"location":"candidate.go:4","severity":"CRITICAL","claim":"the committed candidate returns the wrong value","proof_refs":["candidate.go:4 is introduced by the committed candidate"],"evidence_class":"deterministic","causal_disposition":"introduced"}],"evidence":["the frozen committed candidate returns 1 instead of the required value"]}\n' "$subject" > "$output"
`
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(codex), 0o755); err != nil {
		return err
	}
	sandbox.PathOverride = bin
	return nil
}

func issue3748Status(r *journeyRun) (statusEnvelope, error) {
	return readStatusForContract(r, reviewContractV2,
		"--agent", "codex", "--lineage", issue3748CodexLineage,
		"--base-ref", r.sandbox.Scratch["issue-3748-base-tree"], "--committed-only")
}

func issue3748StartCodexReview(r *journeyRun) error {
	status, err := issue3748Status(r)
	if err != nil || status.NextTransition.Kind != "execute" || status.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("committed Codex START transition = %+v, %v", status.NextTransition, err)
	}
	for name, want := range map[string]string{
		"agent": "codex", "lineage": issue3748CodexLineage,
		"base-ref": r.sandbox.Scratch["issue-3748-base-tree"], "committed-only": "true",
	} {
		if status.executeArgument(name) != want {
			return fmt.Errorf("committed Codex START %s = %q, want %q", name, status.executeArgument(name), want)
		}
	}
	relay, err := runPrintedTransition(r, status)
	if err != nil {
		return err
	}
	started, err := resolveAtomicStartConsentAt(r, r.sandbox.Repo, status, relay)
	if err != nil {
		return err
	}
	var result atomicReviewStartResult
	if err := json.Unmarshal([]byte(started.Stdout), &result); err != nil {
		return err
	}
	if result.LineageID != issue3748CodexLineage || result.State != "reviewing" || len(result.SelectedLenses) != 1 {
		return fmt.Errorf("committed Codex START = %+v", result)
	}
	return nil
}

func issue3748CaptureCodexBlocker(r *journeyRun) error {
	status, err := issue3748Status(r)
	if err != nil || status.NextTransition.Kind != "collect" || len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("committed Codex capture transition = %+v, %v", status.NextTransition, err)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "reviewer_result" || input.CaptureOperation != "review.capture-result" {
		return fmt.Errorf("committed Codex capture input = %+v", input)
	}
	// The reviewing authority's own target identity is the frozen committed
	// range the correction must keep binding after an unrelated untracked file
	// appears (#6058).
	if status.TargetIdentity == "" {
		return errors.New("committed Codex reviewing STATUS carried no target identity")
	}
	r.sandbox.Scratch["issue-3748-target-identity"] = status.TargetIdentity
	arguments := []string{"review", "capture-result"}
	for _, argument := range input.Arguments {
		if argument.Token == "" {
			return errors.New("committed Codex capture omitted an ordered argument token")
		}
		arguments = append(arguments, argument.Token)
	}
	closure := r.run(arguments, true)
	if closure.ExitCode != 0 {
		return fmt.Errorf("committed Codex capture: %s", firstLine(closure.Stderr))
	}
	var result lastEventClosure
	if err := json.Unmarshal([]byte(strings.TrimSpace(closure.Stdout)), &result); err != nil {
		return err
	}
	if result.LineageID != issue3748CodexLineage || result.State != "correction_required" || result.StatusContinuation == nil {
		return fmt.Errorf("committed Codex closure = %+v", result)
	}
	r.sandbox.Scratch["issue-3748-closure"] = closure.Stdout
	return nil
}

func issue3748ExecuteCodexContinuation(r *journeyRun) error {
	closure := Observation{Stdout: r.sandbox.Scratch["issue-3748-closure"]}
	var result lastEventClosure
	if err := json.Unmarshal([]byte(closure.Stdout), &result); err != nil {
		return err
	}
	want := map[string]bool{
		"--lineage=" + issue3748CodexLineage:                      false,
		"--base-ref=" + r.sandbox.Scratch["issue-3748-base-tree"]: false,
		"--committed-only=true":                                   false,
		"--agent=codex":                                           false,
	}
	for _, argument := range result.StatusContinuation.Arguments {
		if _, found := want[argument.Token]; found {
			want[argument.Token] = true
		}
	}
	for token, found := range want {
		if !found {
			return fmt.Errorf("Codex correction continuation omitted %q", token)
		}
	}
	// #6058: an unrelated untracked file exists when the exact returned
	// continuation runs. The frozen empty selection is still the authority's
	// decision, so STATUS must keep offering the same correction plan instead
	// of demanding a fresh live declaration.
	const untracked = "unrelated-local-notes.md"
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, untracked), "local-only notes outside the frozen committed range\n"); err != nil {
		return err
	}
	porcelain, err := gitOut(r.sandbox, r.sandbox.Repo, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	if !strings.Contains(porcelain, "?? "+untracked) {
		return fmt.Errorf("fixture did not leave %q untracked: %s", untracked, porcelain)
	}
	status, terminal, err := correctionStatusFromLastEventCapture(r, closure)
	if err != nil || !terminal {
		return fmt.Errorf("execute Codex correction continuation beside %q: terminal=%t err=%v", untracked, terminal, err)
	}
	frozenTarget := r.sandbox.Scratch["issue-3748-target-identity"]
	if frozenTarget == "" {
		return errors.New("fixture did not record the frozen committed target identity")
	}
	if status.Authority.LineageID != issue3748CodexLineage || status.Authority.State != "correction_required" ||
		status.TargetIdentity != frozenTarget || status.Projection.BaseTree != r.sandbox.Scratch["issue-3748-base-tree"] ||
		status.NextTransition.ReasonCode != "correction_plan_required" || status.NextTransition.Kind != "collect" ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("Codex committed correction STATUS beside %q = authority=%+v target=%q projection=%+v transition=%+v, want the frozen target %q and correction_plan_required (#6058)",
			untracked, status.Authority, status.TargetIdentity, status.Projection, status.NextTransition, frozenTarget)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "correction_lines" || input.CaptureOperation != "review.capture-correction-plan" ||
		status.argument("lineage") != issue3748CodexLineage || status.argument("target") != frozenTarget ||
		status.argument("expected-revision") == "" || status.argument("request-hash") == "" {
		return fmt.Errorf("Codex committed correction plan input = %+v lineage=%q target=%q, want the frozen target %q (#6058)",
			input, status.argument("lineage"), status.argument("target"), frozenTarget)
	}
	// The inventory was published: the unrelated file was eligible, and the
	// frozen exclusion still won. This is what distinguishes the fix from a
	// status that never saw the file.
	if status.EligibleUntrackedInventory == "" {
		return errors.New("Codex committed correction STATUS beside an unrelated untracked file published no eligible_untracked_inventory (#6058)")
	}
	return nil
}
