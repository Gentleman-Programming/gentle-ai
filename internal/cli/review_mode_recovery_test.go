package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// An unreadable kill-switch value is not a disabled switch: it resolves to
// managed, and every review verb correctly refuses. What it never did was tell
// the operator anything actionable. The value is readable, so the product knows
// exactly which file holds it and exactly which command overwrites it, and it
// named neither.
//
// These tests never assert on wording alone. They do what an operator does:
// read the message, run what it names, and try again -- and they require that
// loop to terminate with the block gone. A message that names a command which
// does not clear the block fails here, and so does one that loops forever.

// reviewModeCorruption selects which of the two independent scopes holds a
// readable but nonsense mode value.
type reviewModeCorruption struct {
	global bool
	clone  bool
}

func TestUnreadableGlobalModeNamesItsFileAndACommandThatClearsIt(t *testing.T) {
	assertUnreadableModeIsRecoverable(t, reviewModeCorruption{global: true})
}

func TestUnreadableCloneLocalModeNamesItsFileAndACommandThatClearsIt(t *testing.T) {
	reviewEnabledHome(t)
	assertUnreadableModeIsRecoverable(t, reviewModeCorruption{clone: true})
}

// Naming only the first unreadable scope would send the operator round the
// loop: they overwrite what the message named, rerun, and hit the same wall
// from the other source.
func TestBothUnreadableModeSourcesAreNamed(t *testing.T) {
	assertUnreadableModeIsRecoverable(t, reviewModeCorruption{global: true, clone: true})
}

func assertUnreadableModeIsRecoverable(t *testing.T, corruption reviewModeCorruption) {
	t.Helper()

	repo, message := reviewModeUnreadableFixture(t, corruption)
	for _, path := range reviewModeCorruptPaths(t, corruption, repo) {
		if !strings.Contains(message, path) {
			t.Fatalf("refusal does not name the file holding the unreadable value %q:\n%s", path, message)
		}
	}
	if corruption.global && corruption.clone {
		if commands := reviewModeNamedCommands(message); len(commands) < 4 {
			t.Fatalf("both scopes are unreadable but the refusal names %d continuations:\n%s", len(commands), message)
		}
	}

	// Follow the messages exactly as an operator would, and require the loop to
	// terminate. Two unreadable scopes need at most two repairs, so a bound of
	// three rounds also proves the recovery makes progress rather than
	// re-emitting the same wall.
	t.Run("following the named recovery clears the block", func(t *testing.T) {
		repo, message := reviewModeUnreadableFixture(t, corruption)
		for round := 0; round < 3; round++ {
			command := reviewModeNamedEnableCommand(t, message)
			var output bytes.Buffer
			runErr := RunReviewMode(reviewModeCommandArgs(command, repo), &output)
			var status bytes.Buffer
			statusErr := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &status)
			if statusErr == nil {
				if runErr != nil {
					t.Fatalf("the switch is readable again but %q reported failure: %v", command, runErr)
				}
				// The operator wanted to review, so the recovery has to end
				// with a review that runs.
				if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				output.Reset()
				if err := RunReview([]string{"start", "--cwd", repo}, &output); err != nil {
					t.Fatalf("review start still blocked after the named recovery: %v\n%s", err, output.String())
				}
				return
			}
			message = statusErr.Error()
		}
		t.Fatalf("following the named recovery never cleared the block; last message:\n%s", message)
	})
}

// reviewModeUnreadableFixture builds a repository whose named scopes hold a
// readable but nonsense mode value, and returns the message `review mode
// status` -- the read-only diagnostic an operator reaches for first -- emits.
//
// The operator this models wants to review, so the fixture starts from an
// explicit global "on": receipt-driven development is opt-in, and without that
// opinion clearing a corrupted clone-local override would land on the off
// default and the recovery would look like it never worked. A global
// corruption below overwrites that same field, so the global case is unchanged.
func reviewModeUnreadableFixture(t *testing.T, corruption reviewModeCorruption) (string, string) {
	t.Helper()
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)

	if corruption.clone {
		var output bytes.Buffer
		if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone"}, &output); err != nil {
			t.Fatalf("seed clone-local override: %v\n%s", err, output.String())
		}
		path := reviewModeCloneRecordPath(t, repo)
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		tampered := strings.Replace(string(payload), `"mode":"off"`, `"mode":"sometimes"`, 1)
		if tampered == string(payload) {
			t.Fatalf("clone-local override does not carry the expected mode field: %s", payload)
		}
		if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if corruption.global {
		persisted, err := state.Read(home)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		persisted.RDDMode = "sometimes"
		if err := state.Write(home, persisted); err != nil {
			t.Fatal(err)
		}
	}

	var output bytes.Buffer
	err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &output)
	if err == nil {
		t.Fatalf("an unreadable switch was reported as an ordinary state: %s", output.String())
	}
	return repo, err.Error()
}

func reviewModeCorruptPaths(t *testing.T, corruption reviewModeCorruption, repo string) []string {
	t.Helper()
	paths := []string{}
	if corruption.global {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, state.Path(home))
	}
	if corruption.clone {
		paths = append(paths, reviewModeCloneRecordPath(t, repo))
	}
	return paths
}

func reviewModeCloneRecordPath(t *testing.T, repo string) string {
	t.Helper()
	root := filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority", "v1", "rdd-mode")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("clone-local override directory: %v", err)
	}
	head := ""
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "gen-") && strings.HasSuffix(entry.Name(), ".json") && entry.Name() > head {
			head = entry.Name()
		}
	}
	if head == "" {
		t.Fatalf("no clone-local override generation under %s", root)
	}
	return filepath.Join(root, head)
}

// reviewModeNamedCommands lifts every `gentle-ai review mode ...` invocation the
// message names, returning the arguments RunReviewMode consumes.
func reviewModeNamedCommands(message string) [][]string {
	commands := [][]string{}
	for _, match := range reviewContinuationPattern.FindAllStringSubmatch(message, -1) {
		fields := strings.Fields(match[1])
		if len(fields) < 4 || fields[0] != "gentle-ai" || fields[1] != "review" || fields[2] != "mode" {
			continue
		}
		commands = append(commands, fields[3:])
	}
	return commands
}

func reviewModeNamedEnableCommand(t *testing.T, message string) []string {
	t.Helper()
	for _, command := range reviewModeNamedCommands(message) {
		if command[0] == "enable" {
			return command
		}
	}
	t.Fatalf("refusal names no runnable way to turn reviews back on:\n%s", message)
	return nil
}

// reviewModeCommandArgs runs the named command exactly as printed. A command
// that already names its repository is left untouched; the global scope is
// repository-independent, so it is pointed at the fixture.
func reviewModeCommandArgs(command []string, repo string) []string {
	for _, argument := range command {
		if argument == "--cwd" || strings.HasPrefix(argument, "--cwd=") {
			return command
		}
	}
	return append(append([]string{}, command...), "--cwd", repo)
}
