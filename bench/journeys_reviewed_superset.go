package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	reviewedSupersetJourneyID           = "j82-reviewed-superset-pre-push-allows-unpublished-subset"
	reviewedSupersetMovingBaseJourneyID = "j83-pre-pr-moving-advertised-base-binds-merge-base"
	reviewedSupersetLineage             = "reviewed-superset-pre-push"
	reviewedSupersetPathA               = "docs/reviewed-a.md"
	reviewedSupersetPathB               = "docs/reviewed-b.md"
	reviewedSupersetBaseAdvancePath     = "docs/disjoint-base-advance.md"
)

var reviewedSupersetFinalizeCapability = &Capability{
	Verb:  []string{"review", "finalize"},
	Flags: []string{"--cwd", "--lineage"},
}

// reviewedSupersetStatus is the native status envelope evidence this journey
// needs to distinguish an approved full receipt from a merely valid Git setup.
type reviewedSupersetStatus struct {
	Authority *struct {
		LineageID string `json:"lineage_id"`
		State     string `json:"state"`
	} `json:"authority"`
	Receipt struct {
		Status   string `json:"status"`
		Identity string `json:"identity"`
	} `json:"receipt"`
	Projection struct {
		BaseTree             string   `json:"base_tree"`
		CurrentCandidateTree string   `json:"current_candidate_tree"`
		PathsDigest          string   `json:"paths_digest"`
		Paths                []string `json:"paths"`
	} `json:"projection"`
}

// reviewedSupersetFixture creates the approved candidate's real publication
// topology before review: feature tracks origin/feature while both start at
// main, and the committed candidate changes exactly A and B.
func reviewedSupersetFixture(sandbox *Sandbox) error {
	if err := baseRepoWithRemote(sandbox); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "-b", "feature"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "push", "-q", "-u", "origin", "feature"); err != nil {
		return err
	}
	upstream, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "--abbrev-ref", "@{upstream}")
	if err != nil {
		return err
	}
	if upstream != "origin/feature" {
		return fmt.Errorf("fixture feature upstream = %q, want origin/feature", upstream)
	}
	base, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	sandbox.Scratch["reviewed-superset-base"] = base
	baseTree, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	sandbox.Scratch["reviewed-superset-base-tree"] = baseTree

	for path, content := range map[string]string{
		reviewedSupersetPathA: "# reviewed A\n\nFirst reviewed path.\n",
		reviewedSupersetPathB: "# reviewed B\n\nSecond reviewed path.\n",
	} {
		if err := sandbox.write(filepath.Join(sandbox.Repo, path), content); err != nil {
			return err
		}
	}
	if err := sandbox.git(sandbox.Repo, "add", "--", reviewedSupersetPathA); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "docs: reviewed A"); err != nil {
		return err
	}
	prefix, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	sandbox.Scratch["reviewed-superset-prefix"] = prefix
	if err := sandbox.git(sandbox.Repo, "add", "--", reviewedSupersetPathB); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "docs: reviewed B"); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--name-only", base+"..HEAD")
	if err != nil {
		return err
	}
	if paths != reviewedSupersetPathA+"\n"+reviewedSupersetPathB {
		return fmt.Errorf("fixture candidate paths = %q, want {%s,%s}", paths, reviewedSupersetPathA, reviewedSupersetPathB)
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	if err != nil {
		return err
	}
	sandbox.Scratch["reviewed-superset-tree"] = tree
	return nil
}

// startReviewedSupersetBaseDiff follows the product's negotiated path because
// direct committed base-diff starts are a distinct compatibility surface.
func startReviewedSupersetBaseDiff(run *journeyRun) error {
	envelope, err := readStatusFor(run, "--lineage", reviewedSupersetLineage, "--base-ref", run.sandbox.Scratch["reviewed-superset-base"])
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "execute" || envelope.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("committed reviewed-superset start transition = %+v", envelope.NextTransition)
	}
	started, err := runPrintedTransition(run, envelope)
	if err != nil {
		return err
	}
	if started.ExitCode != 0 {
		return fmt.Errorf("committed reviewed-superset start exited %d: %s", started.ExitCode, firstLine(started.Stderr))
	}
	if err := rememberLineage(run.sandbox, started); err != nil {
		return err
	}
	if run.sandbox.Lineage != reviewedSupersetLineage {
		return fmt.Errorf("committed reviewed-superset start lineage = %q, want %q", run.sandbox.Lineage, reviewedSupersetLineage)
	}
	return nil
}

func readReviewedSupersetStatus(run *journeyRun) (reviewedSupersetStatus, error) {
	observation := run.run(productArgsFor(run, "review", "status", "--contract", reviewContract, "--next-transition",
		"--lineage", reviewedSupersetLineage, "--base-ref", run.sandbox.Scratch["reviewed-superset-base"]), false)
	var status reviewedSupersetStatus
	if observation.ExitCode != 0 {
		return status, fmt.Errorf("reviewed-superset receipt status exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
		return status, fmt.Errorf("parse reviewed-superset receipt status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	return status, nil
}

func proveReviewedSupersetReceipt(run *journeyRun) error {
	status, err := readReviewedSupersetStatus(run)
	if err != nil {
		return err
	}
	if status.Authority == nil || status.Authority.LineageID != run.sandbox.Lineage || status.Authority.State != "approved" ||
		status.Receipt.Status != "present" || status.Receipt.Identity == "" ||
		status.Projection.BaseTree != run.sandbox.Scratch["reviewed-superset-base-tree"] ||
		status.Projection.CurrentCandidateTree != run.sandbox.Scratch["reviewed-superset-tree"] ||
		status.Projection.PathsDigest == "" ||
		strings.Join(status.Projection.Paths, "\x00") != reviewedSupersetPathA+"\x00"+reviewedSupersetPathB {
		return fmt.Errorf("approved full receipt was not proven: %+v", status)
	}
	return nil
}

// publishReviewedPrefix publishes only the already reviewed A prefix and proves
// that B is the sole outgoing path while HEAD stays the exact full candidate.
func publishReviewedPrefix(sandbox *Sandbox) error {
	prefix := sandbox.Scratch["reviewed-superset-prefix"]
	if err := sandbox.git(sandbox.Repo, "push", "-q", "origin", prefix+":feature"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "fetch", "-q", "origin", "feature"); err != nil {
		return err
	}

	head, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	headTree, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	upstream, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "@{upstream}")
	if err != nil {
		return err
	}
	outgoingPaths, err := gitOut(sandbox, sandbox.Repo, "diff", "--name-only", "@{upstream}..HEAD")
	if err != nil {
		return err
	}
	unpublished, err := gitOut(sandbox, sandbox.Repo, "rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		return err
	}
	clean, err := gitOut(sandbox, sandbox.Repo, "status", "--porcelain")
	if err != nil {
		return err
	}
	if head == prefix || headTree != sandbox.Scratch["reviewed-superset-tree"] || upstream != prefix ||
		outgoingPaths != reviewedSupersetPathB || unpublished != "1" || clean != "" {
		return fmt.Errorf("reviewed-prefix publication proof failed: head=%s prefix=%s head_tree=%s receipt_tree=%s upstream=%s outgoing_paths=%q unpublished=%s status=%q",
			head, prefix, headTree, sandbox.Scratch["reviewed-superset-tree"], upstream, outgoingPaths, unpublished, clean)
	}
	sandbox.Scratch["reviewed-superset-head-tree"] = headTree
	sandbox.Scratch["reviewed-superset-outgoing-paths"] = outgoingPaths
	return nil
}

// requireReviewedSupersetPrePushAllow records the native gate's denial stage and
// code in the RED failure, so a setup error cannot masquerade as issue #2127.
func requireReviewedSupersetPrePushAllow(sandbox *Sandbox, observation Observation) error {
	var gate waveGateResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &gate); err != nil {
		if observation.ExitCode != 0 {
			return fmt.Errorf("reviewed supersets pre-push exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
		}
		return fmt.Errorf("parse reviewed-superset pre-push result: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if !gate.Allowed || gate.Result != "allow" || gate.Context.LineageID != sandbox.Lineage {
		stage, code := "", ""
		if gate.Context.Denial != nil {
			stage, code = gate.Context.Denial.Stage, gate.Context.Denial.Code
		}
		return fmt.Errorf("reviewed supersets pre-push rejected: exit=%d result=%q allowed=%t receipt_tree=%s local_head_tree=%s receipt_paths={%s,%s} outgoing_paths={%s} denial_stage=%q denial_code=%q gate=%+v; want allow for the monotonic unpublished subset",
			observation.ExitCode, gate.Result, gate.Allowed, sandbox.Scratch["reviewed-superset-tree"], sandbox.Scratch["reviewed-superset-head-tree"], reviewedSupersetPathA, reviewedSupersetPathB, sandbox.Scratch["reviewed-superset-outgoing-paths"], stage, code, gate)
	}
	return nil
}

// advanceReviewedSupersetAdvertisedMain moves the advertised publication
// boundary from a sibling clone. The feature candidate remains untouched and
// the new base path is intentionally disjoint from the reviewed paths.
func advanceReviewedSupersetAdvertisedMain(sandbox *Sandbox) error {
	sibling := filepath.Join(sandbox.Root, "main-advance")
	if err := sandbox.git(sandbox.Root, "clone", "-q", "--no-checkout", sandbox.Remote, sibling); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "checkout", "-q", "-B", "main", "origin/main"); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "config", "user.email", "bench@example.invalid"); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "config", "user.name", "Bench"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sibling, reviewedSupersetBaseAdvancePath), "# Disjoint base advance\n"); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "add", "--", reviewedSupersetBaseAdvancePath); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "commit", "-qm", "docs: advance main"); err != nil {
		return err
	}
	if err := sandbox.git(sibling, "push", "-q", "origin", "main"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "fetch", "-q", "origin", "main"); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--name-only", sandbox.Scratch["reviewed-superset-base"]+"..origin/main")
	if err != nil {
		return err
	}
	if paths != reviewedSupersetBaseAdvancePath {
		return fmt.Errorf("advertised main paths = %q, want %q", paths, reviewedSupersetBaseAdvancePath)
	}
	return nil
}

func requireReviewedSupersetPrePRAllow(sandbox *Sandbox, observation Observation) error {
	var gate waveGateResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &gate); err != nil {
		if observation.ExitCode != 0 {
			return fmt.Errorf("moving-base pre-PR exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
		}
		return fmt.Errorf("parse moving-base pre-PR result: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if !gate.Allowed || gate.Result != "allow" || gate.Context.LineageID != sandbox.Lineage {
		stage, code := "", ""
		if gate.Context.Denial != nil {
			stage, code = gate.Context.Denial.Stage, gate.Context.Denial.Code
		}
		return fmt.Errorf("moving advertised base pre-PR rejected: exit=%d result=%q allowed=%t denial_stage=%q denial_code=%q gate=%+v", observation.ExitCode, gate.Result, gate.Allowed, stage, code, gate)
	}
	return nil
}

// executeReviewedSupersetMovingBaseTransition executes the exact validate
// command that negotiated STATUS prints after the advertised base advances.
func executeReviewedSupersetMovingBaseTransition(run *journeyRun) error {
	envelope, err := readStatusFor(run, "--gate", "pre-pr", "--base-ref", "origin/main")
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "execute" || envelope.NextTransition.Execute.Operation != "review.validate" ||
		envelope.executeArgument("gate") != "pre-pr" || envelope.executeArgument("base-ref") != "origin/main" {
		return fmt.Errorf("moving-base pre-PR transition = %+v, want review.validate --gate pre-pr --base-ref origin/main", envelope.NextTransition)
	}
	observation, err := runPrintedTransition(run, envelope)
	if err != nil {
		return err
	}
	return requireReviewedSupersetPrePRAllow(run.sandbox, observation)
}

func reviewedSupersetJourneys() []Journey {
	return []Journey{
		{
			ID:     reviewedSupersetJourneyID,
			Review: reviewOptedIn,
			Title:  "Approved full receipt: pre-push allows the unpublished reviewed subset",
			Source: "#2127 ratified content-addressed authorization: an outgoing subset of the exact approved candidate remains authorized",
			Steps: []Step{
				{Name: "fixture: feature tracks origin and commits reviewed paths A and B", Fixture: reviewedSupersetFixture},
				{Name: "negotiate and execute review start for the full candidate against main", Requires: statusCapability, Composite: startReviewedSupersetBaseDiff},
				{Name: "review finalize the full candidate", Requires: reviewedSupersetFinalizeCapability, Args: productArgs("review", "finalize", "--lineage", reviewedSupersetLineage)},
				{Name: "native status proves the approved receipt covers the exact full candidate and paths A and B", Requires: statusCapability, Composite: proveReviewedSupersetReceipt},
				{Name: "fixture: publish only reviewed prefix A and prove B remains unpublished", Fixture: publishReviewedPrefix},
				{Name: "pre-push allows the one-path unpublished subset B", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-push"), After: requireReviewedSupersetPrePushAllow},
			},
		},
		{
			ID:     reviewedSupersetMovingBaseJourneyID,
			Review: reviewOptedIn,
			Title:  "Approved feature receipt: pre-PR binds the merge-base after advertised main advances",
			Source: "#2127 R2 ratified content proof: an advertised ref is a publication boundary, while the unique merge-base binds candidate comparison",
			Steps: []Step{
				{Name: "fixture: feature tracks origin and commits reviewed paths A and B", Fixture: reviewedSupersetFixture},
				{Name: "negotiate and execute review start for the full feature candidate", Requires: statusCapability, Composite: startReviewedSupersetBaseDiff},
				{Name: "review finalize the full feature candidate", Requires: reviewedSupersetFinalizeCapability, Args: productArgs("review", "finalize", "--lineage", reviewedSupersetLineage)},
				{Name: "fixture: sibling clone advances advertised main on a disjoint path and feature fetches it", Fixture: advanceReviewedSupersetAdvertisedMain},
				{Name: "negotiated pre-PR STATUS executes its exact printed validation for origin/main", Requires: statusCapability, Composite: executeReviewedSupersetMovingBaseTransition},
				{Name: "unqualified pre-PR validation against origin/main allows the approved feature candidate", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-pr", "--base-ref", "origin/main"), After: requireReviewedSupersetPrePRAllow},
			},
		},
	}
}
