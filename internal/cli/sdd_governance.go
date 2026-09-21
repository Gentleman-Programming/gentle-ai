package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
)

// resolveGovernanceChangeRoot resolves the workspace root from cwd
// (falling back to the process working directory when cwd is empty, the
// same convention sddstatus.resolveWorkspaceRoot uses) and the absolute
// path to an EXISTING change directory named change, checking first
// openspec/changes/<change> and then openspec/changes/archive/<change>.
//
// It never creates a directory: `axiom sdd kickoff`/`axiom sdd gate` write
// inside a change that already exists (task 8.2). Checking the archived
// location too — rather than only the active one — is what lets a bare,
// separator-free --change value (already required by
// kickoff.validateChangeName's containment rule) still resolve to an
// archived change, so RefuseArchivedRoot below can reject it by D-14
// instead of this function reporting a misleading "does not exist".
//
// It also refuses a change root already living under
// openspec/changes/archive/ (D-14) here, once, so
// RunSDDKickoff/RunSDDGate never need two independent copies of that check
// (task 10.4).
func resolveGovernanceChangeRoot(cwd, change string) (workspaceRoot, changeRoot string, err error) {
	root := cwd
	if strings.TrimSpace(root) == "" {
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			return "", "", fmt.Errorf("resolver el directorio de trabajo actual: %w", wdErr)
		}
		root = wd
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolver --cwd %q: %w", cwd, err)
	}
	info, statErr := os.Stat(absRoot)
	if statErr != nil {
		return "", "", fmt.Errorf("--cwd %q no existe o no es accesible: %w", cwd, statErr)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("--cwd %q no es un directorio", cwd)
	}

	activeRoot := filepath.Join(absRoot, "openspec", "changes", change)
	archivedRoot := filepath.Join(absRoot, "openspec", "changes", "archive", change)

	switch {
	case isDirectory(activeRoot):
		changeRoot = activeRoot
	case isDirectory(archivedRoot):
		changeRoot = archivedRoot
	default:
		return "", "", fmt.Errorf("el cambio %q no existe en %q ni en %q; estos verbos escriben dentro de un cambio ya creado, nunca crean uno nuevo", change, activeRoot, archivedRoot)
	}

	if err := kickoff.RefuseArchivedRoot(absRoot, changeRoot); err != nil {
		return "", "", err
	}
	return absRoot, changeRoot, nil
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// hasGovernanceHelpFlag reports whether args requests help, mirroring
// hasSDDArchiveComposeHelp's exact --help/-h check (sdd_archive_compose.go)
// for the two new kickoff/gate verbs.
func hasGovernanceHelpFlag(args []string) bool {
	for _, argument := range args {
		if argument == "--help" || argument == "-h" {
			return true
		}
	}
	return false
}
