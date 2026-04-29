package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

func AgentOptions() []model.AgentID {
	agents := catalog.AllAgents()
	ids := make([]model.AgentID, 0, len(agents))
	for _, agent := range agents {
		ids = append(ids, agent.ID)
	}
	return ids
}

func RenderAgents(selected []model.AgentID, cursor int, warnings []string) string {
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
	agentMeta := make(map[model.AgentID]catalog.Agent, len(catalog.AllAgents()))
	for _, info := range catalog.AllAgents() {
		agentMeta[info.ID] = info
	}

	for idx, agent := range agents {
		_, checked := selectedSet[agent]
		focused := idx == cursor
		label := string(agent)
		if info, ok := agentMeta[agent]; ok {
			label = info.Name
			if len(info.CapabilityLabels) > 0 {
				label = fmt.Sprintf("%s [%s]", label, strings.Join(info.CapabilityLabels, ", "))
			}
		}
		b.WriteString(renderCheckbox(label, checked, focused))
	}

	for _, warning := range warnings {
		if strings.TrimSpace(warning) == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(styles.WarningStyle.Render(warning))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	actions := []string{"Continue", "Back"}
	b.WriteString(renderOptions(actions, cursor-len(agents)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("space: toggle • enter: confirm • esc: back"))

	return b.String()
}
