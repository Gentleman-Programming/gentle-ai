package sdd

type OpenCodeCommand struct {
	Name        string
	Description string
	Body        string
}

func OpenCodeCommands() []OpenCodeCommand {
	return []OpenCodeCommand{
		{Name: "sdd-init", Description: "Initialize SDD context", Body: "/sdd-init"},
		{Name: "sdd-new", Description: "Start a new SDD change", Body: "/sdd-new ${change-name}"},
		{Name: "sdd-continue", Description: "Continue next pending artifact", Body: "/sdd-continue ${change-name}"},
		{Name: "sdd-ff", Description: "Generate all planning artifacts", Body: "/sdd-ff ${change-name}"},
		{Name: "sdd-apply", Description: "Implement tasks", Body: "/sdd-apply ${change-name}"},
		{Name: "sdd-verify", Description: "Verify implementation", Body: "/sdd-verify ${change-name}"},
		{Name: "sdd-archive", Description: "Archive completed change", Body: "/sdd-archive ${change-name}"},
		{Name: "sdd-onboard", Description: "Guided SDD walkthrough", Body: "/sdd-onboard"},
	}
}
