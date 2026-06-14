package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInstallChannel(t *testing.T) {
	// Point HOME at an empty temp dir so the saved-channel preference cache is
	// absent and the default/unset paths deterministically resolve to stable,
	// regardless of the host's ~/.gentle-ai/channel.
	t.Setenv("HOME", t.TempDir())
	t.Setenv(channelEnvVar, "")

	tests := []struct {
		name      string
		flagValue string
		envValue  string
		want      InstallChannel
		wantErr   bool
	}{
		{name: "default stable", want: ChannelStable},
		{name: "flag beta", flagValue: "beta", want: ChannelBeta},
		{name: "flag nightly aliases beta", flagValue: "nightly", want: ChannelBeta},
		{name: "env beta", envValue: "beta", want: ChannelBeta},
		{name: "flag wins over env", flagValue: "stable", envValue: "beta", want: ChannelStable},
		{name: "invalid", flagValue: "engram-beta", wantErr: true},
		// Empty/whitespace env is treated as unset and falls through to the saved
		// file; with no saved file present (temp HOME) that resolves to stable.
		{name: "empty env string falls through to stable", flagValue: "", envValue: "", want: ChannelStable},
		{name: "whitespace env string falls through to stable", flagValue: "", envValue: "   ", want: ChannelStable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(channelEnvVar, tt.envValue)

			got, err := ResolveInstallChannel(tt.flagValue)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveInstallChannel(%q) error = %v, wantErr %v", tt.flagValue, err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "unsupported Gentle AI channel") {
				t.Fatalf("error = %q, want unsupported-channel wording", err.Error())
			}
			if got != tt.want {
				t.Fatalf("ResolveInstallChannel(%q) = %q, want %q", tt.flagValue, got, tt.want)
			}
		})
	}
}

// TestSaveInstallChannelPersistsCanonicalForm locks the single-source-of-truth
// contract of SaveInstallChannel: it shares parseInstallChannel for validity and
// always persists the canonical channel, so "nightly" round-trips as beta and an
// invalid value is rejected without writing the file.
func TestSaveInstallChannelPersistsCanonicalForm(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(channelEnvVar, "")

	tests := []struct {
		name    string
		input   InstallChannel
		want    string // persisted file contents; empty means no file should exist
		wantErr bool
	}{
		{name: "stable persists stable", input: ChannelStable, want: "stable\n"},
		{name: "beta persists beta", input: ChannelBeta, want: "beta\n"},
		{name: "nightly canonicalizes to beta", input: InstallChannel("nightly"), want: "beta\n"},
		{name: "invalid value is rejected and not written", input: InstallChannel("engram-beta"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fresh HOME per case so a rejected write cannot read a prior file.
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := filepath.Join(home, ".gentle-ai", "channel")

			err := SaveInstallChannel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SaveInstallChannel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), "unsupported Gentle AI channel") {
					t.Fatalf("error = %q, want unsupported-channel wording consistent with ResolveInstallChannel", err.Error())
				}
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("rejected channel must not write the file, stat err = %v", statErr)
				}
				return
			}

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read channel file: %v", readErr)
			}
			if string(data) != tt.want {
				t.Fatalf("channel file = %q, want %q", string(data), tt.want)
			}
		})
	}
}
