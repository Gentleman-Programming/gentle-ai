package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

const piCodingAgentDirEnv = "PI_CODING_AGENT_DIR"

type piModelInspectionMsg struct {
	requestID  uint64
	inspection pi.ModelRoutingInspection
	err        error
}

type piModelValidationMsg struct {
	requestID uint64
	result    pi.ModelRoutingValidationResult
	err       error
}
type piModelInspectionEnumerateFunc func(cwd, agentDir string) ([]pi.ModelRoutingCandidate, error)
type piModelInspectionSelectFunc func(context.Context, []pi.ModelRoutingCandidate) (pi.ModelRoutingCandidate, pi.Capabilities, error)
type piModelInspectionInspectFunc func(context.Context, pi.ModelRoutingCandidate, pi.Capabilities, pi.ModelRoutingRequestContext) (pi.ModelRoutingInspection, error)
type piModelValidationFunc func(context.Context, pi.ModelRoutingCandidate, pi.Capabilities, pi.ModelRoutingRequestContext, pi.ModelRoutingDraft) (pi.ModelRoutingValidationResult, error)

var (
	piModelInspectionGetwdFn   = os.Getwd
	piModelInspectionHomeDirFn = os.UserHomeDir
	piModelInspectionEnvFn     = os.Getenv

	piModelInspectionAgentDirFn                                 = resolvePiModelInspectionAgentDir
	piModelInspectionEnumerateFn piModelInspectionEnumerateFunc = pi.EnumerateModelRoutingCandidates
	piModelInspectionSelectFn    piModelInspectionSelectFunc    = pi.SelectCompatibleModelRoutingCandidate
	piModelInspectionInspectFn   piModelInspectionInspectFunc   = inspectPiModelRoutingClient
	piModelInspectionLoadFn                                     = loadPiModelInspection
	piModelValidationFn          piModelValidationFunc          = validatePiModelRoutingClient
	piModelValidationLoadFn                                     = loadPiModelValidation
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

func validatePiModelRoutingClient(ctx context.Context, candidate pi.ModelRoutingCandidate, capabilities pi.Capabilities, request pi.ModelRoutingRequestContext, draft pi.ModelRoutingDraft) (pi.ModelRoutingValidationResult, error) {
	return pi.NewModelRoutingClient(candidate, capabilities).Validate(ctx, request, clonePiModelRoutingDraft(draft))
}
func loadPiModelValidation(ctx context.Context, cwd, agentDir string, target pi.ModelRoutingTarget, draft pi.ModelRoutingDraft) (pi.ModelRoutingValidationResult, error) {
	candidates, err := piModelInspectionEnumerateFn(cwd, agentDir)
	if err != nil {
		return pi.ModelRoutingValidationResult{}, err
	}
	candidate, capabilities, err := piModelInspectionSelectFn(ctx, candidates)
	if err != nil {
		return pi.ModelRoutingValidationResult{}, err
	}
	request := pi.ModelRoutingRequestContext{CWD: cwd, AgentDir: agentDir, Target: target}
	return piModelValidationFn(ctx, candidate, capabilities, request, clonePiModelRoutingDraft(draft))
}
func clonePiModelRoutingDraft(draft pi.ModelRoutingDraft) pi.ModelRoutingDraft {
	if draft == nil {
		return nil
	}
	clone := make(pi.ModelRoutingDraft, len(draft))
	for name, assignment := range draft {
		if assignment.Model != nil {
			model := *assignment.Model
			assignment.Model = &model
		}
		if assignment.Thinking != nil {
			thinking := *assignment.Thinking
			assignment.Thinking = &thinking
		}
		clone[name] = assignment
	}
	return clone
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

var piModelValidationCmd = func(ctx context.Context, requestID uint64, target pi.ModelRoutingTarget, draft pi.ModelRoutingDraft) tea.Cmd {
	cwd, err := piModelInspectionGetwdFn()
	if err != nil {
		return func() tea.Msg { return piModelValidationMsg{requestID: requestID, err: err} }
	}
	agentDir, err := piModelInspectionAgentDirFn()
	if err != nil {
		return func() tea.Msg { return piModelValidationMsg{requestID: requestID, err: err} }
	}
	draft = clonePiModelRoutingDraft(draft)
	return func() tea.Msg {
		result, err := piModelValidationLoadFn(ctx, cwd, agentDir, target, clonePiModelRoutingDraft(draft))
		return piModelValidationMsg{requestID: requestID, result: result, err: err}
	}
}

func (m *Model) beginPiModelInspection() tea.Cmd {
	m.PiModelInspection = screens.NewPiModelInspectionState()
	m.piModelInspectionRequest++
	m.piModelValidationRequest++
	m.setScreen(ScreenPiModelInspection)
	return piModelInspectionCmd(context.Background(), m.piModelInspectionRequest)
}

func (m *Model) beginPiModelValidation() tea.Cmd {
	if !m.PiModelInspection.BeginValidation() {
		return nil
	}
	m.piModelValidationRequest++
	return piModelValidationCmd(context.Background(), m.piModelValidationRequest, m.PiModelInspection.Target, clonePiModelRoutingDraft(m.PiModelInspection.Draft))
}
func (m *Model) leavePiModelInspection() {
	m.piModelInspectionRequest++
	m.piModelValidationRequest++
	m.PiModelInspection = screens.PiModelInspectionState{}
	m.setScreen(ScreenModelConfig)
}

func (m Model) handlePiModelInspectionKey(key string) (tea.Model, tea.Cmd) {
	switch m.PiModelInspection.Mode {
	case screens.PiModelInspectionModeValidating, screens.PiModelInspectionModeValidationResult, screens.PiModelInspectionModeReviewReady:
		if key == "esc" {
			m.piModelValidationRequest++
			m.PiModelInspection.ClearValidation()
		}
		return m, nil
	}
	if m.PiModelInspection.Mode != screens.PiModelInspectionModeAgents {
		switch key {
		case "esc":
			m.PiModelInspection.BackEditor()
		case "up", "k":
			m.PiModelInspection.MoveEditor(-1, m.Height)
		case "down", "j":
			m.PiModelInspection.MoveEditor(1, m.Height)
		case "enter", " ":
			m.PiModelInspection.SelectEditor()
		}
		return m, nil
	}
	switch key {
	case "esc":
		m.leavePiModelInspection()
	case "up", "k":
		m.PiModelInspection.Move(-1, m.Height)
	case "down", "j":
		m.PiModelInspection.Move(1, m.Height)
	case "enter", " ":
		if m.PiModelInspection.BeginEdit() {
			m.piModelValidationRequest++
		}
	case "v":
		return m, m.beginPiModelValidation()
	case "g":
		m.PiModelInspection.SelectTarget(pi.ModelRoutingTargetGlobal)
	case "p":
		m.PiModelInspection.SelectTarget(pi.ModelRoutingTargetProject)
	case "tab":
		if m.PiModelInspection.Target == pi.ModelRoutingTargetGlobal {
			m.PiModelInspection.SelectTarget(pi.ModelRoutingTargetProject)
		} else {
			m.PiModelInspection.SelectTarget(pi.ModelRoutingTargetGlobal)
		}
	}
	return m, nil
}
