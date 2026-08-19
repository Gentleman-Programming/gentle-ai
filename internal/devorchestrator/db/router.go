package db

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Impact represents the level of database impact for a change.
type Impact string

const (
	ImpactNone     Impact = "none"
	ImpactSimple   Impact = "simple"
	ImpactHighRisk Impact = "high-risk"
)

// frontmatter represents the metadata block at the top of an artifact.
type frontmatter struct {
	DBImpact string `yaml:"db_impact"`
}

// Router interprets the database impact of an SDD artifact (e.g. tasks.md).
type Router struct{}

// New creates a new DB Router.
func New() *Router {
	return &Router{}
}

// EvaluateImpact parses the YAML frontmatter of the given text to determine the DB impact.
// It defaults to ImpactNone if not specified or unparseable.
func (r *Router) EvaluateImpact(artifactText string) Impact {
	if !strings.HasPrefix(artifactText, "---\n") && !strings.HasPrefix(artifactText, "---\r\n") {
		return ImpactNone
	}

	parts := strings.SplitN(artifactText, "---", 3)
	if len(parts) < 3 {
		return ImpactNone
	}

	// The YAML block is between the first and second '---'
	yamlContent := parts[1]

	var fm frontmatter
	err := yaml.Unmarshal([]byte(yamlContent), &fm)
	if err != nil {
		return ImpactNone
	}

	switch Impact(strings.ToLower(strings.TrimSpace(fm.DBImpact))) {
	case ImpactSimple:
		return ImpactSimple
	case ImpactHighRisk:
		return ImpactHighRisk
	default:
		return ImpactNone
	}
}
