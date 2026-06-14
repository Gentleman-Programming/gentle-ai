package cli

import (
	"strings"
	"testing"
)

func TestResolveInstallChannel(t *testing.T) {
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
		// Spec: empty-string env var treated as unset → stable (slice 3 channel-honoring).
		{name: "empty env string defaults stable", flagValue: "", envValue: "", want: ChannelStable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(channelEnvVar, tt.envValue)

			got, err := ResolveInstallChannel(tt.flagValue)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveInstallChannel(%q) error = %v, wantErr %v", tt.flagValue, err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "nightly") {
				t.Fatalf("error = %q, want nightly mentioned", err.Error())
			}
			if got != tt.want {
				t.Fatalf("ResolveInstallChannel(%q) = %q, want %q", tt.flagValue, got, tt.want)
			}
		})
	}
}
