package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
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

func TestProceedToDependencyTreeFromInstallFlow_noSQLiteArtifacts_skipsEngramPrompt(t *testing.T) {
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

	if m.Screen != ScreenDependencyTree {
		t.Fatalf("Screen = %v, want ScreenDependencyTree", m.Screen)
	}
	if m.installFlowEngramActive {
		t.Fatal("installFlowEngramActive should stay false when no sqlite files exist")
	}
}

func TestProceedToDependencyTreeFromInstallFlow_existingSQLite_showsInstallEngramPrompt(t *testing.T) {
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
	if m.Cursor != 0 {
		t.Fatalf("Cursor = %d, want 0", m.Cursor)
	}
	if len(m.DependencyPlan.OrderedComponents) == 0 {
		t.Fatal("DependencyPlan should be populated after buildDependencyPlan")
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
