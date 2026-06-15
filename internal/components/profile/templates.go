// Package profile provides role-based profile management for Gentle-AI.
package profile

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// TemplateEngine handles base and role template merging with variable
// substitution. Base templates apply to all roles; role-specific templates
// override the base when present.
type TemplateEngine struct {
	baseTemplates map[string]string                          // name -> content
	roleTemplates map[model.RoleEnum]map[string]string       // role -> name -> content
}

// NewTemplateEngine returns a TemplateEngine with empty template registries.
func NewTemplateEngine() *TemplateEngine {
	return &TemplateEngine{
		baseTemplates: make(map[string]string),
		roleTemplates: make(map[model.RoleEnum]map[string]string),
	}
}

// RegisterBase adds a base template that applies to all roles.
func (te *TemplateEngine) RegisterBase(name, content string) {
	te.baseTemplates[name] = content
}

// RegisterRole adds a role-specific template that overrides the base
// for the given role.
func (te *TemplateEngine) RegisterRole(role model.RoleEnum, name, content string) {
	if te.roleTemplates[role] == nil {
		te.roleTemplates[role] = make(map[string]string)
	}
	te.roleTemplates[role][name] = content
}

// Render produces the output for a named template, scoped to a role.
// Role-specific templates take precedence over base templates.
// Variables are substituted using Go's simple {{.Key}} syntax.
func (te *TemplateEngine) Render(name string, role model.RoleEnum, vars map[string]string) (string, error) {
	// Check role-specific first
	if roleTmpls, ok := te.roleTemplates[role]; ok {
		if content, ok := roleTmpls[name]; ok {
			return substitute(content, vars), nil
		}
	}

	// Fall back to base
	content, ok := te.baseTemplates[name]
	if !ok {
		return "", fmt.Errorf("template %q not found", name)
	}

	return substitute(content, vars), nil
}

// HasTemplate reports whether a template with the given name exists
// (either as a base template or a role-specific override).
func (te *TemplateEngine) HasTemplate(name string, role model.RoleEnum) bool {
	if roleTmpls, ok := te.roleTemplates[role]; ok {
		if _, ok := roleTmpls[name]; ok {
			return true
		}
	}
	_, ok := te.baseTemplates[name]
	return ok
}

// HasRoleTemplate reports whether a role-specific override exists for
// the given template name and role.
func (te *TemplateEngine) HasRoleTemplate(name string, role model.RoleEnum) bool {
	roleTmpls, ok := te.roleTemplates[role]
	if !ok {
		return false
	}
	_, ok = roleTmpls[name]
	return ok
}

// ListBaseTemplates returns the names of all registered base templates.
func (te *TemplateEngine) ListBaseTemplates() []string {
	names := make([]string, 0, len(te.baseTemplates))
	for name := range te.baseTemplates {
		names = append(names, name)
	}
	return names
}

// substitute replaces {{.Key}} placeholders with values from vars.
// Missing keys are replaced with an empty string.
func substitute(tmpl string, vars map[string]string) string {
	result := tmpl
	for key, val := range vars {
		result = strings.ReplaceAll(result, "{{."+key+"}}", val)
	}
	return result
}
