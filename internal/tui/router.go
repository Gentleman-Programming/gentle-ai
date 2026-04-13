package tui

type Route struct {
	Forward  Screen
	Backward Screen
}

var linearRoutes = map[Screen]Route{
	ScreenWelcome:                {Forward: ScreenDetection},
	ScreenDetection:              {Forward: ScreenAgents, Backward: ScreenWelcome},
	ScreenAgents:                 {Forward: ScreenPersona, Backward: ScreenDetection},
	ScreenPersona:                {Forward: ScreenPreset, Backward: ScreenAgents},
	ScreenPreset:                 {Forward: ScreenDependencyTree, Backward: ScreenPersona},
	ScreenClaudeModelPicker:      {Forward: ScreenDependencyTree, Backward: ScreenPreset},
	ScreenKiroModelPicker:        {Forward: ScreenDependencyTree, Backward: ScreenPreset},
	ScreenSDDMode:                {Forward: ScreenStrictTDD, Backward: ScreenPreset},
	ScreenStrictTDD:              {Forward: ScreenDependencyTree, Backward: ScreenSDDMode},
	ScreenModelPicker:            {Forward: ScreenStrictTDD, Backward: ScreenSDDMode},
	ScreenDependencyTree:         {Forward: ScreenReview, Backward: ScreenPreset},
	ScreenSkillPicker:            {Forward: ScreenReview, Backward: ScreenDependencyTree},
	ScreenReview:                 {Forward: ScreenInstalling, Backward: ScreenDependencyTree},
	ScreenInstalling:             {Forward: ScreenComplete, Backward: ScreenReview},
	ScreenComplete:               {Backward: ScreenInstalling},
	ScreenBackups:                {Backward: ScreenWelcome},
	ScreenRestoreConfirm:         {Backward: ScreenBackups},
	ScreenRestoreResult:          {Backward: ScreenBackups},
	ScreenDeleteConfirm:          {Backward: ScreenBackups},
	ScreenDeleteResult:           {Backward: ScreenBackups},
	ScreenRenameBackup:           {Backward: ScreenBackups},
	ScreenUpgrade:                {Backward: ScreenWelcome},
	ScreenSync:                   {Backward: ScreenWelcome},
	ScreenUpgradeSync:            {Backward: ScreenWelcome},
	ScreenModelConfig:            {Backward: ScreenWelcome},
	ScreenProfiles:               {Backward: ScreenWelcome},
	ScreenProfileCreate:          {Backward: ScreenProfiles},
	ScreenProfileDelete:          {Backward: ScreenProfiles},
	ScreenAgentBuilderEngine:     {Backward: ScreenWelcome},
	ScreenAgentBuilderPrompt:     {Backward: ScreenAgentBuilderEngine},
	ScreenAgentBuilderSDD:        {Backward: ScreenAgentBuilderPrompt},
	ScreenAgentBuilderSDDPhase:   {Backward: ScreenAgentBuilderSDD},
	ScreenAgentBuilderGenerating: {Backward: ScreenAgentBuilderPrompt},
	ScreenAgentBuilderPreview:    {Backward: ScreenAgentBuilderPrompt},
	ScreenAgentBuilderInstalling: {Forward: ScreenAgentBuilderComplete},
	ScreenAgentBuilderComplete:   {Backward: ScreenWelcome},
	ScreenUninstallMode:          {Backward: ScreenWelcome},
	ScreenUninstall:              {Backward: ScreenUninstallMode},
	ScreenUninstallComponents:    {Backward: ScreenUninstall},
	ScreenUninstallProfiles:      {Backward: ScreenUninstallComponents},
	ScreenUninstallResult:        {Backward: ScreenWelcome},
}

func NextScreen(screen Screen) (Screen, bool) {
	route, ok := linearRoutes[screen]
	if !ok || route.Forward == ScreenUnknown {
		return ScreenUnknown, false
	}

	return route.Forward, true
}

func PreviousScreen(screen Screen) (Screen, bool) {
	route, ok := linearRoutes[screen]
	if !ok || route.Backward == ScreenUnknown {
		return ScreenUnknown, false
	}

	return route.Backward, true
}
