package engram

import (
	"strings"
	"testing"
)

// The save cadence in the full protocol must agree with the gentle-engram Pi
// plugin protocol (engram #2348): a request with several fixes is saved once,
// when the request is done, instead of an immediate save after every trigger.
func TestProtocolFullUsesPerRequestSaveCadence(t *testing.T) {
	got := protocolFull()

	for _, want := range []string{
		"Call `mem_save` WITHOUT BEING ASKED after any of these:",
		"A request with several fixes is saved once, when the request is done, unless one fix has a non-obvious root cause worth its own entry. The save may share a turn with other tool calls; it does not need a turn of its own.",
		`If yes, call mem_save before your final answer."`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("protocolFull() missing cadence text %q", want)
		}
	}

	for _, stale := range []string{
		"Call `mem_save` IMMEDIATELY",
		"call mem_save NOW",
	} {
		if strings.Contains(got, stale) {
			t.Errorf("protocolFull() still carries per-trigger immediate-save text %q", stale)
		}
	}
}

// The cadence change must not drop the rules around it.
func TestProtocolFullKeepsRulesAroundSaveCadence(t *testing.T) {
	got := protocolFull()

	for _, want := range []string{
		"### PROACTIVE SAVE TRIGGERS (mandatory — do NOT wait for user to ask)",
		"- Bug fix completed (include root cause)",
		"Self-check after EVERY task",
		"### DELIVERY GUARANTEE — saving is not replying",
		"- **title**:",
		"- **type**:",
		"- **scope**:",
		"- **topic_key**",
		"- **content**",
		"### SESSION CLOSE PROTOCOL (mandatory)",
		"### AFTER COMPACTION",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("protocolFull() lost kept rule %q", want)
		}
	}
}
