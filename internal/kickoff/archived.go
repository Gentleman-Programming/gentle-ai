package kickoff

import (
	"fmt"
	"path/filepath"
	"strings"
)

// archivedChangesPrefix is the workspace-relative, slash-normalized path
// segment that marks a change root as already archived (D-14). It is
// compared only against the slash-normalized result of filepath.Rel, never
// against a raw platform-separator string, so the check behaves the same
// on Windows and POSIX.
const archivedChangesPrefix = "openspec/changes/archive"

// RefuseArchivedRoot rejects a write whose changeRoot has escaped
// workspaceRoot, or whose changeRoot already lives under
// openspec/changes/archive/. Post-archive immutability of ownership is
// declared, not filesystem-enforced (D-14): Axiom cannot stop an editor
// from touching files under archive/ directly, but every write path this
// package owns calls this guard first, so at least no Axiom-owned verb
// reopens an archived change on the caller's behalf.
func RefuseArchivedRoot(workspaceRoot, changeRoot string) error {
	rel, err := filepath.Rel(workspaceRoot, changeRoot)
	if err != nil {
		return fmt.Errorf("resolver la raiz del cambio %q respecto al workspace %q: %w", changeRoot, workspaceRoot, err)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return fmt.Errorf("la raiz del cambio %q queda fuera del workspace %q", changeRoot, workspaceRoot)
	}
	if rel == archivedChangesPrefix || strings.HasPrefix(rel, archivedChangesPrefix+"/") {
		return fmt.Errorf("el cambio %q ya esta archivado; gestiona la correccion mediante un ticket de bug o un nuevo incremento en vez de reabrirlo (REQ-21.18)", rel)
	}
	return nil
}
