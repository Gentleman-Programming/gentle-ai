package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

func TestProceedToDependencyTreeFromInstallFlow_gateResolved_skipsEngramPrompt(t *testing.T) {
	base := NewModel(system.DetectionResult{}, "dev")
	base.Selection.Preset = model.PresetCustom
	base.Selection.Agents = []model.AgentID{model.AgentCursor}
	base.Selection.Components = []model.ComponentID{model.ComponentEngram}
	base.installEngramGateResolved = true
	m := &base

	m.proceedToDependencyTreeFromInstallFlow()

	if m.Screen != ScreenDependencyTree {
		t.Fatalf("Screen = %v, want ScreenDependencyTree", m.Screen)
	}
	if m.installFlowEngramActive {
		t.Fatal("installFlowEngramActive should stay false when gate already resolved")
	}
}

func TestProceedToDependencyTreeFromInstallFlow_noEngramComponent_skipsEngramPrompt(t *testing.T) {
	tmp := t.TempDir()
	base := NewModel(system.DetectionResult{}, "dev")
	base.HomeDir = tmp
	base.Selection.Preset = model.PresetCustom
	base.Selection.Agents = []model.AgentID{model.AgentCursor}
	base.Selection.Components = nil
	m := &base

	m.proceedToDependencyTreeFromInstallFlow()

	if m.Screen != ScreenDependencyTree {
		t.Fatalf("Screen = %v, want ScreenDependencyTree", m.Screen)
	}
	if m.installFlowEngramActive {
		t.Fatal("installFlowEngramActive should stay false when Engram is not selected")
	}
}

func TestProceedToDependencyTreeFromInstallFlow_emptyDataDir_showsInstallDirChooser(t *testing.T) {
	tmp := t.TempDir()
	base := NewModel(system.DetectionResult{}, "dev")
	base.HomeDir = tmp
	base.EngramDataDir = filepath.Join(tmp, "edata")
	if err := os.MkdirAll(base.EngramDataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	base.Selection.Preset = model.PresetCustom
	base.Selection.Agents = []model.AgentID{model.AgentCursor}
	base.Selection.Components = []model.ComponentID{model.ComponentEngram}
	m := &base

	m.proceedToDependencyTreeFromInstallFlow()

	if m.Screen != ScreenEngramDataDirInstall {
		t.Fatalf("Screen = %v, want ScreenEngramDataDirInstall", m.Screen)
	}
	if !m.installFlowEngramActive {
		t.Fatal("installFlowEngramActive should be true for install-time chooser")
	}
	if m.installFlowEngramExistingData {
		t.Fatal("installFlowEngramExistingData should be false when data dir is empty")
	}
	if len(m.engramDirLocations) == 0 {
		t.Fatal("engramDirLocations should include default destination")
	}
}

func TestProceedToDependencyTreeFromInstallFlow_existingData_showsInstallEngramPrompt(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "edata")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := engram.DBPath(dataDir)
	if err := os.WriteFile(dbPath, []byte("sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}

	base := NewModel(system.DetectionResult{}, "dev")
	base.HomeDir = tmp
	base.EngramDataDir = dataDir
	base.Selection.Preset = model.PresetCustom
	base.Selection.Agents = []model.AgentID{model.AgentCursor}
	base.Selection.Components = []model.ComponentID{model.ComponentEngram}
	m := &base

	m.proceedToDependencyTreeFromInstallFlow()

	if m.Screen != ScreenEngramDataDirInstall {
		t.Fatalf("Screen = %v, want ScreenEngramDataDirInstall", m.Screen)
	}
	if !m.installFlowEngramActive {
		t.Fatal("installFlowEngramActive should be true for install-time gate")
	}
	if !m.installFlowEngramExistingData {
		t.Fatal("installFlowEngramExistingData should be true when data dir has content")
	}
	if m.Cursor != 0 {
		t.Fatalf("Cursor = %d, want 0", m.Cursor)
	}
	if len(m.DependencyPlan.OrderedComponents) == 0 {
		t.Fatal("DependencyPlan should be populated after buildDependencyPlan")
	}
}

func TestInstallEngramDirChooser_DefaultProceedsWithDefaultDir(t *testing.T) {
	tmp := t.TempDir()
	base := NewModel(system.DetectionResult{}, "dev")
	base.HomeDir = tmp
	base.Selection.Components = []model.ComponentID{model.ComponentEngram}
	base.installFlowEngramActive = true
	base.installFlowEngramExistingData = false
	base.Screen = ScreenEngramDataDirInstall
	base.engramDirLocations = engram.SuggestLocationsWithKnown(tmp, engram.DefaultDir(tmp), nil)
	base.Cursor = 0

	next, cmd := base.confirmSelection()
	if cmd != nil {
		t.Fatal("default install data dir selection should not start an async command")
	}
	m := next.(Model)
	if m.Screen != ScreenDependencyTree {
		t.Fatalf("Screen = %v, want ScreenDependencyTree", m.Screen)
	}
	if m.Selection.EngramDataDir != "" {
		t.Fatalf("EngramDataDir = %q, want empty default", m.Selection.EngramDataDir)
	}
	if !m.installEngramGateResolved {
		t.Fatal("installEngramGateResolved should be true")
	}
}

func TestInstallEngramDirChooser_CustomPathProceedsWithCustomDir(t *testing.T) {
	tmp := t.TempDir()
	custom := filepath.Join(tmp, "custom-engram")
	base := NewModel(system.DetectionResult{}, "dev")
	base.HomeDir = tmp
	base.installFlowEngramActive = true
	base.installFlowEngramExistingData = false
	base.Screen = ScreenEngramDataDirCustomPath
	base.engramDirOp = model.EngramDataDirOpSet
	base.engramDirDstIdx = screens.EngramCustomPathDstIdx
	base.engramDirCustomPath = custom
	base.engramDirCustomPos = len([]rune(custom))

	next, cmd := base.handleEngramCustomPathInput(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("custom install data dir selection should not start an async command")
	}
	m := next.(Model)
	if m.Screen != ScreenDependencyTree {
		t.Fatalf("Screen = %v, want ScreenDependencyTree", m.Screen)
	}
	if m.Selection.EngramDataDir != filepath.Clean(custom) {
		t.Fatalf("EngramDataDir = %q, want %q", m.Selection.EngramDataDir, filepath.Clean(custom))
	}
}

func TestCheckEngramCopyMoveDiskSpace_invalidDestinationIgnored(t *testing.T) {
	base := NewModel(system.DetectionResult{}, "dev")
	base.engramDirLocations = nil
	m := &base

	if !m.checkEngramCopyMoveDiskSpace(-1) {
		t.Fatal("invalid negative index should allow proceed (no disk check)")
	}
	if m.EngramSpaceErr != "" {
		t.Fatalf("EngramSpaceErr = %q, want empty", m.EngramSpaceErr)
	}
	if !m.checkEngramCopyMoveDiskSpace(0) {
		t.Fatal("out-of-range index should allow proceed (no disk check)")
	}
}
