package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// The TUI selector keeps its default Stable channel field and carries the SDK
// source in the profile. The worker must parse exactly the request that was
// inspected: the Main preview profile selects the Main channel, and every
// experience field survives the handoff unchanged.
func TestWindowsShellInstallTUIHandoffCarriesChannelAndExperience(t *testing.T) {
	for _, tc := range []struct {
		name, channel string
		profile       shellinstaller.UserExperience
	}{
		{"stable all on", "stable", shellinstaller.UserExperience{Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on", GlobalRDD: "on", CodeGraph: "on", Context7: "on", SDKSource: "stable"}},
		{"stable custom all off", "stable", shellinstaller.UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", StrictTDD: "off", GlobalRDD: "off", CodeGraph: "off", Context7: "off", SDKSource: "stable"}},
		{"main preview all on", "main", shellinstaller.UserExperience{Persona: "neutral", BackgroundSubagents: "on", StrictTDD: "on", GlobalRDD: "on", CodeGraph: "on", Context7: "on", SDKSource: "main-preview"}},
		{"main preview all off", "main", shellinstaller.UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", StrictTDD: "off", GlobalRDD: "off", CodeGraph: "off", Context7: "off", SDKSource: "main-preview"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := tc.profile
			req := shellinstaller.UserInstallRequest{Destination: `C:\owned\shell`, Mode: "separate", Confirmation: "token", Channel: "stable", Experience: &profile}
			args := ShellInstallArguments(ShellInstallSelection{Request: req, Confirmed: true})
			if len(args) != 10 || args[0] != "install" || args[6] != "--channel" || args[8] != "--experience" {
				t.Fatalf("worker ABI differs: %q", args)
			}
			parsed, err := shellinstaller.UserInstallFromEntry(args[1:])
			if err != nil {
				t.Fatalf("worker refused the TUI handoff: %v", err)
			}
			if parsed.Destination != req.Destination || parsed.Mode != "separate" || parsed.Confirmation != "token" || parsed.SharedPrefix != "" || parsed.SharedAgent != "" {
				t.Fatalf("positional values lost: %+v", parsed)
			}
			if parsed.Channel != tc.channel {
				t.Fatalf("worker channel = %q, want %q (the inspected channel)", parsed.Channel, tc.channel)
			}
			if parsed.Experience == nil || *parsed.Experience != tc.profile {
				t.Fatalf("experience lost before the worker: %+v", parsed.Experience)
			}
		})
	}
	// A direct caller holding a profile gets no positional values, so the
	// profile can never be dropped from the worker arguments.
	profile := shellinstaller.DefaultUserExperience()
	if values := shellEntryValues(shellinstaller.UserInstallRequest{Destination: `C:\owned\shell`, Mode: "separate", Experience: &profile}); values != nil {
		t.Fatalf("profile dropped into positional values: %q", values)
	}
}
