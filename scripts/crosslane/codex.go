package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const codexLane = "codex"

const codexModel = `#!/bin/sh
set -eu
log=${CROSSLANE_CODEX_LOG:?}; printf '%s\n' "$@" > "$log/argv"
cat > "$log/stdin"
scratch= output=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -C) scratch=$2; shift 2 ;;
    --output-last-message) output=$2; shift 2 ;;
    *) shift ;;
  esac
done
[ "$scratch" = "$PWD" ] && [ -z "$(ls -A "$PWD")" ] && [ "$output" = "$PWD/result" ]
hash=$(sed -n 's/.*"subject_hash"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$log/stdin" | head -n 1)
[ -n "$hash" ]
printf '{"subject_hash":"%s","inspection":{"status":"completed","paths":["src/add.js"]},"evidence":["deterministic Codex model completed immutable inspection"],"findings":[]}' "$hash" | tee "$output" > "$log/raw"
`

func (b *battery) runCodexLane() {
	repo, ok := b.hostMediumCandidate(codexLane, "codex-lane")
	if !ok {
		return
	}
	log := filepath.Join(b.workRoot, "codex-model")
	if err := os.MkdirAll(filepath.Join(log, "bin"), 0o755); err != nil {
		b.fail(codexLane, "fake model setup", err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(log, "bin", "codex"), []byte(codexModel), 0o755); err != nil {
		b.fail(codexLane, "fake model setup", err.Error())
		return
	}
	env := []string{"CROSSLANE_CODEX_LOG=" + log, "PATH=" + filepath.Join(log, "bin") + string(os.PathListSeparator) + os.Getenv("PATH")}
	if !b.hostNegotiatedMediumStart(codexLane, repo, "codex", env) {
		return
	}
	status, stderr, _ := b.statusEnv(repo, "codex", env)
	input := collectInput(status)
	if input == nil || input["capture_operation"] != "review.capture-result" {
		b.fail(codexLane, "compiled adapter capture", "no Codex capture slot: "+firstLine(stderr))
		return
	}
	subject := getString(input, "artifact_subject", "subject_hash")
	if subject == "" {
		b.fail(codexLane, "compiled adapter capture", "Codex capture slot omitted artifact subject hash")
		return
	}
	if capture, stderr, code := b.runJSONEnv("result-artifact", repo, env,
		append([]string{"review", "capture-result"}, argumentTokens(input)...)...); code != 0 || getString(capture, "admission_decision") != "completed" {
		b.fail(codexLane, "compiled adapter capture", fmt.Sprintf("exit=%d admission=%q %s", code, getString(capture, "admission_decision"), firstLine(stderr)))
		return
	}
	if !b.checkCodexBoundary(log, subject) {
		return
	}
	b.finishApproved(codexLane, "capture, final evidence, and burned lifecycle", repo, "codex", env)
}

func (b *battery) checkCodexBoundary(log, subject string) bool {
	argv, argvErr := os.ReadFile(filepath.Join(log, "argv"))
	stdin, stdinErr := os.ReadFile(filepath.Join(log, "stdin"))
	raw, rawErr := os.ReadFile(filepath.Join(log, "raw"))
	var result map[string]any
	if argvErr != nil || stdinErr != nil || rawErr != nil || json.Unmarshal(raw, &result) != nil || !strings.HasPrefix(string(argv), "exec\n--skip-git-repo-check\n--ignore-user-config\n--sandbox\nread-only\n-C\n") || strings.Contains(string(argv), "GENTLE_AI_REVIEW_") || !strings.Contains(string(stdin), subject) || getString(result, "subject_hash") != subject {
		b.fail(codexLane, "adapter argv/stdin/raw boundary", "compiled adapter did not preserve the isolated opaque raw-result boundary")
		return false
	}
	b.pass(codexLane, "adapter argv/stdin/raw boundary", "fake model received opaque stdin, echoed subject_hash, and wrote raw output in the empty adapter scratch")
	return true
}
