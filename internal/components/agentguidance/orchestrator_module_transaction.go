package agentguidance

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/mutationjournal"
)

// #5256 U1b: the private transaction used by global Claude routing and pilot
// retirement. It publishes the core before changing the modules it references.
//
// The core and the modules keep separate journals, both rooted at the Claude
// config directory, because a rollback must restore the core before touching
// any module: a core that cannot be restored may still reference the new
// modules, and one shared Restore would keep going and remove them.

// commitOrchestratorCore merges the orchestrator into CLAUDE.md under
// configDir, the absolute Claude config directory, and installs modules
// beside it in this order:
//
//  1. capture the CLAUDE.md before-image and preflight every module, before
//     any write;
//  2. stage the referenced modules, keeping old module names and the ledger;
//  3. publish the merged core, keeping an existing file's mode;
//  4. finalize: retire unused owned modules and write the ledger once.
//
// merge receives the captured CLAUDE.md bytes, never a second read, so the
// core write compare-and-swaps against exactly what was merged. Text outside
// the managed sections merge replaces is preserved by merge itself.
//
// On failure the core is restored first and the modules only when that
// succeeded. When the core restore fails, no module is rolled back: staged
// modules stay for whatever core is on disk, but unused modules a failed
// finalize already retired stay absent. Nothing recovers them automatically;
// the error joins the cause with each restore failure. Result.Changed is
// false only when nothing changed or the rollback fully succeeded, and
// Result.Files reports retired modules too. Nothing here is atomic across
// files or locked against concurrent writers, and the compare-and-swap
// checks bytes only, not file modes.
func commitOrchestratorCore(configDir string, merge guidanceMerge, modules []orchestratorModule) (Result, error) {
	return commitObservedOrchestratorCore(configDir, merge, modules, func() error { return nil })
}

// commitObservedOrchestratorCore calls published right after the core write
// and before finalize; an error from it is handled like a core write that
// failed after publication, so tests can drive the rollback.
func commitObservedOrchestratorCore(configDir string, merge guidanceMerge, modules []orchestratorModule, published func() error) (Result, error) {
	return commitPreparedOrchestratorCore(configDir, merge, func() (*orchestratorModuleInstall, error) {
		return prepareOrchestratorModuleInstall(configDir, modules)
	}, published)
}

// commitPreparedOrchestratorCore is the transaction itself. prepare plans the
// module side once the core is captured: an install, or the removal that
// retires the pilot behind a monolithic core.
func commitPreparedOrchestratorCore(configDir string, merge guidanceMerge, prepare func() (*orchestratorModuleInstall, error), published func() error) (Result, error) {
	if !filepath.IsAbs(configDir) {
		return Result{}, fmt.Errorf("orchestrator core config directory %q is not absolute", configDir)
	}
	corePath := filepath.Join(configDir, "CLAUDE.md")
	core := mutationjournal.New(configDir)
	if err := core.Capture(corePath); err != nil {
		return Result{}, fmt.Errorf("capture orchestrator core: %w", err)
	}
	before := core.OwnedFile(corePath, "", false, false).Before
	existing := ""
	if before != nil {
		existing = *before
	}
	merged := merge(existing)

	install, err := prepare()
	if err != nil {
		return Result{}, err
	}
	files := []string{corePath}
	for _, write := range slices.Concat(install.writes, install.adopted) {
		files = append(files, write.path)
	}
	files = append(append(files, install.retire...), install.ledgerPath)

	rollback := func(cause error) (Result, error) {
		if err := core.Restore(); err != nil {
			return Result{Changed: true, Files: files}, errors.Join(cause,
				fmt.Errorf("restore orchestrator core; modules not rolled back: %w", err))
		}
		if err := install.restore(); err != nil {
			return Result{Changed: true, Files: files}, errors.Join(cause, fmt.Errorf("restore orchestrator modules: %w", err))
		}
		return Result{Files: files}, cause
	}

	if err := install.stage(); err != nil {
		return rollback(err)
	}
	if _, err := core.WriteWithMode(corePath, []byte(merged), 0o644); err != nil {
		return rollback(fmt.Errorf("publish orchestrator core: %w", err))
	}
	if err := published(); err != nil {
		return rollback(err)
	}
	if err := install.finalize(); err != nil {
		return rollback(err)
	}
	changed := before == nil || *before != merged || len(install.writes) > 0 || len(install.retire) > 0 || install.writeLedger
	return Result{Changed: changed, Files: files}, nil
}

// orchestratorCorePaths lists, without touching the disk, every file
// commitOrchestratorCore may change under configDir: CLAUDE.md, each known
// module name, used or not, and the ledger. Backups must be planned from this
// list before the commit, never from its Result.
func orchestratorCorePaths(configDir string) []string {
	dir := filepath.Join(configDir, "gentle-ai", "orchestrator")
	paths := []string{filepath.Join(configDir, "CLAUDE.md")}
	for _, name := range orchestratorModuleNames {
		paths = append(paths, filepath.Join(dir, orchestratorModuleFile(name)))
	}
	return append(paths, filepath.Join(dir, orchestratorModuleLedgerName))
}
