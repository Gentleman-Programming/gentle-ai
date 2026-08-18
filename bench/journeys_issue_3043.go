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
	sandbox.Shell = "/bin/zsh"
	if err := os.WriteFile(filepath.Join(sandbox.Home, ".zprofile"), []byte("export BENCH_PROFILE=1\n"), 0o644); err != nil {
		return err
	}
	return nil
}

func issue3043InstallArgs(sandbox *Sandbox) ([]string, error) {
	return []string{"install", "--agent", "opencode", "--component", "sdd", "--opencode-background-subagents=on"}, nil
}

func issue3043VerifyInstall(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("OpenCode background install failed: %s", firstLine(observation.Stderr))
	}
	if !strings.Contains(observation.Stdout, "OpenCode background activation status: pending") ||
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
	cmd := issue3043LoginShell("command -v opencode; opencode")
	cmd.Dir = sandbox.Repo
	cmd.Env = sandbox.env()
	output, err := cmd.Output()
	if err != nil || !strings.Contains(string(output), launcher) || !strings.HasSuffix(strings.TrimSpace(string(output)), "runtime:true") {
		return fmt.Errorf("fresh login shell did not resolve managed launcher with background env: %q, %v", output, err)
	}
	cmd = issue3043LoginShell("opencode")
	cmd.Dir = sandbox.Repo
	cmd.Env = append(sandbox.env(), "OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=false")
	output, err = cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) != "runtime:false" {
		return fmt.Errorf("managed launcher overwrote explicit false: %q, %v", output, err)
	}
	return nil
}

// CI images may omit zsh. The fallback starts a fresh shell that explicitly
// sources .zprofile, proving the product selected and wrote the zsh profile and
// that bare opencode resolves through it; a real zsh login shell is exercised
// whenever available.
func issue3043LoginShell(command string) *exec.Cmd {
	if zsh, err := exec.LookPath("zsh"); err == nil {
		return exec.Command(zsh, "-l", "-c", command)
	}
	return exec.Command("sh", "-c", ". \"$HOME/.zprofile\"; "+command)
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
