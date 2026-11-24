//go:build linux || darwin

package shellinstaller

import (
	"context"
	"testing"
)

// The in-process darwin installer has no worker stdout to parse: steps,
// including the publish step, must reach the caller through the context (#6177).
func TestWithInstallProgressDeliversSteps(t *testing.T) {
	var got []UserInstallProgress
	ctx := WithInstallProgress(context.Background(), func(p UserInstallProgress) { got = append(got, p) })
	userShellStepBegin(ctx, 5, 5, "Publish")
	if len(got) != 1 || got[0].Index != 5 || got[0].State != "running" {
		t.Fatalf("progress = %+v", got)
	}
	userShellStepBegin(WithInstallProgress(context.Background(), nil), 5, 5, "Publish")
}
