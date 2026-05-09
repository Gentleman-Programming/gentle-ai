package tui

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// proceedToDependencyTreeFromInstallFlow builds the dependency plan and either
// continues to the dependency tree or intercepts with the install-time Engram
// prompt when SQLite artifacts already exist (single gate per session unless resolved).
func (m *Model) proceedToDependencyTreeFromInstallFlow() {
	m.buildDependencyPlan()
	if m.installEngramGateResolved {
		m.setScreen(ScreenDependencyTree)
		return
	}
	paths, err := engram.ExistingSQLiteArtifacts(m.resolvedEngramDir())
	if err != nil || len(paths) == 0 {
		m.setScreen(ScreenDependencyTree)
		return
	}
	m.installFlowEngramActive = true
	m.Cursor = 0
	m.setScreen(ScreenEngramDataDirInstall)
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
	ok, needed, avail, err := engram.DiskSpaceOKForSQLiteArtifacts(m.resolvedEngramDir(), dstPath)
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
