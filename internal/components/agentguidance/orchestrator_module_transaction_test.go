package agentguidance

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/managedownership"
)

// #5256 U1b covers the private core/module transaction against a temporary
// Claude config directory. Nothing here wires it into a public install.

const coreTransactionUserCore = "# My rules\n\nprefix text\n\n<!-- gentle-ai:orchestrator -->\nold monolith\n<!-- /gentle-ai:orchestrator -->\n\nsuffix text\n"

var errCoreTransactionInjected = errors.New("injected failure after core publication")

type coreTransactionFixture struct {
	configDir, corePath, ledgerPath, ledger, unusedPath string
	bundle                                              orchestratorModuleBundle
}

// newCoreTransactionFixture lays out a user CLAUDE.md around a monolithic
// orchestrator block, a hash-owned referenced module holding old bytes, a
// hash-owned module the new bundle no longer references, and their ledger.
func newCoreTransactionFixture(t *testing.T) coreTransactionFixture {
	t.Helper()
	f := coreTransactionFixture{bundle: buildClaudeTestModules(t)}
	f.configDir = filepath.Join(t.TempDir(), "claude")
	dir := filepath.Join(f.configDir, "gentle-ai", "orchestrator")
	f.corePath = filepath.Join(f.configDir, "CLAUDE.md")
	f.ledgerPath = filepath.Join(dir, orchestratorModuleLedgerName)
	writeModuleTestFile(t, f.corePath, coreTransactionUserCore, 0o600)

	referenced := f.bundle.modules[0].file
	var unused string
	for _, name := range orchestratorModuleNames {
		file := orchestratorModuleFile(name)
		if !slices.ContainsFunc(f.bundle.modules, func(m orchestratorModule) bool { return m.file == file }) {
			unused = file
			break
		}
	}
	f.unusedPath = filepath.Join(dir, unused)
	writeModuleTestFile(t, filepath.Join(dir, referenced), "old referenced\n", 0o644)
	writeModuleTestFile(t, f.unusedPath, "old unused\n", 0o644)
	f.ledger = moduleLedgerJSON(map[string]string{
		referenced: managedownership.Hash([]byte("old referenced\n")),
		unused:     managedownership.Hash([]byte("old unused\n")),
	})
	writeModuleTestFile(t, f.ledgerPath, f.ledger, 0o644)
	return f
}

func (f coreTransactionFixture) merge(existing string) string {
	return injectOrchestratorSection(existing, f.bundle.core)
}

func (f coreTransactionFixture) mergedCore() string { return f.merge(coreTransactionUserCore) }

func TestOrchestratorCoreTransactionPublishesCoreThenFinalizesModules(t *testing.T) {
	f := newCoreTransactionFixture(t)
	result, err := commitObservedOrchestratorCore(f.configDir, f.merge, f.bundle.modules, func() error {
		// The new core is published while the old ledger and the unused
		// module it may still reference stay in place.
		assertModuleFile(t, f.corePath, f.mergedCore(), 0o600)
		assertModuleFile(t, f.ledgerPath, f.ledger, 0o644)
		assertModuleFile(t, f.unusedPath, "old unused\n", 0o644)
		return nil
	})
	if err != nil || !result.Changed || !slices.Contains(result.Files, f.corePath) {
		t.Fatalf("commit = %+v, %v; want changed core", result, err)
	}

	core := f.mergedCore()
	if !strings.HasPrefix(core, "# My rules\n\nprefix text\n\n<!-- gentle-ai:orchestrator -->\n") ||
		!strings.HasSuffix(core, "<!-- /gentle-ai:orchestrator -->\n\nsuffix text\n") ||
		strings.Contains(core, "old monolith") || !strings.Contains(core, strings.TrimSpace(f.bundle.core)) {
		t.Fatalf("merged core does not replace the managed block in place:\n%s", core)
	}
	assertModuleFile(t, f.corePath, core, 0o600)
	for _, module := range f.bundle.modules {
		assertModuleFile(t, filepath.Join(filepath.Dir(f.ledgerPath), module.file), module.content, 0o644)
	}
	assertModuleFile(t, f.ledgerPath, moduleLedgerJSON(moduleHashes(f.bundle.modules)), 0o644)
	if _, err := os.Lstat(f.unusedPath); !os.IsNotExist(err) {
		t.Fatalf("unused owned module after finalize: %v, want retired", err)
	}
}

func TestOrchestratorCoreTransactionRestoresCoreThenModules(t *testing.T) {
	f := newCoreTransactionFixture(t)
	before := snapshotModuleTree(t, f.configDir)
	result, err := commitObservedOrchestratorCore(f.configDir, f.merge, f.bundle.modules, func() error {
		assertModuleFile(t, f.corePath, f.mergedCore(), 0o600)
		return errCoreTransactionInjected
	})
	if !errors.Is(err, errCoreTransactionInjected) || result.Changed {
		t.Fatalf("commit = %+v, %v; want unchanged result with the injected error", result, err)
	}
	if after := snapshotModuleTree(t, f.configDir); !reflect.DeepEqual(after, before) {
		t.Fatalf("config dir after rollback:\n%v\nwant:\n%v", after, before)
	}
}

func TestOrchestratorCoreTransactionKeepsModulesWhenCoreRestoreConflicts(t *testing.T) {
	f := newCoreTransactionFixture(t)
	const userEdit = "user edit during install\n"
	result, err := commitObservedOrchestratorCore(f.configDir, f.merge, f.bundle.modules, func() error {
		if err := os.WriteFile(f.corePath, []byte(userEdit), 0o600); err != nil {
			t.Fatal(err)
		}
		return errCoreTransactionInjected
	})
	if !errors.Is(err, errCoreTransactionInjected) || !strings.Contains(err.Error(), "mutation journal conflict") || !result.Changed {
		t.Fatalf("commit = %+v, %v; want changed result joining the injected error and the core conflict", result, err)
	}
	// The concurrent core may reference the new modules, so they stay, and
	// the old ledger and unused module stay for the core they replaced.
	assertModuleFile(t, f.corePath, userEdit, 0o600)
	for _, module := range f.bundle.modules {
		assertModuleFile(t, filepath.Join(filepath.Dir(f.ledgerPath), module.file), module.content, 0o644)
	}
	assertModuleFile(t, f.ledgerPath, f.ledger, 0o644)
	assertModuleFile(t, f.unusedPath, "old unused\n", 0o644)
}
