package agentguidance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/mutationjournal"
	"github.com/gentleman-programming/gentle-ai/v4/internal/managedownership"
)

// #5256 U1a: the private installer for the user-global Claude orchestrator
// modules, driven only by the core transaction (commitPreparedOrchestratorCore)
// for the explicit global opt-in and for uninstall. It owns only the module
// files and their ledger under <Claude config dir>/gentle-ai/orchestrator; the
// core that points at them, its ownership and the cross-file ordering belong
// to that transaction.
//
// The steps are ordered for that caller:
//
//  1. prepareOrchestratorModuleInstall classifies the ledger and every known
//     module name without writing anything, and rejects the whole plan on any
//     unsafe path, modified or unowned referenced file, or invalid ledger.
//  2. stage writes only the referenced module files. Old module names and the
//     ledger stay untouched, so the currently published core keeps resolving.
//  3. finalize MUST run only after the new core is published: it retires
//     hash-owned unused modules and writes the ledger once.
//  4. restore reinstates every before-image this plan changed. A caller
//     rolling back MUST restore the core first and restore modules only when
//     that succeeded; if the core restore fails, keep old and new modules.
//
// Nothing here is atomic across files: each write is atomic on its own, and
// restore is compare-and-swap protected and best-effort.
//
// prepareOrchestratorModuleRemoval reuses the same plan with nothing
// referenced, to retire the pilot on uninstall (U2c).

// orchestratorModuleLedgerName records the bytes Gentle AI installed for each
// module file, next to the modules.
const orchestratorModuleLedgerName = ".gentle-ai-orchestrator-module-ownership.json"

// orchestratorModuleLedger is the module ledger's own type, so JSON decode
// errors name it; its layout is the shared managedownership ledger.
type orchestratorModuleLedger struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

type orchestratorModuleWrite struct {
	path string
	data []byte
}

// orchestratorModuleInstall is a preflighted module mutation. Its journal is
// rooted at the Claude config directory and holds the before-image of the
// ledger and of every known module path.
type orchestratorModuleInstall struct {
	dir, ledgerPath   string
	journal           *mutationjournal.Journal
	writes            []orchestratorModuleWrite // referenced modules whose bytes change
	adopted           []orchestratorModuleWrite // referenced modules already holding their bytes
	retire            []string                  // hash-owned module paths no longer referenced
	ledger            orchestratorModuleLedger  // referenced modules only
	writeLedger       bool
	removal           bool // finalize removes the ledger instead of writing it
	staged, finalized bool
}

// prepareOrchestratorModuleInstall plans installing modules under configDir,
// the absolute Claude config directory. configDir may itself be or sit under
// a symlink, as the atomic writer allows; the gentle-ai/orchestrator
// directories this installer owns must be real directories when present.
//
// A referenced module is written when absent or hash-owned by the ledger,
// adopted when it already holds the exact desired bytes, and otherwise
// rejected. An unused known module is retired only when its bytes match its
// ledger hash; any other unused file is preserved and leaves the ledger.
// Other names in the directory are never enumerated or touched.
//
// Every decision is made from the journal's captured before-image, the same
// bytes stage and finalize compare-and-swap against, so a file changed after
// it was captured fails the later write instead of being overwritten.
func prepareOrchestratorModuleInstall(configDir string, modules []orchestratorModule) (*orchestratorModuleInstall, error) {
	return prepareObservedOrchestratorModuleInstall(configDir, modules, func(string) {})
}

// prepareObservedOrchestratorModuleInstall calls observed after it checks the
// ledger or a module path and before the journal captures it, so tests can
// change a file inside that window.
func prepareObservedOrchestratorModuleInstall(configDir string, modules []orchestratorModule, observed func(path string)) (*orchestratorModuleInstall, error) {
	desired, err := desiredOrchestratorModules(modules)
	if err != nil {
		return nil, err
	}
	return prepareDesiredOrchestratorModules(configDir, desired, observed)
}

// prepareOrchestratorModuleRemoval plans retiring every module: with nothing
// referenced, hash-owned modules are retired, every other file is kept, and
// finalize removes an existing ledger instead of writing an empty one.
func prepareOrchestratorModuleRemoval(configDir string) (*orchestratorModuleInstall, error) {
	p, err := prepareDesiredOrchestratorModules(configDir, map[string][]byte{}, func(string) {})
	if err != nil {
		return nil, err
	}
	p.removal = true
	p.writeLedger = p.journal.OwnedFile(p.ledgerPath, "", false, false).Before != nil
	return p, nil
}

// prepareDesiredOrchestratorModules is the shared preflight; desired maps each
// referenced module file to its bytes and may be empty only for removal.
func prepareDesiredOrchestratorModules(configDir string, desired map[string][]byte, observed func(path string)) (*orchestratorModuleInstall, error) {
	if !filepath.IsAbs(configDir) {
		return nil, fmt.Errorf("orchestrator module config directory %q is not absolute", configDir)
	}
	dir := filepath.Join(configDir, "gentle-ai", "orchestrator")
	for _, path := range []string{filepath.Dir(dir), dir} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("stat orchestrator module directory: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("orchestrator module directory %s is not a real directory", path)
		}
	}

	names := make([]string, 0, len(orchestratorModuleNames))
	for _, name := range orchestratorModuleNames {
		names = append(names, orchestratorModuleFile(name))
	}
	p := &orchestratorModuleInstall{
		dir:        dir,
		ledgerPath: filepath.Join(dir, orchestratorModuleLedgerName),
		journal:    mutationjournal.New(configDir),
		ledger:     orchestratorModuleLedger{Version: managedownership.Version, Files: map[string]string{}},
	}
	old, exists, err := managedownership.Read[orchestratorModuleLedger](p.ledgerPath, names)
	if err != nil {
		return nil, err
	}
	observed(p.ledgerPath)
	if err := p.journal.Capture(p.ledgerPath); err != nil {
		return nil, fmt.Errorf("capture orchestrator module ledger: %w", err)
	}
	// The shared Read validated the ledger from its own read; ownership holds
	// only if the captured before-image decodes to that same ledger.
	if !p.capturedLedgerMatches(old, exists) {
		return nil, fmt.Errorf("orchestrator module ledger %s changed during preflight", p.ledgerPath)
	}
	for _, file := range names {
		path := filepath.Join(dir, file)
		if err := checkOrchestratorModuleFile(path); err != nil {
			return nil, err
		}
		observed(path)
		if err := p.journal.Capture(path); err != nil {
			return nil, fmt.Errorf("capture orchestrator module %s: %w", file, err)
		}
		before := p.journal.OwnedFile(path, "", false, false).Before
		present := before != nil
		var data []byte
		if present {
			data = []byte(*before)
		}
		recorded, known := old.Files[file]
		owned := present && known && managedownership.Hash(data) == recorded
		want, referenced := desired[file]
		switch {
		case referenced && present && !owned && !bytes.Equal(data, want):
			return nil, fmt.Errorf("orchestrator module %s is modified or not owned by Gentle AI", path)
		case referenced:
			p.ledger.Files[file] = managedownership.Hash(want)
			if !present || !bytes.Equal(data, want) {
				p.writes = append(p.writes, orchestratorModuleWrite{path: path, data: want})
			} else {
				p.adopted = append(p.adopted, orchestratorModuleWrite{path: path, data: want})
			}
		case owned:
			p.retire = append(p.retire, path)
		}
	}
	p.writeLedger = !exists || !maps.Equal(old.Files, p.ledger.Files)
	return p, nil
}

// desiredOrchestratorModules maps each module file name to its bytes, rejecting
// an empty set and any unknown, misnamed, empty or duplicate module.
func desiredOrchestratorModules(modules []orchestratorModule) (map[string][]byte, error) {
	if len(modules) == 0 {
		return nil, errors.New("no orchestrator modules to install")
	}
	desired := make(map[string][]byte, len(modules))
	for _, module := range modules {
		file := orchestratorModuleFile(module.name)
		_, duplicate := desired[file]
		if !slices.Contains(orchestratorModuleNames, module.name) || module.file != file || duplicate || strings.TrimSpace(module.content) == "" {
			return nil, fmt.Errorf("invalid orchestrator module %q (%s)", module.name, module.file)
		}
		desired[file] = []byte(module.content)
	}
	return desired, nil
}

// capturedLedgerMatches reports whether the ledger's captured before-image is
// absent exactly when Read found no ledger, and otherwise decodes to the
// ledger Read validated.
func (p *orchestratorModuleInstall) capturedLedgerMatches(old orchestratorModuleLedger, exists bool) bool {
	before := p.journal.OwnedFile(p.ledgerPath, "", false, false).Before
	if before == nil {
		return !exists
	}
	var captured orchestratorModuleLedger
	return exists && json.Unmarshal([]byte(*before), &captured) == nil && reflect.DeepEqual(captured, old)
}

// checkOrchestratorModuleFile rejects a known module path that exists as a
// symlink, directory or other non-regular file, even for an unused name,
// because retiring or replacing it is never ours.
func checkOrchestratorModuleFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat orchestrator module: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("orchestrator module %s is not a regular file", path)
	}
	return nil
}

// stage writes the referenced modules the plan changes: 0644 when new, the
// existing mode otherwise; the atomic writer creates missing directories
// 0700. Adopted modules go through the same compare-and-swap with their
// identical bytes, a no-op that still rejects an edit made after prepare. It
// neither retires old names nor writes the ledger. On error the caller
// restores, core first, as described above.
func (p *orchestratorModuleInstall) stage() error {
	if p.staged {
		return errors.New("orchestrator modules already staged")
	}
	p.staged = true
	for _, write := range append(slices.Clone(p.writes), p.adopted...) {
		if _, err := p.journal.WriteWithMode(write.path, write.data, 0o644); err != nil {
			return fmt.Errorf("stage orchestrator module: %w", err)
		}
	}
	return nil
}

// finalize MUST run only after the core that references the staged modules is
// published. It removes the hash-owned unused modules and writes the ledger
// once, recording only the referenced modules; a removal plan removes the
// ledger through the same compare-and-swap instead.
func (p *orchestratorModuleInstall) finalize() error {
	if !p.staged || p.finalized {
		return errors.New("orchestrator modules finalize requires one prior stage")
	}
	p.finalized = true
	for _, path := range p.retire {
		if _, err := p.journal.Remove(path); err != nil {
			return fmt.Errorf("retire orchestrator module: %w", err)
		}
	}
	if !p.writeLedger {
		return nil
	}
	if p.removal {
		if _, err := p.journal.Remove(p.ledgerPath); err != nil {
			return fmt.Errorf("remove orchestrator module ledger: %w", err)
		}
		return nil
	}
	encoded, err := json.MarshalIndent(p.ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("encode orchestrator module ledger: %w", err)
	}
	if _, err := p.journal.WriteWithMode(p.ledgerPath, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write orchestrator module ledger: %w", err)
	}
	return nil
}

// restore reinstates every before-image stage and finalize changed, refusing
// files changed since. It removes files the plan created but not the
// directories the atomic writer created, which may remain empty.
func (p *orchestratorModuleInstall) restore() error { return p.journal.Restore() }
