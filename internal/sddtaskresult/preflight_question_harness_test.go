package sddtaskresult

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
)

// preflightQuestionHarnessSource drives the actual embedded OpenCode plugin
// with bun, calling its tool.execute hooks the way the host would: it
// canonicalizes a localized/misordered grouped preflight question set,
// accepts tolerant answers and prepends the confirmed block to a later SDD
// task dispatch, and surfaces the errors thrown for a skipped answer and for
// a wrong question count. The plugin's `import type { Plugin }` line is
// type-only and bun erases it, so the copy under test keeps it; this harness
// only needs a default export that returns the two tool.execute hooks.
const preflightQuestionHarnessSource = `import Factory from "./sdd-task-result-artifacts"

type Hooks = {
  "tool.execute.before": (input: any, output: any) => Promise<void>
  "tool.execute.after": (input: any, output: any) => Promise<void>
}

async function main() {
  const client = {
    session: {
      get: async ({ path }: { path: { id: string } }) => ({ data: { id: path.id, parentID: undefined } }),
    },
  }
  const hooks = (await (Factory as any)({ client })) as Hooks

  const out: Record<string, unknown> = {}

  // Scenario A: a localized/misordered question set with only Q1 carrying
  // the host marker becomes three canonical questions.
  const argsA: any = {
    questions: [
      { question: "Gentle AI SDD preflight 1/3: ¿Qué ritmo prefieres?", options: [{ label: "x" }], multiple: true },
      { question: "¿Qué artefactos usamos?", options: [{ label: "a" }, { label: "b" }, { label: "c" }] },
      { question: "¿Cómo entregamos el PR?", options: [] },
    ],
  }
  await hooks["tool.execute.before"]({ tool: "question", sessionID: "root-a", callID: "call-a" }, { args: argsA })
  out.canonicalized = argsA.questions

  // Scenario B: tolerant answers are accepted, and the confirmed block is
  // prepended to a later SDD task dispatch.
  await hooks["tool.execute.after"](
    { tool: "question", sessionID: "root-a", args: argsA },
    { title: "q", output: "", metadata: { answers: [["automatic"], ["Engram "], ["ask me"]] } },
  )
  const dispatchArgs: any = { subagent_type: "sdd-explore", prompt: "do the sdd-explore work" }
  await hooks["tool.execute.before"]({ tool: "task", sessionID: "root-a", callID: "call-b" }, { args: dispatchArgs })
  out.dispatchedPrompt = dispatchArgs.prompt

  // Scenario C: an empty answer array (a skipped tab) throws.
  const argsC: any = {
    questions: [
      { question: "Gentle AI SDD preflight 1/3: Pace", options: [{ label: "Interactive" }, { label: "Automatic" }] },
      { question: "Gentle AI SDD preflight 2/3: Artifacts", options: [{ label: "OpenSpec" }, { label: "Engram" }, { label: "Both" }] },
      { question: "Gentle AI SDD preflight 3/3: PR strategy", options: [{ label: "Ask me" }, { label: "Single PR" }, { label: "Auto" }] },
    ],
  }
  await hooks["tool.execute.before"]({ tool: "question", sessionID: "root-c", callID: "call-c" }, { args: argsC })
  try {
    await hooks["tool.execute.after"](
      { tool: "question", sessionID: "root-c", args: argsC },
      { title: "q", output: "", metadata: { answers: [[], ["Engram"], ["Ask me"]] } },
    )
    out.emptyAnswerError = null
  } catch (err: any) {
    out.emptyAnswerError = err instanceof Error ? err.message : String(err)
  }

  // Scenario D: two questions instead of three throws in the before-hook.
  const argsD: any = {
    questions: [
      { question: "Gentle AI SDD preflight 1/3: Pace", options: [{ label: "Interactive" }, { label: "Automatic" }] },
      { question: "Gentle AI SDD preflight 2/3: Artifacts", options: [{ label: "OpenSpec" }, { label: "Engram" }, { label: "Both" }] },
    ],
  }
  try {
    await hooks["tool.execute.before"]({ tool: "question", sessionID: "root-d", callID: "call-d" }, { args: argsD })
    out.twoQuestionsError = null
  } catch (err: any) {
    out.twoQuestionsError = err instanceof Error ? err.message : String(err)
  }

  // Scenario E: model-supplied descriptions follow their matching label,
  // never their array position. Q1 offers the canonical labels in swapped
  // order; Q2 offers localized labels that match nothing and must fall back
  // to the built-in descriptions.
  const argsE: any = {
    questions: [
      { question: "Gentle AI SDD preflight 1/3: Pace", header: "Pace", options: [{ label: "automatic", description: "DESC-AUTO" }, { label: "Interactive", description: "DESC-INTERACTIVE" }] },
      { question: "Gentle AI SDD preflight 2/3: Artifacts", header: "Artifacts", options: [{ label: "Ambos", description: "DESC-AMBOS" }, { label: "Engram", description: "DESC-ENGRAM" }, { label: "OpenSpec", description: "DESC-OPENSPEC" }] },
      { question: "Gentle AI SDD preflight 3/3: PR strategy", header: "PR strategy", options: [{ label: "Ask me", description: "DESC-ASK" }, { label: "Single PR", description: "DESC-SINGLE" }, { label: "Auto", description: "DESC-AUTO-PR" }] },
    ],
  }
  await hooks["tool.execute.before"]({ tool: "question", sessionID: "root-e", callID: "call-e" }, { args: argsE })
  out.reordered = argsE.questions

  process.stdout.write(JSON.stringify(out))
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
`

type preflightHarnessQuestion struct {
	Question string `json:"question"`
	Header   string `json:"header"`
	Multiple bool   `json:"multiple"`
	Options  []struct {
		Label       string `json:"label"`
		Description string `json:"description"`
	} `json:"options"`
}

type preflightHarnessResult struct {
	Canonicalized     []preflightHarnessQuestion `json:"canonicalized"`
	DispatchedPrompt  string                     `json:"dispatchedPrompt"`
	EmptyAnswerError  *string                    `json:"emptyAnswerError"`
	TwoQuestionsError *string                    `json:"twoQuestionsError"`
	Reordered         []preflightHarnessQuestion `json:"reordered"`
}

// TestOpenCodePreflightQuestionCanonicalizationAndTolerantMatching runs the
// actual embedded OpenCode plugin through bun, proving the runtime-owned
// canonicalization and tolerant-answer matching behave the way the plugin
// contract in this package describes, rather than only pinning literal
// source substrings. It is skipped when bun is not on PATH.
func TestOpenCodePreflightQuestionCanonicalizationAndTolerantMatching(t *testing.T) {
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not on PATH; skipping OpenCode plugin harness test")
	}

	pluginSource := assets.MustRead("opencode/plugins/sdd-task-result-artifacts.ts")
	// The Plugin import is type-only and bun erases it, but strip it
	// defensively in case bun ever tries to resolve the package.
	pluginSource = strings.Replace(pluginSource, `import type { Plugin } from "@opencode-ai/plugin"`+"\n", "", 1)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sdd-task-result-artifacts.ts"), []byte(pluginSource), 0o644); err != nil {
		t.Fatalf("write plugin copy: %v", err)
	}
	harnessPath := filepath.Join(dir, "harness.ts")
	if err := os.WriteFile(harnessPath, []byte(preflightQuestionHarnessSource), 0o644); err != nil {
		t.Fatalf("write harness: %v", err)
	}

	cmd := exec.Command(bunPath, harnessPath)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bun harness failed: %v\n%s", err, output)
	}

	var result preflightHarnessResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode harness output: %v\nraw output:\n%s", err, output)
	}

	// Scenario 1: a localized/misordered question set with only Q1 prefixed
	// becomes three canonical questions.
	if len(result.Canonicalized) != 3 {
		t.Fatalf("expected 3 canonicalized questions, got %d", len(result.Canonicalized))
	}
	wantMarkers := []string{"Gentle AI SDD preflight 1/3:", "Gentle AI SDD preflight 2/3:", "Gentle AI SDD preflight 3/3:"}
	wantHeaders := []string{"Pace", "Artifacts", "PR strategy"}
	wantLabels := [][]string{{"Interactive", "Automatic"}, {"OpenSpec", "Engram", "Both"}, {"Ask me", "Single PR", "Auto"}}
	for i, question := range result.Canonicalized {
		if !strings.HasPrefix(question.Question, wantMarkers[i]) {
			t.Errorf("question %d = %q, want prefix %q", i+1, question.Question, wantMarkers[i])
		}
		if question.Header != wantHeaders[i] {
			t.Errorf("question %d header = %q, want %q", i+1, question.Header, wantHeaders[i])
		}
		if question.Multiple {
			t.Errorf("question %d multiple = true, want false", i+1)
		}
		if len(question.Options) != len(wantLabels[i]) {
			t.Fatalf("question %d has %d options, want %d", i+1, len(question.Options), len(wantLabels[i]))
		}
		for j, option := range question.Options {
			if option.Label != wantLabels[i][j] {
				t.Errorf("question %d option %d label = %q, want %q", i+1, j+1, option.Label, wantLabels[i][j])
			}
			if option.Description == "" {
				t.Errorf("question %d option %d has no description", i+1, j+1)
			}
		}
	}

	// Scenario 2: tolerant answers are accepted, and the confirmed block is
	// prepended to the next SDD task dispatch.
	for _, want := range []string{"## SDD Session Preflight", "Pace: auto", "Artifact store: engram", "Delivery strategy: ask-on-risk"} {
		if !strings.Contains(result.DispatchedPrompt, want) {
			t.Errorf("dispatched prompt missing %q; got:\n%s", want, result.DispatchedPrompt)
		}
	}

	// Scenario 3: an empty answer (a skipped tab) throws, naming the
	// question tool as the way to retry.
	if result.EmptyAnswerError == nil {
		t.Fatal("expected an error for an empty preflight answer")
	} else if !strings.Contains(*result.EmptyAnswerError, "question tool") {
		t.Errorf("empty answer error = %q, want it to mention the question tool", *result.EmptyAnswerError)
	}

	// Scenario 4: two questions instead of three throws in the before-hook.
	if result.TwoQuestionsError == nil {
		t.Fatal("expected an error for a two-question preflight")
	} else if !strings.Contains(*result.TwoQuestionsError, "three questions") {
		t.Errorf("two-question error = %q, want it to mention three questions", *result.TwoQuestionsError)
	}

	// Scenario 5: a supplied description stays attached to the label it was
	// written for, so a swapped option order never shows a canonical label
	// next to another option's description; labels that match no canonical
	// option fall back to the built-in descriptions.
	if len(result.Reordered) != 3 {
		t.Fatalf("expected 3 reordered questions, got %d", len(result.Reordered))
	}
	wantReordered := [][2]string{{"Interactive", "DESC-INTERACTIVE"}, {"Automatic", "DESC-AUTO"}}
	for j, want := range wantReordered {
		option := result.Reordered[0].Options[j]
		if option.Label != want[0] || option.Description != want[1] {
			t.Errorf("reordered pace option %d = %q/%q, want %q/%q", j+1, option.Label, option.Description, want[0], want[1])
		}
	}
	for j, option := range result.Reordered[1].Options {
		if option.Label != wantLabels[1][j] {
			t.Errorf("localized artifacts option %d label = %q, want %q", j+1, option.Label, wantLabels[1][j])
		}
		if strings.HasPrefix(option.Description, "DESC-") {
			t.Errorf("localized artifacts option %d kept a positional description %q; want the built-in default", j+1, option.Description)
		}
	}
	wantPR := [][2]string{{"Ask me", "DESC-ASK"}, {"Single PR", "DESC-SINGLE"}, {"Auto", "DESC-AUTO-PR"}}
	for j, want := range wantPR {
		option := result.Reordered[2].Options[j]
		if option.Label != want[0] || option.Description != want[1] {
			t.Errorf("pr strategy option %d = %q/%q, want %q/%q", j+1, option.Label, option.Description, want[0], want[1])
		}
	}
}
