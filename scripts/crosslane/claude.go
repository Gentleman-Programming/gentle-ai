package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const claudeLane = "claude"

// runClaudeLane drives one low-risk terminal capture and one medium-candidate
// consent/v3 round-trip with the claude-code runtime.
// With --with-model it additionally runs the real compiled claude-code
// reviewer runtime over the medium lineage (dev subscription).
func (b *battery) runClaudeLane() {
	b.runClaudeLowLifecycle()
	b.runClaudeMediumConsent()
}

func (b *battery) runClaudeLowLifecycle() {
	repo, err := b.scratchRepo("claude-low")
	if err != nil {
		b.fail(claudeLane, "low lifecycle scratch repository", err.Error())
		return
	}
	err = writeFile(repo, "docs/ordinary-guide.md", "# Ordinary guide\n\nline one\n")
	if err == nil {
		err = commitAll(repo, "docs: guide")
	}
	if err != nil {
		b.fail(claudeLane, "low lifecycle scratch repository", err.Error())
		return
	}
	if err := writeFile(repo, "docs/ordinary-guide.md", "# Ordinary guide\n\nline one\nline two, purely passive documentation\n"); err != nil {
		b.fail(claudeLane, "low lifecycle scratch repository", err.Error())
		return
	}

	statusDoc, stderr, code := b.status(repo, "claude-code")
	target := getString(statusDoc, "target_identity")
	if target == "" || getString(statusDoc, "next_transition", "execute", "operation") != "review.start" {
		b.fail(claudeLane, "low lifecycle: negotiated start", fmt.Sprintf("exit=%d %s", code, firstLine(stderr)))
		return
	}
	startDoc, stderr, code := b.runCommandLine("start", repo, getString(statusDoc, "next_transition", "execute", "command"))
	if code != 0 || getString(startDoc, "risk_level") != "low" || getString(startDoc, "state") != "approved" {
		b.fail(claudeLane, "low lifecycle: start", fmt.Sprintf("exit=%d risk=%q state=%q %s", code, getString(startDoc, "risk_level"), getString(startDoc, "state"), firstLine(stderr)))
		return
	}
	if lensesRequired, _ := startDoc["lenses_required"].(bool); lensesRequired {
		b.fail(claudeLane, "low lifecycle: start", "low-risk start unexpectedly selected lenses")
		return
	}
	if getString(startDoc, "action") != "closed" {
		b.fail(claudeLane, "low lifecycle: start", fmt.Sprintf("action = %q, want closed", getString(startDoc, "action")))
		return
	}
	b.pass(claudeLane, "low lifecycle burned", "zero-lens START closed and burned its review without FINALIZE")
}

func (b *battery) runClaudeMediumConsent() {
	base := "export function mul(a, b) {\n  return a * b;\n}\n"
	candidate := base + "export function twice(a) {\n  return a + a;\n}\n"
	repo, baseTree, ok := b.committedMediumCandidate(claudeLane, "claude-committed", "src/claude.js", base, candidate)
	if !ok || !b.startCommittedMedium(claudeLane, repo, "claude-code", baseTree) {
		return
	}
	b.runClaudeCommittedProcess(repo)
	if b.withModel {
		b.skip(claudeLane, "live reviewer model run", "--with-model remains intentionally disabled; the default lane is deterministic process proof, not live model proof")
	}
}

// runClaudeCommittedProcess executes a local provider-shaped fixture through the
// real Claude adapter process boundary. It intentionally proves transport and
// closure behavior without a subscription, credential, or live model request.
func (b *battery) runClaudeCommittedProcess(repo string) {
	env, promptPath, err := b.prepareClaudeProcessFixture()
	if err != nil {
		b.fail(claudeLane, "committed Claude process fixture", err.Error())
		return
	}
	statusDoc, stderr, _ := b.status(repo, "claude-code")
	input := collectInput(statusDoc)
	if input == nil || input["capture_operation"] != "review.capture-result" {
		b.fail(claudeLane, "committed Claude process collect", fmt.Sprintf("no capture-result collect input; %s", firstLine(stderr)))
		return
	}
	captureArgs := append([]string{"review", "capture-result"}, argumentTokens(input)...)
	captureArgs = append(captureArgs, "--agent=claude-code")
	capture, stderr, code := b.runJSONEnv("result-artifact", repo, env, captureArgs...)
	if code != 0 || !admittedCapture(capture) || operationState(capture) != "correction_required" {
		b.fail(claudeLane, "committed Claude process capture", fmt.Sprintf("exit=%d schema=%q state=%q %s", code, getString(capture, "schema"), operationState(capture), firstLine(stderr)))
		return
	}
	if _, err := os.Stat(promptPath); err != nil {
		b.fail(claudeLane, "committed Claude process capture", "fixture did not receive the Claude adapter stdin prompt: "+err.Error())
		return
	}
	b.pass(claudeLane, "committed Claude process capture", "local provider-shaped fixture ran through the real Claude process transport and produced correction_required")

	status, continuationStderr, continuationCode := b.statusFromClosure(repo, capture)
	if continuationCode != 0 || getString(status, "authority", "lineage_id") != operationLineage(capture) ||
		getString(status, "next_transition", "reason_code") != "correction_plan_required" {
		b.fail(claudeLane, "committed Claude correction re-entry", fmt.Sprintf("exit=%d lineage=%q reason=%q %s",
			continuationCode, getString(status, "authority", "lineage_id"), getString(status, "next_transition", "reason_code"), firstLine(continuationStderr)))
		return
	}
	b.pass(claudeLane, "committed Claude correction re-entry", "closure operation plus ordered tokens preserved the lineage and reached correction_plan_required")
}

func (b *battery) prepareClaudeProcessFixture() ([]string, string, error) {
	dir := filepath.Join(b.workRoot, "claude-process-fixture")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	promptPath := filepath.Join(dir, "provider-prompt.json")
	fixture := `#!/bin/sh
set -eu
prompt_path="${GENTLE_AI_CROSSLANE_CLAUDE_PROMPT:?}"
cat > "$prompt_path"
subject_hash=$(sed -n 's/.*"subject_hash"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$prompt_path" | head -n 1)
if [ -z "$subject_hash" ]; then
  echo "fixture did not receive a provider subject hash" >&2
  exit 1
fi
printf '%s\n' '{"subject_hash":"'"$subject_hash"'","inspection":{"status":"completed","paths":["src/claude.js"]},"evidence":["twice is introduced by the committed candidate and returns the wrong result"],"findings":[{"claim":"twice returns a + a instead of using the required multiplication behavior","severity":"BLOCKER","evidence_class":"deterministic","causal_disposition":"introduced","lens":"review-reliability","location":"src/claude.js:5","proof_refs":["src/claude.js:4-6 is an introduced committed candidate hunk"]}]}'
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(fixture), 0o755); err != nil {
		return nil, "", err
	}
	return []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GENTLE_AI_CROSSLANE_CLAUDE_PROMPT=" + promptPath,
	}, promptPath, nil
}
