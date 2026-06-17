package screens

// sddPhases is the ordered list of SDD phase names used by model pickers.
// Shared across Claude, Kiro, Kilo, and Codex pickers to avoid drift.
var sddPhases = []string{
	"orchestrator",
	"sdd-explore",
	"sdd-propose",
	"sdd-spec",
	"sdd-design",
	"sdd-tasks",
	"sdd-apply",
	"sdd-verify",
	"sdd-archive",
	"sdd-init",
	"sdd-onboard",
}

// sddPhaseLabels maps phase names to human-readable labels for TUI display.
var sddPhaseLabels = map[string]string{
	"orchestrator": "Orchestrator",
	"sdd-explore":  "Explore",
	"sdd-propose":  "Propose",
	"sdd-spec":     "Spec",
	"sdd-design":   "Design",
	"sdd-tasks":    "Tasks",
	"sdd-apply":    "Apply",
	"sdd-verify":   "Verify",
	"sdd-archive":  "Archive",
	"sdd-init":     "Init",
	"sdd-onboard":  "Onboard",
}
