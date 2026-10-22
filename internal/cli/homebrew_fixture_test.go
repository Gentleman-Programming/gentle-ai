package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// This is a protocol fixture, not evidence of real Homebrew's trust policy.
type portableBrewState struct {
	Commands  []string
	Trusted   string
	Installed string
}

func runPortableBrewFixture() int {
	path := os.Getenv("GENTLE_AI_TEST_BREW_STATE")
	var state portableBrewState
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &state) != nil {
			return 99
		}
	}
	args := strings.Join(os.Args[1:], " ")
	if args == "status" {
		_ = json.NewEncoder(os.Stdout).Encode(state)
		return 0
	}
	state.Commands = append(state.Commands, args)
	code := 0
	switch {
	case args == "help trust":
		if os.Getenv("GENTLE_AI_TEST_BREW_LEGACY") == "1" {
			code = 2
		}
	case args == "tap Gentleman-Programming/homebrew-tap":
	case strings.HasPrefix(args, "trust --formula gentleman-programming/tap/"):
		formula := strings.TrimPrefix(args, "trust --formula gentleman-programming/tap/")
		if os.Getenv("GENTLE_AI_TEST_BREW_DENY") == "1" || (formula != "engram" && formula != "gga") {
			fmt.Fprintln(os.Stderr, "fixture: trust refused")
			code = 7
		} else {
			state.Trusted = formula
		}
	case args == "install engram" || args == "reinstall gga":
		formula := strings.Fields(args)[1]
		if os.Getenv("GENTLE_AI_TEST_BREW_LEGACY") != "1" && state.Trusted != formula {
			fmt.Fprintln(os.Stderr, "fixture: formula is untrusted")
			code = 8
		} else {
			state.Installed = formula
		}
	default:
		fmt.Fprintln(os.Stderr, "fixture: unsupported command")
		code = 9
	}
	data, err := json.Marshal(state)
	if err != nil || os.WriteFile(path, data, 0o600) != nil {
		return 99
	}
	return code
}

func TestPortableHomebrewComponentPipeline(t *testing.T) {
	// Copy this native test executable as brew, avoiding shell scripts and skips
	// on Windows. Only child processes enter the fixture branch in TestMain.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "brew"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	brew := filepath.Join(bin, name)
	src, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.OpenFile(brew, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy fixture: %v, %v", copyErr, closeErr)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GENTLE_AI_TEST_BREW_FIXTURE", "1")
	originalRunner := runCommand
	runCommand = executeCommand
	t.Cleanup(func() { runCommand = originalRunner })
	restore := SetCommandOutputStreaming(false)
	t.Cleanup(restore)

	for _, component := range []model.ComponentID{model.ComponentEngram, model.ComponentGGA} {
		for _, mode := range []string{"modern", "legacy", "denied", "untrusted-control"} {
			t.Run(string(component)+"/"+mode, func(t *testing.T) {
				t.Setenv("GENTLE_AI_TEST_BREW_STATE", filepath.Join(t.TempDir(), "state.json"))
				t.Setenv("GENTLE_AI_TEST_BREW_LEGACY", "0")
				t.Setenv("GENTLE_AI_TEST_BREW_DENY", "0")
				if mode == "legacy" {
					t.Setenv("GENTLE_AI_TEST_BREW_LEGACY", "1")
				}
				if mode == "denied" {
					t.Setenv("GENTLE_AI_TEST_BREW_DENY", "1")
				}
				commands, err := installcmd.NewResolver().ResolveComponentInstall(system.PlatformProfile{OS: "darwin", PackageManager: "brew"}, component)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "untrusted-control" {
					// Removing only trust must reproduce the original failure.
					commands = append(commands[:1], commands[2:]...)
				}
				err = runCommandSequence(withResolvedBrewCommand(commands))
				formula := string(component)
				action := "install"
				if component == model.ComponentGGA {
					action = "reinstall"
				}
				want := portableBrewState{Commands: []string{"help trust", "tap Gentleman-Programming/homebrew-tap"}}
				if mode != "legacy" && mode != "untrusted-control" {
					want.Commands = append(want.Commands, "trust --formula gentleman-programming/tap/"+formula)
				}
				if mode != "denied" {
					want.Commands = append(want.Commands, action+" "+formula)
				}
				if mode == "modern" {
					want.Trusted = formula
				}
				if mode == "modern" || mode == "legacy" {
					want.Installed = formula
				}
				if mode == "denied" || mode == "untrusted-control" {
					failedCommand, code, message := "trust --formula gentleman-programming/tap/"+formula, 7, "fixture: trust refused"
					if mode == "untrusted-control" {
						failedCommand, code, message = action+" "+formula, 8, "fixture: formula is untrusted"
					}
					wantError := fmt.Sprintf("run command %q: exit status %d\noutput:\n%s", brew+" "+failedCommand, code, message)
					if err == nil || err.Error() != wantError {
						t.Fatalf("pipeline error = %v, want %q", err, wantError)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				// Read state through the fixture's public command, never its file.
				out, err := exec.Command(brew, "status").Output()
				if err != nil {
					t.Fatal(err)
				}
				var got portableBrewState
				if err := json.Unmarshal(out, &got); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("status = %+v, want %+v", got, want)
				}
			})
		}
	}
}
