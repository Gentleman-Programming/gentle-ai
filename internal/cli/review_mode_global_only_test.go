package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func TestReviewModePrivateGlobalSource(t *testing.T) {
	home := reviewModeHome(t)
	var output bytes.Buffer
	if err := RunReviewMode([]string{"status", "--global-only", "--cwd", "nonexistent", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	status := decodeReviewModeResult(t, output.Bytes())
	if status.Scope != "global" || status.Status.Effective != reviewtransaction.RDDModeOn || status.Status.Source != reviewtransaction.RDDModeSourceDefault {
		t.Fatalf("default global projection: %#v", status)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("read-only status wrote HOME: %v, %v", entries, err)
	}
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"pi"}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		operation string
		want      reviewtransaction.RDDMode
	}{{"enable", reviewtransaction.RDDModeOn}, {"status", reviewtransaction.RDDModeOn}, {"disable", reviewtransaction.RDDModeOff}, {"status", reviewtransaction.RDDModeOff}} {
		output.Reset()
		if err := RunReviewMode([]string{step.operation, "--global-only", "--cwd", "nonexistent", "--json"}, &output); err != nil {
			t.Fatal(err)
		}
		got := decodeReviewModeResult(t, output.Bytes())
		if got.Schema != ReviewModeSchema || got.Operation != step.operation || got.Scope != "global" || got.Status.Effective != step.want || got.Status.Global != step.want || got.Status.Source != reviewtransaction.RDDModeSourceGlobal || got.Status.CloneLocal != "" || got.Status.Revision != "" || got.Status.Reach != "" {
			t.Fatalf("global-only %s: %#v", step.operation, got)
		}
	}
	persisted, err := state.Read(home)
	if err != nil || persisted.RDDMode != "off" || persisted.RDDModeRecordedAt == nil || len(persisted.InstalledAgents) != 1 || persisted.InstalledAgents[0] != "pi" {
		t.Fatalf("real install state lost unrelated fields: %#v, %v", persisted, err)
	}
}

func TestReviewModePrivateGlobalPrevalidation(t *testing.T) {
	for _, operation := range []string{"enable", "disable", "status"} {
		for _, suffix := range [][]string{{"--scope", "clone"}, {"--expected-revision="}, {"--expected-revision=token"}, {"--scope=team"}, {"unexpected"}} {
			t.Run(operation+strings.Join(suffix, " "), func(t *testing.T) {
				home := reviewModeHome(t)
				args := append([]string{operation, "--global-only"}, suffix...)
				var output bytes.Buffer
				if err := RunReviewMode(args, &output); err == nil {
					t.Fatalf("unsupported combination accepted: %v", args)
				} else if (suffix[0] == "--scope" || strings.HasPrefix(suffix[0], "--expected-revision")) && !strings.Contains(err.Error(), "gentle-ai review mode help") {
					t.Fatalf("refusal missing runnable help: %v", err)
				}
				if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
					t.Fatalf("prevalidation wrote HOME: %v, %v", entries, err)
				}
			})
		}
	}
}

func TestReviewModePrivateGlobalUnknownAndRepair(t *testing.T) {
	home := reviewModeHome(t)
	if err := state.Write(home, state.InstallState{RDDMode: "unknown"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(state.Path(home))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = RunReviewMode([]string{"status", "--global-only", "--cwd", "nonexistent", "--json"}, &output)
	var unreadable *ReviewModeUnreadableError
	if !errors.Is(err, reviewtransaction.ErrRDDModeUnknown) || !errors.As(err, &unreadable) || len(unreadable.Scopes) != 1 || unreadable.Scopes[0].Scope != "global" || unreadable.Scopes[0].Path != state.Path(home) || unreadable.Scopes[0].Repo != "" {
		t.Fatalf("global-only refusal: %v", err)
	}
	if got := decodeReviewModeResult(t, output.Bytes()).Status; got.Effective != reviewtransaction.RDDModeOff || got.CloneLocal != "" || got.Revision != "" {
		t.Fatalf("unknown did not fail closed: %#v", got)
	}
	if after, err := os.ReadFile(state.Path(home)); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("status changed state: %v", err)
	}
	for _, command := range unreadable.Scopes[0].commands() {
		if !strings.Contains(command, " --scope=global --global-only") {
			t.Fatalf("repair lost global-only scope: %s", command)
		}
		args := append(strings.Fields(strings.Trim(command, "`"))[3:], "--cwd", "nonexistent")
		if err := RunReviewMode(args, &output); err != nil {
			t.Fatalf("printed repair failed: %s: %v", command, err)
		}
	}
}

func TestReviewModePrivateGlobalLockAndMalformedState(t *testing.T) {
	home := reviewModeHome(t)
	lock, err := reviewtransaction.AcquireAuthorityFileLock(mustInstallStateLockPath(t, home))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Release() })
	var output bytes.Buffer
	if err := RunReviewMode([]string{"status", "--global-only", "--json"}, &output); err != nil {
		t.Fatalf("status acquired a writer lock: %v", err)
	}
	for _, verb := range []string{"enable", "disable"} {
		if err := RunReviewMode([]string{verb, "--global-only"}, &output); !errors.Is(err, reviewtransaction.ErrStoreLockContended) {
			t.Fatalf("%s bypassed writer lock: %v", verb, err)
		}
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if err := state.Write(home, state.InstallState{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.Path(home), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"status", "enable", "disable"} {
		if err := RunReviewMode([]string{verb, "--global-only", "--json"}, &output); err == nil {
			t.Fatalf("%s accepted malformed state", verb)
		}
	}
	if after, err := os.ReadFile(state.Path(home)); err != nil || string(after) != "{" {
		t.Fatalf("unreadable state was overwritten: %q, %v", after, err)
	}
}

func TestReviewModePrivateGlobalPreservesAncestorOff(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	staging := filepath.Join(repo, "fresh-private-staging")
	if err := os.Mkdir(staging, 0700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--scope=clone", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	clone := decodeReviewModeResult(t, output.Bytes()).Status
	record, err := reviewtransaction.CloneLocalRDDModeRecordPath(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewMode([]string{"enable", "--global-only", "--cwd", staging, "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	if got := decodeReviewModeResult(t, output.Bytes()).Status; got.Effective != reviewtransaction.RDDModeOn || got.CloneLocal != "" {
		t.Fatalf("global source adopted ancestor off: %#v", got)
	}
	for _, verb := range []string{"status", "enable"} {
		output.Reset()
		if err := RunReviewMode([]string{verb, "--cwd", staging, "--json"}, &output); err != nil {
			t.Fatal(err)
		}
		got := decodeReviewModeResult(t, output.Bytes()).Status
		if got.Effective != reviewtransaction.RDDModeOff || got.CloneLocal != reviewtransaction.RDDModeOff || got.Global != reviewtransaction.RDDModeOn || got.Revision != clone.Revision {
			t.Fatalf("ordinary %s lost clone precedence: %#v", verb, got)
		}
	}
	if after, err := os.ReadFile(record); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("global writes changed clone record: %v", err)
	}
	if entries, err := os.ReadDir(staging); err != nil || len(entries) != 0 {
		t.Fatalf("global writes touched destination: %v, %v", entries, err)
	}
}
