package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
)

const piCodingAgentDirEnv = "PI_CODING_AGENT_DIR"

type piModelInspectionMsg struct {
	requestID  uint64
	inspection pi.ModelRoutingInspection
	err        error
}

type piModelInspectionEnumerateFunc func(cwd, agentDir string) ([]pi.ModelRoutingCandidate, error)
type piModelInspectionSelectFunc func(context.Context, []pi.ModelRoutingCandidate) (pi.ModelRoutingCandidate, pi.Capabilities, error)
type piModelInspectionInspectFunc func(context.Context, pi.ModelRoutingCandidate, pi.Capabilities, pi.ModelRoutingRequestContext) (pi.ModelRoutingInspection, error)

var (
	piModelInspectionGetwdFn   = os.Getwd
	piModelInspectionHomeDirFn = os.UserHomeDir
	piModelInspectionEnvFn     = os.Getenv

	piModelInspectionAgentDirFn                                 = resolvePiModelInspectionAgentDir
	piModelInspectionEnumerateFn piModelInspectionEnumerateFunc = pi.EnumerateModelRoutingCandidates
	piModelInspectionSelectFn    piModelInspectionSelectFunc    = pi.SelectCompatibleModelRoutingCandidate
	piModelInspectionInspectFn   piModelInspectionInspectFunc   = inspectPiModelRoutingClient
	piModelInspectionLoadFn                                     = loadPiModelInspection
)

func resolvePiModelInspectionAgentDir() (string, error) {
	if dir := piModelInspectionEnvFn(piCodingAgentDirEnv); dir != "" {
		return dir, nil
	}
	home, err := piModelInspectionHomeDirFn()
	if err != nil {
		return "", fmt.Errorf("resolve Pi agent directory: %w", err)
	}
	return filepath.Join(home, ".pi", "agent"), nil
}

func loadPiModelInspection(ctx context.Context, cwd, agentDir string) (pi.ModelRoutingInspection, error) {
	candidates, err := piModelInspectionEnumerateFn(cwd, agentDir)
	if err != nil {
		return pi.ModelRoutingInspection{}, err
	}
	candidate, capabilities, err := piModelInspectionSelectFn(ctx, candidates)
	if err != nil {
		return pi.ModelRoutingInspection{}, err
	}
	request := pi.ModelRoutingRequestContext{
		CWD:      cwd,
		AgentDir: agentDir,
		Target:   pi.ModelRoutingTargetProject,
	}
	return piModelInspectionInspectFn(ctx, candidate, capabilities, request)
}

func inspectPiModelRoutingClient(ctx context.Context, candidate pi.ModelRoutingCandidate, capabilities pi.Capabilities, request pi.ModelRoutingRequestContext) (pi.ModelRoutingInspection, error) {
	return pi.NewModelRoutingClient(candidate, capabilities).Inspect(ctx, request)
}

var piModelInspectionCmd = func(ctx context.Context, requestID uint64) tea.Cmd {
	cwd, err := piModelInspectionGetwdFn()
	if err != nil {
		return func() tea.Msg { return piModelInspectionMsg{requestID: requestID, err: err} }
	}
	agentDir, err := piModelInspectionAgentDirFn()
	if err != nil {
		return func() tea.Msg { return piModelInspectionMsg{requestID: requestID, err: err} }
	}

	return func() tea.Msg {
		inspection, err := piModelInspectionLoadFn(ctx, cwd, agentDir)
		return piModelInspectionMsg{requestID: requestID, inspection: inspection, err: err}
	}
}
