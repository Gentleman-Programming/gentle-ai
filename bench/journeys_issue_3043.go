package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var issue3043LookPath = exec.LookPath

const (
	issue3043SyncMarker   = "issue3043:sync"
	issue3043DoctorMarker = "issue3043:doctor"
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
	output, err := issue3043RunLoginShell(sandbox, `printf 'profile:%s\n' "$BENCH_PROFILE"; command -v opencode; opencode`)
	if err != nil || !strings.Contains(output, "profile:1") || !strings.Contains(output, launcher) || !strings.HasSuffix(strings.TrimSpace(output), "runtime:true") {
		return fmt.Errorf("fresh zsh login shell did not source the real profile and resolve managed launcher with background env: %q, %v", output, err)
	}
	output, err = issue3043RunLoginShell(sandbox, "opencode", "OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=false")
	if err != nil || strings.TrimSpace(output) != "runtime:false" {
		return fmt.Errorf("managed launcher overwrote explicit false: %q, %v", output, err)
	}
	return nil
}

func issue3043ZshUnavailable(*Sandbox) string {
	if _, err := issue3043LookPath("zsh"); err != nil {
		return "zsh is unavailable"
	}
	return ""
}

func issue3043LoginShell(command string) (*exec.Cmd, error) {
	zsh, err := issue3043LookPath("zsh")
	if err != nil {
		return nil, fmt.Errorf("zsh is unavailable: %w", err)
	}
	return exec.Command(zsh, "-l", "-c", command), nil
}

func issue3043RunLoginShell(sandbox *Sandbox, command string, extraEnv ...string) (string, error) {
	cmd, err := issue3043LoginShell(command)
	if err != nil {
		return "", err
	}
	cmd.Dir = sandbox.Repo
	cmd.Env = append(sandbox.env(), extraEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("fresh zsh shell failed: %w", err)
	}
	return string(output), nil
}

func issue3043FreshShellCommand(binary string) string {
	quotedBinary := shellQuote(binary)
	return fmt.Sprintf("printf '%s\\n'; %s sync && printf '%s\\n'; %s doctor", issue3043SyncMarker, quotedBinary, issue3043DoctorMarker, quotedBinary)
}

func issue3043AssertFreshShellOutput(output string) error {
	syncMarker := issue3043SyncMarker + "\n"
	doctorMarker := issue3043DoctorMarker + "\n"
	syncStart := strings.Index(output, syncMarker)
	doctorStart := strings.Index(output, doctorMarker)
	if syncStart < 0 || doctorStart < 0 || doctorStart <= syncStart {
		return fmt.Errorf("fresh zsh shell did not run sync followed by doctor: %q", output)
	}

	syncOutput := output[syncStart+len(syncMarker) : doctorStart]
	doctorOutput := output[doctorStart+len(doctorMarker):]
	runtimeReady := strings.Contains(syncOutput, "OpenCode background runtime ready: true")
	runtimeNotReady := strings.Contains(syncOutput, "OpenCode background runtime ready: false")
	if runtimeReady == runtimeNotReady {
		return fmt.Errorf("fresh zsh sync did not report one post-Apply runtime readiness conclusion: %q", syncOutput)
	}
	if !strings.Contains(syncOutput, "OpenCode background activation status: ready") {
		return fmt.Errorf("fresh zsh sync did not report ready managed activation after Apply: %q", syncOutput)
	}

	var managedActivationLine string
	for _, line := range strings.Split(doctorOutput, "\n") {
		if strings.Contains(line, "opencode:managed_activation") {
			managedActivationLine = line
			break
		}
	}
	if managedActivationLine == "" {
		return fmt.Errorf("fresh zsh doctor did not report managed OpenCode activation: %q", doctorOutput)
	}
	doctorReady := strings.Contains(managedActivationLine, "[ok]")
	doctorNotReady := strings.Contains(managedActivationLine, "[xx]")
	if doctorReady == doctorNotReady {
		return fmt.Errorf("fresh zsh doctor did not report one managed activation conclusion: %q", managedActivationLine)
	}
	if runtimeReady != doctorReady {
		return fmt.Errorf("fresh zsh sync and doctor disagree about managed activation: sync_ready=%t doctor_ready=%t", runtimeReady, doctorReady)
	}
	if !runtimeReady {
		return fmt.Errorf("fresh zsh managed activation remained not ready: sync=%q doctor=%q", syncOutput, managedActivationLine)
	}
	return nil
}

func issue3043VerifyFreshShell(r *journeyRun) error {
	output, err := issue3043RunLoginShell(r.sandbox, issue3043FreshShellCommand(r.sandbox.Binary))
	if err != nil {
		return fmt.Errorf("fresh zsh sync and doctor failed: %w: %s", err, output)
	}
	return issue3043AssertFreshShellOutput(output)
}

func issue3043Journeys() []Journey {
	return []Journey{{
		ID:     "j3043-opencode-managed-background-activation",
		Review: reviewUntouched,
		Title:  "OpenCode background subagents activate through a managed launcher",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3043",
		Steps: []Step{
			{Name: "fixture: isolated OpenCode runtime", Skip: issue3043ZshUnavailable, Fixture: issue3043OpenCodeRuntime},
			{Name: "install reports managed activation", Args: issue3043InstallArgs, After: issue3043VerifyInstall},
			{Name: "fresh zsh sync and doctor agree on managed readiness", Composite: issue3043VerifyFreshShell},
		},
	}}
}
