package agentguidance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// #5256 U2c: retiring the user-global Claude module pilot back to the
// monolithic orchestrator. The uninstall service calls it only when Claude is
// removed completely. The explicit global install opt-in enables the pilot.

// ClaudeGlobalModulePaths reports every file RetireClaudeGlobalModules may
// change under homeDir: CLAUDE.md, each known module name and the module
// ledger. It returns nil when no ledger exists, because retirement then
// changes nothing. Callers snapshot these paths before retiring.
func ClaudeGlobalModulePaths(homeDir string) ([]string, error) {
	return globalModulePaths(homeDir, model.AgentClaudeCode)
}

// CodexGlobalModulePaths reports every file RetireCodexGlobalModules may
// change. It returns nil when no module ledger exists.
func CodexGlobalModulePaths(homeDir string) ([]string, error) {
	return globalModulePaths(homeDir, model.AgentCodex)
}

func globalModulePaths(homeDir string, agent model.AgentID) ([]string, error) {
	configDir, corePath, present, err := globalModuleLedger(homeDir, agent)
	if err != nil || !present {
		return nil, err
	}
	return orchestratorCorePathsFor(configDir, filepath.Base(corePath)), nil
}

// RetireClaudeGlobalModules replaces the module core in the user-global
// CLAUDE.md with the monolithic orchestrator rendered from contract, then
// removes the modules the ledger still owns and the ledger itself.
//
// Without a ledger it changes nothing and claims nothing: the install never
// used modules. An invalid ledger, an unsafe module directory or a non-regular
// known module path fails before any write. Modified, unowned and unknown
// files stay, and so does the directory. Only the orchestrator section of
// CLAUDE.md changes; routing and user text around it are preserved.
//
// It runs the install transaction with nothing referenced, so the monolith is
// published before any module is removed and a failure restores the core
// before any module; the same limits apply (see commitOrchestratorCoreAt).
func RetireClaudeGlobalModules(homeDir string, contract ReviewContractSource) (Result, error) {
	return retireGlobalModules(homeDir, model.AgentClaudeCode, contract)
}

// RetireCodexGlobalModules replaces the user-global Codex module core with the
// monolithic orchestrator and retires only ledger-owned modules.
func RetireCodexGlobalModules(homeDir string, contract ReviewContractSource) (Result, error) {
	return retireGlobalModules(homeDir, model.AgentCodex, contract)
}

func retireGlobalModules(homeDir string, agent model.AgentID, contract ReviewContractSource) (Result, error) {
	configDir, corePath, present, err := globalModuleLedger(homeDir, agent)
	if err != nil || !present {
		return Result{}, err
	}
	monolith, err := RenderOrchestratorWithSource(agent, contract, "")
	if err != nil {
		return Result{}, err
	}
	merge := func(existing string) string { return injectOrchestratorSection(existing, monolith) }
	return commitPreparedOrchestratorCoreAt(configDir, corePath, merge, func() (*orchestratorModuleInstall, error) {
		return prepareOrchestratorModuleRemoval(configDir)
	}, func() error { return nil })
}

func globalModuleLedger(homeDir string, agent model.AgentID) (configDir, corePath string, present bool, err error) {
	adapter, err := agents.NewAdapter(agent)
	if err != nil {
		return "", "", false, err
	}
	configDir = adapter.GlobalConfigDir(homeDir)
	if !filepath.IsAbs(configDir) {
		return "", "", false, fmt.Errorf("%w: %s global modules need an absolute config dir, got %q", ErrInvalidTarget, agent, configDir)
	}
	corePath = adapter.SystemPromptFile(homeDir)
	if !filepath.IsAbs(corePath) || filepath.Dir(corePath) != configDir {
		return "", "", false, fmt.Errorf("%w: %s global module core %q is outside config dir %q", ErrInvalidTarget, agent, corePath, configDir)
	}
	_, err = os.Lstat(filepath.Join(configDir, "gentle-ai", "orchestrator", orchestratorModuleLedgerName))
	// ENOTDIR: a user file sits where a module directory would be, so no ledger.
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return configDir, corePath, false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("stat orchestrator module ledger: %w", err)
	}
	return configDir, corePath, true, nil
}
