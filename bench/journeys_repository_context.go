package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
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

// assertOpaqueRepositoryContext proves the handle is what the capability name
// claims. The declared capability is review.opaque_repository_context and the
// handle is relayed on command lines and through host logs, so a reader holding
// it must be able to learn nothing about the filesystem it names: it is a
// fixed-width digest, it decodes to no structured payload, and it contains no
// path fragment.
func assertOpaqueRepositoryContext(handle string, secrets ...string) error {
	const prefix = "rctx2_"
	encoded, found := strings.CutPrefix(handle, prefix)
	if !found {
		return fmt.Errorf("repository context is not an rctx2 handle: %q", handle)
	}
	digest, err := hex.DecodeString(encoded)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("rctx2 handle is not a bare sha256 digest: %q", handle)
	}
	if encoded != strings.ToLower(encoded) {
		return fmt.Errorf("rctx2 handle is not canonical lowercase hex: %q", handle)
	}
	if json.Valid(digest) {
		return fmt.Errorf("rctx2 handle decodes to structured data: %q", handle)
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(handle, secret) {
			return fmt.Errorf("rctx2 handle is not opaque: %q leaks %q", handle, secret)
		}
	}
	return nil
}

// assertSealedOpenCodeRepositoryContext proves the handle an OpenCode provider
// task carries is the sealed rctx3 shape (#5136, #4516). Only OpenCode hosts
// receive it: their relay runs in the host session directory, so the handle
// names its own root -- sealed under a private per-user key, so a reader still
// learns nothing about the filesystem. It is canonical unpadded base64url,
// decodes to no structured payload, and neither its text nor its decoded bytes
// carry any path fragment. Every other runtime keeps the rctx2 digest above.
func assertSealedOpenCodeRepositoryContext(prompt string, secrets ...string) error {
	encoded, found := strings.CutPrefix(prompt, "GENTLE_AI_REVIEW_PROVIDER_TASK ")
	if !found {
		return fmt.Errorf("OpenCode provider task has no Go-issued binding: %q", firstLine(prompt))
	}
	var binding struct {
		RepositoryContext string `json:"repository_context"`
	}
	if err := json.Unmarshal([]byte(encoded), &binding); err != nil {
		return fmt.Errorf("parse OpenCode provider task binding: %w", err)
	}
	handle := binding.RepositoryContext
	sealedText, found := strings.CutPrefix(handle, "rctx3_")
	if !found {
		return fmt.Errorf("OpenCode provider task repository context is not an rctx3 handle: %q", handle)
	}
	sealed, err := base64.RawURLEncoding.Strict().DecodeString(sealedText)
	// nonce + identity digest + at least one root byte + authentication tag.
	if err != nil || len(sealed) < 12+sha256.Size+1+16 {
		return fmt.Errorf("rctx3 handle is not a canonical sealed payload: %q", handle)
	}
	if json.Valid(sealed) {
		return fmt.Errorf("rctx3 handle decodes to structured data: %q", handle)
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(handle, secret) {
			return fmt.Errorf("rctx3 handle is not opaque: %q carries %q", handle, secret)
		}
		for _, fragment := range []string{secret, filepath.Base(secret)} {
			if bytes.Contains(sealed, []byte(fragment)) {
				return fmt.Errorf("rctx3 handle is not sealed: its decoded bytes carry %q", fragment)
			}
		}
	}
	return nil
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
		!strings.HasPrefix(want.Handle, "rctx2_") || want.Revision == "" || want.TargetIdentity == "" {
		return fmt.Errorf("START repository context = %+v, lineage=%q", want, start.LineageID)
	}
	if err := assertOpaqueRepositoryContext(want.Handle, r.sandbox.Repo, r.sandbox.Home); err != nil {
		return err
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
			status.Authority.LineageID != start.LineageID || status.Authority.Revision == "" ||
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

	foreign := "rctx2_" + strings.Repeat("A", 64)
	if capture.NextTransition.Kind != "collect" || len(capture.NextTransition.Collect.Inputs) != 1 ||
		capture.Authority.LineageID != start.LineageID || capture.TargetIdentity != want.TargetIdentity ||
		capture.argument("lineage") != start.LineageID || capture.argument("target") != want.TargetIdentity ||
		capture.argument("expected-revision") != want.Revision || capture.argument("repository-context") != want.Handle ||
		capture.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash == "" {
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
		Title:  "#3797: opaque rctx2 digest preserves target, repository/worktree, and subject integrity across fresh START and STATUS processes",
		Source: "issue #3797: active rctx2 is an opaque authority-derived digest across fresh processes, carrying no filesystem path; rctx1 locators remain historical read-only only",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: staged ordinary-code candidate", Fixture: stageOrdinaryCode},
			{Name: "drive fresh START, exact active-lineage STATUS retries, and foreign-handle refusal", Requires: frozenLineageStatusCapability, Composite: driveRepositoryContextFreshProcesses},
		},
	}}
}
