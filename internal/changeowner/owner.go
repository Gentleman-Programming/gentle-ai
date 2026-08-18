// Package changeowner is the single authority for the per-change ownership
// marker that distinguishes SDD changes authored by gentle-orchestrator from
// changes authored by dev-orchestrator. It is a stdlib-only leaf package —
// it must never import internal/sddstatus or internal/devorchestrator — so
// both engines can depend on it without creating an import cycle, and the
// marker grammar cannot drift between call sites.
//
// The marker lives as an `engine: <id>` line in the first artifact written
// for a change (explore.md, else proposal.md), following the same
// single-line-regex convention as sddstatus's gatesEnabledPattern and
// repositoryFieldPattern. A change with no marker anywhere defaults to
// EngineGentle — the ONLY place that default lives — so legacy changes are
// byte-identical by construction.
package changeowner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Engine identifies which SDD engine owns a change.
type Engine string

const (
	// EngineGentle is the default owner: gentle-orchestrator (the built-in
	// agent-driven SDD workflow). It is also the default for any change
	// whose artifacts carry no `engine:` marker at all.
	EngineGentle Engine = "gentle-orchestrator"
	// EngineDev is the installable dev-orchestrator peer.
	EngineDev Engine = "dev-orchestrator"
)

// ErrUnknownEngine is returned when an `engine:` marker is present but its
// value does not match a known Engine. The value is never guessed at.
var ErrUnknownEngine = errors.New("changeowner: unrecognized engine marker value")

// ErrForeignEngine is returned (wrapped) when a write is attempted against a
// change owned by a different engine than the one attempting the write.
var ErrForeignEngine = errors.New("changeowner: change is owned by a different engine")

// enginePattern matches an `engine: <id>` declaration on its own line,
// mirroring sddstatus's gatesEnabledPattern/repositoryFieldPattern
// conventions: a single anchored, case-insensitive, multiline regex rather
// than a full YAML frontmatter unmarshal. devorchestrator writes non-strict
// frontmatter, so full YAML parsing would be a new failure mode.
//
// The trailing class is deliberately [ \t]* rather than \s* — \s also
// matches newlines, and in (?m) mode a greedy \s*$ would swallow the line's
// trailing "\n" into the match itself, which made Stamp's in-place
// regex.ReplaceAllString non-idempotent (each call would eat one more
// newline than it restored).
var enginePattern = regexp.MustCompile(`(?im)^[ \t]*engine:[ \t]*(gentle-orchestrator|dev-orchestrator)[ \t]*$`)

// unknownEnginePattern matches an `engine:` line with any value, including
// unrecognized ones, so Parse can distinguish "no marker" from "marker with
// a bad value" and return ErrUnknownEngine instead of silently defaulting.
var unknownEnginePattern = regexp.MustCompile(`(?im)^[ \t]*engine:[ \t]*(\S.*?)[ \t]*$`)

// Parse extracts an engine marker from raw artifact text (e.g. the contents
// of explore.md or proposal.md).
//
// If no `engine:` line is present at all, Parse returns ("", false, nil) —
// the caller (Resolve) is responsible for applying the EngineGentle default;
// Parse itself never defaults.
//
// If an `engine:` line is present with a recognized value, Parse returns
// (that Engine, true, nil).
//
// If an `engine:` line is present with an unrecognized value, Parse returns
// ("", true, ErrUnknownEngine) — the value is never guessed at.
func Parse(content string) (Engine, bool, error) {
	if match := enginePattern.FindStringSubmatch(content); match != nil {
		return Engine(match[1]), true, nil
	}
	if unknownEnginePattern.MatchString(content) {
		return "", true, ErrUnknownEngine
	}
	return "", false, nil
}

// artifactCandidates lists, in precedence order, the artifact file names
// Resolve inspects for an ownership marker. explore.md is checked before
// proposal.md because a greenfield change's first artifact is explore.md;
// the first artifact found to carry a marker wins.
var artifactCandidates = []string{"explore.md", "proposal.md"}

// Resolve determines which engine owns the change rooted at changeRoot.
//
// It reads explore.md then proposal.md (in that order) and returns the
// engine declared by the first one that carries a recognized `engine:`
// marker. If neither artifact exists, or neither carries a marker, Resolve
// returns EngineGentle — the single default for the whole system.
//
// If an artifact carries an `engine:` marker with an unrecognized value,
// Resolve returns ErrUnknownEngine immediately; it does not fall through to
// the next candidate or to the default.
func Resolve(changeRoot string) (Engine, error) {
	for _, name := range artifactCandidates {
		content, err := os.ReadFile(filepath.Join(changeRoot, name))
		if err != nil {
			continue
		}
		engine, found, err := Parse(string(content))
		if err != nil {
			return "", err
		}
		if found {
			return engine, nil
		}
	}
	return EngineGentle, nil
}

// AssertCanWrite returns nil when want is allowed to write into changeRoot.
//
// A changeRoot that does not yet exist on disk is writable by anyone (there
// is nothing to own yet — ownership is stamped at creation time). A
// changeRoot that exists and is owned by a different engine returns
// ErrForeignEngine (wrapped with the change's identity). A changeRoot with
// an unrecognized marker returns ErrUnknownEngine — it is never treated as
// writable.
func AssertCanWrite(changeRoot string, want Engine) error {
	if _, err := os.Stat(changeRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	owner, err := Resolve(changeRoot)
	if err != nil {
		return err
	}
	if owner != want {
		return fmt.Errorf("%w: %s", ErrForeignEngine, RefusalMessage(filepath.Base(changeRoot), owner, want))
	}
	return nil
}

// RefusalMessage renders the single refusal sentence shared by every
// enforcement checkpoint (RouteIntent, GenerateContextForAgent, and the
// sddstatus gate), so all three surfaces speak with one voice.
func RefusalMessage(changeID string, owner, attempted Engine) string {
	return fmt.Sprintf(
		"change %q is owned by %s; %s must not write to it — ownership is stamped at change creation and is not switchable",
		changeID, owner, attempted,
	)
}

// Stamp inserts an `engine: <want>` line into an artifact's frontmatter (or
// whole content, if the artifact has no delimited frontmatter block),
// idempotently: if a recognized `engine:` line is already present, its value
// is replaced in place rather than appending a second marker; the same call
// repeated with the same want yields byte-identical output.
func Stamp(frontmatter string, want Engine) string {
	if enginePattern.MatchString(frontmatter) {
		return enginePattern.ReplaceAllString(frontmatter, "engine: "+string(want))
	}

	trimmed := strings.TrimRight(frontmatter, "\n")
	if trimmed == "" {
		return "engine: " + string(want) + "\n"
	}
	return trimmed + "\nengine: " + string(want) + "\n"
}
