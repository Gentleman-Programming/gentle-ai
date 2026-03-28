package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

// osStatModelCache is a package-level variable so tests can override it to
// simulate a missing or present OpenCode model cache file.
var osStatModelCache = os.Stat

// TickMsg drives the spinner animation on the installing screen.
type TickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// StepProgressMsg is sent from the pipeline goroutine when a step changes status.
type StepProgressMsg struct {
	StepID string
	Status pipeline.StepStatus
	Err    error
}

// PipelineDoneMsg is sent when the pipeline finishes execution.
type PipelineDoneMsg struct {
	Result pipeline.ExecutionResult
}

// BackupRestoreMsg is sent when a backup restore completes.
type BackupRestoreMsg struct {
	Err error
}

// UpdateCheckResultMsg is sent when the background update check completes.
type UpdateCheckResultMsg struct {
	Results []update.UpdateResult
}

// UpgradeDoneMsg is sent when the upgrade operation completes.
type UpgradeDoneMsg struct {
	Report upgrade.UpgradeReport
	Err    error
}

// SyncDoneMsg is sent when the sync operation completes.
type SyncDoneMsg struct {
	FilesChanged int
	Err          error
}

// UpgradePhaseCompletedMsg is sent by startUpgradeSync when the upgrade phase
// finishes (before the sync phase begins). This enables the intermediate "sync
// running" state to be displayed.
type UpgradePhaseCompletedMsg struct {
	Report upgrade.UpgradeReport
	Err    error
}

// UpgradeFunc is the signature of the function injected to perform tool upgrades.
type UpgradeFunc func(ctx context.Context, results []update.UpdateResult) upgrade.UpgradeReport

// SyncFunc is the signature of the function injected to perform config sync.
// Returns the number of files changed and any error.
type SyncFunc func() (int, error)

// ExecuteFunc builds and runs the installation pipeline. It receives a ProgressFunc
// callback to emit step-level progress events, and returns the ExecutionResult.
type ExecuteFunc func(
	selection model.Selection,
	resolved planner.ResolvedPlan,
	detection system.DetectionResult,
	onProgress pipeline.ProgressFunc,
) pipeline.ExecutionResult

// RestoreFunc restores a backup from a manifest.
type RestoreFunc func(manifest backup.Manifest) error

// DeleteBackupFunc deletes the entire backup directory.
type DeleteBackupFunc func(manifest backup.Manifest) error

// RenameBackupFunc updates the backup's Description field in its manifest file.
type RenameBackupFunc func(manifest backup.Manifest, newDescription string) error

// ListBackupsFn returns the current list of available backups.
// When nil, the backup list is not refreshed after restore.
type ListBackupsFn func() []backup.Manifest

type Screen int

const (
	ScreenUnknown Screen = iota
	ScreenWelcome
	ScreenDetection
	ScreenAgents
	ScreenPersona
	ScreenPreset
	ScreenClaudeModelPicker
	ScreenSDDMode
	ScreenDependencyTree
	ScreenSkillPicker
	ScreenReview
	ScreenInstalling
	ScreenModelPicker
	ScreenComplete
	ScreenBackups
	ScreenRestoreConfirm
	ScreenRestoreResult
	ScreenDeleteConfirm
	ScreenDeleteResult
	ScreenRenameBackup
	ScreenUpgrade
	ScreenSync
	ScreenUpgradeSync
	ScreenModelConfig
)

type Model struct {
	Screen         Screen
	PreviousScreen Screen
	Width          int
	Height         int
	Cursor         int
	Version        string
	SpinnerFrame   int

	Selection         model.Selection
	Detection         system.DetectionResult
	DependencyPlan    planner.ResolvedPlan
	Review            planner.ReviewPayload
	Progress          ProgressState
	Execution         pipeline.ExecutionResult
	Backups           []backup.Manifest
	ModelPicker       screens.ModelPickerState
	ClaudeModelPicker screens.ClaudeModelPickerState
	SkillPicker       []model.SkillID
	Err               error

	// SelectedBackup holds the manifest chosen on ScreenBackups, used by the
	// restore confirmation and result screens.
	SelectedBackup backup.Manifest

	// RestoreErr holds the error from the most recent restore attempt.
	// Nil on success, non-nil on failure. Displayed on ScreenRestoreResult.
	RestoreErr error

	// DeleteErr holds the error from the most recent delete attempt.
	// Nil on success, non-nil on failure. Displayed on ScreenDeleteResult.
	DeleteErr error

	// BackupScroll is the scroll offset for the backup list.
	BackupScroll int

	// BackupRenameText is the text input buffer for rename operations.
	BackupRenameText string

	// BackupRenamePos is the cursor position within BackupRenameText.
	BackupRenamePos int

	// ExecuteFn is called to run the real pipeline. When nil, the installing
	// screen falls back to manual step-through (useful for tests/development).
	ExecuteFn ExecuteFunc

	// RestoreFn is called to restore a backup. When nil, restore is a no-op.
	RestoreFn RestoreFunc

	// DeleteBackupFn is called to delete a backup directory.
	DeleteBackupFn DeleteBackupFunc

	// RenameBackupFn is called to rename (update description of) a backup.
	RenameBackupFn RenameBackupFunc

	// ListBackupsFn refreshes the backup list (e.g. after a restore).
	// When nil, the backup list is not refreshed automatically.
	ListBackupsFn ListBackupsFn

	// UpdateResults holds the results of the background update check.
	UpdateResults []update.UpdateResult

	// UpdateCheckDone is true once the background update check has completed.
	UpdateCheckDone bool

	// pipelineRunning tracks whether the pipeline goroutine is active.
	pipelineRunning bool

	// TUI operations — set by startUpgrade / startSync / startUpgradeSync goroutines.

	// UpgradeReport holds the result of the last upgrade run.
	// nil means the upgrade has not been run yet or is currently running.
	UpgradeReport *upgrade.UpgradeReport

	// SyncFilesChanged holds the number of files changed during the last sync run.
	SyncFilesChanged int

	// SyncErr holds the error from the last sync run (nil on success).
	SyncErr error

	// UpgradeFn is injected at construction time and called to perform upgrades.
	UpgradeFn UpgradeFunc

	// SyncFn is injected at construction time and called to perform config sync.
	SyncFn SyncFunc

	// ModelConfigMode is true when the model pickers were reached via the
	// Model Config shortcut, so they return to ScreenWelcome instead of
	// continuing the install flow.
	ModelConfigMode bool

	// OperationRunning is true while an upgrade/sync/upgrade-sync goroutine is
	// executing. Prevents concurrent operation launches.
	OperationRunning bool

	// OperationMode records which operation is running or was last run.
	// Values: "upgrade", "sync", "upgrade-sync".
	OperationMode string

	// HasSyncRun is true once a sync or upgrade-sync operation has completed.
	// It distinguishes "sync hasn't run yet" (false) from "sync ran with 0 changes" (true, filesChanged=0).
	HasSyncRun bool

	// UpgradeErr holds the error from the last upgrade run (nil on success).
	UpgradeErr error
}

func NewModel(detection system.DetectionResult, version string) Model {
	selection := model.Selection{
		Agents:     preselectedAgents(detection),
		Persona:    model.PersonaGentleman,
		Preset:     model.PresetFullGentleman,
		Components: componentsForPreset(model.PresetFullGentleman),
	}

	return Model{
		Screen:    ScreenWelcome,
		Version:   version,
		Selection: selection,
		Detection: detection,
		Progress: NewProgressState([]string{
			"Install dependencies",
			"Configure selected agents",
			"Inject ecosystem components",
		}),
	}
}

func (m Model) Init() tea.Cmd {
	version := m.Version
	profile := m.Detection.System.Profile

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		results := update.CheckAll(ctx, version, profile)
		return UpdateCheckResultMsg{Results: results}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil
	case TickMsg:
		if m.Screen == ScreenInstalling && !m.Progress.Done() {
			m.SpinnerFrame = (m.SpinnerFrame + 1) % 10
			return m, tickCmd()
		}
		// Keep spinner running for operation screens.
		if m.OperationRunning || (m.Screen == ScreenUpgrade && !m.UpdateCheckDone) ||
			(m.Screen == ScreenUpgradeSync && !m.UpdateCheckDone) {
			m.SpinnerFrame = (m.SpinnerFrame + 1) % 10
			return m, tickCmd()
		}
		return m, nil
	case StepProgressMsg:
		return m.handleStepProgress(msg)
	case PipelineDoneMsg:
		return m.handlePipelineDone(msg)
	case BackupRestoreMsg:
		return m.handleBackupRestore(msg)
	case UpdateCheckResultMsg:
		m.UpdateResults = msg.Results
		m.UpdateCheckDone = true
		return m, nil
	case UpgradeDoneMsg:
		m.OperationRunning = false
		m.UpgradeErr = msg.Err
		if msg.Err == nil {
			report := msg.Report
			m.UpgradeReport = &report
		}
		return m, nil
	case SyncDoneMsg:
		m.OperationRunning = false
		m.SyncFilesChanged = msg.FilesChanged
		m.SyncErr = msg.Err
		m.HasSyncRun = true
		return m, nil
	case UpgradePhaseCompletedMsg:
		// Upgrade phase done; sync phase is about to start (OperationRunning stays true).
		m.UpgradeErr = msg.Err
		if msg.Err == nil {
			report := msg.Report
			m.UpgradeReport = &report
		}
		return m, nil
	case tea.KeyMsg:
		if m.Screen == ScreenRenameBackup {
			return m.handleRenameInput(msg)
		}
		return m.handleKeyPress(msg)
	}

	return m, nil
}

func (m Model) handleStepProgress(msg StepProgressMsg) (tea.Model, tea.Cmd) {
	if m.Screen != ScreenInstalling {
		return m, nil
	}

	idx := m.findProgressItem(msg.StepID)
	if idx < 0 {
		return m, nil
	}

	switch msg.Status {
	case pipeline.StepStatusRunning:
		m.Progress.Start(idx)
		m.Progress.AppendLog("running: %s", msg.StepID)
	case pipeline.StepStatusSucceeded:
		m.Progress.Mark(idx, string(pipeline.StepStatusSucceeded))
		m.Progress.AppendLog("done: %s", msg.StepID)
	case pipeline.StepStatusFailed:
		m.Progress.Mark(idx, string(pipeline.StepStatusFailed))
		errMsg := "unknown error"
		if msg.Err != nil {
			errMsg = msg.Err.Error()
		}
		m.Progress.AppendLog("FAILED: %s — %s", msg.StepID, errMsg)
	}

	return m, nil
}

func (m Model) handlePipelineDone(msg PipelineDoneMsg) (tea.Model, tea.Cmd) {
	m.Execution = msg.Result
	m.pipelineRunning = false

	// Rebuild progress from real step results so failed steps show ✗ instead
	// of being blindly marked as succeeded.
	m.Progress = ProgressFromExecution(msg.Result)

	// Surface individual error messages so the user knows WHAT failed.
	appendStepErrors := func(steps []pipeline.StepResult) {
		for _, step := range steps {
			if step.Status == pipeline.StepStatusFailed && step.Err != nil {
				m.Progress.AppendLog("FAILED: %s — %s", step.StepID, step.Err.Error())
			}
		}
	}
	appendStepErrors(msg.Result.Prepare.Steps)
	appendStepErrors(msg.Result.Apply.Steps)

	if msg.Result.Err != nil {
		m.Progress.AppendLog("pipeline completed with errors")
	} else {
		m.Progress.AppendLog("pipeline completed successfully")
	}

	return m, nil
}

func (m Model) handleBackupRestore(msg BackupRestoreMsg) (tea.Model, tea.Cmd) {
	m.RestoreErr = msg.Err
	// Navigate to the result screen regardless of success or failure.
	// The result screen shows success or the error message.
	m.setScreen(ScreenRestoreResult)
	return m, nil
}

func (m Model) findProgressItem(stepID string) int {
	for i, item := range m.Progress.Items {
		if item.Label == stepID {
			return i
		}
	}
	return -1
}

func (m Model) View() string {
	switch m.Screen {
	case ScreenWelcome:
		var banner string
		if m.UpdateCheckDone && update.HasUpdates(m.UpdateResults) {
			banner = "Updates available: " + update.UpdateSummaryLine(m.UpdateResults)
		}
		return screens.RenderWelcome(m.Cursor, m.Version, banner, m.UpdateResults, m.UpdateCheckDone)
	case ScreenUpgrade:
		return screens.RenderUpgrade(m.UpdateResults, m.UpgradeReport, m.UpgradeErr, m.OperationRunning, m.UpdateCheckDone, m.Cursor, m.SpinnerFrame)
	case ScreenSync:
		return screens.RenderSync(m.SyncFilesChanged, m.SyncErr, m.OperationRunning, m.HasSyncRun, m.SpinnerFrame)
	case ScreenModelConfig:
		return screens.RenderModelConfig(m.Cursor)
	case ScreenUpgradeSync:
		return screens.RenderUpgradeSync(m.UpdateResults, m.UpgradeReport, m.SyncFilesChanged, m.UpgradeErr, m.SyncErr, m.OperationRunning, m.UpdateCheckDone, m.Cursor, m.SpinnerFrame)
	case ScreenDetection:
		return screens.RenderDetection(m.Detection, m.Cursor)
	case ScreenAgents:
		return screens.RenderAgents(m.Selection.Agents, m.Cursor)
	case ScreenPersona:
		return screens.RenderPersona(m.Selection.Persona, m.Cursor)
	case ScreenPreset:
		return screens.RenderPreset(m.Selection.Preset, m.Cursor)
	case ScreenClaudeModelPicker:
		return screens.RenderClaudeModelPicker(m.ClaudeModelPicker, m.Cursor)
	case ScreenSDDMode:
		return screens.RenderSDDMode(m.Selection.SDDMode, m.Cursor)
	case ScreenModelPicker:
		return screens.RenderModelPicker(m.Selection.ModelAssignments, m.ModelPicker, m.Cursor)
	case ScreenDependencyTree:
		return screens.RenderDependencyTree(m.DependencyPlan, m.Selection, m.Cursor)
	case ScreenSkillPicker:
		return screens.RenderSkillPicker(m.SkillPicker, m.Cursor)
	case ScreenReview:
		return screens.RenderReview(m.Review, m.Cursor)
	case ScreenInstalling:
		return screens.RenderInstalling(m.Progress.ViewModel(), screens.SpinnerChar(m.SpinnerFrame))
	case ScreenComplete:
		return screens.RenderComplete(screens.CompletePayload{
			ConfiguredAgents:    len(m.Selection.Agents),
			InstalledComponents: len(m.Selection.Components),
			GGAInstalled:        hasSelectedComponent(m.Selection.Components, model.ComponentGGA),
			FailedSteps:         extractFailedSteps(m.Execution),
			RollbackPerformed:   len(m.Execution.Rollback.Steps) > 0,
			MissingDeps:         extractMissingDeps(m.Detection),
			AvailableUpdates:    extractAvailableUpdates(m.UpdateResults),
		})
	case ScreenBackups:
		return screens.RenderBackups(m.Backups, m.Cursor, m.BackupScroll)
	case ScreenRestoreConfirm:
		return screens.RenderRestoreConfirm(m.SelectedBackup, m.Cursor)
	case ScreenRestoreResult:
		return screens.RenderRestoreResult(m.SelectedBackup, m.RestoreErr)
	case ScreenDeleteConfirm:
		return screens.RenderDeleteConfirm(m.SelectedBackup, m.Cursor)
	case ScreenDeleteResult:
		return screens.RenderDeleteResult(m.SelectedBackup, m.DeleteErr)
	case ScreenRenameBackup:
		return screens.RenderRenameBackup(m.SelectedBackup, m.BackupRenameText, m.BackupRenamePos)
	default:
		return ""
	}
}

func (m Model) handleKeyPress(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := key.String()

	// When the model picker is in a sub-mode, delegate navigation there first.
	if m.Screen == ScreenModelPicker && m.ModelPicker.Mode != screens.ModePhaseList {
		handled, updated := screens.HandleModelPickerNav(keyStr, &m.ModelPicker, m.Selection.ModelAssignments)
		if handled {
			m.Selection.ModelAssignments = updated
			return m, nil
		}
	}

	if m.Screen == ScreenClaudeModelPicker {
		handled, updated := screens.HandleClaudeModelPickerNav(keyStr, &m.ClaudeModelPicker, m.Cursor)
		if handled {
			if updated != nil {
				m.Selection.ClaudeModelAssignments = updated
				// In ModelConfigMode, return to ScreenModelConfig instead of continuing install flow.
				if m.ModelConfigMode {
					m.ModelConfigMode = false
					m.setScreen(ScreenModelConfig)
				} else if m.shouldShowSDDModeScreen() {
					m.setScreen(ScreenSDDMode)
				} else if m.Selection.Preset == model.PresetCustom {
					// Custom preset: dependency plan was already built before model picker.
					// Check skill picker before going to review.
					if m.shouldShowSkillPickerScreen() {
						if len(m.SkillPicker) == 0 {
							m.initSkillPicker()
						}
						m.setScreen(ScreenSkillPicker)
					} else {
						m.Review = planner.BuildReviewPayload(m.Selection, m.DependencyPlan)
						m.setScreen(ScreenReview)
					}
				} else {
					m.buildDependencyPlan()
					m.setScreen(ScreenDependencyTree)
				}
			}
			return m, nil
		}
	}

	switch keyStr {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
		// Adjust scroll for the backup list.
		if m.Screen == ScreenBackups {
			if m.Cursor < m.BackupScroll {
				m.BackupScroll = m.Cursor
			}
		}
		return m, nil
	case "down", "j":
		if m.Cursor+1 < m.optionCount() {
			m.Cursor++
		}
		// Adjust scroll for the backup list.
		if m.Screen == ScreenBackups {
			if m.Cursor >= m.BackupScroll+screens.BackupMaxVisible {
				m.BackupScroll = m.Cursor - screens.BackupMaxVisible + 1
			}
		}
		return m, nil
	case "esc":
		// Don't allow going back while pipeline is running.
		if m.Screen == ScreenInstalling && m.pipelineRunning {
			return m, nil
		}
		return m.goBack(), nil
	case " ":
		switch m.Screen {
		case ScreenAgents:
			m.toggleCurrentAgent()
		case ScreenDependencyTree:
			if m.Selection.Preset == model.PresetCustom {
				m.toggleCurrentComponent()
			}
		case ScreenSkillPicker:
			m.toggleCurrentSkill()
		}
		return m, nil
	case "r":
		// Rename: only when on ScreenBackups and cursor is on a backup item (not "Back").
		if m.Screen == ScreenBackups && m.Cursor < len(m.Backups) {
			m.SelectedBackup = m.Backups[m.Cursor]
			m.BackupRenameText = m.SelectedBackup.Description
			m.BackupRenamePos = len([]rune(m.SelectedBackup.Description))
			m.setScreen(ScreenRenameBackup)
			return m, nil
		}
	case "d":
		// Delete: only when on ScreenBackups and cursor is on a backup item (not "Back").
		if m.Screen == ScreenBackups && m.Cursor < len(m.Backups) {
			m.SelectedBackup = m.Backups[m.Cursor]
			m.setScreen(ScreenDeleteConfirm)
			return m, nil
		}
	case "enter":
		return m.confirmSelection()
	}

	return m, nil
}

func (m Model) confirmSelection() (tea.Model, tea.Cmd) {
	switch m.Screen {
	case ScreenWelcome:
		switch m.Cursor {
		case 0:
			m.setScreen(ScreenDetection)
		case 1:
			m = m.withResetOperationState()
			m.setScreen(ScreenUpgrade)
			// Start spinner for update check waiting state.
			if !m.UpdateCheckDone {
				return m, tickCmd()
			}
		case 2:
			m = m.withResetOperationState()
			m.setScreen(ScreenSync)
		case 3:
			m = m.withResetOperationState()
			m.setScreen(ScreenUpgradeSync)
			// Start spinner for update check waiting state.
			if !m.UpdateCheckDone {
				return m, tickCmd()
			}
		case 4:
			m.setScreen(ScreenModelConfig)
		case 5:
			m.setScreen(ScreenBackups)
		case 6:
			return m, tea.Quit
		}
	case ScreenUpgrade:
		// Guard: don't re-launch while running.
		if m.OperationRunning {
			return m, nil
		}
		// If showing results (UpgradeReport != nil or UpgradeErr != nil), return to welcome.
		if m.UpgradeReport != nil || m.UpgradeErr != nil {
			m = m.withResetOperationState()
			m.setScreen(ScreenWelcome)
			return m, nil
		}
		// If update check is not done yet, no-op.
		if !m.UpdateCheckDone {
			return m, nil
		}
		// If no updates available, just return to welcome.
		if !update.HasUpdates(m.UpdateResults) {
			m.setScreen(ScreenWelcome)
			return m, nil
		}
		// Start upgrade.
		m.OperationRunning = true
		m.OperationMode = "upgrade"
		return m, tea.Batch(tickCmd(), m.startUpgrade())
	case ScreenSync:
		// Guard: don't re-launch while running.
		if m.OperationRunning {
			return m, nil
		}
		// If sync already ran, return to welcome.
		if m.HasSyncRun {
			m = m.withResetOperationState()
			m.setScreen(ScreenWelcome)
			return m, nil
		}
		// Start sync.
		m.OperationRunning = true
		m.OperationMode = "sync"
		return m, tea.Batch(tickCmd(), m.startSync())
	case ScreenUpgradeSync:
		// Guard: don't re-launch while running.
		if m.OperationRunning {
			return m, nil
		}
		// If operations are done, return to welcome.
		if m.HasSyncRun || m.UpgradeReport != nil || m.UpgradeErr != nil {
			m = m.withResetOperationState()
			m.setScreen(ScreenWelcome)
			return m, nil
		}
		// Start upgrade+sync.
		m.OperationRunning = true
		m.OperationMode = "upgrade-sync"
		return m, tea.Batch(tickCmd(), m.startUpgradeSync())
	case ScreenModelConfig:
		switch m.Cursor {
		case 0: // Configure Claude models
			m.ModelConfigMode = true
			m.ClaudeModelPicker = screens.NewClaudeModelPickerState()
			m.setScreen(ScreenClaudeModelPicker)
		case 1: // Configure OpenCode models
			m.ModelConfigMode = true
			cachePath := opencode.DefaultCachePath()
			if _, err := osStatModelCache(cachePath); err == nil {
				m.ModelPicker = screens.NewModelPickerState(cachePath)
			} else {
				m.ModelPicker = screens.ModelPickerState{}
			}
			m.setScreen(ScreenModelPicker)
		default: // Back
			m.setScreen(ScreenWelcome)
		}
		return m, nil
	case ScreenDetection:
		if m.Cursor == 0 {
			m.setScreen(ScreenAgents)
			return m, nil
		}
		m.setScreen(ScreenWelcome)
	case ScreenAgents:
		agentCount := len(screens.AgentOptions())
		switch {
		case m.Cursor < agentCount:
			m.toggleCurrentAgent()
		case m.Cursor == agentCount && len(m.Selection.Agents) > 0:
			m.setScreen(ScreenPersona)
		case m.Cursor == agentCount+1:
			m.setScreen(ScreenDetection)
		}
	case ScreenPersona:
		options := screens.PersonaOptions()
		if m.Cursor < len(options) {
			m.Selection.Persona = options[m.Cursor]
			m.setScreen(ScreenPreset)
			return m, nil
		}
		m.setScreen(ScreenAgents)
	case ScreenPreset:
		options := screens.PresetOptions()
		if m.Cursor < len(options) {
			m.Selection.Preset = options[m.Cursor]
			m.Selection.Components = componentsForPreset(options[m.Cursor])
			if m.shouldShowClaudeModelPickerScreen() {
				m.ClaudeModelPicker = screens.NewClaudeModelPickerState()
				m.setScreen(ScreenClaudeModelPicker)
				return m, nil
			}
			if m.shouldShowSDDModeScreen() {
				m.setScreen(ScreenSDDMode)
				return m, nil
			}
			m.buildDependencyPlan()
			m.setScreen(ScreenDependencyTree)
			return m, nil
		}
		m.setScreen(ScreenPersona)
	case ScreenClaudeModelPicker:
		if !m.ClaudeModelPicker.InCustomMode && m.Cursor == screens.ClaudeModelPickerOptionCount(m.ClaudeModelPicker)-1 {
			if m.Selection.Preset == model.PresetCustom {
				m.setScreen(ScreenDependencyTree)
			} else {
				m.setScreen(ScreenPreset)
			}
			return m, nil
		}
	case ScreenSDDMode:
		options := screens.SDDModeOptions()
		if m.Cursor < len(options) {
			m.Selection.SDDMode = options[m.Cursor]
			if m.Selection.SDDMode == model.SDDModeMulti {
				cachePath := opencode.DefaultCachePath()
				if _, err := osStatModelCache(cachePath); err == nil {
					// Cache exists — OpenCode has been run at least once.
					// Show the model picker so the user can assign models.
					m.ModelPicker = screens.NewModelPickerState(cachePath)
					m.Selection.ModelAssignments = nil
					m.setScreen(ScreenModelPicker)
					return m, nil
				}
				// Cache missing — OpenCode hasn't been run yet on this machine.
				// Skip the model picker; models will use OpenCode defaults.
				// The picker empty-state message explains what to do after install.
				m.ModelPicker = screens.ModelPickerState{}
			}
			// Clear assignments for both single mode and multi-no-cache paths.
			m.Selection.ModelAssignments = nil
			if m.Selection.Preset == model.PresetCustom {
				// Custom preset: dependency plan was already built before SDD mode.
				// Check skill picker before going to review.
				if m.shouldShowSkillPickerScreen() {
					if len(m.SkillPicker) == 0 {
						m.initSkillPicker()
					}
					m.setScreen(ScreenSkillPicker)
				} else {
					m.Review = planner.BuildReviewPayload(m.Selection, m.DependencyPlan)
					m.setScreen(ScreenReview)
				}
			} else {
				m.buildDependencyPlan()
				m.setScreen(ScreenDependencyTree)
			}
			return m, nil
		}
		// Back — in custom preset, return to ClaudeModelPicker if applicable,
		// otherwise DependencyTree (component selector).
		// NOTE: SDDMode back logic is also in goBack() — keep in sync.
		if m.Selection.Preset == model.PresetCustom {
			if m.shouldShowClaudeModelPickerScreen() {
				m.setScreen(ScreenClaudeModelPicker)
			} else {
				m.setScreen(ScreenDependencyTree)
			}
		} else {
			// NOTE: Back logic also in goBack() — keep in sync.
			if m.shouldShowClaudeModelPickerScreen() {
				m.setScreen(ScreenClaudeModelPicker)
			} else {
				m.setScreen(ScreenPreset)
			}
		}
	case ScreenModelPicker:
		// When no providers are detected the screen only shows a "Back" option
		// at cursor 0.  Handle that before the normal row logic.
		if len(m.ModelPicker.AvailableIDs) == 0 {
			if m.ModelConfigMode {
				m.ModelConfigMode = false
				m.setScreen(ScreenModelConfig)
				return m, nil
			}
			// Go back to SDD mode so the user can switch to single mode.
			m.setScreen(ScreenSDDMode)
			return m, nil
		}
		rows := screens.ModelPickerRows()
		if m.Cursor < len(rows) {
			// Enter sub-selection: pick provider then model.
			m.ModelPicker.SelectedPhaseIdx = m.Cursor
			m.ModelPicker.Mode = screens.ModeProviderSelect
			m.ModelPicker.ProviderCursor = 0
			m.ModelPicker.ProviderScroll = 0
			return m, nil
		}
		// After the rows: Continue (cursor == len(rows)), Back (cursor == len(rows)+1).
		if m.Cursor == len(rows) {
			// In ModelConfigMode, return to ScreenModelConfig instead of continuing install flow.
			if m.ModelConfigMode {
				m.ModelConfigMode = false
				m.setScreen(ScreenModelConfig)
				return m, nil
			}
			if m.Selection.Preset == model.PresetCustom {
				// Custom preset: dependency plan was already built before SDD mode.
				// Check skill picker before going to review.
				if m.shouldShowSkillPickerScreen() {
					if len(m.SkillPicker) == 0 {
						m.initSkillPicker()
					}
					m.setScreen(ScreenSkillPicker)
				} else {
					m.Review = planner.BuildReviewPayload(m.Selection, m.DependencyPlan)
					m.setScreen(ScreenReview)
				}
			} else {
				// Continue -> proceed to dependency tree.
				m.buildDependencyPlan()
				m.setScreen(ScreenDependencyTree)
			}
			return m, nil
		}
		// Back -> return to SDD mode screen (or ModelConfig in shortcut mode).
		if m.ModelConfigMode {
			m.ModelConfigMode = false
			m.setScreen(ScreenModelConfig)
			return m, nil
		}
		m.setScreen(ScreenSDDMode)
	case ScreenDependencyTree:
		if m.Selection.Preset == model.PresetCustom {
			allComps := screens.AllComponents()
			switch {
			case m.Cursor < len(allComps):
				m.toggleCurrentComponent()
			case m.Cursor == len(allComps):
				m.buildDependencyPlan()
				// Show model picker screens if needed (components are now set).
				if m.shouldShowClaudeModelPickerScreen() {
					m.ClaudeModelPicker = screens.NewClaudeModelPickerState()
					m.setScreen(ScreenClaudeModelPicker)
					return m, nil
				}
				if m.shouldShowSDDModeScreen() {
					m.setScreen(ScreenSDDMode)
					return m, nil
				}
				// Show skill picker if Skills component is selected.
				if m.shouldShowSkillPickerScreen() {
					if len(m.SkillPicker) == 0 {
						m.initSkillPicker()
					}
					m.setScreen(ScreenSkillPicker)
					return m, nil
				}
				m.Review = planner.BuildReviewPayload(m.Selection, m.DependencyPlan)
				m.setScreen(ScreenReview)
			default:
				m.setScreen(ScreenPreset)
			}
			return m, nil
		}
		if m.Cursor == 0 {
			m.Review = planner.BuildReviewPayload(m.Selection, m.DependencyPlan)
			m.setScreen(ScreenReview)
			return m, nil
		}
		// NOTE: Back logic also in goBack() — keep in sync.
		if m.shouldShowSDDModeScreen() {
			if m.Selection.SDDMode == model.SDDModeMulti {
				cachePath := opencode.DefaultCachePath()
				if _, err := osStatModelCache(cachePath); err == nil {
					m.setScreen(ScreenModelPicker)
				} else {
					m.setScreen(ScreenSDDMode)
				}
			} else {
				m.setScreen(ScreenSDDMode)
			}
		} else if m.shouldShowClaudeModelPickerScreen() {
			m.setScreen(ScreenClaudeModelPicker)
		} else {
			m.setScreen(ScreenPreset)
		}
	case ScreenSkillPicker:
		allSkills := screens.AllSkillsOrdered()
		switch {
		case m.Cursor < len(allSkills):
			m.toggleCurrentSkill()
		case m.Cursor == len(allSkills):
			// "Continue" — store selected skills into Selection and proceed to review.
			m.Selection.Skills = make([]model.SkillID, len(m.SkillPicker))
			copy(m.Selection.Skills, m.SkillPicker)
			m.Review = planner.BuildReviewPayload(m.Selection, m.DependencyPlan)
			m.setScreen(ScreenReview)
		default:
			// "Back" — in custom preset, return to the screen that preceded SkillPicker.
			if m.Selection.Preset == model.PresetCustom {
				if m.shouldShowSDDModeScreen() {
					if m.Selection.SDDMode == model.SDDModeMulti {
						cachePath := opencode.DefaultCachePath()
						if _, err := osStatModelCache(cachePath); err == nil {
							m.setScreen(ScreenModelPicker)
						} else {
							m.setScreen(ScreenSDDMode)
						}
					} else {
						m.setScreen(ScreenSDDMode)
					}
				} else if m.shouldShowClaudeModelPickerScreen() {
					m.setScreen(ScreenClaudeModelPicker)
				} else {
					m.setScreen(ScreenDependencyTree)
				}
			} else {
				m.setScreen(ScreenDependencyTree)
			}
		}
	case ScreenReview:
		if m.Cursor == 0 {
			return m.startInstalling()
		}
		// Back — in custom preset, walk back through the screens that were shown.
		if m.Selection.Preset == model.PresetCustom {
			if m.shouldShowSkillPickerScreen() {
				if len(m.SkillPicker) == 0 {
					m.initSkillPicker()
				}
				m.setScreen(ScreenSkillPicker)
			} else if m.shouldShowSDDModeScreen() {
				if m.Selection.SDDMode == model.SDDModeMulti {
					cachePath := opencode.DefaultCachePath()
					if _, err := osStatModelCache(cachePath); err == nil {
						m.setScreen(ScreenModelPicker)
					} else {
						m.setScreen(ScreenSDDMode)
					}
				} else {
					m.setScreen(ScreenSDDMode)
				}
			} else if m.shouldShowClaudeModelPickerScreen() {
				m.setScreen(ScreenClaudeModelPicker)
			} else {
				m.setScreen(ScreenDependencyTree)
			}
		} else {
			m.setScreen(ScreenDependencyTree)
		}
	case ScreenInstalling:
		if m.Progress.Done() {
			m.setScreen(ScreenComplete)
			return m, nil
		}
		// If no ExecuteFn, fall back to manual step-through for dev/tests.
		if m.ExecuteFn == nil && !m.pipelineRunning {
			m.Progress.Mark(m.Progress.Current, "succeeded")
			if m.Progress.Done() {
				m.setScreen(ScreenComplete)
			}
		}
	case ScreenComplete:
		return m, tea.Quit
	case ScreenBackups:
		if m.Cursor < len(m.Backups) {
			// Navigate to confirmation screen instead of immediately restoring.
			m.SelectedBackup = m.Backups[m.Cursor]
			m.setScreen(ScreenRestoreConfirm)
			return m, nil
		}
		m.setScreen(ScreenWelcome)
	case ScreenRestoreConfirm:
		// Cursor 0 = "Restore", Cursor 1 = "Cancel".
		if m.Cursor == 0 {
			return m.restoreBackup(m.SelectedBackup)
		}
		m.setScreen(ScreenBackups)
	case ScreenRestoreResult:
		// Enter on the result screen returns to backup selection.
		// Refresh the backup list to reflect any changes from the restore.
		if m.ListBackupsFn != nil {
			m.Backups = m.ListBackupsFn()
		}
		m.setScreen(ScreenBackups)
	case ScreenDeleteConfirm:
		// Cursor 0 = "Delete", Cursor 1 = "Cancel".
		if m.Cursor == 0 {
			if m.DeleteBackupFn != nil {
				m.DeleteErr = m.DeleteBackupFn(m.SelectedBackup)
			}
			m.setScreen(ScreenDeleteResult)
		} else {
			m.setScreen(ScreenBackups)
		}
	case ScreenDeleteResult:
		// Enter on the result screen returns to backup selection.
		// Refresh the backup list to reflect any changes from the delete.
		if m.ListBackupsFn != nil {
			m.Backups = m.ListBackupsFn()
		}
		m.DeleteErr = nil
		m.setScreen(ScreenBackups)
	}

	return m, nil
}

// startInstalling initializes the progress state from the resolved plan and
// starts the pipeline execution in a goroutine if ExecuteFn is provided.
func (m Model) startInstalling() (tea.Model, tea.Cmd) {
	m.setScreen(ScreenInstalling)
	m.SpinnerFrame = 0

	// Build progress labels from the resolved plan.
	labels := buildProgressLabels(m.DependencyPlan)
	if len(labels) == 0 {
		// Fallback labels when the plan is empty (dev/test).
		labels = []string{
			"Install dependencies",
			"Configure selected agents",
			"Inject ecosystem components",
		}
	}

	m.Progress = NewProgressState(labels)
	m.Progress.Start(0)
	m.Progress.AppendLog("starting installation")

	if m.ExecuteFn == nil {
		// No real executor; fall back to manual step-through.
		return m, tickCmd()
	}

	m.pipelineRunning = true

	// Capture values for the goroutine closure.
	executeFn := m.ExecuteFn
	selection := m.Selection
	resolved := m.DependencyPlan
	detection := m.Detection

	return m, tea.Batch(tickCmd(), func() tea.Msg {
		onProgress := func(event pipeline.ProgressEvent) {
			// NOTE: ProgressFunc is called synchronously from the pipeline goroutine.
			// We cannot use p.Send() here because we don't have a reference to the
			// tea.Program. Instead, these events are collected in the ExecutionResult
			// and the PipelineDoneMsg handles the final state. For real-time updates,
			// we rely on the pipeline calling this synchronously from each step.
		}

		result := executeFn(selection, resolved, detection, onProgress)
		return PipelineDoneMsg{Result: result}
	})
}

// withResetOperationState clears all operation-related state and resets the cursor,
// returning a new Model with these fields cleared (value-receiver pattern for MVU).
func (m Model) withResetOperationState() Model {
	m.UpgradeReport = nil
	m.UpgradeErr = nil
	m.SyncFilesChanged = 0
	m.SyncErr = nil
	m.HasSyncRun = false
	m.OperationRunning = false
	m.OperationMode = ""
	m.Cursor = 0
	return m
}

// startUpgrade launches the upgrade goroutine and returns a tea.Cmd.
func (m Model) startUpgrade() tea.Cmd {
	upgradeFn := m.UpgradeFn
	updateResults := m.UpdateResults
	return func() tea.Msg {
		if upgradeFn == nil {
			return UpgradeDoneMsg{Err: fmt.Errorf("upgrade function not configured")}
		}
		ctx := context.Background()
		report := upgradeFn(ctx, updateResults)
		return UpgradeDoneMsg{Report: report}
	}
}

// startSync launches the sync goroutine and returns a tea.Cmd.
func (m Model) startSync() tea.Cmd {
	syncFn := m.SyncFn
	return func() tea.Msg {
		if syncFn == nil {
			return SyncDoneMsg{Err: fmt.Errorf("sync function not configured")}
		}
		filesChanged, err := syncFn()
		return SyncDoneMsg{FilesChanged: filesChanged, Err: err}
	}
}

// startUpgradeSync runs upgrade then sync sequentially via tea.Sequence.
// Design decision: sync runs unconditionally regardless of upgrade outcome.
// Tool-level upgrade failures are per-tool (in UpgradeReport.Results), not fatal.
// The user sees both results and can re-run if needed.
//
// The first command runs the upgrade and sends UpgradePhaseCompletedMsg
// (so the UI can show State 2: sync running). The second command runs sync
// and sends SyncDoneMsg.
func (m Model) startUpgradeSync() tea.Cmd {
	upgradeFn := m.UpgradeFn
	syncFn := m.SyncFn
	updateResults := m.UpdateResults

	upgradeCmd := func() tea.Msg {
		if upgradeFn == nil {
			return UpgradePhaseCompletedMsg{Err: fmt.Errorf("upgrade function not configured")}
		}
		ctx := context.Background()
		report := upgradeFn(ctx, updateResults)
		return UpgradePhaseCompletedMsg{Report: report}
	}

	syncCmd := func() tea.Msg {
		if syncFn == nil {
			return SyncDoneMsg{Err: fmt.Errorf("sync function not configured")}
		}
		filesChanged, err := syncFn()
		return SyncDoneMsg{FilesChanged: filesChanged, Err: err}
	}

	return tea.Sequence(upgradeCmd, syncCmd)
}

// restoreBackup triggers a backup restore in a goroutine.
func (m Model) restoreBackup(manifest backup.Manifest) (tea.Model, tea.Cmd) {
	if m.RestoreFn == nil {
		m.Err = fmt.Errorf("restore not available")
		return m, nil
	}

	restoreFn := m.RestoreFn
	return m, func() tea.Msg {
		err := restoreFn(manifest)
		return BackupRestoreMsg{Err: err}
	}
}

// buildProgressLabels creates step labels from the resolved plan that match
// the step IDs the pipeline will produce.
func buildProgressLabels(resolved planner.ResolvedPlan) []string {
	labels := make([]string, 0, 2+len(resolved.Agents)+len(resolved.OrderedComponents)+1)

	labels = append(labels, "prepare:check-dependencies")
	labels = append(labels, "prepare:backup-snapshot")
	labels = append(labels, "apply:rollback-restore")

	for _, agent := range resolved.Agents {
		labels = append(labels, "agent:"+string(agent))
	}

	for _, component := range resolved.OrderedComponents {
		labels = append(labels, "component:"+string(component))
	}

	return labels
}

func (m Model) goBack() Model {
	// Block navigation while an operation (upgrade/sync) is running.
	if m.OperationRunning {
		return m
	}

	// ModelConfigMode: pickers reached via Model Config shortcut return to ScreenModelConfig.
	if m.ModelConfigMode && (m.Screen == ScreenClaudeModelPicker || m.Screen == ScreenModelPicker) {
		m.ModelConfigMode = false
		m.setScreen(ScreenModelConfig)
		return m
	}

	// From SkillPicker, go back to the preceding screen.
	// In custom preset: SDDMode/ModelPicker/ClaudeModelPicker precede SkillPicker.
	if m.Screen == ScreenSkillPicker {
		if m.Selection.Preset == model.PresetCustom {
			if m.shouldShowSDDModeScreen() {
				if m.Selection.SDDMode == model.SDDModeMulti {
					cachePath := opencode.DefaultCachePath()
					if _, err := osStatModelCache(cachePath); err == nil {
						m.setScreen(ScreenModelPicker)
					} else {
						m.setScreen(ScreenSDDMode)
					}
				} else {
					m.setScreen(ScreenSDDMode)
				}
			} else if m.shouldShowClaudeModelPickerScreen() {
				m.setScreen(ScreenClaudeModelPicker)
			} else {
				m.setScreen(ScreenDependencyTree)
			}
		} else {
			m.setScreen(ScreenDependencyTree)
		}
		return m
	}

	// If going back from DependencyTree and the SDDMode/ClaudeModelPicker
	// screens were shown BEFORE it (non-custom presets only), navigate to them.
	// In custom mode these screens appear AFTER the dependency tree, so
	// going back should return to the preset screen (handled by linearRoutes).
	// NOTE: DependencyTree back logic also in confirmSelection() — keep in sync.
	if m.Screen == ScreenDependencyTree && m.Selection.Preset != model.PresetCustom {
		if m.shouldShowSDDModeScreen() {
			if m.Selection.SDDMode == model.SDDModeMulti {
				cachePath := opencode.DefaultCachePath()
				if _, err := osStatModelCache(cachePath); err == nil {
					m.setScreen(ScreenModelPicker)
				} else {
					m.setScreen(ScreenSDDMode)
				}
			} else {
				m.setScreen(ScreenSDDMode)
			}
			return m
		}
		if m.shouldShowClaudeModelPickerScreen() {
			m.setScreen(ScreenClaudeModelPicker)
			return m
		}
	}

	// In custom preset, going back from SDDMode should return to ClaudeModelPicker
	// if applicable, otherwise DependencyTree (the component selector).
	// For non-custom, check if ClaudeModelPicker was shown first.
	// NOTE: SDDMode back logic is also in confirmSelection — keep in sync.
	if m.Screen == ScreenSDDMode {
		if m.Selection.Preset == model.PresetCustom {
			if m.shouldShowClaudeModelPickerScreen() {
				m.setScreen(ScreenClaudeModelPicker)
			} else {
				m.setScreen(ScreenDependencyTree)
			}
			return m
		}
		if m.shouldShowClaudeModelPickerScreen() {
			m.setScreen(ScreenClaudeModelPicker)
			return m
		}
	}

	// In custom preset, going back from ClaudeModelPicker should return to DependencyTree.
	if m.Screen == ScreenClaudeModelPicker && m.Selection.Preset == model.PresetCustom {
		m.setScreen(ScreenDependencyTree)
		return m
	}

	// In custom preset, going back from Review walks through intermediate screens.
	if m.Screen == ScreenReview && m.Selection.Preset == model.PresetCustom {
		if m.shouldShowSkillPickerScreen() {
			if len(m.SkillPicker) == 0 {
				m.initSkillPicker()
			}
			m.setScreen(ScreenSkillPicker)
			return m
		}
		if m.shouldShowSDDModeScreen() {
			if m.Selection.SDDMode == model.SDDModeMulti {
				cachePath := opencode.DefaultCachePath()
				if _, err := osStatModelCache(cachePath); err == nil {
					m.setScreen(ScreenModelPicker)
				} else {
					m.setScreen(ScreenSDDMode)
				}
			} else {
				m.setScreen(ScreenSDDMode)
			}
			return m
		}
		if m.shouldShowClaudeModelPickerScreen() {
			m.setScreen(ScreenClaudeModelPicker)
			return m
		}
		m.setScreen(ScreenDependencyTree)
		return m
	}

	previous, ok := PreviousScreen(m.Screen)
	if !ok {
		return m
	}

	m.setScreen(previous)
	return m
}

func (m *Model) setScreen(next Screen) {
	m.PreviousScreen = m.Screen
	m.Screen = next
	m.Cursor = 0
	if next == ScreenBackups {
		m.BackupScroll = 0
	}
}

// handleRenameInput processes key events when the rename backup screen is active.
// It manages text input for the new backup description.
func (m Model) handleRenameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		// Execute rename and return to backups.
		if m.RenameBackupFn != nil {
			_ = m.RenameBackupFn(m.SelectedBackup, m.BackupRenameText)
		}
		if m.ListBackupsFn != nil {
			m.Backups = m.ListBackupsFn()
		}
		m.setScreen(ScreenBackups)
		return m, nil
	case tea.KeyEsc:
		m.setScreen(ScreenBackups)
		return m, nil
	case tea.KeyBackspace:
		if m.BackupRenamePos > 0 {
			runes := []rune(m.BackupRenameText)
			m.BackupRenameText = string(append(runes[:m.BackupRenamePos-1], runes[m.BackupRenamePos:]...))
			m.BackupRenamePos--
		}
		return m, nil
	case tea.KeyLeft:
		if m.BackupRenamePos > 0 {
			m.BackupRenamePos--
		}
		return m, nil
	case tea.KeyRight:
		if m.BackupRenamePos < len([]rune(m.BackupRenameText)) {
			m.BackupRenamePos++
		}
		return m, nil
	case tea.KeyRunes:
		runes := []rune(m.BackupRenameText)
		newRunes := make([]rune, 0, len(runes)+len(msg.Runes))
		newRunes = append(newRunes, runes[:m.BackupRenamePos]...)
		newRunes = append(newRunes, msg.Runes...)
		newRunes = append(newRunes, runes[m.BackupRenamePos:]...)
		m.BackupRenameText = string(newRunes)
		m.BackupRenamePos += len(msg.Runes)
		return m, nil
	}
	return m, nil
}

func (m Model) optionCount() int {
	switch m.Screen {
	case ScreenWelcome:
		return len(screens.WelcomeOptions(m.UpdateResults, m.UpdateCheckDone))
	case ScreenUpgrade:
		if m.UpgradeReport != nil || m.UpgradeErr != nil {
			return 1 // "return" option in results/error state
		}
		if !m.UpdateCheckDone {
			return 0 // no options while checking
		}
		return 1 // "upgrade all" or "return" when up to date
	case ScreenSync:
		return 1
	case ScreenUpgradeSync:
		return 1
	case ScreenModelConfig:
		return len(screens.ModelConfigOptions())
	case ScreenDetection:
		return len(screens.DetectionOptions())
	case ScreenAgents:
		return len(screens.AgentOptions()) + 2
	case ScreenPersona:
		return len(screens.PersonaOptions()) + 1
	case ScreenPreset:
		return len(screens.PresetOptions()) + 1
	case ScreenClaudeModelPicker:
		return screens.ClaudeModelPickerOptionCount(m.ClaudeModelPicker)
	case ScreenSDDMode:
		return len(screens.SDDModeOptions()) + 1
	case ScreenModelPicker:
		if len(m.ModelPicker.AvailableIDs) == 0 {
			return 1 // only "Back to SDD mode"
		}
		return len(screens.ModelPickerRows()) + 2 // rows + Continue + Back
	case ScreenDependencyTree:
		if m.Selection.Preset == model.PresetCustom {
			return len(screens.AllComponents()) + len(screens.DependencyTreeOptions())
		}
		return len(screens.DependencyTreeOptions())
	case ScreenSkillPicker:
		return screens.SkillPickerOptionCount()
	case ScreenReview:
		return len(screens.ReviewOptions())
	case ScreenInstalling:
		return 1
	case ScreenComplete:
		return 1
	case ScreenBackups:
		return len(m.Backups) + 1
	case ScreenRestoreConfirm:
		return 2 // "Restore" + "Cancel"
	case ScreenRestoreResult:
		return 1 // "Done" / continue
	case ScreenDeleteConfirm:
		return 2 // "Delete" + "Cancel"
	case ScreenDeleteResult:
		return 1 // "Done" / continue
	case ScreenRenameBackup:
		return 0 // text input mode — no cursor navigation
	default:
		return 0
	}
}

func (m *Model) toggleCurrentAgent() {
	options := screens.AgentOptions()
	if m.Cursor >= len(options) {
		return
	}

	agent := options[m.Cursor]
	for idx, selected := range m.Selection.Agents {
		if selected == agent {
			m.Selection.Agents = append(m.Selection.Agents[:idx], m.Selection.Agents[idx+1:]...)
			return
		}
	}

	m.Selection.Agents = append(m.Selection.Agents, agent)
}

func (m *Model) toggleCurrentComponent() {
	allComps := screens.AllComponents()
	if m.Cursor >= len(allComps) {
		return
	}

	compID := allComps[m.Cursor].ID
	for idx, selected := range m.Selection.Components {
		if selected == compID {
			m.Selection.Components = append(m.Selection.Components[:idx], m.Selection.Components[idx+1:]...)
			return
		}
	}

	m.Selection.Components = append(m.Selection.Components, compID)
}

func (m *Model) toggleCurrentSkill() {
	allSkills := screens.AllSkillsOrdered()
	if m.Cursor >= len(allSkills) {
		return
	}

	skillID := allSkills[m.Cursor]
	for idx, selected := range m.SkillPicker {
		if selected == skillID {
			m.SkillPicker = append(m.SkillPicker[:idx], m.SkillPicker[idx+1:]...)
			return
		}
	}

	m.SkillPicker = append(m.SkillPicker, skillID)
}

// initSkillPicker pre-selects ALL available skills (custom mode default).
func (m *Model) initSkillPicker() {
	all := screens.AllSkillsOrdered()
	m.SkillPicker = make([]model.SkillID, len(all))
	copy(m.SkillPicker, all)
}

// shouldShowSkillPickerScreen returns true when the custom preset is active
// and the Skills component has been selected.
func (m Model) shouldShowSkillPickerScreen() bool {
	return m.Selection.Preset == model.PresetCustom &&
		hasSelectedComponent(m.Selection.Components, model.ComponentSkills)
}

func (m *Model) buildDependencyPlan() {
	resolved, err := planner.NewResolver(planner.MVPGraph()).Resolve(m.Selection)
	if err != nil {
		m.Err = err
		m.DependencyPlan = planner.ResolvedPlan{}
		return
	}

	m.DependencyPlan = resolved
}

func preselectedAgents(detection system.DetectionResult) []model.AgentID {
	selected := []model.AgentID{}
	for _, state := range detection.Configs {
		if !state.Exists {
			continue
		}

		switch strings.TrimSpace(state.Agent) {
		case string(model.AgentClaudeCode):
			selected = append(selected, model.AgentClaudeCode)
		case string(model.AgentOpenCode):
			selected = append(selected, model.AgentOpenCode)
		case string(model.AgentGeminiCLI):
			selected = append(selected, model.AgentGeminiCLI)
		case string(model.AgentCursor):
			selected = append(selected, model.AgentCursor)
		case string(model.AgentVSCodeCopilot):
			selected = append(selected, model.AgentVSCodeCopilot)
		case string(model.AgentCodex):
			selected = append(selected, model.AgentCodex)
		case string(model.AgentAntigravity):
			selected = append(selected, model.AgentAntigravity)
		case string(model.AgentWindsurf):
			selected = append(selected, model.AgentWindsurf)
		}
	}

	if len(selected) > 0 {
		return selected
	}

	agents := catalog.AllAgents()
	selected = make([]model.AgentID, 0, len(agents))
	for _, agent := range agents {
		selected = append(selected, agent.ID)
	}

	return selected
}

func extractMissingDeps(detection system.DetectionResult) []screens.MissingDep {
	if detection.Dependencies.AllPresent {
		return nil
	}

	var deps []screens.MissingDep
	for _, dep := range detection.Dependencies.Dependencies {
		if !dep.Installed && dep.Required {
			deps = append(deps, screens.MissingDep{Name: dep.Name, InstallHint: dep.InstallHint})
		}
	}
	return deps
}

func extractFailedSteps(result pipeline.ExecutionResult) []screens.FailedStep {
	var failed []screens.FailedStep
	collect := func(steps []pipeline.StepResult) {
		for _, step := range steps {
			if step.Status == pipeline.StepStatusFailed {
				errMsg := "unknown error"
				if step.Err != nil {
					errMsg = step.Err.Error()
				}
				failed = append(failed, screens.FailedStep{ID: step.StepID, Error: errMsg})
			}
		}
	}
	collect(result.Prepare.Steps)
	collect(result.Apply.Steps)
	return failed
}

func extractAvailableUpdates(results []update.UpdateResult) []screens.UpdateInfo {
	var updates []screens.UpdateInfo
	for _, r := range results {
		if r.Status == update.UpdateAvailable {
			updates = append(updates, screens.UpdateInfo{
				Name:             r.Tool.Name,
				InstalledVersion: r.InstalledVersion,
				LatestVersion:    r.LatestVersion,
				UpdateHint:       r.UpdateHint,
			})
		}
	}
	return updates
}

func (m Model) shouldShowSDDModeScreen() bool {
	return m.Selection.HasAgent(model.AgentOpenCode) &&
		hasSelectedComponent(m.Selection.Components, model.ComponentSDD)
}

func (m Model) shouldShowClaudeModelPickerScreen() bool {
	return m.Selection.HasAgent(model.AgentClaudeCode) &&
		hasSelectedComponent(m.Selection.Components, model.ComponentSDD)
}

func componentsForPreset(preset model.PresetID) []model.ComponentID {
	switch preset {
	case model.PresetMinimal:
		return []model.ComponentID{model.ComponentEngram}
	case model.PresetEcosystemOnly:
		return []model.ComponentID{model.ComponentEngram, model.ComponentSDD, model.ComponentSkills, model.ComponentContext7, model.ComponentGGA}
	case model.PresetCustom:
		return nil
	default:
		return []model.ComponentID{
			model.ComponentEngram,
			model.ComponentSDD,
			model.ComponentSkills,
			model.ComponentContext7,
			model.ComponentPersona,
			model.ComponentPermission,
			model.ComponentGGA,
		}
	}
}

func hasSelectedComponent(components []model.ComponentID, target model.ComponentID) bool {
	for _, c := range components {
		if c == target {
			return true
		}
	}
	return false
}
