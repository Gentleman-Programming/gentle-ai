package reviewtransaction

import (
	"strings"
	"testing"
)

// The kill-switch refusal was the last block a black-box tester still hit with
// no runnable way out: it explains which source keeps reviews off and never
// named the command that turns them back on. Turning reviews off is a
// deliberate choice, so refusing here is correct; naming nothing is not.
func TestRDDDisabledErrorNamesTheCommandThatTurnsItBackOn(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		source RDDModeSource
		want   string
	}{
		{name: "global", source: RDDModeSourceGlobal, want: "gentle-ai review mode enable --scope=global"},
		{name: "clone local", source: RDDModeSourceCloneLocal, want: "gentle-ai review mode enable --scope=global then gentle-ai review mode enable --scope=clone"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := &RDDDisabledError{Operation: RDDOperationStart, Source: testCase.source}
			if got := err.Error(); !strings.Contains(got, testCase.want) {
				t.Fatalf("refusal names no runnable continuation.\n got: %s\nwant it to contain: %s", got, testCase.want)
			}
		})
	}
}

// Default ON no longer produces this refusal through mode resolution. Keep
// compatibility coverage for a constructed default-source error: its recovery
// command must still name global enable for both operations.
func TestRDDDisabledErrorSendsTheDefaultSourceToTheGlobalEnable(t *testing.T) {
	for _, operation := range []RDDOperation{RDDOperationStart, RDDOperationMutate} {
		t.Run(string(operation), func(t *testing.T) {
			got := (&RDDDisabledError{Operation: operation, Source: RDDModeSourceDefault}).Error()
			if !strings.Contains(got, "gentle-ai review mode enable --scope=global") {
				t.Fatalf("a default-source error omitted its global recovery command: %s", got)
			}
			if strings.Contains(got, "--scope=clone") {
				t.Fatalf("a clone scope can never turn reviews on, so it must not be offered: %s", got)
			}
		})
	}
}

// A refused mutation is not a refused start. The operator already holds
// in-flight authority, so the refusal has to answer a question a start never
// raises -- what happened to the review I had -- and then say what turning
// reviews back on will let them do with it. It must also never say "mutate",
// which is an internal classification nobody typed.
func TestRDDDisabledMutationSaysTheReviewIsFrozenAndWhatReEnablingResumes(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		source RDDModeSource
		want   string
	}{
		{name: "global", source: RDDModeSourceGlobal, want: "gentle-ai review mode enable --scope=global"},
		{name: "clone local", source: RDDModeSourceCloneLocal, want: "gentle-ai review mode enable --scope=global then gentle-ai review mode enable --scope=clone"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := (&RDDDisabledError{Operation: RDDOperationMutate, Source: testCase.source}).Error()
			if !strings.Contains(got, testCase.want) {
				t.Fatalf("mutation refusal names no runnable continuation.\n got: %s\nwant it to contain: %s", got, testCase.want)
			}
			if !strings.Contains(got, "frozen, not discarded") {
				t.Fatalf("mutation refusal does not say the in-flight review survived: %s", got)
			}
			if strings.Contains(got, "mutate is rejected") {
				t.Fatalf("mutation refusal leaks the internal operation name: %s", got)
			}
		})
	}
}
