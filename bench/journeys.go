package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// reviewContract is the negotiated integration contract the status envelope
// needs before it will emit next_transition.
const reviewContract = "gentle-ai.review-integration/v1"

const rejectedRecaptureLineage = "rejected-capture-recapture"

// statusEnvelope is the subset of `review status --next-transition` this
// benchmark reads. Unknown fields are ignored so older and newer envelopes
// both parse.
type statusEnvelope struct {
	// rawJSON retains the exact STATUS bytes only when a correction closure
	// executes its provider-owned continuation. Reduced fields below are for
	// journey assertions and must never be used to reconstruct that binding.
	rawJSON string

	Schema string `json:"schema"`
	// EligibleUntrackedInventory is issue #4040's fix: the canonical
	// untracked-inventory digest published unconditionally at the STATUS
	// top level (design decision 2), distinct from the same digest embedded
	// inside NextTransition.Collect.Inputs[].Arguments, which only appears
	// while a selection is still undeclared.
	EligibleUntrackedInventory string `json:"eligible_untracked_inventory"`

	Authority struct {
		LineageID string `json:"lineage_id"`
		State     string `json:"state"`
		Revision  string `json:"revision"`
	} `json:"authority"`
	TargetIdentity string `json:"target_identity"`
	Projection     struct {
		BaseTree             string   `json:"base_tree"`
		CurrentCandidateTree string   `json:"current_candidate_tree"`
		PathsDigest          string   `json:"paths_digest"`
		Paths                []string `json:"paths"`
	} `json:"projection"`
	NextTransition struct {
		Kind       string `json:"kind"`
		ReasonCode string `json:"reason_code"`
		Collect    struct {
			Inputs []struct {
				Name             string `json:"name"`
				CaptureOperation string `json:"capture_operation"`
				Arguments        []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
					Token string `json:"token"`
				} `json:"arguments"`
				ArtifactSubject struct {
					SubjectHash string `json:"subject_hash"`
				} `json:"artifact_subject"`
				ChangedPathManifest []struct {
					Path string `json:"path"`
				} `json:"changed_path_manifest"`
			} `json:"inputs"`
		} `json:"collect"`
		Execute struct {
			Operation string `json:"operation"`
			Command   string `json:"command"`
			Arguments []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
				Token string `json:"token"`
			} `json:"arguments"`
		} `json:"execute"`
	} `json:"next_transition"`
}

func (e statusEnvelope) argument(name string) string {
	if len(e.NextTransition.Collect.Inputs) == 0 {
		return ""
	}
	for _, argument := range e.NextTransition.Collect.Inputs[0].Arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	return ""
}

func (e statusEnvelope) executeArgument(name string) string {
	for _, argument := range e.NextTransition.Execute.Arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	return ""
}

func (e statusEnvelope) paths() []string {
	if len(e.NextTransition.Collect.Inputs) == 0 {
		return nil
	}
	paths := []string{}
	for _, entry := range e.NextTransition.Collect.Inputs[0].ChangedPathManifest {
		paths = append(paths, entry.Path)
	}
	return paths
}

// statusCapability is the CLI surface every composite step needs.
var statusCapability = &Capability{
	Verb:  []string{"review", "status"},
	Flags: []string{"--next-transition", "--contract", "--cwd"},
}

var captureResultCapability = &Capability{
	Verb:  []string{"review", "capture-result"},
	Flags: []string{"--lineage", "--target", "--expected-revision", "--lens", "--order", "--input"},
}

var captureEvidenceCapability = &Capability{
	Verb:  []string{"review", "capture-evidence"},
	Flags: []string{"--lineage", "--target", "--expected-revision", "--input"},
}

// readStatus issues one `review status --next-transition`. The invocation is
// counted: an agent driving this flow really does have to spend it.
func readStatus(r *journeyRun) (statusEnvelope, error) {
	return readStatusFor(r)
}

func readStatusFor(r *journeyRun, selectors ...string) (statusEnvelope, error) {
	return readStatusForContract(r, reviewContract, selectors...)
}

func readStatusForContract(r *journeyRun, contract string, selectors ...string) (statusEnvelope, error) {
	args := append([]string{"review", "status", "--cwd", r.sandbox.Repo, "--contract", contract, "--next-transition"}, selectors...)
	observation := r.run(args, false)
	var envelope statusEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &envelope); err != nil {
		return envelope, fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	return envelope, nil
}

// synthesizeReviewerResult builds a clean reviewer result from the binary's
// OWN collect envelope. This is what makes "model runs" countable without
// spending a single real token: the subject hash and inspected paths come
// straight from the product, so the capture is accepted for the same reason a
// real reviewer's would be.
func synthesizeReviewerResult(subjectHash string, paths []string) ([]byte, error) {
	if subjectHash == "" {
		return nil, errors.New("collect envelope carried no subject hash")
	}
	paths = append([]string{}, paths...)
	for left, right := 0, len(paths)-1; left < right; left, right = left+1, right-1 {
		paths[left], paths[right] = paths[right], paths[left]
	}
	return json.Marshal(map[string]any{
		"subject_hash": subjectHash,
		"inspection": map[string]any{
			"status": "completed",
			"paths":  paths,
		},
		"findings": []any{},
		"evidence": []string{"inspected the complete frozen candidate scope named by the capture binding"},
	})
}

func writeScratch(sandbox *Sandbox, name string, content []byte) (string, error) {
	path := filepath.Join(sandbox.Root, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// captureAllLenses drives the collect loop the product itself dictates: read
// the next transition, synthesize the reviewer result it asks for, capture it,
// repeat. Each capture counts as one model run.
func captureAllLenses(r *journeyRun) error {
	return captureAllLensesFor(r)
}

func captureAllLensesFor(r *journeyRun, selectors ...string) error {
	_, err := captureAllLensesWithLastCaptureFor(r, selectors...)
	return err
}

// captureAllLensesWithLastCaptureFor preserves the final capture response for
// callers whose next lifecycle step is provider-owned by that closure.
func captureAllLensesWithLastCaptureFor(r *journeyRun, selectors ...string) (Observation, error) {
	var last Observation
	for round := 0; round < 8; round++ {
		envelope, err := readStatusFor(r, selectors...)
		if err != nil {
			return Observation{}, err
		}
		if envelope.NextTransition.Kind != "collect" || envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
			return last, nil
		}
		result, err := synthesizeReviewerResult(
			envelope.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash, envelope.paths())
		if err != nil {
			return Observation{}, err
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("reviewer-%d.json", round), result)
		if err != nil {
			return Observation{}, err
		}
		last = r.run([]string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", envelope.argument("lineage"),
			"--target", envelope.argument("target"),
			"--expected-revision", envelope.argument("expected-revision"),
			"--lens", envelope.argument("lens"),
			"--order", envelope.argument("order"),
			"--input", path,
		}, true)
		if last.ExitCode != 0 {
			return Observation{}, fmt.Errorf("capture reviewer result: %s", firstLine(last.Stderr))
		}
		var closure lastEventClosure
		if json.Unmarshal([]byte(strings.TrimSpace(last.Stdout)), &closure) == nil && closure.State == "correction_required" {
			return last, nil
		}
	}
	return Observation{}, errors.New("lens capture loop did not converge")
}

const correctionPlanStatusContinuationKeyPrefix = "last-event-correction-plan-status:"

type lastEventClosure struct {
	LineageID          string `json:"lineage_id"`
	State              string `json:"state"`
	StatusContinuation *struct {
		Operation string `json:"operation"`
		Arguments []struct {
			Token string `json:"token"`
		} `json:"arguments"`
	} `json:"status_continuation"`
}

// correctionStatusFromLastEventCapture executes the closure's status
// continuation as operation plus ordered provider-issued tokens. It never
// reconstructs selectors from a lineage, fixture, or retained status state.
func correctionStatusFromLastEventCapture(r *journeyRun, capture Observation) (statusEnvelope, bool, error) {
	if capture.ExitCode != 0 {
		return statusEnvelope{}, false, fmt.Errorf("terminal reviewer capture failed: %s", firstLine(capture.Stderr))
	}
	var closure lastEventClosure
	if err := json.Unmarshal([]byte(strings.TrimSpace(capture.Stdout)), &closure); err != nil {
		return statusEnvelope{}, false, fmt.Errorf("decode last-event closure: %w", err)
	}
	if closure.State != "correction_required" {
		return statusEnvelope{}, false, nil
	}
	if closure.LineageID == "" || closure.StatusContinuation == nil || closure.StatusContinuation.Operation != "review.status" {
		return statusEnvelope{}, false, fmt.Errorf("correction closure omitted its status continuation: %+v", closure)
	}
	arguments := []string{"review", "status"}
	for _, argument := range closure.StatusContinuation.Arguments {
		if argument.Token == "" {
			return statusEnvelope{}, false, errors.New("correction status continuation omitted an argument token")
		}
		arguments = append(arguments, argument.Token)
	}
	statusObservation := r.run(arguments, false)
	if statusObservation.ExitCode != 0 {
		return statusEnvelope{}, false, fmt.Errorf("execute correction status continuation: %s", firstLine(statusObservation.Stderr))
	}
	var status statusEnvelope
	if err := json.Unmarshal([]byte(statusObservation.Stdout), &status); err != nil {
		return statusEnvelope{}, false, fmt.Errorf("decode correction status continuation: %w", err)
	}
	status.rawJSON = statusObservation.Stdout
	if status.Authority.LineageID != closure.LineageID || status.Authority.State != "correction_required" ||
		status.NextTransition.ReasonCode != "correction_plan_required" {
		return statusEnvelope{}, false, fmt.Errorf("correction status continuation = authority=%+v transition=%+v, want lineage %q and correction_plan_required", status.Authority, status.NextTransition, closure.LineageID)
	}
	return status, true, nil
}

// rememberCorrectionStatusContinuation retains the exact correction-plan STATUS
// returned by the provider-owned continuation. Its only destructive consumer is
// the successful bounded-plan capture in captureCorrectionPlanFor.
func rememberCorrectionStatusContinuation(r *journeyRun, lineage string, status statusEnvelope) error {
	if status.rawJSON == "" {
		return errors.New("correction status continuation omitted raw STATUS JSON")
	}
	r.sandbox.Scratch[correctionPlanStatusContinuationKeyPrefix+lineage] = status.rawJSON
	return nil
}

func readCorrectionPlanStatusContinuation(r *journeyRun, lineage string) (string, bool, error) {
	payload, found := r.sandbox.Scratch[correctionPlanStatusContinuationKeyPrefix+lineage]
	if !found {
		return "", false, nil
	}
	return payload, true, nil
}

// takeCorrectionStatusContinuation is retained for assertion helpers. Reading a
// carried correction-plan STATUS is deliberately non-destructive; only the plan
// consumer clears it after a successful bounded-plan advancement.
func takeCorrectionStatusContinuation(r *journeyRun, lineage string) (statusEnvelope, bool, error) {
	payload, found, err := readCorrectionPlanStatusContinuation(r, lineage)
	if err != nil || !found {
		return statusEnvelope{}, found, err
	}
	var status statusEnvelope
	if err := json.Unmarshal([]byte(payload), &status); err != nil {
		return statusEnvelope{}, false, fmt.Errorf("decode carried correction status continuation: %w", err)
	}
	return status, true, nil
}

func clearCorrectionPlanStatusContinuation(r *journeyRun, lineage string) {
	delete(r.sandbox.Scratch, correctionPlanStatusContinuationKeyPrefix+lineage)
}

// captureFinalEvidence answers the verification-evidence collect step.
func captureFinalEvidence(r *journeyRun) error {
	return captureFinalEvidenceFor(r)
}

func captureFinalEvidenceFor(r *journeyRun, selectors ...string) error {
	envelope, err := readStatusFor(r, selectors...)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "collect" {
		return nil
	}
	path, err := writeScratch(r.sandbox, "final-evidence.txt",
		[]byte("go build ./... ok\ngo test ./... ok\nall packages passed\n"))
	if err != nil {
		return err
	}
	r.run([]string{
		"review", "capture-evidence", "--cwd", r.sandbox.Repo,
		"--lineage", envelope.argument("lineage"),
		"--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"),
		"--outcome", "passed",
		"--input", path,
	}, false)
	return nil
}

// rejectedThenRecapture spends one reviewer run on a result the product
// refuses (the subject hash does not echo the binding), then spends a second
// run on the correct one. Both count as model runs: the rejected one really
// was paid for.
func rejectedThenRecaptureFor(r *journeyRun, lineage string) error {
	envelope, err := readAtomicReviewStatus(r, lineage)
	if err != nil {
		return err
	}
	if envelope.Authority.LineageID != lineage || envelope.NextTransition.Kind != "collect" ||
		len(envelope.NextTransition.Collect.Inputs) == 0 || envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		return errors.New("expected an exact active-lineage reviewer-result collect transition")
	}
	bad, err := synthesizeReviewerResult(
		envelope.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash, nil)
	if err != nil {
		return err
	}
	badPath, err := writeScratch(r.sandbox, "reviewer-rejected.json", bad)
	if err != nil {
		return err
	}
	refused := r.run([]string{
		"review", "capture-result", "--cwd", r.sandbox.Repo,
		"--lineage", envelope.argument("lineage"),
		"--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"),
		"--lens", envelope.argument("lens"),
		"--order", envelope.argument("order"),
		"--input", badPath,
	}, true)
	if refused.ExitCode == 0 {
		return errors.New("incomplete exact active-lineage reviewer inspection was accepted")
	}
	return captureExactSelectedReviewerSlots(r, lineage, false)
}

func finalizeRejectedRecapture(r *journeyRun) error {
	observation := r.run(productArgsFor(r, "review", "finalize", "--lineage", rejectedRecaptureLineage, "--captured-evidence=true"), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("finalize rejected-recapture evidence: %s", firstLine(observation.Stderr))
	}
	if err := requirePendingApproval(rejectedRecaptureLineage)(r.sandbox, observation); err != nil {
		return err
	}
	return requireAtomicLineageAcknowledged(r, rejectedRecaptureLineage)
}

// executeNextTransitionVerbatim is the guide's flow 11: take the tokens the
// product prints and run them exactly, with no repair. If a token is not a
// complete flag, this step fails and the friction is visible.
func executeNextTransitionVerbatim(r *journeyRun) error {
	_, err := runNextTransitionVerbatim(r)
	return err
}

// runNextTransitionVerbatim is the same drive, returning what the transition
// answered. Steps that only need it to have run use the wrapper above; a step
// that has to prove the transition CHANGED something needs the observation,
// because running verbatim and succeeding is not the same as arriving anywhere.
func runNextTransitionVerbatim(r *journeyRun) (Observation, error) {
	envelope, err := readStatus(r)
	if err != nil {
		return Observation{}, err
	}
	return runPrintedTransition(r, envelope)
}

// runPrintedTransition runs the command the product PRINTED, exactly as
// printed.
//
// It deliberately does not re-derive the verb from the operation name, which
// is what this corpus used to do. That re-derivation was the reason a journey
// named "the printed transition executes exactly as printed" kept passing over
// an execute transition whose `command` was empty: the benchmark quietly
// assembled the command the product owed the reader, ran its own, and reported
// the flow as continuing. "Runs verbatim" has to mean the printed bytes, or it
// measures the benchmark instead of the product.
//
// It is also more correct than the split it replaces. An operation name is not
// a verb -- "review.retry_final_verification" and "review.bind_sdd" spell
// their verbs with hyphens -- so splitting on "." only ever produced a runnable
// verb by coincidence.
func runPrintedTransition(r *journeyRun, envelope statusEnvelope) (Observation, error) {
	args, err := printedTransitionArguments(envelope)
	if err != nil {
		return Observation{}, err
	}
	return r.run(args, false), nil
}

// runPrintedTransitionAt preserves the exact rendered command while letting a
// worktree-isolation journey run it in the worktree that STATUS bound.
func runPrintedTransitionAt(r *journeyRun, cwd string, envelope statusEnvelope) (Observation, error) {
	args, err := printedTransitionArguments(envelope)
	if err != nil {
		return Observation{}, err
	}
	return r.runAt(cwd, args, false), nil
}

func printedTransitionArguments(envelope statusEnvelope) ([]string, error) {
	if envelope.NextTransition.Kind != "execute" {
		return nil, fmt.Errorf("expected an execute transition, got %q", envelope.NextTransition.Kind)
	}
	args, err := printedCommandArguments(envelope.NextTransition.Execute.Command)
	if err != nil {
		return nil, fmt.Errorf("execute transition for %q %w", envelope.NextTransition.Execute.Operation, err)
	}
	return args, nil
}

// printedCommandArguments turns one printed command line into the argv a POSIX
// shell would hand the product, and refuses anything that is not a complete,
// immediately runnable `gentle-ai ...` invocation.
func printedCommandArguments(command string) ([]string, error) {
	words, err := splitPrintedCommandWords(command)
	if err != nil {
		return nil, err
	}
	if len(words) == 0 {
		return nil, errors.New("carried no command to run")
	}
	if words[0] != productName {
		return nil, fmt.Errorf("printed a command that starts with %q, not %q", words[0], productName)
	}
	if len(words) == 1 {
		return nil, fmt.Errorf("printed a command that names no arguments: %q", command)
	}
	if !HasRunnableCommand(command) {
		return nil, fmt.Errorf("printed a command that is not runnable as printed: %q", command)
	}
	return words[1:], nil
}

// splitPrintedCommandWords splits a printed command line into shell words.
//
// It understands the single and double quotes the product emits. Within double
// quotes it applies POSIX's narrow backslash rules without evaluating any shell
// syntax. Anything else -- an unterminated quote or a trailing escape -- is a
// line that would not run as printed, and saying so is the point.
func splitPrintedCommandWords(line string) ([]string, error) {
	words := []string{}
	var word strings.Builder
	var quote rune
	escaped, doubleQuotedEscape, started := false, false, false
	for _, char := range line {
		switch {
		case escaped:
			if char != '\n' {
				if doubleQuotedEscape && char != '$' && char != '`' && char != '"' && char != '\\' {
					word.WriteRune('\\')
				}
				word.WriteRune(char)
			}
			escaped, doubleQuotedEscape = false, false
		case quote != 0 && char == quote:
			quote = 0
		case quote == '"' && char == '\\':
			escaped, doubleQuotedEscape, started = true, true, true
		case quote != 0:
			word.WriteRune(char)
		case char == '\\':
			escaped, started = true, true
		case char == '\'' || char == '"':
			quote, started = char, true
		case char == ' ' || char == '\t' || char == '\n' || char == '\r':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("printed a command with an unterminated quote: %q", line)
	}
	if escaped {
		return nil, fmt.Errorf("printed a command with a trailing escape: %q", line)
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func baseRepo(sandbox *Sandbox) error {
	if err := sandbox.initRepo(sandbox.Repo); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "README.md"), "# demo\n\nhello\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "initial")
}

func withRemote(sandbox *Sandbox) error {
	sandbox.Remote = filepath.Join(sandbox.Home, "demo-remote.git")
	if err := sandbox.git(sandbox.Home, "init", "--bare", "-q", sandbox.Remote); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "remote", "add", "origin", sandbox.Remote); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "push", "-q", "-u", "origin", "HEAD")
}

func baseRepoWithRemote(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	return withRemote(sandbox)
}

func stageDocs(marker string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		path := filepath.Join(sandbox.Repo, "docs", marker+".md")
		if err := sandbox.write(path, "# "+marker+"\n\nplain prose, no executable content.\n"); err != nil {
			return err
		}
		return sandbox.git(sandbox.Repo, "add", "-A")
	}
}

func stageAuthCode(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Repo, "internal", "auth", "session.go")
	content := "package auth\n\n// CheckToken reports whether a session token is present.\nfunc CheckToken(token string) bool {\n\treturn token != \"\"\n}\n"
	if err := sandbox.write(path, content); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "-A")
}

func stageOrdinaryCode(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Repo, "internal", "format", "text.go")
	content := "package format\n\n// Title upper-cases the first rune of a label.\nfunc Title(label string) string {\n\tif label == \"\" {\n\t\treturn label\n\t}\n\treturn strings.ToUpper(label[:1]) + label[1:]\n}\n"
	if err := sandbox.write(path, content); err != nil {
		return err
	}
	// j12 reverses the inspected-path manifest; two ordinary files make that proof observable.
	path = filepath.Join(sandbox.Repo, "internal", "format", "whitespace.go")
	content = "package format\n\n// IsBlank reports whether a label contains no characters.\nfunc IsBlank(label string) bool {\n\treturn label == \"\"\n}\n"
	if err := sandbox.write(path, content); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "-A")
}

func stageLargeDocs(sandbox *Sandbox) error {
	for index := 0; index < 4; index++ {
		builder := strings.Builder{}
		builder.WriteString(fmt.Sprintf("# chapter %d\n\n", index))
		for line := 0; line < 300; line++ {
			builder.WriteString(fmt.Sprintf("Paragraph %d of chapter %d, ordinary prose with no executable content.\n", line, index))
		}
		if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", fmt.Sprintf("chapter-%d.md", index)), builder.String()); err != nil {
			return err
		}
	}
	return sandbox.git(sandbox.Repo, "add", "-A")
}

func commitStaged(message string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error { return sandbox.git(sandbox.Repo, "commit", "-qm", message) }
}

func pushHead(sandbox *Sandbox) error {
	return sandbox.git(sandbox.Repo, "push", "-q", "origin", "HEAD")
}

func breakRemoteForIssue1890(sandbox *Sandbox) error {
	return sandbox.git(sandbox.Repo, "remote", "set-url", "origin", filepath.Join(sandbox.Root, "missing-remote.git"))
}

func addUnreachableRemoteForIssue1890(sandbox *Sandbox) error {
	return sandbox.git(sandbox.Repo, "remote", "add", "backup", filepath.Join(sandbox.Root, "missing-remote.git"))
}

func issue1890PrePushArgs(baseRef string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		if strings.TrimSpace(sandbox.Lineage) == "" {
			return nil, errors.New("review start did not leave a lineage for the pre-push gate")
		}
		return []string{"review", "validate", "--lineage", sandbox.Lineage, "--gate", "pre-push", "--base-ref", baseRef, "--cwd", sandbox.Repo}, nil
	}
}

func assertIssue1890RemoteFailure(_ *Sandbox, observation Observation) error {
	if observation.ExitCode == 0 || !strings.Contains(observation.Stderr, "git ls-remote --heads origin main failed") {
		return fmt.Errorf("pre-push did not preserve the qualified ls-remote failure: %s", observation.Stderr)
	}
	return nil
}

func assertIssue1890ValidRemoteWins(_ *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("pre-push did not use the valid advertised remote: %s", observation.Stderr)
	}
	return nil
}

func unbornRepo(sandbox *Sandbox) error {
	sandbox.Repo = filepath.Join(sandbox.Home, "unborn")
	if err := sandbox.initRepo(sandbox.Repo); err != nil {
		return err
	}
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	if err := sandbox.write(filepath.Join(sandbox.Repo, "main.go"), content); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "-A")
}

// ---------------------------------------------------------------------------
// Argument builders
// ---------------------------------------------------------------------------

func productArgs(parts ...string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		args := make([]string, 0, len(parts)+2)
		args = append(args, parts...)
		return append(args, "--cwd", sandbox.Repo), nil
	}
}

func rememberLineage(sandbox *Sandbox, observation Observation) error {
	if lineage, ok := envelopeString(observation.Stdout, "lineage_id"); ok {
		sandbox.Lineage = lineage
	}
	if target, ok := envelopeString(observation.Stdout, "target_identity"); ok {
		sandbox.Target = target
	}
	if revision, ok := envelopeString(observation.Stdout, "store_revision"); ok {
		sandbox.Revision = revision
	}
	return nil
}

// assertReviewParseRefusalsPreflight keeps parser refusals inside a composite
// because a direct unknown-flag step is classified as unsupported before its
// After assertion can inspect the negotiated envelope. The positive equals
// forms remain covered by TestReviewBooleanFlagSpacedValueNamesTheEqualsForm
// and core journey j02's --captured-results=true transition.
func assertReviewParseRefusalsPreflight(run *journeyRun, operation, booleanFlag string) error {
	cases := []struct {
		name, cause string
		args        []string
		usage       bool
	}{
		{name: "unknown flag", args: []string{"--unknown-" + operation + "-flag"}, cause: "flag provided but not defined: -unknown-" + operation + "-flag", usage: true},
		{name: "detached boolean", args: []string{"--" + booleanFlag, "true"}, cause: "boolean flag --" + booleanFlag + " takes --" + booleanFlag + " or --" + booleanFlag + "=true, not a separate value; got \"true\""},
	}
	for _, test := range cases {
		for _, negotiated := range []bool{true, false} {
			mode := "plain"
			args := []string{"review", operation}
			if negotiated {
				mode = "negotiated"
				args = append(args, "--contract", reviewContract)
			}
			observation := run.run(productArgsFor(run, append(args, test.args...)...), false)
			if observation.ExitCode == 0 {
				return fmt.Errorf("%s %s %s accepted a parser refusal", operation, test.name, mode)
			}
			if negotiated {
				var failure struct {
					Code            string `json:"code"`
					Phase           string `json:"phase"`
					MutationOutcome string `json:"mutation_outcome"`
					RetrySafe       bool   `json:"retry_safe"`
					Replayability   string `json:"replayability"`
					NextAction      string `json:"next_action"`
					Cause           string `json:"cause"`
				}
				if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &failure); err != nil {
					return fmt.Errorf("decode %s %s %s envelope: %w (stderr: %s)", operation, test.name, mode, err, firstLine(observation.Stderr))
				}
				if failure.Code != "invalid_request" || failure.Phase != "preflight" ||
					failure.MutationOutcome != "not_started" || !failure.RetrySafe ||
					failure.Replayability != "not_replayable" || failure.NextAction != "correct_request" || failure.Cause != test.cause {
					return fmt.Errorf("%s %s %s failure = code=%q phase=%q mutation_outcome=%q retry_safe=%t replayability=%q next_action=%q cause=%q", operation, test.name, mode, failure.Code, failure.Phase, failure.MutationOutcome, failure.RetrySafe, failure.Replayability, failure.NextAction, failure.Cause)
				}
			} else {
				if got := strings.TrimSpace(observation.Stderr); got != "Error: "+test.cause {
					return fmt.Errorf("%s %s plain diagnostic = %q, want %q", operation, test.name, got, "Error: "+test.cause)
				}
				usage := "Usage: gentle-ai review " + operation + " [flags]"
				if got := strings.Contains(observation.Stdout, usage); got != test.usage {
					return fmt.Errorf("%s %s plain usage %t, want %t", operation, test.name, got, test.usage)
				}
			}
			if _, err := os.Stat(filepath.Join(run.sandbox.Repo, ".git", "gentle-ai", "defect-reports")); !errors.Is(err, os.ErrNotExist) {
				if err == nil {
					return fmt.Errorf("%s %s %s refusal wrote a defect report", operation, test.name, mode)
				}
				return fmt.Errorf("inspect %s %s %s defect reports: %w", operation, test.name, mode, err)
			}
		}
	}
	return nil
}

var startCapability = &Capability{Verb: []string{"review", "start"}, Flags: []string{"--cwd"}}
var startParseRefusalCapability = &Capability{Verb: []string{"review", "start"}, Flags: []string{"--cwd", "--contract", "--committed-only"}}
var finalizeParseRefusalCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd", "--contract", "--captured-results"}}

// Capabilities declare only the flags the step actually uses. Over-declaring
// would report an older binary as `unsupported` for a step it can in fact run.
var finalizeCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd"}}
var finalizeResultsCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd", "--captured-results"}}
var finalizeEvidenceCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd", "--captured-evidence"}}
var validateCapability = &Capability{Verb: []string{"review", "validate"}, Flags: []string{"--cwd", "--gate"}}
var validateBaseRefCapability = &Capability{Verb: []string{"review", "validate"}, Flags: []string{"--cwd", "--gate", "--base-ref", "--lineage"}}
var modeCapability = &Capability{Verb: []string{"review", "mode"}, Flags: []string{"--cwd", "--json"}}
var abandonCapability = &Capability{Verb: []string{"review", "abandon"}, Flags: []string{"--lineage", "--expected-revision", "--reason", "--actor", "--maintainer-authorization"}}

// ---------------------------------------------------------------------------
// Corpus
// ---------------------------------------------------------------------------

// Journeys is the corpus, as data. Adding one is appending a Journey.
//
// It is deliberately weighted toward failure paths: the happy path is where a
// product looks good, and friction lives everywhere else.
//
// coreJourneys below are the flows drawn from the community testing guide and
// the failure paths it collected; edgeJourneys in journeys_edge.go are the edge
// cases those flows never reached; sddJourneys in journeys_sdd.go is the SDD
// remediation successor cycle and fail-closed authority controls; and
// waveOneJourneys pins integrated community fixes at their real CLI boundary.
func Journeys() []Journey {
	journeys := append(coreJourneys(), edgeJourneys()...)
	journeys = append(journeys, sddJourneys()...)
	journeys = append(journeys, issue2891Journeys()...)
	journeys = append(journeys, issue2696Journeys()...)
	journeys = append(journeys, sddChainJourneys()...)
	journeys = append(journeys, issue3094Journeys()...)
	journeys = append(journeys, issue3065Journeys()...)
	journeys = append(journeys, captureEvidenceDescriptorJourneys()...)
	journeys = append(journeys, scopeChangedFixtureJourneys()...)
	journeys = append(journeys, waveOneJourneys()...)
	journeys = append(journeys, waveThreeJourneys()...)
	journeys = append(journeys, atomicReviewJourneys()...)
	journeys = append(journeys, waveFiveJourneys()...)
	journeys = append(journeys, zeroDeltaJourneys()...)
	journeys = append(journeys, lensContextBudgetJourneys()...)
	journeys = append(journeys, localGateBaseAdvanceJourneys()...)
	journeys = append(journeys, intendedUntrackedJourneys()...)
	journeys = append(journeys, selectedUntrackedSDDJourneys()...)
	journeys = append(journeys, captureResultDryRunJourneys()...)
	journeys = append(journeys, issue2031Journeys()...)
	journeys = append(journeys, findingIDPrefixJourneys()...)
	journeys = append(journeys, rescopeWriteGuardJourneys()...)
	journeys = append(journeys, rescopeEvidenceOnlyRetryJourneys()...)
	journeys = append(journeys, consecutiveRescopeRepairJourneys()...)
	journeys = append(journeys, reviewedSupersetJourneys()...)
	journeys = append(journeys, stagedDeliveryJourneys()...)
	journeys = append(journeys, frozenLineageResumeJourneys()...)
	journeys = append(journeys, issue1800Journeys()...)
	journeys = append(journeys, issue2879Journeys()...)
	journeys = append(journeys, managedAssetJourneys()...)
	journeys = append(journeys, issue2906Journeys()...)
	journeys = append(journeys, issue2138Journeys()...)
	journeys = append(journeys, issue3043Journeys()...)
	journeys = append(journeys, issue3557Journeys()...)
	journeys = append(journeys, issue3561Journeys()...)
	journeys = append(journeys, repositoryContextJourneys()...)
	journeys = append(journeys, providerCaptureRetryJourneys()...)
	journeys = append(journeys, capturedProviderValidatorJourneys()...)
	journeys = append(journeys, sddSharedScaffoldingJourneys()...)
	journeys = append(journeys, sddPostReviewVerifyReportJourneys()...)
	journeys = append(journeys, issue3564Journeys()...)
	journeys = append(journeys, issue3321Journeys()...)
	journeys = append(journeys, issue3587Journeys()...)
	journeys = append(journeys, issue3748Journeys()...)
	journeys = append(journeys, issue3772Journeys()...)
	journeys = append(journeys, issue3776Journeys()...)
	journeys = append(journeys, issue3766Journeys()...)
	journeys = append(journeys, issue3813Journeys()...)
	journeys = append(journeys, issue3842Journeys()...)
	journeys = append(journeys, handoffJourneys()...)
	journeys = append(journeys, stopHookJourneys()...)
	journeys = append(journeys, untrackedInventoryRecoveryLoopJourneys()...)
	journeys = removeRetiredAtomicJourneys(journeys)
	return declareCoreJourneyReviewModes(journeys)
}

func coreJourneys() []Journey {
	return []Journey{
		{
			ID:     "j01-docs-happy-path",
			Review: reviewOptedIn,
			Title:  "Documentation change: review, approve, commit, push gate",
			Source: "guide flow 3 + flow 9 step 2",
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage docs", Fixture: stageDocs("intro")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "gate post-apply", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit", Fixture: commitStaged("docs: intro")},
				{Name: "gate pre-push", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push")},
			},
		},
		{
			ID:     "j03-kill-switch",
			Review: reviewUntouched,
			Title:  "Kill switch: disable, start refused, re-enable, review",
			Source: "guide flow 2",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "mode status", Requires: modeCapability, Args: productArgs("review", "mode", "status", "--json")},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "mode status after disable", Requires: modeCapability, Args: productArgs("review", "mode", "status", "--json")},
				{Name: "fixture: stage docs", Fixture: stageDocs("switch")},
				{Name: "review start while disabled", Requires: startCapability, Args: productArgs("review", "start")},
				{Name: "mode enable", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--json")},
				{Name: "review start after enable", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
			},
		},
		{
			ID:     "j04-size-does-not-escalate",
			Review: reviewOptedIn,
			Title:  "#3417: 1200 lines of prose remain low risk and burn their terminal transaction",
			Source: "#3417 atomic review keeps risk selection separate from durable delivery authorization",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage 1200 lines of docs", Fixture: stageLargeDocs},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "low-risk finalization burns the transaction", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: func(sandbox *Sandbox, observation Observation) error {
					return requirePendingApproval(sandbox.Lineage)(sandbox, observation)
				}},
			},
		},
		{
			ID:     "j05-gate-without-any-review",
			Review: reviewOptedIn,
			Title:  "#3417: lifecycle validation before review is informational and unmanaged",
			Source: "#3417 removes ordinary-path receipt gates; delivery remains under repository policy",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageDocs("ungated")},
				{Name: "pre-commit validation is informational without a review", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
						return requireUnmanagedShippedGate(observation, "pre-commit")
					}},
			},
		},
		{
			ID:     "j06-pre-push-after-publication",
			Review: reviewOptedIn,
			Title:  "Failure path: pre-push after the reviewed commit was already pushed",
			Source: "guide flow 9",
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage docs", Fixture: stageDocs("published")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit", Fixture: commitStaged("docs: published")},
				{Name: "gate pre-push before publishing", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push")},
				{Name: "fixture: push", Fixture: pushHead},
				{Name: "gate pre-push after publishing", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-push"), AbortOnBlock: true},
			},
		},
		{
			ID:     "j97-pre-push-preserves-ls-remote-failure",
			Review: reviewOptedIn,
			Title:  "Failure path: pre-push preserves an advertised remote query failure",
			Source: "issue #1890: advertised remote identity and ls-remote failures must not become semantic selector errors",
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage docs", Fixture: stageDocs("remote-failure")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit", Fixture: commitStaged("docs: remote failure")},
				{Name: "fixture: make origin unavailable", Fixture: breakRemoteForIssue1890},
				{Name: "gate pre-push preserves ls-remote failure", Requires: validateBaseRefCapability,
					Args: issue1890PrePushArgs("origin/main"), After: assertIssue1890RemoteFailure, AbortOnBlock: true},
			},
		},
		{
			ID:     "j100-pre-push-unqualified-selector-ignores-unreachable-remote",
			Review: reviewOptedIn,
			Title:  "Failure path: pre-push selects the valid remote for an unqualified selector",
			Source: "issue #1890: an unqualified advertised branch ignores unrelated remote identity or query failures when exactly one valid match remains",
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage docs", Fixture: stageDocs("unqualified-remote-failure")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
				{Name: "gate pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
				{Name: "fixture: commit", Fixture: commitStaged("docs: unqualified remote failure")},
				{Name: "fixture: add unrelated unreachable remote", Fixture: addUnreachableRemoteForIssue1890},
				{Name: "gate pre-push selects valid advertised remote", Requires: validateBaseRefCapability,
					Args: issue1890PrePushArgs("main"), After: assertIssue1890ValidRemoteWins, AbortOnBlock: true},
			},
		},
		{
			ID:     "j07-disabled-with-stale-receipts",
			Review: reviewOptedIn,
			Title:  "Failure path: reviews disabled with two stale receipts present",
			Source: "guide flow 6 + flow 9 step 5",
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage first docs", Fixture: stageDocs("first")},
				{Name: "review start (first)", Requires: startCapability, Args: productArgs("review", "start")},
				{Name: "review finalize (first)", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
				{Name: "fixture: commit first", Fixture: commitStaged("docs: first")},
				{Name: "fixture: stage second docs", Fixture: stageDocs("second")},
				{Name: "review start (second)", Requires: startCapability, Args: productArgs("review", "start")},
				{Name: "review finalize (second)", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
				{Name: "fixture: commit second", Fixture: commitStaged("docs: second")},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "fixture: stage third docs", Fixture: stageDocs("third")},
				{Name: "fixture: commit third", Fixture: commitStaged("docs: third")},
				{Name: "gate pre-push while disabled", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-push"), AbortOnBlock: true},
			},
		},
		{
			ID:     "j08-finalize-without-reviewer-results",
			Review: reviewOptedIn,
			Title:  "Failure path: finalize a high-risk review with no reviewer results",
			Source: "community failure path",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "finalize with no results", Requires: finalizeCapability,
					Args: productArgs("review", "finalize"), AbortOnBlock: true},
			},
		},
		{
			ID:     "j09-finalize-without-evidence",
			Review: reviewOptedIn,
			Title:  "Failure path: finalize with results but no captured evidence",
			Source: "guide flow 12",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "finalize without evidence", Requires: finalizeCapability,
					Args: productArgs("review", "finalize"), AbortOnBlock: true},
			},
		},
		{
			ID:     "j10-invalid-flag-combination",
			Review: reviewOptedIn,
			Title:  "Failure path: staged projection combined with a base ref",
			Source: "guide flow 13",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageDocs("ambiguous")},
				{Name: "start with staged projection and base ref",
					Requires:     &Capability{Verb: []string{"review", "start"}, Flags: []string{"--projection", "--base-ref"}},
					Args:         productArgs("review", "start", "--projection", "staged", "--base-ref", "HEAD"),
					AbortOnBlock: true},
			},
		},
		{
			ID:     "j11-unborn-head",
			Review: reviewOptedIn,
			Title:  "Failure path: first commit in a repository with no history",
			Source: "guide flow 10",
			Steps: []Step{
				{Name: "fixture: unborn repo with staged code", Fixture: unbornRepo},
				{Name: "review start on unborn HEAD", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
			},
		},
		{
			ID:     "j12-rejected-capture-then-recapture",
			Review: reviewOptedIn,
			Title:  "#3587: an exact active-lineage reviewer result is rejected, then the full selected set recaptures",
			Source: "#2614 under #3587: incomplete inspection coverage refuses on its exact active lineage, then an unordered complete manifest recaptures",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage ordinary code", Fixture: stageOrdinaryCode},
				{Name: "review start with an exact active lineage", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", rejectedRecaptureLineage), After: rememberLineage},
				{Name: "exact active-lineage rejected capture then full selected-set recapture", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return rejectedThenRecaptureFor(r, rejectedRecaptureLineage)
				}},
				{Name: "the final accepted capture exposes acknowledgement before the exact active-lineage transaction burns", Requires: statusCapability, Composite: func(r *journeyRun) error {
					return requireAtomicLineageAcknowledged(r, rejectedRecaptureLineage)
				}},
			},
		},
		{
			ID:     "j13-next-transition-runs-verbatim",
			Review: reviewOptedIn,
			Title:  "Agent path: the printed transition executes exactly as printed",
			Source: "guide flow 11",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "run the printed transition verbatim", Requires: statusCapability, Composite: executeNextTransitionVerbatim},
			},
		},
		{
			ID:     "j14-abandon-needs-a-hand-built-token",
			Review: reviewOptedIn,
			Title:  "Maintainer path: abandoning a non-terminal lineage binds its discarded work",
			Source: "review abandon contract",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage high-risk code", Fixture: stageAuthCode},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "abandon a non-terminal lineage with its V2 binding", Requires: abandonCapability, Composite: abandonNonTerminalLineage},
			},
		},
		{
			ID:     "j85-review-parse-refusals-are-preflight",
			Review: reviewOptedIn,
			Title:  "Historical parser/refusal compatibility: START and FINALIZE remain preflight and non-mutating",
			Source: "#1956 historical parser/refusal compatibility: argv parsing happens before review authority can mutate",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "START parser refusals preserve their preflight contract", Requires: startParseRefusalCapability, Composite: func(run *journeyRun) error { return assertReviewParseRefusalsPreflight(run, "start", "committed-only") }},
				{Name: "FINALIZE parser refusals preserve their preflight contract", Requires: finalizeParseRefusalCapability, Composite: func(run *journeyRun) error {
					return assertReviewParseRefusalsPreflight(run, "finalize", "captured-results")
				}},
			},
		},
	}
}

// abandonNonTerminalLineage is the manual_tokens exhibit. Abandoning a lineage
// needs an exact nine-line LF-only V2 binding assembled from its status row,
// including the discarded-work summary the gate re-derives.
func abandonNonTerminalLineage(r *journeyRun) error {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo}, false)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &head); err != nil {
		return fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if len(head.Entries) != 1 {
		return fmt.Errorf("review status listed %d authorities, want exactly one", len(head.Entries))
	}
	entry := head.Entries[0]
	const actor = "bench"
	const reason = "operator_disposition"

	// Attempt one: everything the help text lists except the token.
	r.run([]string{
		"review", "abandon", "--cwd", r.sandbox.Repo,
		"--lineage", entry.LineageID,
		"--expected-revision", entry.Revision,
		"--reason", reason,
		"--actor", actor,
	}, false)

	authorization := renderAbandonAuthorization(entry, actor, reason)

	r.run([]string{
		"review", "abandon", "--cwd", r.sandbox.Repo,
		"--lineage", entry.LineageID,
		"--expected-revision", entry.Revision,
		"--reason", reason,
		"--actor", actor,
		"--maintainer-authorization", authorization,
	}, false)
	return nil
}
