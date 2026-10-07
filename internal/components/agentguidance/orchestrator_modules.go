package agentguidance

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// #5256 builds an orchestrator core plus on-demand modules from the same
// canonical shared Markdown the public monolithic render uses. Explicit
// fragment markers in odd-orchestrator-sections.md, not headings, select what
// moves; the core keeps one pointer per fragment, and the ordered fragment
// list reconstructs the public render byte for byte. Only the explicit
// user-global Claude opt-in installs these files; RenderOrchestratorWithSource
// and every other runtime and scope stay monolithic. The marker grammar itself is
// decoded by orchestrator_module_fragments.go.

// orchestratorModuleIntro opens every module after its title.
const orchestratorModuleIntro = "Read this file on demand, only when an orchestrator core pointer names it and its triggering reason applies; it is not always loaded.\n"

var (
	errOrchestratorModulesUnsupported = errors.New("orchestrator modules are not supported for this runtime")
	errInvalidOrchestratorModuleDir   = errors.New("invalid orchestrator module directory binding")
)

// orchestratorModuleNames is the Gentle Shell module layout, in its order. A
// module is built only when at least one fragment belongs to it.
var orchestratorModuleNames = []string{"delegation", "verification", "tracking", "memory", "writer", "prompts", "skills"}

// orchestratorFragmentSpec names one marked fragment. The module is the ID
// prefix before the first dot, so several disjoint fragments can share one
// module; reason is the trigger the core pointer states.
type orchestratorFragmentSpec struct{ id, title, reason string }

// orchestratorFragmentSpecs are the fragments the shared Markdown marks.
// Authority, safety, consent and the native review contract stay in the core,
// and so do the Delegated Verification Gate and Simple Delegation routing:
// RDD/consent fallback, prelaunch verification, mandatory delegation and
// incident handling must apply before a pointer could fire.
var orchestratorFragmentSpecs = []orchestratorFragmentSpec{
	{id: "writer.edit-surfaces", title: "Allowed edit surfaces", reason: "you prepare a bounded writer or relay its edit-surface request"},
	{id: "delegation.key-learnings", title: "Key Learnings closing block", reason: "you write a delegated exploration, writer, or verification prompt"},
	{id: "skills.discovery", title: "Intent-Driven Skill Discovery", reason: "a request is skill-shaped or names a known workflow"},
	{id: "skills.registry", title: "Skill Registry Protocol", reason: "you resolve skills before a first delegation or a subagent reports skill_resolution"},
}

type orchestratorModule struct{ name, file, content string }

// orchestratorFragment is one moved body: pointer sits at offset in the core
// and body holds the exact bytes it replaced.
type orchestratorFragment struct {
	id, module, title, pointer, body string
	offset                           int
}

// orchestratorModuleBundle is a core, its modules and the ordered fragment
// metadata that reverses the split.
type orchestratorModuleBundle struct {
	core      string
	modules   []orchestratorModule
	fragments []orchestratorFragment
}

// buildOrchestratorModules splits the rendered orchestrator of agent into a
// core and on-demand modules referenced under moduleDir, a user-global
// directory reference such as "~/.claude/gentle-ai/orchestrator".
func buildOrchestratorModules(agent model.AgentID, source ReviewContractSource, moduleDir string) (orchestratorModuleBundle, error) {
	if agent != model.AgentClaudeCode {
		return orchestratorModuleBundle{}, fmt.Errorf("build orchestrator modules for %q: %w", agent, errOrchestratorModulesUnsupported)
	}
	annotated, err := renderOrchestrator(agent, source, "", true)
	if err != nil {
		return orchestratorModuleBundle{}, err
	}
	return splitOrchestratorModules(annotated, orchestratorFragmentSpecs, moduleDir)
}

// validateOrchestratorModuleDir accepts only a home-relative or absolute
// reference: a relative one would resolve against whatever project is open.
// Absolute filesystem bindings use native path semantics, including Windows
// drive-absolute paths. Installation scope and ownership are checked separately.
func validateOrchestratorModuleDir(dir string) error {
	switch {
	case strings.TrimSpace(dir) == "":
		return fmt.Errorf("%w: empty", errInvalidOrchestratorModuleDir)
	case dir != strings.TrimSpace(dir) || strings.ContainsAny(dir, "\r\n`"):
		return fmt.Errorf("%w: %q cannot be quoted on one pointer line", errInvalidOrchestratorModuleDir, dir)
	case !strings.HasPrefix(dir, "~/") && !strings.HasPrefix(dir, "/") && !filepath.IsAbs(dir):
		return fmt.Errorf("%w: %q is not a user-global path", errInvalidOrchestratorModuleDir, dir)
	}
	return nil
}

// splitOrchestratorModules moves every marked fragment of an annotated render
// into its module. An invalid fragment spec, an unresolved template token, or
// any marker decodeOrchestratorFragments rejects fails the whole split without
// a partial result.
func splitOrchestratorModules(annotated string, specs []orchestratorFragmentSpec, moduleDir string) (orchestratorModuleBundle, error) {
	if err := validateOrchestratorModuleDir(moduleDir); err != nil {
		return orchestratorModuleBundle{}, err
	}
	invalid := func(format string, args ...any) (orchestratorModuleBundle, error) {
		return orchestratorModuleBundle{}, fmt.Errorf("%w: "+format, append([]any{errInvalidOrchestratorFragments}, args...)...)
	}
	byID := map[string]orchestratorFragmentSpec{}
	for _, spec := range specs {
		module, _, _ := strings.Cut(spec.id, ".")
		if !slices.Contains(orchestratorModuleNames, module) {
			return invalid("fragment %q names unknown module %q", spec.id, module)
		}
		if _, dup := byID[spec.id]; dup || strings.TrimSpace(spec.title) == "" || strings.TrimSpace(spec.reason) == "" {
			return invalid("fragment spec %q is duplicate or incomplete", spec.id)
		}
		byID[spec.id] = spec
	}
	if strings.Contains(annotated, "{{GENTLE_AI_") {
		return invalid("unresolved template placeholder")
	}

	ids := make([]string, 0, len(specs))
	for _, spec := range specs {
		ids = append(ids, spec.id)
	}
	spans, err := decodeOrchestratorFragments(annotated, ids)
	if err != nil {
		return orchestratorModuleBundle{}, err
	}

	// The core keeps every byte outside the spans, in order, with one pointer
	// in place of each fragment and its two marker lines.
	var core strings.Builder
	var fragments []orchestratorFragment
	cursor := 0
	for _, span := range spans {
		core.WriteString(annotated[cursor:span.start])
		spec := byID[span.id]
		module, _, _ := strings.Cut(span.id, ".")
		modulePath := strings.TrimRight(moduleDir, "/") + "/" + orchestratorModuleFile(module)
		// Native Windows references use the same separators as installed files.
		// Keep Unix and home-relative pointer text byte-for-byte compatible.
		if filepath.IsAbs(moduleDir) && filepath.VolumeName(moduleDir) != "" {
			modulePath = filepath.Join(moduleDir, orchestratorModuleFile(module))
		}
		pointer := "On demand: when " + spec.reason + ", read `" + modulePath + "`, section \"" + spec.title + "\".\n"
		body := annotated[span.bodyStart:span.bodyEnd]
		fragments = append(fragments, orchestratorFragment{id: span.id, module: module, title: spec.title, pointer: pointer, body: body, offset: core.Len()})
		core.WriteString(pointer)
		cursor = span.end
	}
	core.WriteString(annotated[cursor:])

	var modules []orchestratorModule
	for _, name := range orchestratorModuleNames {
		var content strings.Builder
		for _, fragment := range fragments {
			if fragment.module != name {
				continue
			}
			if content.Len() == 0 {
				content.WriteString("# Gentle AI Orchestrator — " + strings.ToUpper(name[:1]) + name[1:] + " Module\n\n" + orchestratorModuleIntro)
			}
			content.WriteString("\n## " + fragment.title + "\n\n" + fragment.body)
		}
		if content.Len() > 0 {
			modules = append(modules, orchestratorModule{name: name, file: orchestratorModuleFile(name), content: content.String()})
		}
	}
	return orchestratorModuleBundle{core: core.String(), modules: modules, fragments: fragments}, nil
}

func orchestratorModuleFile(module string) string { return "orchestrator-" + module + ".md" }
