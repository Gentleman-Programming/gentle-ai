//go:build !windows

package shellinstaller

import (
	"crypto/sha256"
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
	Experience   *UserExperience `json:",omitempty"`
	// Scope lists the components to refresh as the canonical comma-separated
	// component names (supervisor, node, sdk, codegraph, context7; see
	// UserComponent*). Empty is a full install: every component is provisioned
	// fresh. Components outside the scope are reused from the existing
	// installation at Destination instead of re-provisioned. The field stays a
	// comparable string so inspected requests keep their value semantics.
	Scope string `json:",omitempty"`
}

// Confirmation binds the human's approval to the inspected physical selection,
// not a boolean flag or an unvalidated path alias. The component scope is part
// of the approved selection: a different scope yields a different token.
func userConfirmation(req UserInstallRequest, identity string) string {
	fields := []string{req.Destination, req.Mode, req.SharedPrefix, req.SharedAgent, identity}
	if req.Scope != "" {
		fields = append(fields, req.Scope)
	}
	data, err := userExperienceConfirmationData(fields, req.Experience)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func UserInstallFromEntry(args []string) (UserInstallRequest, error) {
	args, profile, scope, err := userEntryExperience(args)
	if err != nil {
		return UserInstallRequest{}, err
	}
	if len(args) != 5 {
		return UserInstallRequest{}, fmt.Errorf("invalid internal install arguments")
	}
	return UserInstallRequest{
		Destination: args[0], Mode: args[1], SharedPrefix: args[2], SharedAgent: args[3], Confirmation: args[4], Experience: profile, Scope: scope,
	}, nil
}
