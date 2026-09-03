package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const windowsErrorPrivilegeNotHeld = syscall.Errno(1314)

func issue3557DanglingSymlinkFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}

	configPath := filepath.Join(sandbox.Home, ".config", "opencode")
	targetPath := filepath.Join(sandbox.Root, "missing-external-opencode-config")
	if err := os.Symlink(targetPath, configPath); err != nil {
		if !errors.Is(err, windowsErrorPrivilegeNotHeld) {
			return err
		}
		sandbox.Scratch["issue-3557-symlink-unavailable"] = err.Error()
		return nil
	}
	if err := os.MkdirAll(filepath.Join(sandbox.Home, ".claude"), 0o755); err != nil {
		return err
	}
	statePath := filepath.Join(sandbox.Home, ".gentle-ai", "state.json")
	state := fmt.Sprintf(`{"installed_agents":["opencode","claude-code"],"last_update_check":%q}`, time.Now().UTC().Format(time.RFC3339Nano))
	if err := sandbox.write(statePath, state); err != nil {
		return err
	}
	claudeConfig := `{"mcpServers":{"engram":{"command":"engram-not-installed-for-issue-3557","args":[]}}}`
	if err := sandbox.write(filepath.Join(sandbox.Home, ".claude.json"), claudeConfig); err != nil {
		return err
	}

	sandbox.Scratch["issue-3557-config"] = configPath
	sandbox.Scratch["issue-3557-target"] = targetPath
	sandbox.Scratch["issue-3557-state"] = statePath
	sandbox.Scratch["issue-3557-state-content"] = state
	sandbox.Scratch["issue-3557-link-target"] = targetPath
	return nil
}

func issue3557DoctorArgs(*Sandbox) ([]string, error) { return []string{"doctor"}, nil }

func issue3557SymlinkSkip(sandbox *Sandbox) string {
	if reason := sandbox.Scratch["issue-3557-symlink-unavailable"]; reason != "" {
		return "managed config symlink unavailable: " + reason
	}
	return ""
}

func issue3557VerifyDoctor(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("gentle-ai doctor exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	if strings.Contains(observation.Stdout, "gentle-ai sync") {
		return fmt.Errorf("doctor recommended the unrunnable sync recovery: %s", observation.Stdout)
	}
	for _, want := range []string{sandbox.Scratch["issue-3557-config"], "inspect", "gentle-ai doctor"} {
		if !strings.Contains(observation.Stdout, want) {
			return fmt.Errorf("doctor output missing %q: %s", want, observation.Stdout)
		}
	}

	configPath := sandbox.Scratch["issue-3557-config"]
	info, err := os.Lstat(configPath)
	if err != nil {
		return fmt.Errorf("lstat managed config symlink after doctor: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("doctor changed managed config path into %s", info.Mode())
	}
	linkTarget, err := os.Readlink(configPath)
	if err != nil {
		return fmt.Errorf("read managed config symlink after doctor: %w", err)
	}
	if linkTarget != sandbox.Scratch["issue-3557-link-target"] {
		return fmt.Errorf("doctor changed symlink target from %q to %q", sandbox.Scratch["issue-3557-link-target"], linkTarget)
	}
	targetPath := sandbox.Scratch["issue-3557-target"]
	if _, err := os.Lstat(targetPath); err == nil {
		return fmt.Errorf("doctor unexpectedly created %q", targetPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("doctor changed %q: %w", targetPath, err)
	}
	state, err := os.ReadFile(sandbox.Scratch["issue-3557-state"])
	if err != nil {
		return fmt.Errorf("read state after doctor: %w", err)
	}
	if string(state) != sandbox.Scratch["issue-3557-state-content"] {
		return fmt.Errorf("doctor changed state.json: got %q, want %q", string(state), sandbox.Scratch["issue-3557-state-content"])
	}
	backupPath := filepath.Join(sandbox.Home, ".gentle-ai", "backups")
	if _, err := os.Lstat(backupPath); err == nil {
		return fmt.Errorf("doctor unexpectedly created %q", backupPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("doctor could not inspect %q: %w", backupPath, err)
	}
	return nil
}

func issue3557Journeys() []Journey {
	return []Journey{{
		ID:     "j117-doctor-dangling-managed-config",
		Review: reviewUntouched,
		Title:  "Doctor identifies a dangling managed config symlink without recommending sync",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3557",
		Steps: []Step{
			{Name: "fixture: dangling OpenCode managed config symlink", Fixture: issue3557DanglingSymlinkFixture},
			{Name: "doctor reports manual repair and preserves the filesystem", Skip: issue3557SymlinkSkip, Args: issue3557DoctorArgs, After: issue3557VerifyDoctor},
		},
	}}
}
