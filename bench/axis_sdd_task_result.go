package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const sddTaskResultAxis = "sdd-task-result"

func init() {
	RegisterAxis(Axis{
		Name:     sddTaskResultAxis,
		Title:    "OpenCode SDD task-result transport failures",
		BlackBox: false,
		Review:   reviewUntouched,
		Properties: []string{
			"Runs the installed OpenCode plugin through Node against accepted and rejected host task-result shapes; it does not drive the gentle-ai CLI alone.",
			"Requires GENTLE_AI_BENCH_SDD_PLUGIN and a Node runtime that executes .mts files with built-in TypeScript type stripping; skips honestly when the plugin or runtime capability is unavailable.",
		},
		Journeys: sddTaskResultJourneys,
	})
}

func sddTaskResultJourneys() []Journey {
	return []Journey{
		{
			ID:     "tr01-sdd-background-launch-is-nonterminal",
			Review: reviewOptedIn,
			Title:  "OpenCode background launch acknowledgement cannot latch SDD before child completion",
			Source: "issue #3417 OpenCode background task lifecycle reproduction",
			Steps: []Step{{
				Name:      "background launch acknowledgement reaches the installed OpenCode plugin before child completion",
				Skip:      sddTaskResultUnavailable,
				Composite: sddBackgroundSummaryTaskResult,
			}},
		},
		{
			ID:     "tr02-sdd-empty-task-result",
			Review: reviewOptedIn,
			Title:  "Empty SDD task result: typed terminal failure without artifact mutation or downstream launch",
			Source: "issue #2117 provider transport report",
			Steps: []Step{{
				Name:      "provider-shaped empty task result reaches the installed OpenCode plugin",
				Skip:      sddTaskResultUnavailable,
				Composite: sddEmptyTaskResult,
			}},
		},
		{
			ID:     "tr03-sdd-task-result-grammar",
			Review: reviewOptedIn,
			Title:  "OpenCode SDD task-result grammar accepts only supported host envelopes",
			Source: "issue #3417 OpenCode transport grammar boundary",
			Steps: []Step{{
				Name:      "accepted and rejected task-result shapes reach the installed OpenCode plugin",
				Skip:      sddTaskResultUnavailable,
				Composite: sddTaskResultGrammar,
			}},
		},
	}
}

func sddTaskResultUnavailable(*Sandbox) string {
	path := os.Getenv("GENTLE_AI_BENCH_SDD_PLUGIN")
	if path == "" {
		return "GENTLE_AI_BENCH_SDD_PLUGIN is not set"
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "GENTLE_AI_BENCH_SDD_PLUGIN does not name an installed plugin"
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return "node is unavailable"
	}
	return sddTaskResultNodeUnavailable(node)
}

func sddTaskResultNodeUnavailable(node string) string {
	dir, err := os.MkdirTemp("", "gentle-ai-sdd-task-result-node-*")
	if err != nil {
		return "node TypeScript capability check could not create a temporary directory"
	}
	defer os.RemoveAll(dir)
	capability := filepath.Join(dir, "capability.mts")
	if err := os.WriteFile(capability, []byte(`const marker: string = "gentle-ai-sdd-task-result"`), 0o600); err != nil {
		return "node TypeScript capability check could not write a temporary .mts file"
	}
	ctx, cancel := context.WithTimeout(context.Background(), sddTaskResultHarnessTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, node, capability).CombinedOutput()
	if err == nil {
		return ""
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" && ctx.Err() != nil {
		detail = ctx.Err().Error()
	}
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Sprintf("node cannot execute .mts TypeScript with type stripping required by the installed plugin/harness: %s", detail)
}

const sddTaskResultHarness = `import { readFile, writeFile } from "node:fs/promises"
const source = process.argv[2]
const cwd = process.argv[3]
await writeFile("./plugin.mts", await readFile(source))
const { default: plugin } = await import("./plugin.mts")
const hooks = await plugin({ directory: cwd, worktree: cwd })
const artifact = cwd + "/proposal.md"
await writeFile(artifact, "existing artifact")
const input = { tool: "task", sessionID: "bench-sdd", callID: "empty", args: { subagent_type: "sdd-propose", prompt: "create a proposal" } }
let failure = "NO_ERROR"
try {
  await hooks["tool.execute.after"](input, { title: "", output: "<task id=\"phase\" state=\"completed\">\n<task_result>\n\n</task_result>\n</task>", metadata: {} })
} catch (cause) {
  failure = cause instanceof Error ? cause.message : String(cause)
}
let downstream = "NO_ERROR"
try {
  await hooks["tool.execute.before"]({ ...input, callID: "downstream", args: { subagent_type: "sdd-apply", prompt: "continue" } }, { args: { subagent_type: "sdd-apply", prompt: "continue" } })
} catch (cause) {
  downstream = cause instanceof Error ? cause.message : String(cause)
}
console.log([failure, downstream, await readFile(artifact, "utf8")].join("\n---\n"))
`

const sddTaskResultBackgroundSummaryHarness = `import { readFile, writeFile } from "node:fs/promises"
const source = process.argv[2]
const cwd = process.argv[3]
await writeFile("./plugin.mts", await readFile(source))
const { default: plugin } = await import("./plugin.mts")
const hooks = await plugin({ directory: cwd, worktree: cwd })
const artifact = cwd + "/proposal.md"
await writeFile(artifact, "no artifact yet")
const input = { tool: "task", sessionID: "bench-sdd", callID: "background-launch", args: { subagent_type: "sdd-explore", background: true, prompt: "explore the repository" } }
let failure = "NO_ERROR"
try {
  await hooks["tool.execute.after"](input, { title: "", output: "<task id=\"ses_...\" state=\"running\">\n<summary>Background task started: ...</summary>\n</task>", metadata: { status: "accepted" } })
} catch (cause) {
  failure = cause instanceof Error ? cause.message : String(cause)
}
await writeFile(artifact, "completed artifact")
let sameSession = "NO_ERROR"
try {
  await hooks["tool.execute.before"]({ ...input, callID: "after-completion", args: { subagent_type: "sdd-propose", prompt: "create a proposal" } }, { args: { subagent_type: "sdd-propose", prompt: "create a proposal" } })
} catch (cause) {
  sameSession = cause instanceof Error ? cause.message : String(cause)
}
let newSession = "NO_ERROR"
try {
  await hooks["tool.execute.before"]({ ...input, sessionID: "bench-sdd-after-completion", callID: "new-session", args: { subagent_type: "sdd-propose", prompt: "create a proposal" } }, { args: { subagent_type: "sdd-propose", prompt: "create a proposal" } })
} catch (cause) {
  newSession = cause instanceof Error ? cause.message : String(cause)
}
console.log([failure, sameSession, newSession, await readFile(artifact, "utf8")].join("\n---\n"))
`

const sddTaskResultGrammarHarness = `import { readFile, writeFile } from "node:fs/promises"
const source = process.argv[2]
const cwd = process.argv[3]
const taskOutput = process.argv[4]
const callID = process.argv[5]
await writeFile("./plugin.mts", await readFile(source))
const { default: plugin } = await import("./plugin.mts")
const hooks = await plugin({ directory: cwd, worktree: cwd })
const artifact = cwd + "/proposal.md"
await writeFile(artifact, "existing artifact")
const input = { tool: "task", sessionID: "bench-sdd", callID, args: { subagent_type: "sdd-propose", prompt: "create a proposal" } }
let failure = "NO_ERROR"
try {
  await hooks["tool.execute.after"](input, { title: "", output: taskOutput, metadata: {} })
} catch (cause) {
  failure = cause instanceof Error ? cause.message : String(cause)
}
let downstream = "NO_ERROR"
try {
  await hooks["tool.execute.before"]({ ...input, callID: callID + "-downstream", args: { subagent_type: "sdd-apply", prompt: "continue" } }, { args: { subagent_type: "sdd-apply", prompt: "continue" } })
} catch (cause) {
  downstream = cause instanceof Error ? cause.message : String(cause)
}
console.log(JSON.stringify({ failure, downstream, artifact: await readFile(artifact, "utf8") }))
`

const sddTaskResultHarnessTimeout = 30 * time.Second

func sddBackgroundSummaryTaskResult(r *journeyRun) error {
	root := filepath.Join(r.sandbox.Root, "sdd-background-summary-task-result")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "harness.mts"), []byte(sddTaskResultBackgroundSummaryHarness), 0o600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), sddTaskResultHarnessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "harness.mts", os.Getenv("GENTLE_AI_BENCH_SDD_PLUGIN"), work)
	cmd.Dir = root
	cmd.Env = r.sandbox.env()
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	observation := Observation{Args: []string{"opencode", "task", "sdd-propose"}, ExitCode: 0, Stdout: output.String(), StdoutCaptured: true, StderrCaptured: true}
	if err != nil {
		observation.ExitCode = 1
		observation.Stderr = output.String()
	}
	record := r.accumulator.observe(r.step, observation, nil, true)
	r.accumulator.records = append(r.accumulator.records, record)
	if err != nil {
		return fmt.Errorf("run OpenCode task-result harness: %w: %s", err, output.String())
	}
	parts := strings.Split(strings.TrimSpace(output.String()), "\n---\n")
	if len(parts) != 4 {
		return fmt.Errorf("task-result harness output = %q", output.String())
	}
	if parts[0] != "NO_ERROR" || parts[1] != "NO_ERROR" || parts[2] != "NO_ERROR" {
		return fmt.Errorf("background launch was treated as terminal or latched: %q", parts[:3])
	}
	if parts[3] != "completed artifact" {
		return fmt.Errorf("background child completion artifact = %q, want completed artifact", parts[3])
	}
	return nil
}

type sddTaskResultGrammarCase struct {
	name       string
	taskOutput string
	wantCode   string
}

type sddTaskResultGrammarObservation struct {
	Failure    string `json:"failure"`
	Downstream string `json:"downstream"`
	Artifact   string `json:"artifact"`
}

func sddTaskResultGrammar(r *journeyRun) error {
	root := filepath.Join(r.sandbox.Root, "sdd-task-result-grammar")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "harness.mts"), []byte(sddTaskResultGrammarHarness), 0o600); err != nil {
		return err
	}

	for _, tc := range []sddTaskResultGrammarCase{
		{name: "background summary", taskOutput: `<task id="ses_..." state="completed">
<summary>Background task completed: ...</summary>
<task_result>
non-empty result
</task_result>
</task>`},
		{name: "legacy strict envelope", taskOutput: `<task id="phase" state="completed">
<task_result>
non-empty result
</task_result>
</task>`},
		{name: "bare non-empty output", taskOutput: "non-empty result"},
		{name: "empty output", taskOutput: "", wantCode: "sdd_task_result_empty"},
		{name: "empty task result", taskOutput: `<task id="phase" state="completed">
<task_result>

</task_result>
</task>`, wantCode: "sdd_task_result_empty"},
		{name: "nested task", taskOutput: `<task id="phase" state="completed">
<task_result>
<task id="nested" state="completed">
result
</task>
</task_result>
</task>`, wantCode: "sdd_task_result_malformed"},
		{name: "nested task result", taskOutput: `<task id="phase" state="completed">
<task_result>
<task_result>
result
</task_result>
</task_result>
</task>`, wantCode: "sdd_task_result_malformed"},
		{name: "duplicate task result", taskOutput: `<task id="phase" state="completed">
<task_result>
first
</task_result>
<task_result>
second
</task_result>
</task>`, wantCode: "sdd_task_result_malformed"},
		{name: "missing task result", taskOutput: `<task id="phase" state="completed">
</task>`, wantCode: "sdd_task_result_malformed"},
		{name: "malformed task frame", taskOutput: `<task id="phase" state="completed" host="metadata">
<task_result>
result
</task_result>
</task>`, wantCode: "sdd_task_result_malformed"},
		{name: "non-completed task frame", taskOutput: `<task id="phase" state="failed">
<task_result>
result
</task_result>
</task>`, wantCode: "sdd_task_result_malformed"},
		{name: "summary with nested tag", taskOutput: `<task id="phase" state="completed">
<summary>Background <b>completed</b></summary>
<task_result>
result
</task_result>
</task>`, wantCode: "sdd_task_result_malformed"},
		{name: "duplicate summary", taskOutput: `<task id="phase" state="completed">
<summary>Background task completed: first</summary>
<summary>Background task completed: second</summary>
<task_result>
result
</task_result>
</task>`, wantCode: "sdd_task_result_malformed"},
		{name: "empty summary", taskOutput: `<task id="phase" state="completed">
<summary></summary>
<task_result>
result
</task_result>
</task>`, wantCode: "sdd_task_result_malformed"},
	} {
		result, err := runSDDTaskResultGrammarCase(r, root, work, tc)
		if err != nil {
			return err
		}
		if result.Artifact != "existing artifact" {
			return fmt.Errorf("%s mutated artifact state: %q", tc.name, result.Artifact)
		}
		if tc.wantCode == "" {
			if result.Failure != "NO_ERROR" || result.Downstream != "NO_ERROR" {
				return fmt.Errorf("%s was rejected or latched: failure=%q downstream=%q", tc.name, result.Failure, result.Downstream)
			}
			continue
		}
		if !strings.Contains(result.Failure, `"code":"`+tc.wantCode+`"`) {
			return fmt.Errorf("%s failure = %q, want %q", tc.name, result.Failure, tc.wantCode)
		}
		if !strings.Contains(result.Downstream, `"code":"sdd_task_dispatch_latched"`) || !strings.Contains(result.Downstream, `"latchedCode":"`+tc.wantCode+`"`) {
			return fmt.Errorf("%s downstream = %q, want latched %q", tc.name, result.Downstream, tc.wantCode)
		}
	}
	return nil
}

func runSDDTaskResultGrammarCase(r *journeyRun, root, work string, tc sddTaskResultGrammarCase) (sddTaskResultGrammarObservation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sddTaskResultHarnessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "harness.mts", os.Getenv("GENTLE_AI_BENCH_SDD_PLUGIN"), work, tc.taskOutput, tc.name)
	cmd.Dir = root
	cmd.Env = r.sandbox.env()
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	observation := Observation{Args: []string{"opencode", "task", "sdd-propose", tc.name}, ExitCode: 0, Stdout: output.String(), StdoutCaptured: true, StderrCaptured: true}
	if err != nil {
		observation.ExitCode = 1
		observation.Stderr = output.String()
	}
	record := r.accumulator.observe(r.step, observation, nil, true)
	r.accumulator.records = append(r.accumulator.records, record)
	if err != nil {
		return sddTaskResultGrammarObservation{}, fmt.Errorf("run OpenCode task-result grammar harness for %s: %w: %s", tc.name, err, output.String())
	}
	var result sddTaskResultGrammarObservation
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return sddTaskResultGrammarObservation{}, fmt.Errorf("decode OpenCode task-result grammar harness for %s: %w: %q", tc.name, err, output.String())
	}
	return result, nil
}

func sddEmptyTaskResult(r *journeyRun) error {
	root := filepath.Join(r.sandbox.Root, "sdd-task-result")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "harness.mts"), []byte(sddTaskResultHarness), 0o600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), sddTaskResultHarnessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "harness.mts", os.Getenv("GENTLE_AI_BENCH_SDD_PLUGIN"), work)
	cmd.Dir = root
	cmd.Env = r.sandbox.env()
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	observation := Observation{Args: []string{"opencode", "task", "sdd-propose"}, ExitCode: 0, Stdout: output.String(), StdoutCaptured: true, StderrCaptured: true}
	if err != nil {
		observation.ExitCode = 1
		observation.Stderr = output.String()
	}
	record := r.accumulator.observe(r.step, observation, nil, true)
	r.accumulator.records = append(r.accumulator.records, record)
	if err != nil {
		return fmt.Errorf("run OpenCode task-result harness: %w: %s", err, output.String())
	}
	parts := strings.Split(strings.TrimSpace(output.String()), "\n---\n")
	if len(parts) != 3 {
		return fmt.Errorf("task-result harness output = %q", output.String())
	}
	if !strings.Contains(parts[0], "sdd_task_result_empty") || !strings.Contains(parts[1], "sdd_task_result_empty") {
		return fmt.Errorf("empty result was not routed as one typed terminal failure: %q", parts[:2])
	}
	if parts[2] != "existing artifact" {
		return fmt.Errorf("task-result failure mutated artifact state: %q", parts[2])
	}
	return nil
}
