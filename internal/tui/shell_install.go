package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

var shellInstallStepLabels = []string{
	"Validate the selection and target",
	"Prepare the private stage and preferences",
	"Install the runtime and pinned suppliers",
	"Apply optional runtimes and the SDK channel",
	"Publish, verify and register the installation",
}

type shellInstallStep struct {
	Label   string
	State   string
	Details []string
}

type shellInstallRun struct {
	steps             []shellInstallStep
	done              bool
	err               error
	spinner           int
	bloom             int
	fallbackAvailable bool
}

type shellInstallTick struct{}

type shellInstallEvent struct {
	progress shellinstaller.UserInstallProgress
	done     bool
	err      error
}

// WithShellInstallRunner lets the owning application inject its validated
// in-process worker. The runner receives the inspected request, never a new
// confirmation.
func (m Model) WithShellInstallRunner(runner func(context.Context, shellinstaller.UserInstallRequest, func(shellinstaller.UserInstallProgress)) error) Model {
	m.shellInstallRunner = runner
	return m
}

func waitShellInstallEvent(events <-chan shellInstallEvent) tea.Cmd {
	return func() tea.Msg {
		select {
		case event, ok := <-events:
			if !ok {
				return shellInstallEvent{done: true, err: errors.New("installation ended without a result")}
			}
			return event
		case <-time.After(120 * time.Millisecond):
			return shellInstallTick{}
		}
	}
}

func (m Model) beginShellInstall() (tea.Model, tea.Cmd) {
	return m.requestShellInstall(m.shellPlanRequest, false)
}

// shellRootRiskGate is injectable for tests; production uses the installer
// package's entry gate.
var shellRootRiskGate = shellinstaller.RootRiskGate

// shellRootRiskPrompt holds the typed root acceptance for plan-driven
// installs: the plan screen confirms without the selection model, so the
// root warning must live here too, never only behind the review screen.
type shellRootRiskPrompt struct {
	request  shellinstaller.UserInstallRequest
	fallback bool
	input    string
}

// requestShellInstall routes every host TUI launch through the root gate:
// an unaccepted root operator sees the typed warning before any worker runs.
func (m Model) requestShellInstall(request shellinstaller.UserInstallRequest, fallback bool) (tea.Model, tea.Cmd) {
	if err := shellRootRiskGate(); err != nil {
		m.shellRootRisk = &shellRootRiskPrompt{request: request, fallback: fallback}
		return m, nil
	}
	return m.launchShellInstall(request, fallback)
}

func (m Model) updateShellRootRisk(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "esc":
		m.shellRootRisk = nil
		return m, nil
	case "enter":
		if strings.TrimSpace(m.shellRootRisk.input) != shellinstaller.RootRiskPhrase {
			m.shellRootRisk.input = ""
			return m, nil
		}
		if err := os.Setenv(shellinstaller.RootRiskEnv, shellinstaller.RootRiskBinding); err != nil {
			m.shellRootRisk = nil
			return m, nil
		}
		prompt := *m.shellRootRisk
		m.shellRootRisk = nil
		return m.launchShellInstall(prompt.request, prompt.fallback)
	case "backspace":
		if input := []rune(m.shellRootRisk.input); len(input) > 0 {
			m.shellRootRisk.input = string(input[:len(input)-1])
		}
		return m, nil
	}
	if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
		m.shellRootRisk.input += key.String()
	}
	return m, nil
}

func (m Model) launchShellInstall(request shellinstaller.UserInstallRequest, fallback bool) (tea.Model, tea.Cmd) {
	if fallback {
		_ = os.Setenv("GENTLE_AI_MAIN_FALLBACK", "1")
	} else {
		_ = os.Unsetenv("GENTLE_AI_MAIN_FALLBACK")
	}
	m.shellInstallSelection = cli.ShellInstallSelection{Request: request, Confirmed: true, Executed: true}
	steps := make([]shellInstallStep, len(shellInstallStepLabels))
	for index, label := range shellInstallStepLabels {
		steps[index] = shellInstallStep{Label: label, State: "pending"}
	}
	steps[0].State = "running"
	events := make(chan shellInstallEvent, 32)
	m.shellInstallRun = &shellInstallRun{steps: steps}
	m.shellInstallEvents = events
	m.shellPlan, m.shellPlanRequest = nil, shellinstaller.UserInstallRequest{}
	runner := m.shellInstallRunner
	if runner == nil {
		runner = func(ctx context.Context, req shellinstaller.UserInstallRequest, report func(shellinstaller.UserInstallProgress)) error {
			return cli.RunShellInstallInline(ctx, cli.ShellInstallSelection{Request: req, Confirmed: true, Executed: true}, report, io.Discard)
		}
	}
	go func() {
		// The inspected request is immutable here; the runner revalidates its
		// token and physical selection before any effect.
		err := runner(context.Background(), request, func(progress shellinstaller.UserInstallProgress) {
			events <- shellInstallEvent{progress: progress}
		})
		events <- shellInstallEvent{done: true, err: err}
		close(events)
	}()
	return m, waitShellInstallEvent(events)
}

type shellUpdateState struct {
	install  shellinstaller.UserShellInstall
	config   shellExperienceConfig
	info     shellinstaller.UserShellUpdate
	err      error
	checking bool
	cursor   int
}

type shellUpdateResult struct {
	info shellinstaller.UserShellUpdate
	err  error
}

func shellUpdateCommand(install shellinstaller.UserShellInstall) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		info, err := shellinstaller.CheckUserShellUpdate(ctx, install)
		return shellUpdateResult{info: info, err: err}
	}
}

type shellUninstallResult struct {
	backup string
	err    error
}

type shellManageState struct {
	install      shellinstaller.UserShellInstall
	config       shellExperienceConfig
	preferences  []screens.ShellManageChange
	components   []screens.ShellManageChange
	cursor       int
	applied      bool
	err          error
	confirm      bool
	uninstalled  bool
	working      bool
	backup       string
	uninstallErr error
	fromConfig   bool
}

func (m Model) beginShellUpdate(config shellExperienceConfig) (tea.Model, tea.Cmd) {
	if len(config.existing) != 1 {
		config.err = errors.New("the managed installation is no longer available")
		m.shellExperience = &config
		return m, nil
	}
	install := config.existing[0]
	info := shellinstaller.UserShellUpdate{Channel: install.Channel}
	m.shellUpdate = &shellUpdateState{install: install, config: config, info: info, checking: true}
	m.shellExperience = nil
	return m, shellUpdateCommand(install)
}

func (m Model) restoreUpdateConfig(update *shellUpdateState) Model {
	m.shellExperience, m.shellUpdate = &update.config, nil
	return m
}

// beginComponentUpdate reconfigures with the SELECTED profile: the installed
// profile would silently drop every component and preference delta.
func (m Model) beginComponentUpdate(update *shellUpdateState) (tea.Model, error) {
	return m.beginShellUpdateInstall(update.install, shellForceGGAOff(update.config.profile), update.info.Scope)
}

// shellUpdateComponentLabels names the components in plans and reuse lines.
var shellUpdateComponentLabels = map[string]string{
	shellinstaller.UserComponentSupervisor: "Gentle Shell supervisor",
	shellinstaller.UserComponentNode:       "Node runtime",
	shellinstaller.UserComponentSDK:        "gentle-pi SDK",
	shellinstaller.UserComponentCodeGraph:  "CodeGraph",
	shellinstaller.UserComponentContext7:   "Context7",
}

// shellUpdatePlanSteps renders the plan for an update. A scoped update names
// exactly the components that will be refreshed and the ones reused from the
// installation; an unscoped update keeps the classic install plan.
func shellUpdatePlanSteps(scope []string) []string {
	if len(scope) == 0 {
		return append([]string(nil), shellInstallStepLabels...)
	}
	requested := map[string]bool{}
	var updating, reusing []string
	for _, name := range scope {
		requested[name] = true
		updating = append(updating, shellUpdateComponentLabels[name])
	}
	for _, name := range []string{shellinstaller.UserComponentNode, shellinstaller.UserComponentSDK, shellinstaller.UserComponentCodeGraph, shellinstaller.UserComponentContext7} {
		if !requested[name] {
			reusing = append(reusing, shellUpdateComponentLabels[name])
		}
	}
	steps := []string{"Validate the selection and target"}
	if len(reusing) > 0 {
		steps = append(steps, "Reuse unchanged: "+strings.Join(reusing, ", "))
	}
	return append(steps, "Update: "+strings.Join(updating, ", "), "Publish, verify and register the installation")
}

// beginShellUpdateInstall prepares the shared plan/confirmation flow for a
// component update of the given profile. The scope carries the components to
// refresh; an empty scope is a full install. The scope rides inside the
// inspected request, so the confirmation token binds the exact component set.
func (m Model) beginShellUpdateInstall(install shellinstaller.UserShellInstall, profile shellinstaller.UserExperience, scope []string) (tea.Model, error) {
	if home, homeErr := os.UserHomeDir(); homeErr == nil {
		_ = os.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", home)
	}
	_ = os.Setenv("GENTLE_AI_RECONFIGURE_TARGET", install.Target)
	request := shellinstaller.UserInstallRequest{Destination: install.Target, Mode: install.Mode, SharedPrefix: install.Prefix, SharedAgent: install.Agent, Experience: &profile}
	if install.Mode == "separate" {
		request.SharedPrefix, request.SharedAgent = "", ""
	}
	request.Scope = shellinstaller.CanonicalUpdateScope(scope)
	token, err := shellinstaller.InspectUserInstall(request)
	if err != nil {
		return m, err
	}
	request.Confirmation = token
	m.shellPlanRequest = request
	m.shellPlanConfig = nil
	m.shellPlan = shellUpdatePlanSteps(scope)
	m.shellInstallModel = cli.NewShellInstallModelWithExperience(nil, profile)
	m.shellUpdate = nil
	return m, nil
}

// shellChannelOnlyDelta reports whether the selected profile differs from the
// installed profile only in the SDK update channel. Only such deltas route
// Update-now to the in-place channel overlay; any preference or component
// change keeps the full reconfigure.
func shellChannelOnlyDelta(current, selected shellinstaller.UserExperience) bool {
	if selected.SDKSource != "stable" && selected.SDKSource != "main-preview" {
		return false
	}
	if current.SDKSource == "" {
		current.SDKSource = "stable" // A missing preference is the stable channel.
	}
	if current.SDKSource == selected.SDKSource {
		return false
	}
	// GGA is retired and inert; it never counts as a delta.
	current.GGA, selected.GGA = "off", "off"
	current.GGAProvider, selected.GGAProvider = "", ""
	current.SDKSource, selected.SDKSource = "", ""
	return current == selected
}

// beginChannelOverlayUpdate routes a channel-only change to the in-place
// overlay: it persists the channel preference first (the overlay never
// touches configuration), then prepares the shared update flow. The request's
// experience carries the target channel, which the inspection derives and the
// Windows worker recognizes as the channel-only reconfigure.
func (m Model) beginChannelOverlayUpdate(update *shellUpdateState) (tea.Model, error) {
	if err := shellinstaller.ApplyUserExperiencePreferences(update.install, update.config.profile); err != nil {
		return m, err
	}
	profile, err := shellinstaller.LoadUserShellExperience(update.install)
	if err != nil {
		return m, err
	}
	return m.beginShellUpdateInstall(update.install, profile, nil)
}

func (m Model) updateShellUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	update := m.shellUpdate
	if update == nil {
		return m, nil
	}
	if result, ok := msg.(shellUpdateResult); ok {
		update.checking, update.err = false, result.err
		if result.err == nil {
			update.info = result.info
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if update.checking {
		if key.String() == "esc" || key.String() == "ctrl+c" {
			return m.restoreUpdateConfig(update), nil
		}
		return m, nil
	}
	actions := 1
	if update.info.Available {
		actions = 2
	}
	switch key.String() {
	case "j", "down":
		update.cursor = min(actions-1, update.cursor+1)
	case "k", "up":
		update.cursor = max(0, update.cursor-1)
	case "esc", "ctrl+c":
		return m.restoreUpdateConfig(update), nil
	case "enter":
		if update.info.Available && update.cursor == 0 {
			// A channel-only delta routes Update-now to the in-place overlay;
			// every other change keeps the full component reconfigure.
			if loaded, loadErr := shellinstaller.LoadUserShellExperience(update.install); loadErr == nil && shellChannelOnlyDelta(loaded, update.config.profile) {
				next, err := m.beginChannelOverlayUpdate(update)
				if err != nil {
					update.err = err
					return m, nil
				}
				return next, nil
			}
			next, err := m.beginComponentUpdate(update)
			if err != nil {
				update.err = err
				return m, nil
			}
			return next, nil
		}
		return m.restoreUpdateConfig(update), nil
	}
	return m, nil
}

func manageDisplay(value, fallback string) string {
	if value == "" {
		return fallback
	}
	switch value {
	case "on":
		return "On"
	case "off":
		return "Off"
	case "gentleman":
		return "Gentleman"
	case "neutral":
		return "Neutral"
	case "unmanaged":
		return "Custom"
	case "main-preview":
		return "Main preview"
	case "stable":
		return "Stable"
	default:
		return value
	}
}

func (m Model) beginManageChanges(config shellExperienceConfig) (tea.Model, tea.Cmd) {
	if len(config.existing) != 1 {
		config.err = errors.New("the managed installation is no longer available")
		m.shellExperience = &config
		return m, nil
	}
	install := config.existing[0]
	current, err := shellinstaller.LoadUserShellExperience(install)
	if err != nil {
		config.err = err
		m.shellExperience = &config
		return m, nil
	}
	selected := config.profile
	inPlace := install.Mode == "separate"
	preferences, components := manageChanges(current, selected, inPlace)
	m.shellManage = &shellManageState{install: install, config: config, preferences: preferences, components: components}
	m.shellExperience = nil
	m.shellPlan = nil
	return m, nil
}

func manageChanges(current, selected shellinstaller.UserExperience, inPlace bool) ([]screens.ShellManageChange, []screens.ShellManageChange) {
	var preferences, components []screens.ShellManageChange
	add := func(label, from, to string, component bool) {
		if from == to {
			return
		}
		change := screens.ShellManageChange{Label: label, From: manageDisplay(from, "Inherit"), To: manageDisplay(to, "Inherit"), Component: component}
		if component || !inPlace {
			components = append(components, change)
		} else {
			preferences = append(preferences, change)
		}
	}
	add("Persona", current.Persona, selected.Persona, false)
	add("Background", current.BackgroundSubagents, selected.BackgroundSubagents, false)
	add("Strict TDD", current.StrictTDD, selected.StrictTDD, false)
	add("Global RDD", current.GlobalRDD, selected.GlobalRDD, false)
	add("Context7", current.Context7, selected.Context7, false)
	add("CodeGraph", current.CodeGraph, selected.CodeGraph, true)
	// SDK source is an update-channel preference, not a component: Manage can
	// apply Stable ↔ Main in place, and the Update action then queries that
	// channel. No component replacement happens here.
	add("SDK source", current.SDKSource, selected.SDKSource, false)
	return preferences, components
}

// shellManageComponentScope maps the manage screen's component deltas onto
// the scoped update request. A CodeGraph toggle becomes a codegraph-scoped
// update: OFF→ON provisions the library, ON→OFF publishes the tree without
// it. Every other component and the whole operator state (logins, sessions,
// user models) is reused from the installation instead of rebuilt.
func shellManageComponentScope(components []screens.ShellManageChange) []string {
	var scope []string
	for _, change := range components {
		switch change.Label {
		case "CodeGraph":
			scope = append(scope, shellinstaller.UserComponentCodeGraph)
		case "Context7":
			scope = append(scope, shellinstaller.UserComponentContext7)
		}
	}
	return scope
}

func manageContinueLabel(config shellExperienceConfig) string {
	if !config.reconfigure || len(config.existing) != 1 {
		return "Continue"
	}
	current, err := shellinstaller.LoadUserShellExperience(config.existing[0])
	if err != nil {
		return "Continue"
	}
	preferences, components := manageChanges(current, config.profile, config.mode == "separate")
	if len(components) > 0 {
		return "Update installation"
	}
	if len(preferences) > 0 {
		return "Apply changes to installation"
	}
	return "Continue"
}

func manageActions(manage *shellManageState) []string {
	if manage.uninstalled || manage.applied {
		return []string{"Back"}
	}
	var actions []string
	// Component deltas route through a scoped update, which also applies every
	// preference change of the same delta; preference-only deltas stay in place.
	if len(manage.components) > 0 {
		actions = append(actions, "Update installation")
	} else if len(manage.preferences) > 0 {
		actions = append(actions, "Apply preferences")
	}
	actions = append(actions, "Back")
	if manage.install.Mode == "separate" || manage.install.Mode == "agent" {
		actions = append(actions, "Uninstall this installation")
	}
	return actions
}

func (m Model) beginUninstallFromConfig(config shellExperienceConfig) Model {
	if len(config.existing) != 1 {
		config.err = errors.New("the managed installation is no longer available")
		m.shellExperience = &config
		return m
	}
	m.shellManage = &shellManageState{install: config.existing[0], config: config, confirm: true, fromConfig: true}
	m.shellExperience = nil
	return m
}

func (m Model) updateShellManage(msg tea.Msg) (tea.Model, tea.Cmd) {
	manage := m.shellManage
	if manage == nil {
		return m, nil
	}
	if result, ok := msg.(shellUninstallResult); ok {
		manage.working = false
		manage.uninstallErr = nil
		if result.err != nil {
			// An uninstall that completed with a note (for example a Pi tool
			// kept because Pi replaced it) is a success; on the uninstalled
			// screen uninstallErr carries that note, rendered as a note.
			var note interface{ UninstallNote() string }
			if result.backup == "" || !errors.As(result.err, &note) {
				manage.uninstallErr = result.err
				return m, nil
			}
			manage.uninstallErr = errors.New(note.UninstallNote())
		}
		manage.backup, manage.uninstalled = result.backup, true
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if manage.working {
		return m, nil
	}
	backToMenu := func() (tea.Model, tea.Cmd) {
		m.shellManage = nil
		m.shellExperience, m.shellInstallModel = nil, nil
		m.shellInstallSelection = cli.ShellInstallSelection{}
		m.setScreen(ScreenWelcome)
		return m, nil
	}
	// Both final summaries promise "Enter: back to menu"; detection reloads
	// the installed preferences when the operator returns.
	if manage.uninstalled || manage.applied {
		if key.String() == "enter" || key.String() == "esc" || key.String() == "ctrl+c" {
			return backToMenu()
		}
		return m, nil
	}
	if manage.confirm {
		switch key.String() {
		case "j", "down":
			manage.cursor = min(1, manage.cursor+1)
		case "k", "up":
			manage.cursor = max(0, manage.cursor-1)
		case "esc", "ctrl+c":
			if manage.fromConfig {
				m.shellExperience, m.shellManage = &manage.config, nil
				return m, nil
			}
			manage.confirm, manage.uninstallErr, manage.cursor = false, nil, 0
		case "enter":
			if manage.cursor == 1 {
				if manage.fromConfig {
					m.shellExperience, m.shellManage = &manage.config, nil
					return m, nil
				}
				manage.confirm, manage.uninstallErr, manage.cursor = false, nil, 0
				return m, nil
			}
			home, err := os.UserHomeDir()
			if err != nil {
				manage.uninstallErr = err
				return m, nil
			}
			manage.working = true
			manage.uninstallErr = nil
			install := manage.install
			return m, func() tea.Msg {
				backup, err := shellinstaller.UninstallUserShellInstall(home, install)
				return shellUninstallResult{backup: backup, err: err}
			}
		}
		return m, nil
	}
	actions := manageActions(manage)
	last := len(actions) - 1
	switch key.String() {
	case "j", "down":
		manage.cursor = min(last, manage.cursor+1)
	case "k", "up":
		manage.cursor = max(0, manage.cursor-1)
	case "esc", "ctrl+c":
		m.shellExperience, m.shellManage = &manage.config, nil
		return m, nil
	case "enter":
		switch actions[min(max(0, manage.cursor), last)] {
		case "Apply preferences":
			if err := shellinstaller.ApplyUserExperiencePreferences(manage.install, manage.config.profile); err != nil {
				manage.err = err
				return m, nil
			}
			manage.applied, manage.err = true, nil
		case "Update installation":
			next, err := m.beginShellUpdateInstall(manage.install, shellForceGGAOff(manage.config.profile), shellManageComponentScope(manage.components))
			if err != nil {
				manage.err = err
				return m, nil
			}
			planned := next.(Model)
			planned.shellManage, planned.shellPlanConfig = nil, &manage.config
			return planned, nil
		case "Uninstall this installation":
			manage.confirm, manage.cursor = true, 0
		default:
			m.shellExperience, m.shellManage = &manage.config, nil
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateShellDetected(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	last := len(m.shellDetected) - 1
	if len(m.shellDetected) == 1 {
		// Single install: Manage is the focused action and Back is below it.
		last = 1
	}
	switch key.String() {
	case "j", "down":
		m.shellDetectedCursor = min(last, m.shellDetectedCursor+1)
	case "k", "up":
		m.shellDetectedCursor = max(0, m.shellDetectedCursor-1)
	case "enter":
		if len(m.shellDetected) == 1 && m.shellDetectedCursor == 1 {
			return m.leaveDetectedInstall(), nil
		}
		return m.manageDetectedInstall(), nil
	case "esc", "x", "ctrl+c":
		return m.leaveDetectedInstall(), nil
	}
	return m, nil
}

func (m Model) leaveDetectedInstall() Model {
	m.shellDetected, m.shellDetectedProfiles = nil, nil
	m.shellExperience, m.shellInstallModel = nil, nil
	m.shellDetectedCursor = 0
	m.setScreen(ScreenWelcome)
	return m
}

func (m Model) manageDetectedInstall() Model {
	if m.shellDetectedCursor < 0 || m.shellDetectedCursor >= len(m.shellDetected) {
		return m.leaveDetectedInstall()
	}
	install := m.shellDetected[m.shellDetectedCursor]
	profile := m.shellDetectedProfiles[m.shellDetectedCursor]
	if loaded, err := shellinstaller.LoadUserShellExperience(install); err == nil {
		profile = loaded
	}
	_, _, found, note := shellDetectPi()
	config, _ := shellPlatformConfigure(shellExperienceConfig{
		profile: profile, mode: install.Mode, choice: "existing",
		prefix: install.Prefix, agent: install.Agent, piFound: found, piNote: note,
		existing: []shellinstaller.UserShellInstall{install}, reconfigure: true, target: install.Target,
	}, nil)
	m.shellExperience = &config
	m.shellInstallModel = cli.NewShellInstallModelWithExperience(nil, profile)
	m.shellDetected, m.shellDetectedProfiles = nil, nil
	m.shellDetectedCursor = 0
	return m
}

func (m Model) updateShellInstallRun(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(shellInstallTick); ok {
		if m.shellInstallRun == nil {
			return m, nil
		}
		if m.shellInstallRun.done {
			if m.shellInstallRun.err == nil && m.shellInstallRun.bloom < screens.ShellBloomFrames-1 {
				m.shellInstallRun.bloom++
				return m, tea.Tick(screens.ShellRoseDelay(m.shellInstallRun.bloom), func(time.Time) tea.Msg { return shellInstallTick{} })
			}
			return m, nil
		}
		m.shellInstallRun.spinner++
		return m, waitShellInstallEvent(m.shellInstallEvents)
	}
	event, ok := msg.(shellInstallEvent)
	if ok {
		run := m.shellInstallRun
		if run == nil {
			return m, nil
		}
		if event.done {
			run.done, run.err = true, event.err
			if event.err != nil {
				request := m.shellInstallSelection.Request
				if request.Experience != nil && request.Experience.SDKSource == "main-preview" && strings.Contains(event.err.Error(), "Main SDK") && os.Getenv("GENTLE_AI_MAIN_FALLBACK") != "1" {
					run.fallbackAvailable = true
				}
			}
			if event.err == nil {
				for index := range run.steps {
					run.steps[index].State = "done"
				}
				return m, tea.Tick(screens.ShellRoseDelay(0), func(time.Time) tea.Msg { return shellInstallTick{} })
			}
			for index := range run.steps {
				if run.steps[index].State == "running" {
					run.steps[index].State = "failed"
				}
			}
			return m, nil
		}
		progress := event.progress
		if progress.Index >= 1 && progress.Index <= len(run.steps) {
			step := &run.steps[progress.Index-1]
			switch progress.State {
			case "running":
				step.State = "running"
			case "detail":
				if progress.Detail != "" {
					step.Details = append(step.Details, progress.Detail)
					if len(step.Details) > 3 {
						step.Details = step.Details[len(step.Details)-3:]
					}
				}
			case "done":
				step.State = "done"
				if progress.Index < len(run.steps) && run.steps[progress.Index].State == "pending" {
					run.steps[progress.Index].State = "running"
				}
			}
		}
		return m, waitShellInstallEvent(m.shellInstallEvents)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	run := m.shellInstallRun
	if run == nil || !run.done {
		// The worker owns the operation; no key can claim completion or cancel
		// a possibly uncertain publication.
		return m, nil
	}
	if key.Type != tea.KeyEnter {
		return m, nil
	}
	if run.err != nil {
		if run.fallbackAvailable {
			switch key.Type {
			case tea.KeyEnter:
				request := m.shellInstallSelection.Request
				return m.requestShellInstall(request, true)
			case tea.KeyEsc, tea.KeyCtrlC:
				_ = os.Unsetenv("GENTLE_AI_MAIN_FALLBACK")
				m.shellInstallRun, m.shellInstallEvents = nil, nil
				m.shellInstallSelection = cli.ShellInstallSelection{}
				m.setScreen(ScreenWelcome)
				return m, nil
			}
			return m, nil
		}
		m.shellInstallRun, m.shellInstallEvents = nil, nil
		m.shellInstallSelection = cli.ShellInstallSelection{}
		m.setScreen(ScreenWelcome)
		return m, nil
	}
	_ = os.Unsetenv("GENTLE_AI_MAIN_FALLBACK")
	return m, tea.Quit
}

// WithShellInstaller opts into selection within this existing TUI program.
func (m Model) WithShellInstaller() Model {
	m.shellInstallEnabled = true
	return m
}

// ShellInstallSelection is consumed by the host after its program exits.
func (m Model) ShellInstallSelection() cli.ShellInstallSelection {
	return m.shellInstallSelection
}

func (m Model) updateShellInstall(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.shellRootRisk != nil {
		return m.updateShellRootRisk(msg)
	}
	if len(m.shellDetected) > 0 {
		return m.updateShellDetected(msg)
	}
	if m.shellUpdate != nil {
		return m.updateShellUpdate(msg)
	}
	if m.shellManage != nil {
		return m.updateShellManage(msg)
	}
	if m.shellInstallRun != nil {
		return m.updateShellInstallRun(msg)
	}
	if m.shellPlan != nil {
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch key.Type {
		case tea.KeyEnter:
			return m.beginShellInstall()
		case tea.KeyEsc:
			m.shellExperience, m.shellPlanConfig = m.shellPlanConfig, nil
			m.shellPlan, m.shellPlanRequest = nil, shellinstaller.UserInstallRequest{}
			return m, nil
		case tea.KeyCtrlC:
			m.shellPlan, m.shellPlanConfig, m.shellExperience = nil, nil, nil
			m.shellPlanRequest, m.shellInstallModel = shellinstaller.UserInstallRequest{}, nil
			_ = os.Unsetenv("GENTLE_AI_RECONFIGURE_TARGET")
			m.setScreen(ScreenWelcome)
			return m, nil
		}
		return m, nil
	}
	if m.shellExperience != nil {
		// Platform-unsupported targets stay disabled with their reason.
		config, refused := shellPlatformConfigure(*m.shellExperience, msg)
		m.shellExperience = &config
		if refused {
			return m, nil
		}
		return m.configureShellExperience(msg)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if next, handled := m.shellExperienceNavigation(key); handled {
			return next, nil
		}
	}
	before, _ := cli.ShellInstallPresentationOf(m.shellInstallModel)
	if key, ok := msg.(tea.KeyMsg); ok && before.Review && key.Type == tea.KeyEnter {
		switch m.shellReadyCursor {
		case 0:
			// Reuse the existing physical-confirmation path, never a worker.
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
		case 1:
			m.shellInstallModel = cli.NewShellInstallModelWithRequest(nil, before.Request)
			m.shellInstallModel, _ = m.shellInstallModel.Update(tea.WindowSizeMsg{Width: m.Width, Height: m.Height})
			m.shellInstallSelection = cli.ShellInstallSelection{}
			m.shellReadyCursor, m.shellReadyOffset = 0, 0
			return m, nil
		case 2:
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if state, ok := cli.ShellInstallPresentationOf(m.shellInstallModel); ok && !state.Review {
			if state.Field == 1 && (key.Type == tea.KeyLeft || key.Type == tea.KeyRight) {
				return m, nil // Install target is chosen in configuration.
			}
			if state.Field >= 2 && (key.Type == tea.KeyRunes || key.Type == tea.KeyBackspace) {
				return m, nil // Detected Shared paths are read-only.
			}
			if m.shellReconfigure != "" && state.Field == 0 && (key.Type == tea.KeyRunes || key.Type == tea.KeyBackspace) {
				return m, nil // The existing install target is fixed.
			}
		}
	}
	m.shellInstallModel, _ = m.shellInstallModel.Update(msg)
	after, _ := cli.ShellInstallPresentationOf(m.shellInstallModel)
	if before.Review != after.Review {
		m.shellReadyCursor, m.shellReadyOffset = 0, 0
	}
	if key, ok := msg.(tea.KeyMsg); ok && (key.Type == tea.KeyEsc || key.Type == tea.KeyCtrlC) {
		m.shellReconfigure = ""
		_ = os.Unsetenv("GENTLE_AI_RECONFIGURE_TARGET")
		m.shellInstallSelection = cli.ShellInstallSelection{}
		m.shellInstallModel = nil
		m.shellReadyCursor, m.shellReadyOffset = 0, 0
		m.setScreen(ScreenWelcome)
		return m, nil
	}
	selection, err := cli.ShellInstallOutcome(m.shellInstallModel)
	return m.finishShellInstallSelection(selection, err)
}

func (m Model) finishShellInstallSelection(selection cli.ShellInstallSelection, err error) (tea.Model, tea.Cmd) {
	if err == nil && selection.Confirmed {
		m.shellInstallSelection = selection
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			_ = os.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", home)
		}
		if m.shellReconfigure != "" {
			_ = os.Setenv("GENTLE_AI_RECONFIGURE_TARGET", m.shellReconfigure)
		} else {
			_ = os.Unsetenv("GENTLE_AI_RECONFIGURE_TARGET")
		}
		return m, tea.Quit
	}
	// This child performs synchronous selection only. Its sole command is
	// Quit, handled above; never run an installer from inside the parent TUI.
	return m, nil
}
