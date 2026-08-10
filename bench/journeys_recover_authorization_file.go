package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	recoverAuthorizationFilePredecessor = "recover-authorization-file-predecessor"
	recoverAuthorizationFileSuccessor   = "recover-authorization-file-successor"
)

var recoverAuthorizationFileCapability = &Capability{
	Verb: []string{"review", "recover"},
	Flags: []string{
		"--cwd", "--predecessor-lineage", "--expected-predecessor-revision",
		"--successor-lineage", "--disposition", "--actor", "--reason",
		"--maintainer-authorization-file",
	},
}

func recoverThroughAuthorizationFile(r *journeyRun) error {
	started, err := decodeWaveOperation(r.run(productArgsFor(r,
		"review", "start", "--lineage", recoverAuthorizationFilePredecessor), false), "authorization-file predecessor start")
	if err != nil {
		return err
	}
	status := r.run(productArgsFor(r, "review", "status"), false)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(status.Stdout)), &head); err != nil {
		return fmt.Errorf("parse authorization-file predecessor status: %w (stderr: %s)", err, firstLine(status.Stderr))
	}
	predecessor, ok := head.entry(recoverAuthorizationFilePredecessor)
	if !ok {
		return fmt.Errorf("authorization-file predecessor status listed no authority")
	}
	invalidated, err := decodeWaveOperation(r.run([]string{
		"review", "invalidate", "--cwd", r.sandbox.Repo,
		"--lineage", recoverAuthorizationFilePredecessor,
		"--expected-revision", predecessor.Revision,
		"--reason", "operator abandoned",
	}, false), "authorization-file predecessor invalidation")
	if err != nil {
		return err
	}

	const actor = "Maintainer <maintainer@example.com>"
	const reason = "authorize recovery from a file"
	authorization := strings.Join([]string{
		"gentle-ai.review-recovery-authorization/v1",
		"predecessor_lineage=" + recoverAuthorizationFilePredecessor,
		"predecessor_revision=" + invalidated.StoreRevision,
		"target_identity=" + started.TargetIdentity,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
	authorizationPath, err := writeScratch(r.sandbox, "recovery-authorization.txt", []byte(authorization+"\r\n"))
	if err != nil {
		return err
	}

	recovered, err := decodeWaveOperation(r.run(productArgsFor(r,
		"review", "recover",
		"--predecessor-lineage", recoverAuthorizationFilePredecessor,
		"--expected-predecessor-revision", invalidated.StoreRevision,
		"--successor-lineage", recoverAuthorizationFileSuccessor,
		"--disposition", "invalidated",
		"--actor", actor,
		"--reason", reason,
		"--maintainer-authorization-file", authorizationPath), false), "authorization-file recovery")
	if err != nil {
		return err
	}
	if recovered.LineageID != recoverAuthorizationFileSuccessor || recovered.State != "reviewing" {
		return fmt.Errorf("authorization-file recovery = %+v, want reviewing successor %q", recovered, recoverAuthorizationFileSuccessor)
	}
	return nil
}

func recoverAuthorizationFileJourneys() []Journey {
	return []Journey{
		{
			ID:     "j95-review-recover-accepts-authorization-file",
			Title:  "Recovery authorization: provider reads exact binding from a file with one trailing line ending",
			Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/1612",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: staged predecessor", Fixture: stageDocs("recover-authorization-file")},
				{Name: "mode enable", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--json")},
				{Name: "public recover reads exact authorization from file", Requires: recoverAuthorizationFileCapability, Composite: recoverThroughAuthorizationFile},
			},
		},
	}
}
