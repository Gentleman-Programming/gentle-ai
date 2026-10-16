//go:build !windows

package shellinstaller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Shared mode selects an existing owned global Pi installation and agent
// directory. Both command bindings use those same physical objects. Separate
// mode creates a private installation, HOME, agent, state and project.
// There is no channel field: Linux has no channel selection, so the request
// cannot carry a selector the backend would ignore. Other non-Windows hosts
// share this contract only to reach the unsupported-platform refusal.
type UserInstallRequest struct {
	Destination  string
	Mode         string
	SharedPrefix string
	SharedAgent  string
	Confirmation string
}

// Confirmation binds the human's approval to the inspected physical selection,
// not a boolean flag or an unvalidated path alias.
func userConfirmation(req UserInstallRequest, identity string) string {
	data, _ := json.Marshal([]string{req.Destination, req.Mode, req.SharedPrefix, req.SharedAgent, identity})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func UserInstallFromEntry(args []string) (UserInstallRequest, error) {
	if len(args) != 5 {
		return UserInstallRequest{}, fmt.Errorf("invalid internal install arguments")
	}
	return UserInstallRequest{
		Destination: args[0], Mode: args[1], SharedPrefix: args[2], SharedAgent: args[3], Confirmation: args[4],
	}, nil
}
