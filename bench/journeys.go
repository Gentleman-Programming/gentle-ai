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
	NextTransition struct {
		Kind    string `json:"kind"`
		Collect struct {
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
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo, "--contract", reviewContract, "--next-transition"}, false)
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
	if len(paths) == 0 {
		paths = []string{"."}
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
	for round := 0; round < 8; round++ {
		envelope, err := readStatus(r)
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
	envelope, err := readStatus(r)
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
		"sha256:0000000000000000000000000000000000000000000000000000000000000000", envelope.paths())
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
	if envelope.NextTransition.Kind != "execute" {
		return Observation{}, fmt.Errorf("expected an execute transition, got %q", envelope.NextTransition.Kind)
	}
	verb := strings.SplitN(envelope.NextTransition.Execute.Operation, ".", 2)
	if len(verb) != 2 {
		return Observation{}, fmt.Errorf("execute operation %q is not <verb>.<subcommand>", envelope.NextTransition.Execute.Operation)
	}
	args := []string{verb[0], verb[1], "--cwd", r.sandbox.Repo}
	for _, argument := range envelope.NextTransition.Execute.Arguments {
		if argument.Token == "" {
			return Observation{}, fmt.Errorf("argument %q carried no runnable token", argument.Name)
		}
		args = append(args, argument.Token)
	}
	return r.run(args, false), nil
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

var startCapability = &Capability{Verb: []string{"review", "start"}, Flags: []string{"--cwd"}}

// Capabilities declare only the flags the step actually uses. Over-declaring
// would report an older binary as `unsupported` for a step it can in fact run.
var finalizeCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd"}}
var finalizeResultsCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd", "--captured-results"}}
var finalizeEvidenceCapability = &Capability{Verb: []string{"review", "finalize"}, Flags: []string{"--cwd", "--captured-evidence"}}
var validateCapability = &Capability{Verb: []string{"review", "validate"}, Flags: []string{"--cwd", "--gate"}}
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
// remediation successor cycle and the two surfaces that meet it, which nothing
// in the first two parts had ever driven.
func Journeys() []Journey {
	journeys := append(coreJourneys(), edgeJourneys()...)
	return append(journeys, sddJourneys()...)
}

func coreJourneys() []Journey {
	return []Journey{
		{
			ID:     "j01-docs-happy-path",
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
			ID:     "j07-disabled-with-stale-receipts",
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
			Title:  "Failure path: a reviewer result the product rejects, then a recapture",
			Source: "community failure path: binding mismatch",
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
			Title:  "Maintainer path: abandoning a pristine lineage needs an assembled authorization",
			Source: "review abandon contract",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage docs", Fixture: stageDocs("abandoned")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "abandon a pristine lineage", Requires: abandonCapability, Composite: abandonPristineLineage},
			},
		},
	}
}

// abandonPristineLineage is the manual_tokens exhibit. Abandoning a lineage
// needs an exact six-line LF-only binding assembled by hand from three values
// the caller must first go and read out of the authority.
func abandonPristineLineage(r *journeyRun) error {
	envelope, err := readStatus(r)
	if err != nil {
		return err
	}
	lineage := envelope.Authority.LineageID
	revision := envelope.Authority.Revision
	target := envelope.TargetIdentity
	const actor = "bench"
	const reason = "benchmark abandons a pristine lineage"

	// Attempt one: everything the help text lists except the token.
	r.run([]string{
		"review", "abandon", "--cwd", r.sandbox.Repo,
		"--lineage", lineage,
		"--expected-revision", revision,
		"--reason", reason,
		"--actor", actor,
	}, false)

	authorization := strings.Join([]string{
		"gentle-ai.review-abandon-authorization/v1",
		"lineage=" + lineage,
		"revision=" + revision,
		"snapshot_identity=" + target,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")

	r.run([]string{
		"review", "abandon", "--cwd", r.sandbox.Repo,
		"--lineage", lineage,
		"--expected-revision", revision,
		"--reason", reason,
		"--actor", actor,
		"--maintainer-authorization", authorization,
	}, false)
	return nil
}
