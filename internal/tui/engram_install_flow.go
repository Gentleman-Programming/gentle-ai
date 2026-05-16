package tui

import (
	"fmt"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// proceedToDependencyTreeFromInstallFlow builds the dependency plan and either
// continues to the dependency tree or intercepts with the install-time Engram
// dir chooser (single gate per session unless resolved).
func (m *Model) proceedToDependencyTreeFromInstallFlow() {
	m.buildDependencyPlan()
	if m.installEngramGateResolved {
		m.setScreen(ScreenDependencyTree)
		return
	}
	if !hasSelectedComponent(m.DependencyPlan.OrderedComponents, model.ComponentEngram) {
		m.setScreen(ScreenDependencyTree)
		return
	}
	currentDir := m.resolvedEngramDir()
	m.engramDirLocations = engram.SuggestLocationsWithKnown(m.HomeDir, currentDir, m.EngramKnownDataDirs)
	m.engramDirDoneMsg = nil
	m.engramDirOp = model.EngramDataDirOpNone
	m.engramDirDstIdx = -1
	m.engramDirCustomPath = ""
	m.engramDirCustomPos = 0
	m.EngramSpaceErr = ""

	m.installFlowEngramExistingData = engram.DataDirHasContent(m.resolvedEngramDir())
	m.installFlowEngramActive = true
	m.Cursor = 0
	m.setScreen(ScreenEngramDataDirInstall)
}

func (m *Model) chooseInstallEngramDir(dst string) {
	clean := filepath.Clean(dst)
	defaultDir := filepath.Clean(engram.DefaultDir(m.HomeDir))
	if clean == "." || clean == defaultDir {
		m.Selection.EngramDataDir = ""
		m.EngramDataDir = ""
	} else {
		m.Selection.EngramDataDir = clean
		m.EngramDataDir = clean
		m.addKnownEngramDir(clean)
	}
	m.Selection.EngramDataDirOp = model.EngramDataDirOpSet
	m.installEngramGateResolved = true
	m.installFlowEngramActive = false
	m.installFlowEngramExistingData = false
	m.EngramSpaceErr = ""
	m.setScreen(ScreenDependencyTree)
}

// checkEngramCopyMoveDiskSpace sets m.EngramSpaceErr when the destination volume
// lacks space for a copy/move; returns false if the operation should not proceed to confirm.
func (m *Model) checkEngramCopyMoveDiskSpace(dstIdx int) bool {
	if dstIdx < 0 || dstIdx >= len(m.engramDirLocations) {
		m.EngramSpaceErr = ""
		return true
	}
	return m.checkEngramCopyMoveDiskSpacePath(m.engramDirLocations[dstIdx].Path)
}

func (m *Model) checkEngramCopyMoveDiskSpacePath(dstPath string) bool {
	m.EngramSpaceErr = ""
	if dstPath == "" {
		return true
	}
	ok, needed, avail, err := engram.DiskSpaceOKForDataDir(m.resolvedEngramDir(), dstPath)
	if err != nil {
		m.EngramSpaceErr = err.Error()
		return false
	}
	if !ok {
		m.EngramSpaceErr = fmt.Sprintf(
			"Not enough disk space: need %s to copy Engram data; %s available on destination volume.",
			storage.FormatBytes(needed),
			storage.FormatBytes(avail),
		)
		return false
	}
	return true
}
