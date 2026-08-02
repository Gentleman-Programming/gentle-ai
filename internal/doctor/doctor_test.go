package doctor

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestToolCheckID(t *testing.T) {
	tests := []struct {
		tool string
		want CheckID
	}{
		{tool: "gentle-ai", want: "tool:gentle-ai"},
		{tool: "engram", want: "tool:engram"},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			if got := ToolCheckID(tt.tool); got != tt.want {
				t.Fatalf("ToolCheckID(%q) = %q, want %q", tt.tool, got, tt.want)
			}
		})
	}
}

func TestRunnerPreservesDeclarationOrderAndStableIDs(t *testing.T) {
	var order []CheckID
	checks := []Check{
		{ID: CheckStateJSON, Run: recordingCheck(&order, CheckStateJSON, Result{Name: "unstable", Status: StatusPass})},
		{ID: CheckEngramReachable, Run: func(context.Context) Result { order = append(order, CheckEngramReachable); panic("secret panic value") }},
		{ID: CheckDiskSpace, Run: recordingCheck(&order, CheckDiskSpace, Result{Status: StatusWarn})},
	}
	want := []Result{{Name: CheckStateJSON, Status: StatusPass}, {Name: CheckEngramReachable, Status: StatusFail, Detail: "check failed unexpectedly"}, {Name: CheckDiskSpace, Status: StatusWarn}}
	got := (Runner{Checks: checks}).Run(context.Background())
	if !reflect.DeepEqual(got.Checks, want) || !reflect.DeepEqual(order, []CheckID{CheckStateJSON, CheckEngramReachable, CheckDiskSpace}) {
		t.Fatalf("checks/order = %#v/%v, want %#v", got.Checks, order, want)
	}
	if strings.Contains(got.Checks[1].Detail, "secret") {
		t.Fatalf("panic value leaked in evidence: %q", got.Checks[1].Detail)
	}
	if got := (Runner{}).Run(context.Background()).Checks; got == nil || len(got) != 0 {
		t.Fatalf("empty checks = %#v, want non-nil empty slice", got)
	}
}

func TestNewRemedyClassifiesCurrentRemedies(t *testing.T) {
	tests := []struct {
		id       RemedyID
		category RemedyCategory
		mode     ActionMode
		eligible bool
	}{
		{RemedyInstallTool, RemedyCategoryInstall, ActionManualOnly, false},
		{RemedyRemoveDuplicates, RemedyCategoryEnvironment, ActionManualOnly, false},
		{RemedyInstall, RemedyCategoryInstall, ActionManualOnly, false},
		{RemedyRepairState, RemedyCategoryConfiguration, ActionManualOnly, false},
		{RemedySync, RemedyCategoryConfiguration, ActionConfirmation, true},
		{RemedyStartEngram, RemedyCategoryService, ActionManualOnly, false},
		{RemedyInspectEngram, RemedyCategoryService, ActionManualOnly, false},
		{RemedyFreeDiskSpace, RemedyCategoryStorage, ActionManualOnly, false},
	}
	for _, tt := range tests {
		r := NewRemedy(tt.id, "guidance")
		if r.Description != "guidance" || r.Category != tt.category || r.ActionMode != tt.mode || r.Eligible != tt.eligible || r.EligibilityReason == "" || len(r.SupportedPlatforms) != 4 {
			t.Errorf("NewRemedy(%q) = %#v", tt.id, r)
		}
	}
}

func recordingCheck(order *[]CheckID, id CheckID, result Result) func(context.Context) Result {
	return func(context.Context) Result { *order = append(*order, id); return result }
}
