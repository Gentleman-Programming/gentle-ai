package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func issue3043OpenCodeRuntime(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	path := filepath.Join(sandbox.Root, "bin")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	launcher := filepath.Join(path, "opencode")
	content := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf '1.15.11\\n'; exit 0; fi\nprintf 'runtime:%s\\n' \"${OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS-unset}\"\n"
	if err := os.WriteFile(launcher, []byte(content), 0o755); err != nil {
		return err
	}
	sandbox.PathOverride = path
	sandbox.Scratch["issue-3043-opencode"] = launcher
	return nil
}

func issue3043InstallArgs(sandbox *Sandbox) ([]string, error) {
	return []string{"install", "--agent", "opencode", "--component", "sdd", "--opencode-background-subagents=on"}, nil
}

func issue3043VerifyInstall(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("OpenCode background install failed: %s", firstLine(observation.Stderr))
	}
	if !strings.Contains(observation.Stdout, "OpenCode background activation status: ready") ||
		!strings.Contains(observation.Stdout, "OpenCode background restart required: true") {
		return fmt.Errorf("install omitted ready activation evidence: %s", observation.Stdout)
	}
	if strings.Contains(observation.Stdout, "OPENCODE_EXPERIMENTAL=true") {
		return fmt.Errorf("install emitted legacy shell mutation guidance: %s", observation.Stdout)
	}
	launcher := filepath.Join(sandbox.Home, ".gentle-ai", "bin", "opencode")
	data, err := os.ReadFile(launcher)
	if err != nil || !strings.Contains(string(data), "gentle-ai:managed-opencode-launcher/v1") {
		return fmt.Errorf("managed launcher missing or unowned: %q, %v", data, err)
	}
	cmd := exec.Command(launcher)
	cmd.Dir = sandbox.Repo
	cmd.Env = sandbox.env()
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) != "runtime:true" {
		return fmt.Errorf("managed launcher did not inject background env: %q, %v", output, err)
	}
	cmd = exec.Command(launcher)
	cmd.Dir = sandbox.Repo
	cmd.Env = append(sandbox.env(), "OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=false")
	output, err = cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) != "runtime:false" {
		return fmt.Errorf("managed launcher overwrote explicit false: %q, %v", output, err)
	}
	return nil
}

func issue3043Journeys() []Journey {
	return []Journey{{
		ID:     "j3043-opencode-managed-background-activation",
		Review: reviewUntouched,
		Title:  "OpenCode background subagents activate through a managed launcher",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3043",
		Steps: []Step{
			{Name: "fixture: isolated OpenCode runtime", Fixture: issue3043OpenCodeRuntime},
			{Name: "install reports managed activation", Args: issue3043InstallArgs, After: issue3043VerifyInstall},
		},
	}}
}
