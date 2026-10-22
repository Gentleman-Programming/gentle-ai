package cli

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

func TestShellInstallProfileTransport(t *testing.T) {
	selection, _ := ShellInstallOutcome(NewShellInstallModel(nil))
	profile := shellinstaller.UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GlobalRDD: "off", Context7: "off", StrictTDD: "off"}
	selection.Request.Destination = t.TempDir() + "/target"
	selection.Request.Confirmation = "model-only-transport-token"
	selection.Request.Experience, selection.Confirmed = &profile, true
	args := ShellInstallArguments(selection)
	if len(args) < 3 || args[len(args)-2] != "--experience" {
		t.Fatalf("profile has no complete worker arguments: %q", args)
	}
	req, err := shellinstaller.UserInstallFromEntry(args[1:])
	if err != nil || !reflect.DeepEqual(req, selection.Request) {
		t.Fatalf("worker lost reviewed profile: %+v, %v", req, err)
	}
	inspect, entry := shellInstallInspect, shellInstallEntry
	t.Cleanup(func() { shellInstallInspect, shellInstallEntry = inspect, entry })
	// Transport seam only: this is not physical human consent or installation.
	shellInstallInspect = func(got shellinstaller.UserInstallRequest) (string, error) {
		if !reflect.DeepEqual(got, req) {
			t.Fatal("execution inspection lost preferences")
		}
		return req.Confirmation, nil
	}
	calls := 0
	shellInstallEntry = func(_ context.Context, _ string, got []string, _ io.Reader, _, _ io.Writer) error {
		calls++
		if !reflect.DeepEqual(got, args) {
			t.Fatalf("post-terminal worker dropped profile: %q", got)
		}
		return nil
	}
	if err := runShellInstallRequest(context.Background(), "model-only-self", req, false, io.Discard); err != nil || calls != 1 {
		t.Fatalf("profile handoff: %v, calls=%d", err, calls)
	}
}

func TestShellInstallProfileTransportRefusesInvalid(t *testing.T) {
	for _, mutate := range []func(*shellinstaller.UserExperience){
		func(p *shellinstaller.UserExperience) { p.GlobalRDD = "invalid" },
		func(p *shellinstaller.UserExperience) { p.Context7 = "invalid" },
		func(p *shellinstaller.UserExperience) { p.StrictTDD = "invalid" },
	} {
		selection, _ := ShellInstallOutcome(NewShellInstallModel(nil))
		profile := shellinstaller.UserExperience{Persona: "gentleman", BackgroundSubagents: "on"}
		mutate(&profile)
		selection.Request.Experience, selection.Confirmed = &profile, true
		if args := ShellInstallArguments(selection); len(args) != 0 {
			t.Fatal("invalid profile becomes worker arguments")
		}
	}
}
