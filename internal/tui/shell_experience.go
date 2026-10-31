package tui

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

// The ordinary Welcome starts with the agreed product preset. Incoming requests
// retain their explicit OFF/inherited choices; the transport defaults stay inert.
func shellWelcomeExperience() shellinstaller.UserExperience {
	// GGA is retired: every constructed experience is forced OFF with an empty
	// provider so no environment can silently re-enable it.
	return shellForceGGAOff(shellinstaller.UserExperience{
		Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on",
		GlobalRDD: "on", CodeGraph: "on", Context7: "on",
	})
}

// errShellAgentCodeGraph names why the agent target never offers CodeGraph ON:
// the CodeGraph library stages only into a Separate (or Shared) install.
var errShellAgentCodeGraph = errors.New("CodeGraph needs a Separate install; it stays off for On my existing Pi")

// shellForceGGAOff retires GGA on any constructed or loaded shell experience:
// old installations, manifests or environments can never re-enable it here.
func shellForceGGAOff(profile shellinstaller.UserExperience) shellinstaller.UserExperience {
	profile.GGA, profile.GGAProvider = "off", ""
	return profile
}

// shellDetectPi reports the single local Pi install Shared may reuse. Detection
// lives in the installer package; the TUI never invents paths.
func shellDetectPi() (string, string, bool, string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", false, "missing"
	}
	detection := shellinstaller.DetectUserPi(home)
	note := detection.Problem
	if detection.Detail != "" {
		note = detection.Detail
	}
	return detection.Prefix, detection.Agent, detection.Found, note
}

// shellDefaultDestination keeps the operator out of path editing. The parent is
// created private 0700; an existing install refuses a blind overwrite.
func shellDefaultDestination() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(home, ".local", "share", "gentle-shell")
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			return "", errors.New("LOCALAPPDATA is not set")
		}
		base = filepath.Join(local, "gentle-shell")
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm() != 0700) {
		return "", errors.New("default install parent is not a private 0700 directory: " + base)
	}
	destination := filepath.Join(base, "install")
	if _, err := os.Lstat(destination); err == nil {
		return "", errors.New("Gentle-Shell is already installed at " + destination + "; remove it before reinstalling")
	}
	return destination, nil
}

func shellAgentUsable(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

func shellDiscoverInstalls() []shellinstaller.UserShellInstall {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return shellinstaller.DiscoverUserShellInstalls(home)
}

func (m *Model) beginShellExperience() {
	m.shellReadyCursor, m.shellReadyOffset = 0, 0
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Setenv("GENTLE_AI_SHELL_REGISTRY_HOME", home)
	}
	_ = os.Unsetenv("GENTLE_AI_RECONFIGURE_TARGET")
	profile := shellWelcomeExperience()
	prefix, agent, found, note := shellDetectPi()
	installs := shellDiscoverInstalls()
	if len(installs) > 0 {
		profiles := make([]shellinstaller.UserExperience, len(installs))
		for index := range installs {
			if detected, err := shellinstaller.LoadUserShellExperience(installs[index]); err == nil {
				profiles[index] = shellForceGGAOff(detected)
			}
		}
		m.shellDetected, m.shellDetectedProfiles, m.shellDetectedCursor = installs, profiles, 0
		m.shellExperience, m.shellInstallModel = nil, nil
		return
	}
	m.shellExperience = &shellExperienceConfig{profile: profile, mode: "separate", choice: "separate", prefix: prefix, agent: agent, piFound: found, piNote: note, existing: installs}
	m.shellInstallModel = cli.NewShellInstallModelWithExperience(nil, profile)
}

type shellExperienceConfig struct {
	profile     shellinstaller.UserExperience
	cursor      int
	err         error
	mode        string
	choice      string
	prefix      string
	agent       string
	piFound     bool
	piNote      string
	existing    []shellinstaller.UserShellInstall
	reconfigure bool
	target      string
}

func (m Model) configureShellExperience(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyEsc || key.Type == tea.KeyCtrlC {
		m.shellExperience, m.shellInstallModel = nil, nil
		m.shellInstallSelection = cli.ShellInstallSelection{}
		m.setScreen(ScreenWelcome)
		return m, nil
	}
	config := *m.shellExperience
	config.err = nil
	reuseAgent := ""
	if shellAgentUsable(config.agent) {
		reuseAgent = config.agent
	}
	targets := screens.ShellInstallTargetEntries(config.mode, config.piFound, config.piNote, reuseAgent, config.existing, config.reconfigure)
	canUninstall := config.reconfigure && (config.mode == "separate" || config.mode == "agent")
	options := screens.ShellExperienceOptionsWithUninstall(config.profile, canUninstall, targets...)
	config.cursor = min(max(0, config.cursor), len(options)-1)
	entry := options[config.cursor]
	pressed := key.String()
	if strings.EqualFold(pressed, "x") {
		return m.configureShellExperience(tea.KeyMsg{Type: tea.KeyEsc})
	}
	if strings.EqualFold(pressed, "c") {
		for i, option := range options {
			if option.Control == screens.ShellExperienceContinue {
				config.cursor, entry = i, option
			}
		}
		pressed = "enter"
	}
	switch pressed {
	case "up", "k":
		config.cursor = max(0, config.cursor-1)
	case "down", "j", "tab":
		config.cursor = min(len(options)-1, config.cursor+1)
	case "home":
		config.cursor = 0
	case "end":
		for i, option := range options {
			if option.Control == screens.ShellExperienceContinue {
				config.cursor = i
			}
		}
	case "enter":
		switch entry.Control {
		case screens.ShellExperiencePersona:
			config.profile.Persona = entry.Choice
		case screens.ShellExperienceBackground:
			if config.profile.BackgroundSubagents == "on" {
				config.profile.BackgroundSubagents = "off"
			} else {
				config.profile.BackgroundSubagents = "on"
			}
		case screens.ShellExperienceInstallTarget:
			config.reconfigure, config.target = false, ""
			if entry.Choice == "shared" {
				if !config.piFound {
					detail := "No single npm-style Pi install was detected; choose Separate install"
					if config.piNote != "missing" && config.piNote != "ambiguous" && config.piNote != "" {
						detail = config.piNote
					}
					config.err = errors.New(detail)
				} else {
					config.mode, config.choice = "shared", "shared"
				}
			} else if entry.Choice == "agent" {
				if !shellAgentUsable(config.agent) {
					config.err = errors.New("No Pi agent was detected; choose Separate install")
				} else {
					config.mode, config.choice, config.prefix = "agent", "agent", ""
					if config.profile.CodeGraph == "on" {
						config.profile.CodeGraph, config.err = "off", errShellAgentCodeGraph
					}
				}
			} else if entry.Choice == "existing" && len(config.existing) == 1 {
				install := config.existing[0]
				profile, loadErr := shellinstaller.LoadUserShellExperience(install)
				if loadErr != nil {
					config.err = loadErr
				} else {
					config.profile, config.mode, config.choice = shellForceGGAOff(profile), install.Mode, "existing"
					config.prefix, config.agent = install.Prefix, install.Agent
					config.reconfigure, config.target = true, install.Target
				}
			} else {
				config.mode, config.choice = "separate", "separate"
			}
		case screens.ShellExperienceSDK:
			// The channel never consumes an optional preference. Only the
			// platform qualification locks it; unsupported preference
			// pairings are named when continuing.
			if entry.Choice == "main-preview" {
				config.err = shellMainPlatformError()
			}
			if config.err == nil {
				config.profile.SDKSource = entry.Choice
			}
		case screens.ShellExperienceContinue:
			// GGA is retired; force it inert before any plan or manage handoff.
			config.profile = shellForceGGAOff(config.profile)
			config.err = config.profile.Validate()
			if config.err == nil && config.mode == "agent" && config.profile.CodeGraph == "on" {
				config.err = errShellAgentCodeGraph
			}
			if config.err == nil && config.profile.SDKSource == "main-preview" {
				config.err = shellMainSDKChoiceError(config.profile)
			}
			if config.err == nil && config.reconfigure && len(config.existing) == 1 {
				return m.beginManageChanges(config)
			}
			if config.err == nil {
				var destination string
				var destErr error
				if config.reconfigure && config.target != "" {
					destination = config.target
				} else {
					destination, destErr = shellDefaultDestination()
				}
				if config.reconfigure && config.target != "" {
					_ = os.Setenv("GENTLE_AI_RECONFIGURE_TARGET", config.target)
				} else {
					_ = os.Unsetenv("GENTLE_AI_RECONFIGURE_TARGET")
				}
				if destErr != nil {
					config.err = destErr
				} else {
					state, _ := cli.ShellInstallPresentationOf(m.shellInstallModel)
					request := state.Request
					request.Destination = destination
					request.Mode = config.mode
					if config.mode == "shared" {
						request.SharedPrefix, request.SharedAgent = config.prefix, config.agent
					} else if config.mode == "agent" {
						request.SharedPrefix, request.SharedAgent = "", config.agent
					} else {
						request.SharedPrefix, request.SharedAgent = "", ""
					}
					request.Experience = &config.profile
					token, inspectErr := shellinstaller.InspectUserInstall(request)
					if inspectErr != nil {
						config.err = inspectErr
					} else {
						request.Confirmation = token
						m.shellPlanRequest = request
						m.shellPlanConfig = &config
						m.shellPlan = append([]string(nil), shellInstallStepLabels...)
						m.shellExperience = nil
						m.shellInstallModel, _ = m.shellInstallModel.Update(tea.WindowSizeMsg{Width: m.Width, Height: m.Height})
						return m, nil
					}
				}
			}
		case screens.ShellExperienceUpdate:
			return m.beginShellUpdate(config)
		case screens.ShellExperienceUninstall:
			return m.beginUninstallFromConfig(config), nil
		case screens.ShellExperienceCancel:
			return m.configureShellExperience(tea.KeyMsg{Type: tea.KeyEsc})
		default:
			fields := []*string{&config.profile.StrictTDD, &config.profile.GlobalRDD, &config.profile.CodeGraph, &config.profile.Context7}
			field := fields[entry.Control-screens.ShellExperienceTDD]
			if entry.Control == screens.ShellExperienceCodeGraph && config.mode == "agent" && *field != "on" {
				// The agent target has no CodeGraph producer; never offer ON.
				config.err = errShellAgentCodeGraph
			} else if *field == "on" {
				*field = "off"
			} else {
				*field = "on"
			}
		}
	}
	m.shellExperience = &config
	return m, nil
}

func shellDetectedMode(mode string) string {
	switch mode {
	case "shared":
		return "Shared Pi prefix"
	case "agent":
		return "On my existing Pi"
	default:
		return "Separate install"
	}
}

func shellDetectedChannel(channel string) string {
	if channel == "main-preview" {
		return "Main preview"
	}
	return "Stable"
}

func (m Model) shellInstallView() string {
	if len(m.shellDetected) == 1 {
		profile := m.shellDetectedProfiles[0]
		install := m.shellDetected[0]
		return screens.RenderShellDetected(screens.ShellDetectedInstall{
			Target: install.Target, Mode: shellDetectedMode(install.Mode), Channel: shellDetectedChannel(install.Channel),
			Persona: profile.Persona, Background: profile.BackgroundSubagents,
			TDD: profile.StrictTDD, RDD: profile.GlobalRDD, CodeGraph: profile.CodeGraph, Context7: profile.Context7,
		}, m.shellDetectedCursor, m.Width, m.Height)
	}
	if len(m.shellDetected) > 1 {
		entries := make([]screens.ShellDetectedEntry, len(m.shellDetected))
		for index, install := range m.shellDetected {
			entries[index] = screens.ShellDetectedEntry{Target: install.Target, Mode: shellDetectedMode(install.Mode), Channel: shellDetectedChannel(install.Channel)}
		}
		return screens.RenderShellDetectedList(entries, m.shellDetectedCursor, m.Width, m.Height)
	}
	if m.shellUpdate != nil {
		update := m.shellUpdate
		errText := ""
		if update.err != nil {
			errText = update.err.Error()
		}
		return screens.RenderShellUpdate(update.install.Target, update.install.Mode, update.info.Channel, update.checking, update.info.Available, update.info.Components, update.info.Detail, errText, update.cursor, m.Width, m.Height)
	}
	if m.shellManage != nil {
		manage := m.shellManage
		if manage.confirm || manage.uninstalled {
			errText := ""
			if manage.uninstallErr != nil {
				errText = manage.uninstallErr.Error()
			}
			return screens.RenderShellUninstall(manage.install.Target, manage.install.Mode, manage.uninstalled, manage.working, manage.backup, errText, manage.cursor, m.Width, m.Height)
		}
		errText := ""
		if manage.err != nil {
			errText = manage.err.Error()
		}
		return screens.RenderShellManage(manage.install.Target, manage.install.Mode, manage.preferences, manage.components, manage.applied, errText, manageActions(manage), manage.cursor, m.Width, m.Height)
	}
	if m.shellInstallRun != nil {
		steps := make([]screens.ShellInstallStep, len(m.shellInstallRun.steps))
		for index, step := range m.shellInstallRun.steps {
			steps[index] = screens.ShellInstallStep{Label: step.Label, State: step.State, Details: step.Details}
		}
		failure := ""
		published := false
		if m.shellInstallRun.err != nil {
			failure = m.shellInstallRun.err.Error()
			if destination := m.shellInstallSelection.Request.Destination; destination != "" {
				_, statErr := os.Lstat(destination)
				published = statErr == nil
			}
		}
		launch := "pi"
		if m.shellInstallSelection.Request.Mode == "separate" {
			launch = "gentle-shell"
		}
		return screens.RenderShellInstall(steps, failure, m.shellInstallRun.done, m.shellInstallRun.spinner, m.shellInstallRun.bloom, published, m.shellInstallRun.fallbackAvailable, m.shellInstallSelection.Request.Mode, launch, m.Width, m.Height)
	}
	if m.shellRootRisk != nil {
		return screens.RenderShellRootRisk(m.shellRootRisk.input, m.Width, m.Height)
	}
	if m.shellPlan != nil {
		return screens.RenderShellPlan(m.shellPlan, m.Width, m.Height)
	}
	if m.shellExperience != nil {
		reuseAgent := ""
		if shellAgentUsable(m.shellExperience.agent) {
			reuseAgent = m.shellExperience.agent
		}
		targets := screens.ShellInstallTargetEntries(m.shellExperience.mode, m.shellExperience.piFound, m.shellExperience.piNote, reuseAgent, m.shellExperience.existing, m.shellExperience.reconfigure)
		canUninstall := m.shellExperience.reconfigure && (m.shellExperience.mode == "separate" || m.shellExperience.mode == "agent")
		if canUninstall {
			return screens.RenderShellExperienceWithUninstallAction(m.shellExperience.profile, m.shellExperience.cursor, m.shellExperience.err, m.Width, m.Height, true, manageContinueLabel(*m.shellExperience), targets...)
		}
		return screens.RenderShellExperience(m.shellExperience.profile, m.shellExperience.cursor, m.shellExperience.err, m.Width, m.Height, targets...)
	}
	state, ok := cli.ShellInstallPresentationOf(m.shellInstallModel)
	if ok && state.Review {
		return screens.RenderShellReady(state.Request, state.Preview, m.Width, m.Height, m.shellReadyOffset, m.shellReadyCursor)
	}
	if ok {
		return screens.RenderUserInstallDestination(state.Request, state.Field, state.Error, m.Width, m.Height)
	}
	return m.shellInstallModel.View()
}

func (m Model) shellExperienceNavigation(key tea.KeyMsg) (Model, bool) {
	state, ok := cli.ShellInstallPresentationOf(m.shellInstallModel)
	if !ok {
		return m, false
	}
	if key.Type == tea.KeyCtrlE {
		profile := shellWelcomeExperience()
		if state.Request.Experience != nil {
			profile = *state.Request.Experience
		}
		m.shellInstallModel = cli.NewShellInstallModelWithRequest(nil, state.Request)
		m.shellInstallModel, _ = m.shellInstallModel.Update(tea.WindowSizeMsg{Width: m.Width, Height: m.Height})
		m.shellInstallSelection = cli.ShellInstallSelection{}
		m.shellReadyOffset, m.shellReadyCursor = 0, 0
		prefix, agent, found, note := shellDetectPi()
		mode := "separate"
		if state.Request.Mode == "shared" {
			mode, prefix, agent = "shared", state.Request.SharedPrefix, state.Request.SharedAgent
		}
		m.shellExperience = &shellExperienceConfig{profile: profile, mode: mode, choice: mode, prefix: prefix, agent: agent, piFound: found, piNote: note, existing: shellDiscoverInstalls()}
		return m, true
	}
	if !state.Review {
		return m, false
	}
	switch key.String() {
	case "up", "k":
		m.shellReadyCursor = max(0, m.shellReadyCursor-1)
	case "down", "j":
		m.shellReadyCursor = min(2, m.shellReadyCursor+1)
	case "pgup":
		m.shellReadyOffset -= screens.ShellReadyPageRows(m.Width, m.Height)
	case "pgdown":
		m.shellReadyOffset += screens.ShellReadyPageRows(m.Width, m.Height)
	case "home":
		m.shellReadyOffset = 0
	case "end":
		m.shellReadyOffset = screens.ShellReadyLastOffset(state.Request, state.Preview, m.Width, m.Height)
	default:
		return m, false
	}
	m.shellReadyOffset = min(max(0, m.shellReadyOffset), screens.ShellReadyLastOffset(state.Request, state.Preview, m.Width, m.Height))
	return m, true
}

// This visible choice gate never grants worker or physical-selection authority.
func shellMainPlatformError() error {
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		return nil
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return nil
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if runtime.GOOS == "darwin" {
		return errors.New("Main preview is qualified on macOS arm64 only; choose Stable to install")
	}
	return errors.New("SDK Main + stock provider is qualified on Linux amd64 today; macOS support is being completed")
}

func shellMainSDKChoiceError(p shellinstaller.UserExperience) error {
	if err := shellMainPlatformError(); err != nil {
		return err
	}
	return nil
}
