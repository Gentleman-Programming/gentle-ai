package agentdef

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Loader discovers and parses agent definition files from the agents/ subdirectory
// of the project's agent config dir (e.g. .claude/agents/*.md).
type Loader struct {
	// ConfigDir is the agent config directory relative to the project root (e.g. ".claude").
	ConfigDir string
}

// Load reads all *.md files from <projectRoot>/<ConfigDir>/agents/ and
// returns parsed AgentDef values. Files that cannot be parsed are skipped
// with an error logged to the returned errors slice.
func (l *Loader) Load(projectRoot string) ([]AgentDef, []error) {
	agentsDir := filepath.Join(projectRoot, l.ConfigDir, "agents")

	entries, err := os.ReadDir(agentsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{fmt.Errorf("read agents dir: %w", err)}
	}

	var defs []AgentDef
	var errs []error

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(agentsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", path, err))
			continue
		}

		def, err := ParseFrontmatter(data)
		if err != nil {
			errs = append(errs, fmt.Errorf("parse %s: %w", path, err))
			continue
		}

		defs = append(defs, def)
	}

	return defs, errs
}

// LoadFS loads agent definitions from an fs.FS (for use with embed.FS in templates).
func LoadFS(fsys fs.FS, agentsDir string) ([]AgentDef, error) {
	entries, err := fs.ReadDir(fsys, agentsDir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir from FS: %w", err)
	}

	var defs []AgentDef
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := fs.ReadFile(fsys, agentsDir+"/"+entry.Name())
		if err != nil {
			continue
		}

		def, err := ParseFrontmatter(data)
		if err != nil {
			continue
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// FindByName returns the first AgentDef with the given name.
func FindByName(defs []AgentDef, name string) (AgentDef, bool) {
	for _, d := range defs {
		if d.Name == name {
			return d, true
		}
	}
	return AgentDef{}, false
}

// Resolve applies inheritance: if def.Extends is set, it merges the parent's
// fields into def (child fields take precedence).
func Resolve(def AgentDef, all []AgentDef) AgentDef {
	if def.Extends == "" {
		return def
	}

	parent, ok := FindByName(all, def.Extends)
	if !ok {
		return def
	}

	// Merge: child fields override parent; empty child fields inherit from parent
	resolved := parent
	if def.Name != "" {
		resolved.Name = def.Name
	}
	if def.Description != "" {
		resolved.Description = def.Description
	}
	if def.Model != "" {
		resolved.Model = def.Model
	}
	if len(def.AllowedTools) > 0 {
		resolved.AllowedTools = def.AllowedTools
	}
	if def.Context != "" {
		resolved.Context = def.Context
	}
	if def.Body != "" {
		resolved.Body = def.Body
	}
	// Merge hooks (child overrides parent keys)
	if len(def.Hooks) > 0 {
		if resolved.Hooks == nil {
			resolved.Hooks = make(map[string]string)
		}
		for k, v := range def.Hooks {
			resolved.Hooks[k] = v
		}
	}
	// Merge rules (child overrides parent keys)
	if len(def.Rules) > 0 {
		if resolved.Rules == nil {
			resolved.Rules = make(map[string]any)
		}
		for k, v := range def.Rules {
			resolved.Rules[k] = v
		}
	}

	return resolved
}
