package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// hostCodexLane drives the REAL Codex host application: gentle-ai's compiled
// codex adapter spawns `codex exec --skip-git-repo-check --ignore-user-config
// --sandbox read-only -C <empty scratch> --output-last-message <result>` with
// the Go-materialized opaque provider prompt on stdin, exactly the transport
// the compiled boundary admits for the codex runtime identity.
const hostCodexLane = "host-codex"

// runHostCodexLane runs one medium candidate review where the reviewer
// capture executes with --agent codex: real codex process, prompt-carried
// evidence, native admission, lifecycle to the approved receipt.
func (b *battery) runHostCodexLane() {
	if _, err := exec.LookPath("codex"); err != nil {
		b.skip(hostCodexLane, "codex host lane", "codex binary is not on PATH")
		return
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if _, statErr := os.Stat(filepath.Join(home, ".codex", "auth.json")); statErr != nil {
			b.skip(hostCodexLane, "codex host lane", "no codex credentials (~/.codex/auth.json); the real reviewer spawn would fail without them")
			return
		}
	}

	repo, ok := b.hostMediumCandidate(hostCodexLane, "host-codex")
	if !ok {
		return
	}
	if !b.hostNegotiatedMediumStart(hostCodexLane, repo, "codex", nil) {
		return
	}
	statusDoc, stderr, _ := b.statusEnv(repo, "codex", nil)
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" || !hasArgument(input, "agent") {
		b.fail(hostCodexLane, "reviewer capture admitted", "no capture-result collect input carrying an agent argument; "+firstLine(stderr))
		return
	}
	if !b.hostCaptureLens(hostCodexLane, repo, nil, input) {
		return
	}
	b.hostFollowToReceipt(hostCodexLane, repo, "codex", nil)
}
