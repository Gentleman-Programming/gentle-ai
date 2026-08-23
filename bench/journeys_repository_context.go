package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type repositoryContextReference struct {
	Capability     string `json:"capability"`
	Handle         string `json:"handle"`
	Revision       string `json:"revision"`
	TargetIdentity string `json:"target_identity"`
}

type repositoryContextStart struct {
	LineageID         string                     `json:"lineage_id"`
	RepositoryContext repositoryContextReference `json:"repository_context"`
}

type repositoryContextStatus struct {
	Authority struct {
		LineageID string `json:"lineage_id"`
		Revision  string `json:"revision"`
	} `json:"authority"`
	TargetIdentity    string                      `json:"target_identity"`
	RepositoryContext *repositoryContextReference `json:"repository_context"`
}

func driveRepositoryContextFreshProcesses(r *journeyRun) error {
	negotiated, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	if negotiated.NextTransition.Kind != "execute" || negotiated.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("repository-context START transition = %+v", negotiated.NextTransition)
	}
	args, err := printedCommandArguments(negotiated.NextTransition.Execute.Command)
	if err != nil {
		return err
	}
	for index, argument := range args {
		if argument == "--consent=relay" {
			args[index] = "--consent=granted"
		}
	}
	started := r.run(args, false)
	var start repositoryContextStart
	if err := decodeRepositoryContextObservation(r, started, &start, "START"); err != nil {
		return err
	}
	want := start.RepositoryContext
	if start.LineageID == "" || want.Capability != "review.opaque_repository_context" ||
		!strings.HasPrefix(want.Handle, "rctx1_") || want.Revision == "" || want.TargetIdentity == "" {
		return fmt.Errorf("START repository context = %+v, lineage=%q", want, start.LineageID)
	}
	if leaksRepositoryPath(started.Stdout, r.sandbox.Repo) {
		return fmt.Errorf("START leaked repository path in its public envelope")
	}

	var capture statusEnvelope
	for attempt := 1; attempt <= 2; attempt++ {
		observation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContractV2,
			"--lineage", start.LineageID, "--next-transition"), false)
		var status repositoryContextStatus
		if err := decodeRepositoryContextObservation(r, observation, &status, fmt.Sprintf("STATUS retry %d", attempt)); err != nil {
			return err
		}
		if status.RepositoryContext == nil || *status.RepositoryContext != want ||
			status.Authority.LineageID != start.LineageID || status.Authority.Revision != want.Revision ||
			status.TargetIdentity != want.TargetIdentity {
			return fmt.Errorf("STATUS retry %d changed repository context: authority=%+v context=%+v", attempt, status.Authority, status.RepositoryContext)
		}
		if leaksRepositoryPath(observation.Stdout, r.sandbox.Repo) {
			return fmt.Errorf("STATUS retry %d leaked repository path in its public envelope", attempt)
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &capture); err != nil {
			return fmt.Errorf("parse STATUS capture binding: %w", err)
		}
	}

	foreign := "gairc_v1_" + strings.Repeat("0", 64)
	if capture.NextTransition.Kind != "collect" || len(capture.NextTransition.Collect.Inputs) != 1 ||
		capture.Authority.LineageID != start.LineageID || capture.TargetIdentity != want.TargetIdentity ||
		capture.argument("lineage") != start.LineageID || capture.argument("target") != want.TargetIdentity ||
		capture.argument("expected-revision") != want.Revision || capture.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash == "" {
		return fmt.Errorf("STATUS did not preserve target, repository/worktree context, and subject binding: %+v", capture)
	}
	payload, err := synthesizeReviewerResult(capture.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash, capture.paths())
	if err != nil {
		return err
	}
	input, err := writeScratch(r.sandbox, "foreign-context-reviewer.json", payload)
	if err != nil {
		return err
	}
	refused := r.run([]string{"review", "capture-result",
		"--lineage", capture.argument("lineage"), "--target", capture.argument("target"),
		"--expected-revision", capture.argument("expected-revision"), "--lens", capture.argument("lens"),
		"--order", capture.argument("order"), "--repository-context", foreign, "--input", input}, false)
	if refused.ExitCode == 0 || !strings.Contains(refused.Stderr, "repository context") {
		return fmt.Errorf("foreign repository context was not rejected: exit=%d stderr=%q", refused.ExitCode, firstLine(refused.Stderr))
	}
	after := r.run(productArgsFor(r, "review", "status", "--contract", reviewContractV2, "--lineage", start.LineageID, "--next-transition"), false)
	var unchanged repositoryContextStatus
	if err := decodeRepositoryContextObservation(r, after, &unchanged, "STATUS after foreign handle"); err != nil {
		return err
	}
	if unchanged.RepositoryContext == nil || *unchanged.RepositoryContext != want {
		return fmt.Errorf("foreign-handle refusal changed repository context: %+v", unchanged.RepositoryContext)
	}
	return nil
}

func decodeRepositoryContextObservation(r *journeyRun, observation Observation, value any, label string) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("%s exited %d: %s", label, observation.ExitCode, firstLine(observation.Stderr))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), value); err != nil {
		return fmt.Errorf("parse %s: %w", label, err)
	}
	return nil
}

func leaksRepositoryPath(output, repo string) bool {
	return strings.Contains(output, repo) || strings.Contains(output, strings.ReplaceAll(repo, `\`, `\\`))
}

func repositoryContextJourneys() []Journey {
	return []Journey{{
		ID:     "j104-repository-context-survives-fresh-process",
		Review: reviewOptedIn,
		Title:  "#3417: repository context preserves target, repository/worktree, and subject integrity across fresh START and STATUS processes",
		Source: "issue #1875 under #3417: the current path-free repository context binds target, repository/worktree, and reviewer subject integrity; retired event/outcome placeholders do not exist",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: staged ordinary-code candidate", Fixture: stageOrdinaryCode},
			{Name: "drive fresh START, exact active-lineage STATUS retries, and foreign-handle refusal", Requires: frozenLineageStatusCapability, Composite: driveRepositoryContextFreshProcesses},
		},
	}}
}
