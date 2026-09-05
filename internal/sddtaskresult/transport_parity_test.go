package sddtaskresult

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

// #3818: Go owns the phase result contract, and the OpenCode plugin still
// evaluates it in-process because it sits in a hot path where a subprocess per
// task result would be a new failure mode. That is only safe while the two
// cannot disagree, so this pins the plugin's literals against this package.
//
// A failure here means the contract was changed on one side. Change both, or
// move the plugin to the command this package backs.
func TestOpenCodeTransportMatchesTheGoContract(t *testing.T) {
	// A JavaScript regex literal must escape forward slashes; Go's backtick
	// literal must not. Normalise that syntax difference so the comparison is
	// about the grammar rather than about literal punctuation.
	plugin := strings.ReplaceAll(assets.MustRead("opencode/plugins/sdd-task-result-artifacts.ts"), `\/`, "/")

	for what, want := range map[string]string{
		"failure prefix":       HandoffPrefix,
		"handoff schema":       handoffSchema,
		"empty code":           ClassEmpty.FailureCode(),
		"malformed code":       ClassMalformed.FailureCode(),
		"latched code":         "sdd_task_dispatch_latched",
		"retry guidance":       retryGuidance,
		"task result grammar":  taskResultEnvelope.String(),
		"task tag grammar":     taskTag.String(),
		"continuation command": "gentle-ai sdd-status --cwd ",
	} {
		if !strings.Contains(plugin, want) {
			t.Errorf("OpenCode transport lost the %s this package defines: %q", what, want)
		}
	}
}
