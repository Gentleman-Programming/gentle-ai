// Package openrecord installs and detects openrecord as an opt-in gentle-ai
// community tool: the binary, its qmd prerequisite, and the skill fan-out to
// every selected agent. communitytool.Install stays CodeGraph-only; the three
// call sites branch here instead of widening that switch.
package openrecord

import (
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const goImportPath = "github.com/franwerner/openrecord/cmd/openrecord"

// installScriptURL points at the script's documented curl|bash invocation
// (copia/open-record/scripts/install.sh header comment), always the current
// default branch: the script itself resolves the release to install.
const installScriptURL = "https://raw.githubusercontent.com/franwerner/open-record/master/scripts/install.sh"

// Install runs the ordered sequence behind the opt-in checkbox: install the
// binary only when missing, install qmd unconditionally, emit skills with qmd
// into a staging directory, then fan them out to every selected agent that
// exposes a skills directory. A step does not run when the step before it
// failed, so skills are never emitted describing a tool whose qmd half is
// missing.
func Install(homeDir string, selectedAgents []model.AgentID, runner communitytool.Runner, detector communitytool.Detector) (communitytool.Result, error) {
	if runner == nil {
		return communitytool.Result{}, fmt.Errorf("openrecord runner is not configured")
	}
	if detector == nil {
		detector = defaultDetector()
	}

	result := communitytool.Result{Tool: model.CommunityToolOpenRecord}
	before := DetectStatus(homeDir, detector, runner)
	result.StatusBefore = &before

	if before.CLI != communitytool.AvailabilityAvailable {
		command, err := binaryInstallCommand(detector)
		if err != nil {
			return result, err
		}
		result.CommandsRun = append(result.CommandsRun, strings.Join(command, " "))
		if err := runner.Run(command[0], command[1:]...); err != nil {
			return result, fmt.Errorf("install openrecord binary: %w", err)
		}
	}

	result.CommandsRun = append(result.CommandsRun, "openrecord qmd install")
	if err := runner.Run("openrecord", "qmd", "install"); err != nil {
		return result, fmt.Errorf("openrecord qmd install: %w", err)
	}

	staging, err := os.MkdirTemp("", "gentle-ai-openrecord-emit-*")
	if err != nil {
		return result, fmt.Errorf("stage openrecord skills: %w", err)
	}
	defer os.RemoveAll(staging)

	result.CommandsRun = append(result.CommandsRun, fmt.Sprintf("openrecord skills --emit %s --with-qmd", staging))
	if err := runner.Run("openrecord", "skills", "--emit", staging, "--with-qmd"); err != nil {
		return result, fmt.Errorf("openrecord skills --emit: %w", err)
	}

	fanned, err := fanOut(staging, homeDir, selectedAgents)
	if err != nil {
		return result, err
	}
	if fanned == 0 {
		result.ManualActions = append(result.ManualActions, "openrecord was installed, but no selected agent exposes a skills directory — skills were not fanned out.")
	} else {
		result.ManualActions = append(result.ManualActions, fmt.Sprintf("openrecord skills were fanned out to %d agent(s).", fanned))
	}

	after := DetectStatus(homeDir, detector, runner)
	result.StatusAfter = &after
	return result, nil
}

// binaryInstallCommand mirrors effectiveMethod's own predicate (Go on PATH
// preferred over the install script) so the first-time install and the
// upgrade path never disagree about which copy is authoritative.
func binaryInstallCommand(detector communitytool.Detector) ([]string, error) {
	if detector == nil {
		return nil, fmt.Errorf("openrecord install: no detector configured")
	}
	if _, err := detector.LookPath("go"); err == nil {
		return []string{"go", "install", goImportPath + "@latest"}, nil
	}
	return []string{"sh", "-c", fmt.Sprintf("curl -fsSL %s | env WITH_QMD=no bash", installScriptURL)}, nil
}
