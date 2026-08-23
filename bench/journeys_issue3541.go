package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// issue3541Journeys drives the compiled product through settlement while Git
// reproduces the pre-2.38 successful echo of --show-object-format. The product
// must use the repository-local object-format config instead of that probe.
func issue3541Journeys() []Journey {
	return []Journey{{
		ID:     "j114-sdd-attempt-settle-uses-local-object-format-config",
		Review: reviewUntouched,
		Title:  "SDD attempt settlement ignores the legacy Git object-format probe",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3541",
		Steps: []Step{
			{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
			{Name: "fixture: Git echoes the unsupported object-format probe", Fixture: issue3541LegacyGitFixture},
			{Name: "acquire a bounded SDD attempt", Requires: sddAttemptRemediationCapability, Composite: issue3541Acquire},
			{Name: "settle the attempt despite the legacy Git probe", Requires: sddAttemptRemediationCapability, Composite: issue3541Settle},
		},
	}}
}

func issue3541LegacyGitFixture(sandbox *Sandbox) error {
	realGit, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	bin := filepath.Join(sandbox.Root, "legacy-git-bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(bin, name)
	source := "package main\nimport (\"os\"; \"os/exec\")\nconst realGit = " + strconv.Quote(realGit) + `
func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--show-object-format" {
			_, _ = os.Stdout.WriteString("--show-object-format\n")
			return
		}
	}
	command := exec.Command(realGit, os.Args[1:]...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		panic(err)
	}
}
`
	sourcePath := filepath.Join(bin, "main.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		return err
	}
	if output, err := exec.Command("go", "build", "-o", path, sourcePath).CombinedOutput(); err != nil {
		return fmt.Errorf("build legacy Git shim: %w: %s", err, strings.TrimSpace(string(output)))
	}
	sandbox.PathOverride = bin
	return nil
}

func issue3541Acquire(r *journeyRun) error {
	observation := r.run(append([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", "issue3541-acquire",
	}, sddChainVerifyObjective...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil ||
		observation.ExitCode != 0 || result.State != "proceed" || result.Token == "" {
		return fmt.Errorf("legacy-Git acquire = %#v exit=%d err=%v", result, observation.ExitCode, err)
	}
	r.sandbox.Scratch["issue3541-token"] = result.Token
	return nil
}

func issue3541Settle(r *journeyRun) error {
	token := r.sandbox.Scratch["issue3541-token"]
	if token == "" {
		return errors.New("legacy-Git journey has no attempt token")
	}
	observation := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--token", token, "--request-id", "issue3541-settle", "--outcome", "passed",
		"--evidence-revision", sddCorrectedEvidence,
	}, sddTerminalEvidence...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil ||
		observation.ExitCode != 0 || result.State != "complete" || result.Reason == "authority_failure" {
		return fmt.Errorf("legacy-Git settle = %#v exit=%d err=%v", result, observation.ExitCode, err)
	}
	return nil
}
