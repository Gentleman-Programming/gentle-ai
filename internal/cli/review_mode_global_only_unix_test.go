//go:build linux || darwin

package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// These POSIX fixtures prove invocation and file-access behavior, not native
// Windows DACL enforcement or authenticity of a released installation.
func TestReviewModePrivateGlobalNeverInvokesGit(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	metadata := filepath.Join(repo, ".git")
	if err := os.Chmod(metadata, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(metadata, 0755) })
	bin := t.TempDir()
	trace := filepath.Join(bin, "git-invocations")
	shim := "#!/bin/sh\nprintf 'git\\n' >> " + quotePOSIXShellToken(trace) + "\nexit 128\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(shim), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	var output bytes.Buffer
	// Ordinary resolution may legitimately fall back to global. Invocation,
	// not Git's classification of inaccessible metadata, is the control.
	_ = RunReviewMode([]string{"status", "--cwd", repo}, &output)
	if calls, err := os.ReadFile(trace); err != nil || len(calls) == 0 {
		t.Fatalf("ordinary negative control did not invoke Git: %q, %v", calls, err)
	}
	if err := os.WriteFile(trace, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{repo, filepath.Join(t.TempDir(), "nonexistent")} {
		if err := state.Write(home, state.InstallState{RDDMode: "unknown"}); err != nil {
			t.Fatal(err)
		}
		output.Reset()
		err := RunReviewMode([]string{"status", "--global-only", "--cwd", cwd}, &output)
		var unreadable *ReviewModeUnreadableError
		if !errors.As(err, &unreadable) || len(unreadable.Scopes) != 1 {
			t.Fatalf("global refusal lost source: %v", err)
		}
		for _, command := range unreadable.Scopes[0].commands() {
			if err := state.Write(home, state.InstallState{RDDMode: "unknown"}); err != nil {
				t.Fatal(err)
			}
			args := append(strings.Fields(strings.Trim(command, "`"))[3:], "--cwd", cwd)
			if err := RunReviewMode(args, &output); err != nil {
				t.Fatalf("printed repair failed under unusable cwd: %s: %v", command, err)
			}
		}
		for _, verb := range []string{"enable", "status", "disable"} {
			output.Reset()
			if err := RunReviewMode([]string{verb, "--global-only", "--cwd", cwd, "--json"}, &output); err != nil {
				t.Fatal(err)
			}
			got := decodeReviewModeResult(t, output.Bytes())
			want := reviewtransaction.RDDModeOn
			if verb == "disable" {
				want = reviewtransaction.RDDModeOff
			}
			if got.Scope != "global" || got.Status.Global != want || got.Status.Effective != want || got.Status.CloneLocal != "" || got.Status.Revision != "" {
				t.Fatalf("global-only %s projected repository state: %#v", verb, got)
			}
		}
		if calls, err := os.ReadFile(trace); err != nil || len(calls) != 0 {
			t.Fatalf("global-only status/mutation/printed repair invoked Git: %q, %v", calls, err)
		}
	}
}

func TestReviewModePrivateGlobalUnreadableFilePreserved(t *testing.T) {
	home := reviewModeHome(t)
	if err := state.Write(home, state.InstallState{RDDMode: "off", InstalledAgents: []string{"pi"}}); err != nil {
		t.Fatal(err)
	}
	path := state.Path(home)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	identity, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"status", "enable", "disable"} {
		var output bytes.Buffer
		if err := RunReviewMode([]string{verb, "--global-only", "--cwd", "nonexistent", "--json"}, &output); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("%s bypassed unreadable source: %v", verb, err)
		}
		got := decodeReviewModeResult(t, output.Bytes()).Status
		if got.Effective != reviewtransaction.RDDModeOff || got.CloneLocal != "" || got.Revision != "" {
			t.Fatalf("unreadable source did not fail closed: %#v", got)
		}
		if after, err := os.Stat(path); err != nil || !os.SameFile(identity, after) || after.Mode() != identity.Mode() || !after.ModTime().Equal(identity.ModTime()) {
			t.Fatalf("refusal changed preimage: %v", err)
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("unreadable state was overwritten: %v", err)
	}
}
