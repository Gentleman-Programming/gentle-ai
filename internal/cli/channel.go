package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InstallChannel string

const (
	ChannelStable InstallChannel = "stable"
	ChannelBeta   InstallChannel = "beta"

	channelEnvVar = "GENTLE_AI_CHANNEL"
)

func ResolveInstallChannel(flagValue string) (InstallChannel, error) {
	// Flag and env are explicit user input: a present non-empty value must be
	// validated and an invalid value must error. The saved file is only a
	// preference cache, so it never errors (see loadSavedInstallChannel).
	raw := strings.TrimSpace(flagValue)

	// An empty/whitespace env value is treated as "unset" and falls through to
	// the saved-file lookup, exactly as if the variable were absent.
	if raw == "" {
		if envValue := strings.TrimSpace(os.Getenv(channelEnvVar)); envValue != "" {
			raw = envValue
		}
	}
	if raw == "" {
		// No explicit input: fall back to the saved preference cache. A missing
		// or corrupt saved value degrades silently to stable.
		return loadSavedInstallChannel(), nil
	}

	channel, ok := parseInstallChannel(raw)
	if !ok {
		return "", fmt.Errorf("unsupported Gentle AI channel %q (use stable, beta, or nightly)", raw)
	}
	return channel, nil
}

// parseInstallChannel maps an input string to a canonical channel. "nightly" is
// an accepted INPUT alias that resolves to beta; it is never persisted as-is.
func parseInstallChannel(raw string) (InstallChannel, bool) {
	switch InstallChannel(strings.ToLower(strings.TrimSpace(raw))) {
	case ChannelStable:
		return ChannelStable, true
	case ChannelBeta, "nightly":
		return ChannelBeta, true
	default:
		return "", false
	}
}

func (c InstallChannel) IsBeta() bool {
	return c == ChannelBeta
}

// SaveInstallChannel persists a channel preference. It shares parseInstallChannel
// as the single source of truth for validity, so the accepted set and error wording
// match ResolveInstallChannel exactly. The value is normalized to its canonical form
// before writing ("nightly" persists as beta), so the saved file always round-trips.
func SaveInstallChannel(channel InstallChannel) error {
	canonical, ok := parseInstallChannel(string(channel))
	if !ok {
		return fmt.Errorf("unsupported Gentle AI channel %q (use stable, beta, or nightly)", channel)
	}
	path, err := channelFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create channel config dir: %w", err)
	}
	return os.WriteFile(path, []byte(string(canonical)+"\n"), 0o644)
}

// loadSavedInstallChannel reads the persisted channel preference. The saved file
// is a best-effort cache: a missing file, an unreadable file, or a corrupt/invalid
// value all degrade silently to ChannelStable. It NEVER returns an error so that a
// garbage ~/.gentle-ai/channel can never brick the launcher.
func loadSavedInstallChannel() InstallChannel {
	path, err := channelFilePath()
	if err != nil {
		return ChannelStable
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ChannelStable
	}
	channel, ok := parseInstallChannel(string(data))
	if !ok {
		return ChannelStable
	}
	return channel
}

func channelFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".gentle-ai", "channel"), nil
}
