package recoverytrace

// Disposition is what the recovery decided to do with one path. The value is
// persisted verbatim in the ledger, so it stays a bare string rather than an
// index that would silently shift when the set grows.
type Disposition string

const (
	DispositionKeep       Disposition = "KEEP"
	DispositionTransplant Disposition = "TRANSPLANT"
	DispositionRewrite    Disposition = "REWRITE"
	DispositionDelete     Disposition = "DELETE"
	DispositionRegenerate Disposition = "REGENERATE"
	DispositionDefer      Disposition = "DEFER"
)

// SystemicContext names the owning system an invariant belongs to. A retained
// invariant without one is unowned, which is what makes it orphaned.
type SystemicContext string

const (
	// ContextHCR is host, install, and command runtime.
	ContextHCR SystemicContext = "HCR"
	// ContextMMI is managed mutation integrity.
	ContextMMI SystemicContext = "MMI"
	// ContextACI is agent capability and instruction projection.
	ContextACI SystemicContext = "ACI"
	// ContextMCA is model catalog and assignment.
	ContextMCA SystemicContext = "MCA"
	// ContextRAR is review authority and receipts.
	ContextRAR SystemicContext = "RAR"
	// ContextEPD is evidence, policy, and diagnostics.
	ContextEPD SystemicContext = "EPD"
	// ContextDSR is desired-state resource reconciliation.
	ContextDSR SystemicContext = "DSR"
	// ContextSDD is SDD lifecycle and artifacts.
	ContextSDD SystemicContext = "SDD"
	// ContextPAD is product admission and delivery.
	ContextPAD SystemicContext = "PAD"
)

// PublicationRef records whether a path exists on one authoritative ref. Both
// halves are recorded because an absent ref and an unchecked ref are different
// facts, and only the first can authorize an early removal.
type PublicationRef struct {
	Ref     string `json:"ref"`
	Present bool   `json:"present"`
}

// ReleaseClass is where one backlog item stands once the exact released SHA is
// known. It is a closed vocabulary: an item the recovery cannot place in one of
// these five is unresolved evidence, not a sixth outcome to be invented.
type ReleaseClass string

const (
	ReleaseClose             ReleaseClass = "close"
	ReleaseSuperseded        ReleaseClass = "superseded"
	ReleasePartiallyCovered  ReleaseClass = "partially_covered"
	ReleaseStillValid        ReleaseClass = "still_valid"
	ReleaseNeedsReproduction ReleaseClass = "needs_reproduction"
)

// BacklogEntry is one imported Appendix B item as the ledger carries it: the
// audit's own record, plus the release classification. Release is optional
// because an item can only be placed against a release that already exists, so
// requiring it up front would force the recovery to guess.
//
// The audit record is embedded rather than copied field by field so the ledger
// cannot drift from what the importer actually read.
type BacklogEntry struct {
	BacklogItem
	Release ReleaseClass `json:"release,omitempty"`
}

// Reconciliation is the counted shape of the recovered backlog. It is compared
// against the frozen expected totals so a partially reconstructed ledger cannot
// pass as a complete one.
type Reconciliation struct {
	Issues         int `json:"issues"`
	PullRequests   int `json:"pullRequests"`
	CollisionPRs   int `json:"collisionPrs"`
	Overlaps       int `json:"overlaps"`
	Decompositions int `json:"decompositions"`
}

// Row is the recovery decision for exactly one path, together with the evidence
// that authorizes it. Destination fields carry the transplanted invariant when a
// deletion moves work elsewhere; NoRetainedInvariant is the explicit alternative
// declaration, never inferred from their absence.
type Row struct {
	Path                string           `json:"path"`
	Disposition         Disposition      `json:"disposition"`
	Context             SystemicContext  `json:"context,omitempty"`
	Invariant           string           `json:"invariant,omitempty"`
	Proof               []string         `json:"proof,omitempty"`
	Contributor         string           `json:"contributor"`
	Publication         []PublicationRef `json:"publication,omitempty"`
	EarlyDeviation      bool             `json:"earlyDeviation,omitempty"`
	DestinationPath     string           `json:"destinationPath,omitempty"`
	DestinationProof    []string         `json:"destinationProof,omitempty"`
	NoRetainedInvariant bool             `json:"noRetainedInvariant,omitempty"`
}

// Ledgers carries the reconciliation totals alongside the items those totals
// count. Backlog is what makes the counts checkable: without the items, a
// reconciliation is only a claim about a backlog nobody can see.
type Ledgers struct {
	Reconciliation Reconciliation `json:"reconciliation"`
	Backlog        []BacklogEntry `json:"backlog"`
	Rows           []Row          `json:"rows"`
}
