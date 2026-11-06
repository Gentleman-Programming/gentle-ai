package geminimodules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/managedownership"
)

// manifestName records every module Publish wrote, by name and SHA-256, in
// the managed-ownership ledger format the native agents use. A module name
// alone proves nothing: only a file the manifest lists with its current bytes
// is Gentle AI's to rewrite or remove, so ~/.gemini/gentle-ai/99-mine.md stays.
const manifestName = ".gentle-ai-modules.json"

func manifestPath(baseDir string) string { return filepath.Join(ModulesDir(baseDir), manifestName) }

// readManifest loads the module manifest; a missing one is empty, and an
// entry that is not a module name fails validation.
func readManifest(baseDir string) (managedownership.Ledger, error) {
	path := manifestPath(baseDir)
	var names []string
	var probe managedownership.Ledger
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &probe) == nil {
		for name := range probe.Files {
			if ownedModulePattern.MatchString(name) {
				names = append(names, name)
			}
		}
	}
	ledger, _, err := managedownership.Read[managedownership.Ledger](path, names)
	if err != nil {
		return ledger, fmt.Errorf("read Gemini CLI module manifest %q: %w", path, err)
	}
	return ledger, nil
}

// moduleState reads the module file name: whether something exists there,
// its bytes when it is a regular file, and whether the manifest recorded
// exactly those bytes.
func moduleState(ledger managedownership.Ledger, dir, name string) (data []byte, exists, owned bool, err error) {
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, true, false, err
	}
	if data, err = os.ReadFile(path); err != nil {
		return nil, true, false, fmt.Errorf("read Gemini CLI module %q: %w", path, err)
	}
	hash, listed := ledger.Files[name]
	return data, true, listed && managedownership.Hash(data) == hash, nil
}

// writeManifest records files, or removes the manifest when there are none.
func writeManifest(baseDir string, files map[string]string) (bool, error) {
	path := manifestPath(baseDir)
	if len(files) == 0 {
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("remove Gemini CLI module manifest: %w", err)
		}
		return err == nil, nil
	}
	data, err := json.MarshalIndent(managedownership.Ledger{Version: managedownership.Version, Files: files}, "", "  ")
	if err != nil {
		return false, err
	}
	write, err := filemerge.WriteFileAtomic(path, append(data, '\n'), 0o644)
	if err != nil {
		return false, fmt.Errorf("write Gemini CLI module manifest: %w", err)
	}
	return write.Changed, nil
}

// withImports puts the import block first, so no user text before it can
// open a code region in which Gemini CLI ignores imports. Removing the block
// with InjectMarkdownSection restores rest byte for byte.
func withImports(rest, imports string) string {
	return filemerge.InjectMarkdownSection("", ImportSectionID, imports) + rest
}
