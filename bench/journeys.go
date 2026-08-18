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

// statusEnvelope is the subset of `review status --next-transition` this
// benchmark reads. Unknown fields are ignored so older and newer envelopes
// both parse.
type statusEnvelope struct {
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
	for round := 0; round < 8; round++ {
		envelope, err := readStatusFor(r, selectors...)
		if err != nil {
			return err
		}
		if envelope.NextTransition.Kind != "collect" {
			return nil
		}
		if envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
			return nil
		}
		result, err := synthesizeReviewerResult(
			envelope.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash, envelope.paths())
		if err != nil {
			return err
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("reviewer-%d.json", round), result)
		if err != nil {
			return err
		}
		r.run([]string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", envelope.argument("lineage"),
			"--target", envelope.argument("target"),
			"--expected-revision", envelope.argument("expected-revision"),
			"--lens", envelope.argument("lens"),
			"--order", envelope.argument("order"),
			"--input", path,
		}, true)
	}
	return errors.New("lens capture loop did not converge")
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
func rejectedThenRecapture(r *journeyRun) error {
	envelope, err := readStatus(r)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "collect" {
		return errors.New("expected a reviewer-result collect transition")
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
	r.run([]string{
		"review", "capture-result", "--cwd", r.sandbox.Repo,
		"--lineage", envelope.argument("lineage"),
		"--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"),
		"--lens", envelope.argument("lens"),
		"--order", envelope.argument("order"),
		"--input", badPath,
	}, true)
	return captureAllLenses(r)
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
	if envelope.NextTransition.Kind != "execute" {
		return Observation{}, fmt.Errorf("expected an execute transition, got %q", envelope.NextTransition.Kind)
	}
	args, err := printedCommandArguments(envelope.NextTransition.Execute.Command)
	if err != nil {
		return Observation{}, fmt.Errorf("execute transition for %q %w", envelope.NextTransition.Execute.Operation, err)
	}
	return r.run(args, false), nil
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
	journeys = append(journeys, captureEvidenceDescriptorJourneys()...)
	journeys = append(journeys, scopeChangedFixtureJourneys()...)
	journeys = append(journeys, waveOneJourneys()...)
	journeys = append(journeys, waveThreeJourneys()...)
	journeys = append(journeys, waveFiveJourneys()...)
	journeys = append(journeys, zeroDeltaJourneys()...)
	journeys = append(journeys, lensContextBudgetJourneys()...)
	journeys = append(journeys, localGateBaseAdvanceJourneys()...)
	journeys = append(journeys, intendedUntrackedJourneys()...)
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
	journeys = append(journeys, repositoryContextJourneys()...)
	journeys = append(journeys, providerCaptureRetryJourneys()...)
	journeys = append(journeys, capturedProviderValidatorJourneys()...)
	journeys = append(journeys, sddSharedScaffoldingJourneys()...)
	journeys = append(journeys, sddPostReviewVerifyReportJourneys()...)
	return append(journeys, handoffJourneys()...)
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
			ID:     "j02-high-risk-four-lens",
			Review: reviewOptedIn,
			Title:  "High-risk code change: four lenses, evidence, approval",
			Source: "guide flow 4 + the full native bounded review contract",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "finalize without evidence", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
				{Name: "gate post-apply", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
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
			Title:  "1200 lines of prose still reviews as low risk",
			Source: "guide flow 4 step 3",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage 1200 lines of docs", Fixture: stageLargeDocs},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
				{Name: "gate post-apply", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "post-apply")},
			},
		},
		{
			ID:     "j05-gate-without-any-review",
			Review: reviewOptedIn,
			Title:  "Failure path: lifecycle gate before any review exists",
			Source: "community failure path: receipt missing",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageDocs("ungated")},
				{Name: "gate pre-commit with no receipt", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), AbortOnBlock: true},
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
			Title:  "Failure path: a reviewer result the product rejects, then a recapture",
			Source: "#2614: incomplete inspection coverage refuses, then an unordered complete manifest recaptures",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage ordinary code", Fixture: stageOrdinaryCode},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "rejected capture then recapture", Requires: captureResultCapability, Composite: rejectedThenRecapture},
				{Name: "finalize with captured results", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--captured-results=true")},
				{Name: "capture final evidence", Requires: captureEvidenceCapability, Composite: captureFinalEvidence},
				{Name: "finalize with captured evidence", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--captured-evidence=true")},
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
				{Name: "fixture: stage docs", Fixture: stageDocs("abandoned")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "abandon a non-terminal lineage with its V2 binding", Requires: abandonCapability, Composite: abandonNonTerminalLineage},
			},
		},
		{
			ID:     "j85-review-parse-refusals-are-preflight",
			Review: reviewOptedIn,
			Title:  "START and FINALIZE parser refusals are preflight and non-mutating",
			Source: "#1956: argv parsing happens before review authority can mutate",
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
