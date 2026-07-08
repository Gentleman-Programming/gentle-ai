package engram

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// engramProtocolAssetPath is the single canonical source for every rendered
// Engram protocol surface (Claude Code full/slim CLAUDE.md sections, Codex
// model_instructions_file, Codex experimental_compact_prompt_file). See
// design.md Decision 3.
const engramProtocolAssetPath = "engram/protocol.md"

// engramProtocolVersionFloor is the minimum engram binary version (Decision 1)
// verified to serve the MCP `instructions` channel, which is the redundant
// channel that makes slimming the Claude Code CLAUDE.md section safe.
const engramProtocolVersionFloor = "1.4.0"

// engramVersionPattern anchors the entire (trimmed) version string to the
// exact `engram version` output shape: an optional "engram " prefix, an
// optional "v" prefix, then bare "X.Y.Z" — nothing more. This intentionally
// rejects pre-release/build-suffixed versions ("1.4.0-beta", "1.4.0-rc1")
// and embedded semver inside arbitrary text ("foo 1.5.0 bar"): both fall
// back to engramVersionMeetsFloor's safe default (full), since a suffixed
// version may predate the MCP instructions wiring and arbitrary text is not
// a trustworthy version-command output (JD-014).
var engramVersionPattern = regexp.MustCompile(`^(?:engram\s+)?v?(\d+)\.(\d+)\.(\d+)$`)

// protocolAssetContent returns the raw canonical protocol asset content.
func protocolAssetContent() string {
	return assets.MustRead(engramProtocolAssetPath)
}

func extractProtocolSection(content, name string) string {
	return filemerge.ExtractHTMLCommentSection(content, name)
}

// protocolFull returns the full Engram protocol section, byte-identical to
// the pre-consolidation claude/engram-protocol.md content. Used for every
// adapter that is NOT on the Decision 1 verified-slim list.
func protocolFull() string {
	return extractProtocolSection(protocolAssetContent(), "full")
}

// protocolSlim returns the slim Engram protocol section (design.md
// Decision 2). Only injected for adapters with a design-verified redundant
// channel — currently Claude Code, gated additionally on the engram binary
// version floor (see protocolFor).
func protocolSlim() string {
	return extractProtocolSection(protocolAssetContent(), "slim")
}

// protocolPassiveCapture returns the Codex-only PASSIVE CAPTURE section.
func protocolPassiveCapture() string {
	return extractProtocolSection(protocolAssetContent(), "passive-capture")
}

// codexInstructions renders the Codex model_instructions_file content: the
// full protocol text concatenated with the passive-capture section, in that
// order (design.md Decision 3 — concatenation, not replacement, so PASSIVE
// CAPTURE content is preserved by construction).
func codexInstructions() string {
	return protocolFull() + "\n" + protocolPassiveCapture()
}

// codexCompact renders the Codex experimental_compact_prompt_file content.
func codexCompact() string {
	return extractProtocolSection(protocolAssetContent(), "compact")
}

// IsVerifiedSlimAdapter implements the Decision 1 redundant-channel
// verification matrix. Exactly one adapter (Claude Code) currently qualifies
// for the slim section, and only when the installed engram binary meets the
// version floor — below-floor, unknown, or unparseable versions fall back to
// the safe default (full). Exported so internal/cli/run.go can reuse the same
// matrix to compute the --protocol setup-forwarding verdict per slug (Per-slug
// forwarding semantics, design.md) — section rendering and flag forwarding
// share one source of truth for "which adapters are slim".
func IsVerifiedSlimAdapter(agent model.AgentID, version string) bool {
	if agent != model.AgentClaudeCode {
		return false
	}
	return engramVersionMeetsFloor(version)
}

// engramVersionMeetsFloor reports whether version is >= engramProtocolVersionFloor.
// It tolerates raw VerifyVersion() output such as "engram 1.18.0" and returns
// false for empty, unknown, or unparseable input (safe default: full).
func engramVersionMeetsFloor(version string) bool {
	match := engramVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return false
	}

	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])

	floorMatch := engramVersionPattern.FindStringSubmatch(engramProtocolVersionFloor)
	floorMajor, _ := strconv.Atoi(floorMatch[1])
	floorMinor, _ := strconv.Atoi(floorMatch[2])
	floorPatch, _ := strconv.Atoi(floorMatch[3])

	if major != floorMajor {
		return major > floorMajor
	}
	if minor != floorMinor {
		return minor > floorMinor
	}
	return patch >= floorPatch
}

// protocolFor selects the rendered protocol section for a given adapter,
// implementing the Decision 1 matrix. opts.Version carries the raw engram
// binary version string (see InjectOptions.Version), threaded from
// VerifyVersion() by the caller (internal/cli/run.go) before injection runs.
func protocolFor(agent model.AgentID, opts InjectOptions) string {
	if IsVerifiedSlimAdapter(agent, opts.Version) {
		return protocolSlim()
	}
	return protocolFull()
}
