package rtk

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// AgentHookResult represents the outcome of configuring RTK hooks for a single agent.
type AgentHookResult struct {
	AgentID model.AgentID
	Success bool
	Err     error
}

// rtkCommand is the package-level var for the rtk binary path, overridable in tests.
var rtkCommand = "rtk"

// execCommand is the package-level var for exec.Command, overridable in tests.
var execCommand = exec.Command

// ConfigureAgentHook runs "rtk init -g" with the correct flags for the given agent.
// Returns nil on success, error on failure.
func ConfigureAgentHook(agentID model.AgentID) error {
	flags := AgentFlags(agentID)
	if flags == "" && agentID == model.AgentAntigravity {
		// Antigravity doesn't support RTK hooks — skip silently
		return nil
	}

	args := []string{"init", "-g"}
	if flags != "" {
		args = append(args, strings.Fields(flags)...)
	}

	cmd := execCommand(rtkCommand, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rtk init failed for agent %s: %w\nOutput: %s", agentID, err, string(out))
	}

	return nil
}

// ConfigureAllHooks iterates over selected agents and calls ConfigureAgentHook.
// Per-agent failures are logged but do not abort the loop.
func ConfigureAllHooks(selectedAgents []model.AgentID) []AgentHookResult {
	results := make([]AgentHookResult, 0, len(selectedAgents))

	for _, agentID := range selectedAgents {
		if !SupportsHook(agentID) {
			results = append(results, AgentHookResult{
				AgentID: agentID,
				Success: false,
				Err:     fmt.Errorf("agent %s does not support RTK hooks", agentID),
			})
			continue
		}

		err := ConfigureAgentHook(agentID)
		results = append(results, AgentHookResult{
			AgentID: agentID,
			Success: err == nil,
			Err:     err,
		})
	}

	return results
}
