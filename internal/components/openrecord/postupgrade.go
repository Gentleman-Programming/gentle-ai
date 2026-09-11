package openrecord

import (
	"context"
	"os"
	"os/exec"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// PostUpgrade is openrecord's update.ToolInfo.PostUpgrade step: re-emit and
// re-fan-out the skills, exactly like install does, after the binary itself
// upgraded successfully.
//
// It gates on the persisted selection rather than running whenever the binary
// upgraded: update.Tools is checked by detection (`openrecord version`), not
// by selection, so an ungated re-emit would write skills for a user who has
// the binary on PATH for their own reasons and never checked the box —
// exactly the "opt-in by explicit selection, never by detection" requirement
// this integration exists to hold.
func PostUpgrade(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	installed, err := state.Read(homeDir)
	if err != nil {
		// No persisted selection to act on — nothing was ever configured.
		return nil
	}
	if !selectedFor(installed.CommunityTools) {
		return nil
	}

	selectedAgents := make([]model.AgentID, 0, len(installed.InstalledAgents))
	for _, agentID := range installed.InstalledAgents {
		selectedAgents = append(selectedAgents, model.AgentID(agentID))
	}

	runner := communitytool.RunnerFunc(func(name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	})

	staging, err := os.MkdirTemp("", "gentle-ai-openrecord-emit-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	if err := runner.Run("openrecord", "skills", "--emit", staging, "--with-qmd"); err != nil {
		return err
	}
	_, err = fanOut(staging, homeDir, selectedAgents)
	return err
}

func selectedFor(tools []string) bool {
	for _, tool := range tools {
		if tool == string(model.CommunityToolOpenRecord) {
			return true
		}
	}
	return false
}
