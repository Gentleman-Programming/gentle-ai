package sddtaskresult

import "testing"

// #3818: the SDD phase result contract was enforced in exactly one runtime, by
// internal/assets/opencode/plugins/sdd-task-result-artifacts.ts. Every other
// runtime received the same contract as prose in a Markdown prompt with no
// enforcement at all, so the typed terminal failure was only reliably produced
// where that plugin runs.
//
// The cases below are the grammar table bench/axis_sdd_task_result.go drives
// through the real plugin. Reusing them is deliberate: this package must agree
// with the shipped behaviour case for case, not merely look reasonable.
func TestClassifyMatchesTheShippedGrammar(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		want   Class
	}{
		{"background summary", "<task id=\"ses_...\" state=\"completed\">\n<summary>Background task completed: ...</summary>\n<task_result>\nnon-empty result\n</task_result>\n</task>", ClassOK},
		{"legacy strict envelope", "<task id=\"phase\" state=\"completed\">\n<task_result>\nnon-empty result\n</task_result>\n</task>", ClassOK},
		{"bare non-empty output", "non-empty result", ClassOK},
		{"empty output", "", ClassEmpty},
		{"whitespace-only output", "   \n\t ", ClassEmpty},
		{"empty task result", "<task id=\"phase\" state=\"completed\">\n<task_result>\n\n</task_result>\n</task>", ClassEmpty},
		{"nested task", "<task id=\"phase\" state=\"completed\">\n<task_result>\n<task id=\"nested\" state=\"completed\">\nresult\n</task>\n</task_result>\n</task>", ClassMalformed},
		{"nested task result", "<task id=\"phase\" state=\"completed\">\n<task_result>\n<task_result>\nresult\n</task_result>\n</task_result>\n</task>", ClassMalformed},
		{"duplicate task result", "<task id=\"phase\" state=\"completed\">\n<task_result>\nfirst\n</task_result>\n<task_result>\nsecond\n</task_result>\n</task>", ClassMalformed},
		{"missing task result", "<task id=\"phase\" state=\"completed\">\n</task>", ClassMalformed},
		{"malformed task frame", "<task id=\"phase\" state=\"completed\" host=\"metadata\">\n<task_result>\nresult\n</task_result>\n</task>", ClassMalformed},
		{"non-completed task frame", "<task id=\"phase\" state=\"failed\">\n<task_result>\nresult\n</task_result>\n</task>", ClassMalformed},
		{"summary with nested tag", "<task id=\"phase\" state=\"completed\">\n<summary>Background <b>completed</b></summary>\n<task_result>\nresult\n</task_result>\n</task>", ClassMalformed},
		{"duplicate summary", "<task id=\"phase\" state=\"completed\">\n<summary>Background task completed: first</summary>\n<summary>Background task completed: second</summary>\n<task_result>\nresult\n</task_result>\n</task>", ClassMalformed},
		{"empty summary", "<task id=\"phase\" state=\"completed\">\n<summary></summary>\n<task_result>\nresult\n</task_result>\n</task>", ClassMalformed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.output); got != tc.want {
				t.Errorf("Classify() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFailureCodeNamesTheShippedTokens pins the two public codes; consumers
// route on these exact strings.
func TestFailureCodeNamesTheShippedTokens(t *testing.T) {
	for class, want := range map[Class]string{
		ClassEmpty:     "sdd_task_result_empty",
		ClassMalformed: "sdd_task_result_malformed",
		ClassOK:        "",
	} {
		if got := class.FailureCode(); got != want {
			t.Errorf("%q.FailureCode() = %q, want %q", class, got, want)
		}
	}
}
