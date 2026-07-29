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
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select AI Agents"))
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
	actions := []string{"Continue", "Back"}
	b.WriteString(renderOptions(actions, cursor-len(agents)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("space: toggle • enter: confirm • esc: back"))

	return b.String()
}
