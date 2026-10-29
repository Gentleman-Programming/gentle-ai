package cli

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// A component-scoped update binds the scope into the inspected confirmation,
// so ShellInstallArguments must carry it to the worker entry verbatim; a
// dropped scope makes the worker recompute a different token and refuse the
// confirmed selection (#5789).
func TestShellInstallArgumentsCarryUpdateScope(t *testing.T) {
	profile := shellinstaller.UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	profile.SDKSource = "main-preview"
	selection := ShellInstallSelection{
		Request: shellinstaller.UserInstallRequest{
			Destination:  t.TempDir() + "/target",
			Mode:         "separate",
			Scope:        shellinstaller.CanonicalUpdateScope([]string{shellinstaller.UserComponentContext7, shellinstaller.UserComponentSDK}),
			Experience:   &profile,
			Confirmation: "scope-transport-token",
		},
		Confirmed: true,
	}
	args := ShellInstallArguments(selection)
	if len(args) == 0 {
		t.Fatal("scoped selection produced no worker arguments")
	}
	parsed, err := shellinstaller.UserInstallFromEntry(args[1:])
	if err != nil {
		t.Fatalf("worker entry refused scoped arguments: %v", err)
	}
	want := wantParsedRequest(t, selection.Request, profile.SDKSource)
	if !reflect.DeepEqual(parsed, want) {
		t.Fatalf("worker entry changed the confirmed request: got %+v, want %+v", parsed, want)
	}
}

// After a gentle-ai upgrade, update detection reports the supervisor next to
// the other changed components; the worker must receive that exact scope.
func TestShellInstallArgumentsCarrySupervisorScope(t *testing.T) {
	profile := shellinstaller.UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
	selection := ShellInstallSelection{
		Request: shellinstaller.UserInstallRequest{
			Destination:  t.TempDir() + "/target",
			Mode:         "separate",
			Scope:        shellinstaller.CanonicalUpdateScope([]string{shellinstaller.UserComponentSupervisor, shellinstaller.UserComponentSDK}),
			Experience:   &profile,
			Confirmation: "supervisor-scope-token",
		},
		Confirmed: true,
	}
	args := ShellInstallArguments(selection)
	if len(args) == 0 {
		t.Fatal("supervisor scope produced no worker arguments")
	}
	parsed, err := shellinstaller.UserInstallFromEntry(args[1:])
	if err != nil {
		t.Fatalf("worker entry refused the detection scope: %v", err)
	}
	if !reflect.DeepEqual(parsed, selection.Request) {
		t.Fatalf("worker entry changed the confirmed request: got %+v, want %+v", parsed, selection.Request)
	}
}

// An unscoped selection keeps the historical entry bytes exactly: no --scope
// pair may appear.
func TestShellInstallArgumentsOmitEmptyScope(t *testing.T) {
	profile := shellinstaller.UserExperience{Persona: "unmanaged", BackgroundSubagents: "off"}
	selection := ShellInstallSelection{
		Request: shellinstaller.UserInstallRequest{
			Destination:  t.TempDir() + "/target",
			Mode:         "separate",
			Experience:   &profile,
			Confirmation: "plain-transport-token",
		},
		Confirmed: true,
	}
	args := ShellInstallArguments(selection)
	for _, arg := range args {
		if arg == "--scope" {
			t.Fatalf("empty scope emitted a --scope pair: %q", args)
		}
	}
	parsed, err := shellinstaller.UserInstallFromEntry(args[1:])
	if err != nil {
		t.Fatal(err)
	}
	want := wantParsedRequest(t, selection.Request, profile.SDKSource)
	if !reflect.DeepEqual(parsed, want) {
		t.Fatalf("unscoped entry changed the request: got %+v, want %+v", parsed, want)
	}
}
