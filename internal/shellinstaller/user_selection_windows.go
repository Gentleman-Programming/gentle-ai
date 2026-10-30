//go:build windows

package shellinstaller

import (
	"crypto/sha256"
	"fmt"
)

// Separate creates private tooling, HOME and agent state; personal Pi stays put.
// Field order is the canonical retained selection.json encoding.
type UserInstallRequest struct {
	Destination, Mode, SharedPrefix, SharedAgent, Confirmation string
	Channel                                                    string
	Experience                                                 *UserExperience `json:",omitempty"`
	// Scope carries the canonical component scope (see user_components.go);
	// the Windows update flow consumes it like the Unix flow.
	Scope string `json:",omitempty"`
}

func UserInstallChannel(channel string) (string, error) {
	if channel == "" {
		channel = "stable"
	}
	if channel != "stable" && channel != "main" {
		return "", fmt.Errorf("unknown Gentle Shell channel %q; use stable or main", channel)
	}
	return channel, nil
}

func userConfirmation(req UserInstallRequest, identity string) string {
	fields := []string{req.Destination, req.Mode, req.SharedPrefix, req.SharedAgent, req.Channel, identity}
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
	if len(args) != 5 && (len(args) != 7 || args[5] != "--channel") {
		return UserInstallRequest{}, fmt.Errorf("invalid internal install arguments")
	}
	channel := "stable"
	if len(args) == 7 {
		channel = args[6]
		if channel == "" {
			return UserInstallRequest{}, fmt.Errorf("missing internal install channel")
		}
	}
	channel, err = UserInstallChannel(channel)
	if err != nil {
		return UserInstallRequest{}, err
	}
	// The TUI hands the profile separately from the positional values, so the
	// experience carries the channel authority exactly as inspection derives
	// it: the parsed request is the request that was inspected and confirmed.
	if profile != nil && profile.SDKSource == "main-preview" {
		channel = userChannelMain
	}
	return UserInstallRequest{Destination: args[0], Mode: args[1], SharedPrefix: args[2], SharedAgent: args[3], Confirmation: args[4], Channel: channel, Experience: profile, Scope: scope}, nil
}
