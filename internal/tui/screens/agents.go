package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func AgentOptions() []model.AgentID {
	agents := catalog.AllAgents()
	ids := make([]model.AgentID, 0, len(agents))
	for _, agent := range agents {
		ids = append(ids, agent.ID)
	}
	return ids
}

func RenderAgents(selected []model.AgentID, cursor int) string {
	return renderAgentChecklist("Select AI Agents", selected, cursor)
}

func RenderEditAgents(selected []model.AgentID, cursor int) string {
	return renderAgentChecklist("Edit Installed Agents", selected, cursor)
}

func renderAgentChecklist(title string, selected []model.AgentID, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("Use j/k to move, space to toggle, enter to continue."))
	b.WriteString("\n\n")

	selectedSet := make(map[model.AgentID]struct{}, len(selected))
	for _, agent := range selected {
		selectedSet[agent] = struct{}{}
	}

	agents := AgentOptions()
	for idx, agent := range agents {
		_, checked := selectedSet[agent]
		focused := idx == cursor
		b.WriteString(renderCheckbox(string(agent), checked, focused))
	}

	b.WriteString("\n")
	if len(selected) == 0 {
		b.WriteString(styles.ErrorStyle.Render("⚠️  Please select at least one agent to continue."))
		b.WriteString("\n\n")
	}

	actions := []string{"Continue", "Back"}
	b.WriteString(renderOptions(actions, cursor-len(agents)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("space: toggle • enter: confirm • esc: back"))

	return b.String()
}
