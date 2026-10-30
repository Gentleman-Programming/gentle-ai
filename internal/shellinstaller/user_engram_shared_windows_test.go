//go:build windows

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUserEngramWindowsWorkerEnvOnlyForLaunch(t *testing.T) {
	original := userEngramLaunchEnv
	t.Cleanup(func() { userEngramLaunchEnv = original })
	var roots []string
	userEngramLaunchEnv = func(_ context.Context, root string) []string {
		roots = append(roots, root)
		return []string{"ENGRAM_URL=decided"}
	}
	for _, args := range [][]string{{"install", `C:\owned`}, {"check"}, {"internal-launch", `C:\owned`, "pi"}, {"launch"}} {
		if env := userEngramWindowsWorkerEnv(context.Background(), args); env != nil || len(roots) != 0 {
			t.Fatalf("%q ran the decision: env %q roots %q", args, env, roots)
		}
	}
	env := userEngramWindowsWorkerEnv(context.Background(), []string{"launch", `C:\owned`, "gentle-shell", "--flag"})
	if !reflect.DeepEqual(env, []string{"ENGRAM_URL=decided"}) || !reflect.DeepEqual(roots, []string{`C:\owned`}) {
		t.Fatalf("launch forwarded %q for roots %q", env, roots)
	}
}

// The owned launch environment gains exactly the worker's Engram entries, or
// a disabled policy when the outer entry forwarded no decision; provision
// keeps its environment byte for byte.
func TestUserWindowsLaunchEnvironmentAddsOnlyEngramEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv(userWindowsSystemGitCandidateEnv, "")
	userWindowsProgramFilesStub(t, "", errors.New("fixture known-folder failure"))
	for _, key := range append(append([]string{}, userEngramSelections...), userEngramPolicyEnv, userEngramNoticeEnv) {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	provision, err := userWindowsEnvironment(root)
	if err != nil {
		t.Fatal(err)
	}
	undecided, err := userWindowsLaunchEnvironment(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := append(append([]string{}, provision...), userEngramDisabled(userEngramNoDecisionNotice)...); !reflect.DeepEqual(undecided, want) {
		t.Fatalf("launch environment without a decision = %q, want %q", undecided, want)
	}
	t.Setenv("ENGRAM_STARTUP_POLICY", "allow")
	t.Setenv("ENGRAM_URL", userEngramDefaultURL)
	t.Setenv("ENGRAM_HTTP_TOKEN", "secret")
	env, err := userWindowsLaunchEnvironment(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := append(append([]string{}, provision...), "ENGRAM_URL="+userEngramDefaultURL, "ENGRAM_STARTUP_POLICY=allow"); !reflect.DeepEqual(env, want) {
		t.Fatalf("launch environment = %q, want %q", env, want)
	}
	if again, err := userWindowsEnvironment(root); err != nil || !reflect.DeepEqual(again, provision) {
		t.Fatalf("provision environment changed: %q (%v)", again, err)
	}
}

func TestUserEngramWindowsCoreRevalidation(t *testing.T) {
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	if _, ok := userEngramWindowsCore(root); ok {
		t.Fatal("a missing core was accepted")
	}
	bin := filepath.Join(root, "agent", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	core := filepath.Join(bin, "engram.exe")
	if err := os.Mkdir(core, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := userEngramWindowsCore(root); ok {
		t.Fatal("a directory core was accepted")
	}
	if err := os.Remove(core); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core, []byte("MZ fixture, never executed"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, ok := userEngramWindowsCore(root); !ok || got != core {
		t.Fatalf("verified core = %q,%v want %q", got, ok, core)
	}
}

func TestUserEngramWindowsHomeComesFromTheProfileFolder(t *testing.T) {
	previous := userWindowsProfileFolder
	t.Cleanup(func() { userWindowsProfileFolder = previous })
	t.Setenv("USERPROFILE", `C:\caller`)
	userWindowsProfileFolder = func() (string, error) { return "", errors.New("fixture failure") }
	if _, ok := userEngramWindowsHome(); ok {
		t.Fatal("a failed profile lookup produced a home")
	}
	if _, err := userEngramWindowsStart(`C:\never\engram.exe`); err == nil {
		t.Fatal("an unverifiable profile started the core")
	}
	folder := t.TempDir()
	userWindowsProfileFolder = func() (string, error) { return folder, nil }
	if home, ok := userEngramWindowsHome(); !ok || home != folder {
		t.Fatalf("home = %q,%v want the profile folder %q", home, ok, folder)
	}
}
