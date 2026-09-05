//go:build bench_fixture

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

func runBenchModelPickerCommand(args []string, stdout io.Writer) (bool, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai bench-model-picker --json")
		return true, nil
	}
	if len(args) != 1 || args[0] != "--json" {
		return true, fmt.Errorf("usage: gentle-ai bench-model-picker --json")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return true, fmt.Errorf("resolve benchmark home: %w", err)
	}
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	picker := screens.NewRuntimeModelPickerStateWithDiscoverer(settingsPath, nil)
	// The fixture supplies the already-effective runtime catalog without a private
	// OpenCode cache, auth file, or process invocation.
	picker.StartRuntimeCatalogDiscovery(1, "bench-fixture")
	picker = picker.Update(screens.RuntimeCatalogDiscoveryMsg{RequestID: 1, ProjectDir: "bench-fixture", Providers: map[string]opencode.Provider{
		"bench-provider": {ID: "bench-provider", Name: "Bench Provider", Models: map[string]opencode.Model{
			"bench-model": {ID: "bench-model", Name: "Bench Model", ToolCall: true},
		}},
	}})
	rows := screens.ModelPickerRowsForState(picker)
	identityRows := screens.ModelPickerRowsForStateWithIdentity(picker)
	target := -1
	for index, row := range identityRows {
		if row.Kind == screens.ModelPickerRowKindSetAllCustom {
			target = index
			break
		}
	}
	if target < 0 {
		return true, fmt.Errorf("benchmark picker did not discover the custom-agent bulk row: %v", rows)
	}

	m := tui.Model{
		Screen:          tui.ScreenModelPicker,
		Width:           120,
		Height:          40,
		ModelPicker:     picker,
		ModelConfigMode: true,
		SyncFn:          tuiSync(home),
	}
	for m.Cursor < target {
		m, err = updateBenchModelPicker(m, tea.KeyDown)
		if err != nil {
			return true, err
		}
	}
	if m.Cursor != target {
		return true, fmt.Errorf("benchmark picker cursor = %d, want custom-agent row %d", m.Cursor, target)
	}

	for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyEnter, tea.KeyEnter} {
		m, err = updateBenchModelPicker(m, key)
		if err != nil {
			return true, err
		}
	}
	selected := m.Selection.ModelAssignments["custom-refactor-agent"]
	m, err = updateBenchModelPicker(m, tea.KeyDown)
	if err != nil {
		return true, err
	}
	rendered := m.View()

	rows = screens.ModelPickerRowsForState(m.ModelPicker)
	for m.Cursor < len(rows) {
		m, err = updateBenchModelPicker(m, tea.KeyDown)
		if err != nil {
			return true, err
		}
	}
	m, err = updateBenchModelPicker(m, tea.KeyEnter)
	if err != nil {
		return true, err
	}
	if m.Screen != tui.ScreenSync {
		return true, fmt.Errorf("benchmark picker continue screen = %v, want sync (cursor=%d rows=%d target=%d custom=%v available=%v profile=%t model-config=%t mode=%v)", m.Screen, m.Cursor, len(rows), target, m.ModelPicker.CustomAgents, m.ModelPicker.AvailableIDs, m.ModelPicker.ForProfile, m.ModelConfigMode, m.ModelPicker.Mode)
	}
	// The sync command is returned by Update, so run the public TUI state
	// transition's command and feed its completion back through Update.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	var ok bool
	m, ok = updated.(tui.Model)
	if !ok {
		return true, fmt.Errorf("benchmark picker sync update returned %T, want tui.Model", updated)
	}
	syncDone, err := benchSyncDone(cmd)
	if err != nil {
		return true, err
	}
	if syncDone.Err != nil {
		return true, fmt.Errorf("benchmark picker sync: %w", syncDone.Err)
	}
	updated, _ = m.Update(syncDone)
	m, ok = updated.(tui.Model)
	if !ok {
		return true, fmt.Errorf("benchmark picker sync completion returned %T, want tui.Model", updated)
	}

	persisted, err := readBenchCustomAgent(settingsPath)
	if err != nil {
		return true, err
	}
	result := map[string]any{
		"custom_agent":          "custom-refactor-agent",
		"row_visible":           strings.Contains(rendered, "custom-refactor-agent"),
		"assignment_visible":    strings.Contains(rendered, "Bench Provider / Bench Model"),
		"selected_assignment":   selected.FullID(),
		"persisted_model":       persisted.ModelID,
		"persisted_variant":     persisted.Variant,
		"variant_present":       persisted.VariantPresent,
		"description_preserved": persisted.DescriptionPreserved,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return true, fmt.Errorf("marshal benchmark picker result: %w", err)
	}
	_, err = fmt.Fprintln(stdout, string(encoded))
	return true, err
}

func updateBenchModelPicker(m tui.Model, key tea.KeyType) (tui.Model, error) {
	updated, _ := m.Update(tea.KeyMsg{Type: key})
	result, ok := updated.(tui.Model)
	if !ok {
		return tui.Model{}, fmt.Errorf("benchmark picker update returned %T, want tui.Model", updated)
	}
	return result, nil
}

func benchSyncDone(cmd tea.Cmd) (tui.SyncDoneMsg, error) {
	if cmd == nil {
		return tui.SyncDoneMsg{}, fmt.Errorf("benchmark picker sync returned no command")
	}
	message := cmd()
	if syncDone, ok := message.(tui.SyncDoneMsg); ok {
		return syncDone, nil
	}
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, inner := range batch {
			if inner == nil {
				continue
			}
			if syncDone, ok := inner().(tui.SyncDoneMsg); ok {
				return syncDone, nil
			}
		}
	}
	return tui.SyncDoneMsg{}, fmt.Errorf("benchmark picker sync command returned %T, want SyncDoneMsg", message)
}

type benchCustomAgentState struct {
	ModelID              string
	Variant              string
	VariantPresent       bool
	DescriptionPreserved bool
}

func readBenchCustomAgent(settingsPath string) (benchCustomAgentState, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return benchCustomAgentState{}, fmt.Errorf("read benchmark opencode settings: %w", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return benchCustomAgentState{}, fmt.Errorf("parse benchmark opencode settings: %w", err)
	}
	agents, ok := root["agent"].(map[string]any)
	if !ok {
		return benchCustomAgentState{}, fmt.Errorf("benchmark opencode settings have no agent map")
	}
	agent, ok := agents["custom-refactor-agent"].(map[string]any)
	if !ok {
		return benchCustomAgentState{}, fmt.Errorf("benchmark opencode settings have no custom-refactor-agent")
	}
	modelID, _ := agent["model"].(string)
	variant, variantPresent := agent["variant"].(string)
	description, _ := agent["description"].(string)
	return benchCustomAgentState{
		ModelID:              modelID,
		Variant:              variant,
		VariantPresent:       variantPresent,
		DescriptionPreserved: description == "preserve this custom agent",
	}, nil
}
