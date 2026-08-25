// Package design interprets Figma design references declared in an SDD
// artifact's frontmatter. It mirrors internal/devorchestrator/db's Router
// shape and parsing precedent exactly, with one deliberate divergence: Ref
// carries a payload (FileKey/NodeID) rather than a classification, because a
// downstream consumer must eventually render it into an agent prompt -- see
// design decision D-A/D-B for the full argument.
package design

import (
	"net/url"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Ref represents a recognized Figma design reference extracted from an
// artifact's design_ref frontmatter value. Both fields are charset-bounded
// by construction (see fileKeyPattern/nodeIDPattern below), never raw input
// bytes, so any future caller can only ever render an injection-safe value.
//
// Do NOT add a Raw field. Any field present here is one a maintainer will
// eventually render, and rendering caller-controlled bytes into the prompt
// (via router.promptTemplate's unescaped text/template) is exactly the
// injection vector this design exists to close (design decision D-A/D-B).
type Ref struct {
	FileKey string
	// NodeID is write-only until S3's Ref.Canonical() reads it (design
	// Risk 5). This is a deliberate one-slice gap, not a bug: the deadcode
	// ratchet cannot flag an unread struct field, so this comment is the
	// accepted, sufficient mitigation until Canonical() lands.
	NodeID string
}

// Present reports whether ref carries a recognized design reference.
func (r Ref) Present() bool {
	return r.FileKey != ""
}

// frontmatter represents the metadata block at the top of an artifact,
// mirroring db.frontmatter's shape and parsing precedent exactly.
type frontmatter struct {
	DesignRef string `yaml:"design_ref"`
}

// allowedHosts is the closed set of recognized Figma hosts, checked by
// exact equality only -- never strings.Contains/HasSuffix, which would also
// match an attacker-controlled host like figma.com.evil.com.
var allowedHosts = map[string]bool{
	"figma.com":     true,
	"www.figma.com": true,
}

// allowedPathPrefixes is the closed set of recognized first path segments.
// "board" (FigJam), "community", and "slides" are deliberately excluded: a
// whiteboard carries no implementable component/token structure, and
// /community/file/<numeric-id> has a different segment layout that would
// mis-parse.
var allowedPathPrefixes = map[string]bool{
	"file":   true,
	"design": true,
	"proto":  true,
}

// fileKeyPattern is deliberately stricter than Figma's real key alphabet
// (design Risk 2, an accepted false-negative-only tradeoff: a stricter
// charset can only reject a real key, never accept an attacker-controlled
// one). Do NOT loosen this even though it may reject some real Figma keys.
var fileKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]{8,64}$`)

// nodeIDPattern matches a decoded node-id query value before normalization
// to the canonical "-" separator form.
var nodeIDPattern = regexp.MustCompile(`^[0-9]+[-:][0-9]+$`)

// Router interprets the design reference of an SDD artifact (e.g.
// tasks.md). It carries no state today; the struct shape (rather than a
// package-level function) mirrors db.Router's convention and leaves room
// for L2 retrieval state (endpoint handle, client seam) without an API
// change.
type Router struct{}

// New creates a new Design Router.
func New() *Router {
	return &Router{}
}

// EvaluateRef parses the YAML frontmatter of the given text to determine
// whether it declares a recognized Figma design reference. It defaults to a
// zero Ref for any recognition failure -- no frontmatter, unparseable YAML,
// an absent/empty design_ref key, or an unrecognized value -- mirroring
// db.Router.EvaluateImpact's default-safe behavior exactly: no error return
// exists for a caller to ignore into a wrong state.
//
// EvaluateRef never reads receiver state, so it is safe to call through a
// nil *Router (see TestEvaluateRef_NilReceiverDoesNotPanic) -- the mitigation
// for design Risk 3's Orchestrator{} literal-construction hazard.
func (r *Router) EvaluateRef(artifactText string) Ref {
	if !strings.HasPrefix(artifactText, "---\n") && !strings.HasPrefix(artifactText, "---\r\n") {
		return Ref{}
	}

	parts := strings.SplitN(artifactText, "---", 3)
	if len(parts) < 3 {
		return Ref{}
	}

	// The YAML block is between the first and second '---'.
	yamlContent := parts[1]

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return Ref{}
	}

	return parseDesignRef(strings.TrimSpace(fm.DesignRef))
}

// parseDesignRef recognizes a Figma URL and extracts its charset-bounded
// components. Any recognition failure returns a zero Ref -- never an error
// -- per design decision D-A.
func parseDesignRef(value string) Ref {
	if value == "" {
		return Ref{}
	}

	u, err := url.Parse(value)
	if err != nil {
		return Ref{}
	}
	if u.Scheme != "https" {
		return Ref{}
	}
	if !allowedHosts[strings.ToLower(u.Hostname())] {
		return Ref{}
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 {
		return Ref{}
	}
	if !allowedPathPrefixes[segments[0]] {
		return Ref{}
	}

	fileKey := segments[1]
	if !fileKeyPattern.MatchString(fileKey) {
		return Ref{}
	}

	ref := Ref{FileKey: fileKey}

	// u.Query().Get already percent-decodes the raw query value.
	if nodeID := u.Query().Get("node-id"); nodeID != "" && nodeIDPattern.MatchString(nodeID) {
		ref.NodeID = strings.ReplaceAll(nodeID, ":", "-")
	}

	return ref
}
