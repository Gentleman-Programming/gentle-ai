package skillregistry

// DestinationStatus reports what one Regenerate run did to a secondary
// destination. The literals are the exact strings the CLI prints (REQ-22.10).
type DestinationStatus string

const (
	// DestUpdated means the destination was rewritten from the scanned set.
	DestUpdated DestinationStatus = "updated"
	// DestUnchanged means the destination was left byte-identical because the
	// fingerprint matched (Reason == "cache-hit").
	DestUnchanged DestinationStatus = "unchanged"
	// DestOmitted means the destination does not exist and was not created:
	// AGENTS.md is never created by this engine (spec §3.6 "No-creación").
	DestOmitted DestinationStatus = "omitted"
)

// DestinationOutcome is one destination's result, rich enough for the CLI to
// declare its per-destination line without re-reading any file (D-09).
type DestinationOutcome struct {
	Status DestinationStatus
	Path   string
	Reason string
}

// MirrorStatus reports the Engram mirror attempt (REQ-22.11). The two literals
// are the exact strings the CLI prints.
type MirrorStatus string

const (
	// MirrorOK is reported only after a MirrorFunc call that returned nil: the
	// command must never claim `mirror ok` for a write it never attempted (T-8).
	MirrorOK MirrorStatus = "mirror ok"
	// MirrorFailed covers every mirror miss: MirrorFunc nil ("mirror not
	// configured"), transport failure, or tool error. It is never fatal
	// (REQ-22.11).
	MirrorFailed MirrorStatus = "mirror failed"
)

// MirrorOutcome reports one mirror attempt. The zero value means "not
// attempted" (cache-hit: no destination is written), which is deliberately
// distinct from MirrorFailed: nothing broke, nothing needed writing.
type MirrorOutcome struct {
	Status MirrorStatus
	Err    error
}

// RegenerateOptions carries the two per-run knobs of the unified engine: the
// fingerprint bypass and the injected Engram mirror port (D-11).
type RegenerateOptions struct {
	// Force skips the fingerprint cache-hit check and regenerates always.
	Force bool
	// Mirror persists the index to the Engram `skill-registry` topic. It is a
	// port, never an import: internal/skillregistry stays a leaf package.
	// A nil MirrorFunc reports MirrorFailed ("mirror not configured") (T-8).
	Mirror MirrorFunc
}

// Result is the outcome of one Regenerate run, with a state per destination so
// `axiom skill index refresh` can declare its per-destination lines from this
// value alone (D-09).
type Result struct {
	Regenerated bool
	SkillCount  int
	// Reason is "cache-hit", "fingerprint-changed" or "forced".
	Reason   string
	Registry string
	Cache    string
	// Agents is the managed `## Skills` section of AGENTS.md.
	Agents DestinationOutcome
	// Mirror is the Engram topic outcome. Zero value: not attempted
	// (cache-hit).
	Mirror MirrorOutcome
}
