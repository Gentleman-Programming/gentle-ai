package cli

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

type InstallChannel string

const (
	ChannelStable InstallChannel = "stable"
	ChannelBeta   InstallChannel = "beta"

	ChannelAxiomEnvVar  = "AXIOM_CHANNEL"
	ChannelGentleEnvVar = "GENTLE_AI_CHANNEL"
	channelEnvVar       = ChannelGentleEnvVar
)

func ResolveInstallChannel(flagValue string) (InstallChannel, error) {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		raw = strings.TrimSpace(system.Getenv(ChannelAxiomEnvVar, ChannelGentleEnvVar))
	}
	if raw == "" {
		return ChannelStable, nil
	}

	switch InstallChannel(strings.ToLower(raw)) {
	case ChannelStable:
		return ChannelStable, nil
	case ChannelBeta, "nightly":
		return ChannelBeta, nil
	default:
		// refusal:by-design operator-knowledge: only the operator knows which channel they meant; the message already states the complete next action (use stable, beta, or nightly), and no runnable command can pick it for them
		return "", fmt.Errorf("unsupported channel %q (use stable, beta, or nightly)", raw)
	}
}

func (c InstallChannel) IsBeta() bool {
	return c == ChannelBeta
}
