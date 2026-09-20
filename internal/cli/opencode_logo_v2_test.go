package cli

import (
	"context"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
)

func TestOpenCodeV2LogoSkippedWithoutWrites(t *testing.T) {
	old := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = old })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.4")}, nil
	}
	home := t.TempDir()
	changed := []string{}
	for _, step := range []pipeline.Step{
		componentApplyStep{id: "logo", component: model.ComponentOpenCodeGentleLogo, homeDir: home, agents: []model.AgentID{model.AgentOpenCode}},
		openCodePluginInstallStep{id: "plugin-logo", plugin: model.OpenCodePluginGentleLogo, homeDir: home},
		componentSyncStep{id: "sync-logo", component: model.ComponentOpenCodeGentleLogo, homeDir: home, agents: []model.AgentID{model.AgentOpenCode}, changedFiles: &changed},
	} {
		result := (pipeline.Runner{}).Run(pipeline.StageApply, []pipeline.Step{step})
		if !result.Success || result.Err != nil || len(result.Steps) != 1 || result.Steps[0].Status != pipeline.StepStatusSkipped {
			t.Fatalf("%s: %#v", step.ID(), result)
		}
		if result.Steps[0].Err == nil {
			t.Fatal("missing omission reason")
		}
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 || len(changed) != 0 {
		t.Fatalf("skip mutated files: %v %v %v", entries, changed, err)
	}
}
