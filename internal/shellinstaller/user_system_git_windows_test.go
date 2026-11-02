//go:build windows

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// userWindowsSystemGitFixture writes a regular fixture git.exe in its own
// directory. It is never executed: only its path and custody are inspected.
func userWindowsSystemGitFixture(t *testing.T, directory, name string) string {
	t.Helper()
	if directory == "" {
		directory = t.TempDir()
	}
	if name == "" {
		name = "git.exe"
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("fixture Git, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// userWindowsProgramFilesStub replaces the known-folder seam and restores it
// at cleanup, so no test depends on the real %ProgramFiles% of the runner.
func userWindowsProgramFilesStub(t *testing.T, folder string, folderErr error) {
	t.Helper()
	previous := userWindowsProgramFilesFolder
	userWindowsProgramFilesFolder = func() (string, error) { return folder, folderErr }
	t.Cleanup(func() { userWindowsProgramFilesFolder = previous })
}

func userWindowsEnvValue(env []string, name string) (string, bool) {
	for _, entry := range env {
		if key, value, found := strings.Cut(entry, "="); found && key == name {
			return value, true
		}
	}
	return "", false
}

func TestUserWindowsSystemGitBinAcceptsValidatedCandidate(t *testing.T) {
	gitExe := userWindowsSystemGitFixture(t, "", "")
	t.Setenv(userWindowsSystemGitCandidateEnv, gitExe)
	bin, ok := userWindowsSystemGitBin()
	if !ok || bin != filepath.Dir(gitExe) {
		t.Fatalf("validated caller git.exe was not accepted: ok=%v bin=%q", ok, bin)
	}
}

func TestUserWindowsSystemGitBinRefusesDoubtfulCandidates(t *testing.T) {
	gitExe := userWindowsSystemGitFixture(t, "", "")
	directoryNamedGit := filepath.Join(t.TempDir(), "git.exe")
	if err := os.Mkdir(directoryNamedGit, 0700); err != nil {
		t.Fatal(err)
	}
	metacharacter := filepath.Join(t.TempDir(), "a%b")
	if err := os.Mkdir(metacharacter, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name      string
		candidate string
	}{
		{"absent candidate", ""},
		{"relative candidate", "git.exe"},
		{"non-canonical candidate", filepath.Dir(gitExe) + string(filepath.Separator) + "." + string(filepath.Separator) + "git.exe"},
		{"missing candidate", filepath.Join(t.TempDir(), "absent", "git.exe")},
		{"directory candidate", directoryNamedGit},
		{"wrong name", userWindowsSystemGitFixture(t, "", "notgit.exe")},
		{"identity-refused candidate", userWindowsSystemGitFixture(t, metacharacter, "")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(userWindowsSystemGitCandidateEnv, tt.candidate)
			if bin, ok := userWindowsSystemGitBin(); ok {
				t.Fatalf("%s was accepted onto PATH: %q", tt.name, bin)
			}
		})
	}
}

func TestUserWindowsSystemGitBinRefusesAliasedCandidate(t *testing.T) {
	gitExe := userWindowsSystemGitFixture(t, "", "")
	aliased := filepath.Join(t.TempDir(), "git.exe")
	userWindowsSymlinkOrSkip(t, gitExe, aliased)
	t.Setenv(userWindowsSystemGitCandidateEnv, aliased)
	if _, ok := userWindowsSystemGitBin(); ok {
		t.Fatal("symlinked git.exe was accepted onto PATH")
	}
}

func TestUserWindowsSystemGitBinRefusesJunctionedDirectory(t *testing.T) {
	directory := t.TempDir()
	userWindowsSystemGitFixture(t, directory, "")
	link := filepath.Join(t.TempDir(), "link")
	userWindowsJunction(t, link, directory)
	t.Setenv(userWindowsSystemGitCandidateEnv, filepath.Join(link, "git.exe"))
	if _, ok := userWindowsSystemGitBin(); ok {
		t.Fatal("git.exe behind a junctioned directory was accepted onto PATH")
	}
}

func TestUserWindowsProgramFilesComesOnlyFromValidatedKnownFolder(t *testing.T) {
	folder := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	userWindowsJunction(t, link, t.TempDir())
	// The caller variable is never a source, whatever the known folder says.
	t.Setenv("ProgramFiles", filepath.Join(t.TempDir(), "caller"))
	for _, tt := range []struct {
		name      string
		folder    string
		folderErr error
		want      string
	}{
		{"validated known folder", folder, nil, folder},
		{"known folder unavailable", "", errors.New("fixture known-folder failure"), ""},
		{"empty known folder", "", nil, ""},
		{"relative known folder", "Program Files", nil, ""},
		{"missing known folder", filepath.Join(t.TempDir(), "absent"), nil, ""},
		{"junctioned known folder", link, nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			userWindowsProgramFilesStub(t, tt.folder, tt.folderErr)
			got, ok := userWindowsProgramFiles()
			if ok != (tt.want != "") || got != tt.want {
				t.Fatalf("ProgramFiles resolution: got (%q, %v); want %q", got, ok, tt.want)
			}
		})
	}
}

// Git and ProgramFiles only ever add to the owned launch environment: every
// existing entry stays byte-identical, the validated Git directory is
// inserted just before System32, and ProgramFiles is the validated known
// folder. Without both, the environment is exactly today's. The only other
// additions are the Engram decision entries, which close the list.
func TestUserWindowsEnvironmentAddsOnlyValidatedGitAndProgramFiles(t *testing.T) {
	root := t.TempDir()
	for _, key := range append(append([]string{}, userEngramSelections...), userEngramPolicyEnv, userEngramNoticeEnv) {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	// No decision was forwarded: a Separate launch fails closed.
	engram := userEngramDisabled(userEngramNoDecisionNotice)
	t.Setenv("ProgramFiles", filepath.Join(t.TempDir(), "caller"))
	t.Setenv(userWindowsSystemGitCandidateEnv, "")
	userWindowsProgramFilesStub(t, "", errors.New("fixture known-folder failure"))
	baseline, err := userWindowsEnvironment(root)
	if err != nil {
		t.Fatalf("owned environment refused: %v", err)
	}
	if _, found := userWindowsEnvValue(baseline, "ProgramFiles"); found {
		t.Fatalf("caller ProgramFiles leaked into the owned environment: %q", baseline)
	}
	basePath, _ := userWindowsEnvValue(baseline, "PATH")

	gitExe := userWindowsSystemGitFixture(t, "", "")
	folder := t.TempDir()
	t.Setenv(userWindowsSystemGitCandidateEnv, gitExe)
	userWindowsProgramFilesStub(t, folder, nil)
	// Provision (install, overlay, verify) keeps today's environment exactly.
	provision, err := userWindowsEnvironment(root)
	if err != nil {
		t.Fatalf("provision environment refused: %v", err)
	}
	if strings.Join(provision, "\n") != strings.Join(baseline, "\n") {
		t.Fatalf("provision environment changed: %q", provision)
	}
	env, err := userWindowsLaunchEnvironment(root)
	if err != nil {
		t.Fatalf("owned launch environment refused: %v", err)
	}
	if len(env) != len(baseline)+1+len(engram) || !reflect.DeepEqual(env[len(baseline)+1:], engram) {
		t.Fatalf("environment gained unexpected entries: %q", env)
	}
	for i, entry := range baseline {
		if strings.HasPrefix(entry, "PATH=") {
			continue
		}
		if env[i] != entry {
			t.Fatalf("existing entry changed: %q became %q", entry, env[i])
		}
	}
	entries := strings.Split(basePath, string(os.PathListSeparator))
	want := append(append(append([]string{}, entries[:len(entries)-1]...), filepath.Dir(gitExe)), entries[len(entries)-1])
	if path, _ := userWindowsEnvValue(env, "PATH"); path != strings.Join(want, string(os.PathListSeparator)) {
		t.Fatalf("PATH = %q; want %q", path, strings.Join(want, string(os.PathListSeparator)))
	}
	if value, _ := userWindowsEnvValue(env, "ProgramFiles"); value != folder || env[len(baseline)] != "ProgramFiles="+folder {
		t.Fatalf("ProgramFiles = %q; want %q", value, folder)
	}

	// The bare worker environment of the outer entry stays System32-only.
	worker, err := userWindowsEnvironment("")
	if err != nil {
		t.Fatalf("worker environment refused: %v", err)
	}
	if path, _ := userWindowsEnvValue(worker, "PATH"); strings.Contains(path, filepath.Dir(gitExe)) {
		t.Fatalf("worker PATH gained Git: %q", path)
	}
	if _, found := userWindowsEnvValue(worker, "ProgramFiles"); found {
		t.Fatalf("worker environment gained ProgramFiles: %q", worker)
	}
}
